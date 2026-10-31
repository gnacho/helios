package ha

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type EntityState struct {
	State       string         `json:"state,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
	LastUpdated string         `json:"lastUpdated,omitempty"`
}

type StatsRow struct {
	Start FlexTime `json:"start"`
	End   FlexTime `json:"end,omitempty"`
	Mean  *float64 `json:"mean,omitempty"`
	State *float64 `json:"state,omitempty"`
	Sum   *float64 `json:"sum,omitempty"`
	Min   *float64 `json:"min,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}

// FlexTime acepta "start" como string ISO o como epoch (HAOS devuelve ambos
// formatos según la versión/consulta; el original usaba new Date(row.start)).
type FlexTime struct {
	time.Time
}

func (f *FlexTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "null" || s == "" {
		return nil
	}
	if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
		if ts > 1e12 {
			f.Time = time.UnixMilli(ts)
		} else {
			f.Time = time.Unix(ts, 0)
		}
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			f.Time = t
			return nil
		}
	}
	return fmt.Errorf("fecha no parseable: %s", s)
}

type Stats map[string][]StatsRow

type HistoryRow struct {
	State       string         `json:"state"`
	LastChanged string         `json:"last_changed"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

type pendingCall struct {
	ch    chan callResult
	timer *time.Timer
}

type callResult struct {
	result any
	err    error
}

type entityChange struct {
	// s llega como string en HAOS real y como NÚMERO en el mock de la demo
	// (visto 22-Oct-2026: "s":1.26). RawMessage tolera ambos.
	S json.RawMessage `json:"s,omitempty"`
	A map[string]any  `json:"a,omitempty"`
	// lu llega como string ISO o como epoch numérico (visto en producción
	// 22-Oct-2026: 1792692300.3598962). RawMessage tolera ambos.
	LU json.RawMessage `json:"lu,omitempty"`
}

// rawToString normaliza un campo JSON tolerante (s / lu) a su forma string.
func rawToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	s := strings.Trim(string(raw), `"`)
	if s == "null" {
		return ""
	}
	return s
}

// luToString normaliza el campo lu a string ISO (o vacío si no parsea).
func luToString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	s := strings.Trim(string(raw), `"`)
	if s == "null" {
		return ""
	}
	if ts, err := strconv.ParseFloat(s, 64); err == nil {
		ms := int64(ts)
		if ts > 1e12 {
			ms = int64(ts)
		} else if ts < 1e11 {
			ms = int64(ts * 1000)
		}
		return time.UnixMilli(ms).UTC().Format(time.RFC3339)
	}
	return s
}

type entityEvent struct {
	A map[string]entityChange `json:"a,omitempty"`
	C map[string]struct {
		Plus  *entityChange `json:"+"`
		Minus any           `json:"-"`
	} `json:"c,omitempty"`
}

type haMessage struct {
	Type    string          `json:"type"`
	ID      int             `json:"id,omitempty"`
	Message string          `json:"message,omitempty"`
	Success bool            `json:"success,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Event *entityEvent `json:"event,omitempty"`
}

// Client is a Home Assistant websocket client with auto-reconnect.
var debugHA = os.Getenv("HELIOS_HA_DEBUG") == "1"

const (
	// maxWSMessageBytes caps a single state message. Entity payloads are a few
	// kilobytes, so 1 MiB leaves room without letting a compromised or buggy
	// instance exhaust server memory.
	maxWSMessageBytes = 1 << 20
	// maxRESTResponseBytes caps the whole body of a REST history response.
	maxRESTResponseBytes = 8 << 20
	// maxHistoryRows caps the rows returned for a single history query.
	maxHistoryRows = 50000
)

type Client struct {
	httpURL string
	wsURL   string
	token   string

	mu sync.RWMutex
	// writeMu serializa TODAS las escrituras al socket: gorilla/websocket solo
	// admite un escritor concurrente (panic "concurrent write" si no).
	writeMu    sync.Mutex
	entities   map[string]EntityState
	pending    map[int]*pendingCall
	msgID      int
	ws         *websocket.Conn
	connected  bool
	stopped    bool
	retries    int
	subscribed []string

	OnEntity     func(id string)
	OnConnected  func()
	OnDisconnect func()
	OnFatal      func(msg string)
	OnWSError    func(msg string)
}

var ErrInsecureTransport = errors.New("HA transport must be encrypted outside loopback")

// ValidateHAURL rejects cleartext transports unless the target is loopback or
// the operator explicitly opted out with allowInsecure.
func ValidateHAURL(raw string, allowInsecure bool) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return errors.New("HA URL is empty")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("invalid HA URL: %w", err)
	}
	if u.Hostname() == "" {
		return errors.New("HA URL has no host")
	}
	if u.User != nil {
		return errors.New("HA URL must not embed credentials")
	}

	tls := false
	switch u.Scheme {
	case "https", "wss":
		tls = true
	case "http", "ws":
		tls = false
	default:
		return fmt.Errorf("unsupported HA scheme %q", u.Scheme)
	}

	host := u.Hostname()
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	if !tls && !allowInsecure {
		return ErrInsecureTransport
	}
	return nil
}

func websocketURL(raw string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(raw), "/")
	switch {
	case strings.HasPrefix(trimmed, "wss://"), strings.HasPrefix(trimmed, "ws://"):
		return trimmed + "/api/websocket"
	case strings.HasPrefix(trimmed, "https://"):
		return "wss://" + strings.TrimPrefix(trimmed, "https://") + "/api/websocket"
	case strings.HasPrefix(trimmed, "http://"):
		return "ws://" + strings.TrimPrefix(trimmed, "http://") + "/api/websocket"
	default:
		return trimmed + "/api/websocket"
	}
}

func NewClient(httpURL, token string) *Client {
	wsURL := websocketURL(httpURL)
	return &Client{
		httpURL:  strings.TrimSuffix(httpURL, "/"),
		wsURL:    wsURL,
		token:    token,
		entities: make(map[string]EntityState),
		pending:  make(map[int]*pendingCall),
	}
}

func (c *Client) Start() {
	c.mu.Lock()
	c.stopped = false
	c.mu.Unlock()
	go c.connect()
}

func (c *Client) Stop() {
	c.mu.Lock()
	c.stopped = true
	ws := c.ws
	c.mu.Unlock()
	if ws != nil {
		ws.Close()
	}
}

func (c *Client) connect() {
	c.mu.RLock()
	stopped := c.stopped
	c.mu.RUnlock()
	if stopped {
		return
	}

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	ws, _, err := dialer.Dial(c.wsURL, nil)
	if err != nil {
		c.emitError(err.Error())
		c.scheduleReconnect()
		return
	}

	c.mu.Lock()
	c.ws = ws
	c.mu.Unlock()

	go c.readLoop(ws)
}

func (c *Client) readLoop(ws *websocket.Conn) {
	defer func() {
		ws.Close()
		c.onClose()
	}()

	// Keep-alive: ping cada 30 s y deadline de lectura de 90 s. Sin esto HAOS
	// cierra la conexión a los 60 s de inactividad (visto en producción
	// 22-Oct-2026: desconexión exacta a los 60 s del arranque).
	ws.SetReadLimit(maxWSMessageBytes)
	ws.SetReadDeadline(time.Now().Add(90 * time.Second))
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				c.writeMu.Lock()
				_ = ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
				c.writeMu.Unlock()
			}
		}
	}()

	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			return
		}
		ws.SetReadDeadline(time.Now().Add(90 * time.Second))
		var msg haMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			if debugHA {
				log.Printf("[ha:debug] unmarshal error: %v (raw %.120s)", err, raw)
			}
			continue
		}
		if debugHA {
			log.Printf("[ha:debug] <- msg type=%s id=%d event=%v rawlen=%d", msg.Type, msg.ID, msg.Event != nil, len(raw))
		}
		c.handle(msg, ws)
	}
}

func (c *Client) onClose() {
	c.mu.Lock()
	wasConnected := c.connected
	c.connected = false
	c.ws = nil
	pending := c.pending
	c.pending = make(map[int]*pendingCall)
	for _, p := range pending {
		if p.timer != nil {
			p.timer.Stop()
		}
		p.ch <- callResult{err: errors.New("ws closed")}
	}
	c.mu.Unlock()

	if wasConnected && c.OnDisconnect != nil {
		c.OnDisconnect()
	}
	c.scheduleReconnect()
}

func (c *Client) scheduleReconnect() {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return
	}
	retries := c.retries
	c.retries++
	c.mu.Unlock()

	base := math.Min(30000, 1000*math.Pow(2, math.Min(float64(retries), 5)))
	delay := time.Duration(base * (0.5 + rand.Float64()))
	time.AfterFunc(delay, c.connect)
}

func (c *Client) handle(msg haMessage, ws *websocket.Conn) {
	switch msg.Type {
	case "auth_required":
		c.writeMu.Lock()
		_ = ws.WriteJSON(map[string]any{"type": "auth", "access_token": c.token})
		c.writeMu.Unlock()
	case "auth_ok":
		c.mu.Lock()
		c.connected = true
		c.retries = 0
		ids := c.subscribed
		c.mu.Unlock()

		if c.OnConnected != nil {
			// OnConnected corre en el readLoop: el callback no debe bloquear
			// (un Call aquí produciría deadlock: el resultado lo procesa el
			// propio readLoop). Se lanza en goroutine.
			go c.OnConnected()
		}
		if len(ids) > 0 {
			go c.resubscribe(ids)
		}
	case "auth_invalid":
		msgText := "token rechazado"
		if msg.Message != "" {
			msgText = msg.Message
		}
		if c.OnFatal != nil {
			c.OnFatal("auth_invalid: " + msgText)
		}
		c.Stop()
	case "result":
		if debugHA {
			log.Printf("[ha:debug] <- result id=%d success=%v", msg.ID, msg.Success)
		}
		c.mu.Lock()
		p, ok := c.pending[msg.ID]
		if ok {
			delete(c.pending, msg.ID)
		}
		c.mu.Unlock()
		if !ok || p == nil {
			return
		}
		if p.timer != nil {
			p.timer.Stop()
		}
		if msg.Success {
			p.ch <- callResult{result: msg.Result}
		} else {
			text := "error desconocido"
			if msg.Error != nil {
				text = msg.Error.Message
			}
			p.ch <- callResult{err: errors.New(text)}
		}
	case "event":
		if msg.Event != nil {
			c.applyEntityEvent(*msg.Event)
		}
	}
}

func (c *Client) applyEntityEvent(ev entityEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if debugHA {
		log.Printf("[ha:debug] applyEntityEvent a=%d c=%d", len(ev.A), len(ev.C))
	}
	for id, v := range ev.A {
		if debugHA && (strings.Contains(id, "scraper") || strings.Contains(id, "grid")) {
			log.Printf("[ha:debug] snapshot entidad %s state=%v attrs=%v", id, rawToString(v.S), v.A)
		}
		st := EntityState{Attributes: v.A}
		st.State = rawToString(v.S)
		st.LastUpdated = luToString(v.LU)
		c.entities[id] = st
		if c.OnEntity != nil {
			go c.OnEntity(id)
		}
	}

	for id, diff := range ev.C {
		if diff.Plus != nil {
			cur, exists := c.entities[id]
			if !exists {
				cur = EntityState{Attributes: map[string]any{}}
			}
			cur.State = rawToString(diff.Plus.S)
			// HAOS envía los atributos de un diff de forma PARCIAL (solo los que
			// cambiaron). Reemplazar el mapa completo perdería el resto
			// (gridDirection/stationName/inverterOnline del scraper, visto en
			// producción 22-Oct-2026) → merge.
			if diff.Plus.A != nil {
				merged := make(map[string]any, len(cur.Attributes)+len(diff.Plus.A))
				for k, v := range cur.Attributes {
					merged[k] = v
				}
				for k, v := range diff.Plus.A {
					merged[k] = v
				}
				cur.Attributes = merged
			}
			cur.LastUpdated = luToString(diff.Plus.LU)
			c.entities[id] = cur
			if c.OnEntity != nil {
				go c.OnEntity(id)
			}
		} else if diff.Minus != nil {
			delete(c.entities, id)
		}
	}
}

func (c *Client) Call(message map[string]any) (json.RawMessage, error) {
	c.mu.Lock()
	if !c.connected || c.ws == nil {
		c.mu.Unlock()
		return nil, errors.New("no conectado")
	}
	ws := c.ws
	c.msgID++
	id := c.msgID
	p := &pendingCall{ch: make(chan callResult, 1)}
	// El timer se crea DENTRO del lock: si el resultado llega inmediatamente,
	// handle() hace p.timer.Stop() y no debe encontrarlo nil.
	p.timer = time.AfterFunc(60*time.Second, func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		select {
		case p.ch <- callResult{err: errors.New("timeout ws")}:
		default:
		}
	})
	c.pending[id] = p
	c.mu.Unlock()

	payload := map[string]any{"id": id}
	for k, v := range message {
		payload[k] = v
	}
	if debugHA {
		log.Printf("[ha:debug] -> call id=%d type=%v", id, message["type"])
	}
	c.writeMu.Lock()
	err := ws.WriteJSON(payload)
	c.writeMu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		p.timer.Stop()
		return nil, err
	}

	res := <-p.ch
	if res.err != nil {
		// Timeout o ws cerrado: result es nil; nunca asertar sobre nil.
		return nil, res.err
	}
	raw, ok := res.result.(json.RawMessage)
	if !ok {
		return nil, fmt.Errorf("respuesta inesperada del WS")
	}
	return raw, nil
}

// SubscribeEntities actualiza la lista de entidades vigiladas. Con conexión y
// lista equivalente no reenvía el comando: cada subscribe_entities apila una
// suscripción nueva en HA y un guardado sin cambios no debe apilar nada.
// Sin conexión solo almacena la lista: auth_ok la reenvía al reconectar.
func (c *Client) SubscribeEntities(ids []string) error {
	c.mu.Lock()
	same := c.connected && sameIDs(c.subscribed, ids)
	c.subscribed = ids
	c.mu.Unlock()
	if same {
		return nil
	}
	if !c.IsConnected() {
		return nil
	}
	return c.subscribe(ids)
}

// resubscribe reenvía el comando sin deduplicar: tras auth_ok HA ha perdido la
// suscripción anterior aunque la lista no haya cambiado.
func (c *Client) resubscribe(ids []string) error {
	c.mu.Lock()
	c.subscribed = ids
	c.mu.Unlock()
	return c.subscribe(ids)
}

func (c *Client) subscribe(ids []string) error {
	_, err := c.Call(map[string]any{"type": "subscribe_entities", "entity_ids": ids})
	if err != nil {
		c.emitError("subscribe_entities: " + err.Error())
		return err
	}
	return nil
}

func sameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := slices.Clone(a)
	y := slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

func (c *Client) CallService(domain, service string, data map[string]any) error {
	msg := map[string]any{"type": "call_service", "domain": domain, "service": service}
	if data != nil {
		if entityID, ok := data["entity_id"]; ok {
			msg["target"] = map[string]any{"entity_id": entityID}
			rest := make(map[string]any)
			for k, v := range data {
				if k != "entity_id" {
					rest[k] = v
				}
			}
			if len(rest) > 0 {
				msg["service_data"] = rest
			}
		} else if len(data) > 0 {
			msg["service_data"] = data
		}
	}
	_, err := c.Call(msg)
	return err
}

func (c *Client) GetState(id string) (EntityState, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	st, ok := c.entities[id]
	return st, ok
}

func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

func (c *Client) StatisticsDuringPeriod(startTime, endTime string, statisticIDs []string, period string, types []string) (Stats, error) {
	raw, err := c.Call(map[string]any{
		"type":          "recorder/statistics_during_period",
		"start_time":    startTime,
		"end_time":      endTime,
		"statistic_ids": statisticIDs,
		"period":        period,
		"types":         types,
	})
	if err != nil {
		return nil, err
	}
	var stats Stats
	if err := json.Unmarshal(raw, &stats); err != nil {
		return nil, fmt.Errorf("decode stats: %w", err)
	}
	return stats, nil
}

// HistoryDuringPeriod uses the REST endpoint (no websocket equivalent).
func (c *Client) HistoryDuringPeriod(startTime, endTime, entityID string) ([]HistoryRow, error) {
	url := fmt.Sprintf("%s/api/history/period/%s?filter_entity_id=%s&end_time=%s&minimal_response",
		c.httpURL, urlEncode(startTime), urlEncode(entityID), urlEncode(endTime))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("history HTTP %d", res.StatusCode)
	}

	var data [][]HistoryRow
	dec := json.NewDecoder(io.LimitReader(res.Body, maxRESTResponseBytes))
	if err := dec.Decode(&data); err != nil {
		return nil, err
	}
	if len(data) > 0 {
		rows := data[0]
		if len(rows) > maxHistoryRows {
			rows = rows[:maxHistoryRows]
		}
		return rows, nil
	}
	return []HistoryRow{}, nil
}

func (c *Client) emitError(msg string) {
	if c.OnWSError != nil {
		c.OnWSError(msg)
	} else {
		log.Printf("[ha] %s", msg)
	}
}

func urlEncode(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, ":", "%3A"), "+", "%2B")
}

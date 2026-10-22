package ha

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
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
	Start string   `json:"start"`
	End   string   `json:"end,omitempty"`
	Mean  *float64 `json:"mean,omitempty"`
	State *float64 `json:"state,omitempty"`
	Sum   *float64 `json:"sum,omitempty"`
	Min   *float64 `json:"min,omitempty"`
	Max   *float64 `json:"max,omitempty"`
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
	S  *string        `json:"s,omitempty"`
	A  map[string]any `json:"a,omitempty"`
	LU *string        `json:"lu,omitempty"`
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
type Client struct {
	httpURL string
	wsURL   string
	token   string

	mu         sync.RWMutex
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

func NewClient(httpURL, token string) *Client {
	wsURL := strings.TrimSuffix(strings.Replace(httpURL, "http", "ws", 1), "/") + "/api/websocket"
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
				_ = ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
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
			continue
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
		p.timer.Stop()
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
		ws.WriteJSON(map[string]any{"type": "auth", "access_token": c.token})
	case "auth_ok":
		c.mu.Lock()
		c.connected = true
		c.retries = 0
		ids := c.subscribed
		c.mu.Unlock()

		if c.OnConnected != nil {
			c.OnConnected()
		}
		if len(ids) > 0 {
			c.SubscribeEntities(ids)
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
		c.mu.Lock()
		p, ok := c.pending[msg.ID]
		if ok {
			delete(c.pending, msg.ID)
		}
		c.mu.Unlock()
		if !ok || p == nil {
			return
		}
		p.timer.Stop()
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

	for id, v := range ev.A {
		st := EntityState{Attributes: v.A}
		if v.S != nil {
			st.State = *v.S
		}
		if v.LU != nil {
			st.LastUpdated = *v.LU
		}
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
			if diff.Plus.S != nil {
				cur.State = *diff.Plus.S
			}
			if diff.Plus.A != nil {
				cur.Attributes = diff.Plus.A
			}
			if diff.Plus.LU != nil {
				cur.LastUpdated = *diff.Plus.LU
			}
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
	c.pending[id] = p
	c.mu.Unlock()

	p.timer = time.AfterFunc(60*time.Second, func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		select {
		case p.ch <- callResult{err: errors.New("timeout ws")}:
		default:
		}
	})

	payload := map[string]any{"id": id}
	for k, v := range message {
		payload[k] = v
	}
	if err := ws.WriteJSON(payload); err != nil {
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

func (c *Client) SubscribeEntities(ids []string) error {
	c.mu.Lock()
	c.subscribed = ids
	c.mu.Unlock()

	_, err := c.Call(map[string]any{"type": "subscribe_entities", "entity_ids": ids})
	if err != nil {
		c.emitError("subscribe_entities: " + err.Error())
		return err
	}
	return nil
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
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return nil, err
	}
	if len(data) > 0 {
		return data[0], nil
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

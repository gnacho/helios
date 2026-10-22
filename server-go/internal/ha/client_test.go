package ha

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeHA simula el handshake y una suscripción de Home Assistant.
func fakeHA(t *testing.T) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/websocket") {
			http.NotFound(w, r)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		conn.WriteJSON(map[string]any{"type": "auth_required"})

		var authMsg map[string]any
		if err := conn.ReadJSON(&authMsg); err != nil {
			return
		}
		if authMsg["access_token"] != "good-token" {
			conn.WriteJSON(map[string]any{"type": "auth_invalid", "message": "bad token"})
			return
		}
		conn.WriteJSON(map[string]any{"type": "auth_ok"})

		for {
			var msg map[string]any
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			id := msg["id"]
			switch msg["type"] {
			case "subscribe_entities":
				conn.WriteJSON(map[string]any{"id": id, "type": "result", "success": true, "result": nil})
				// emite un evento de entidad
				conn.WriteJSON(map[string]any{
					"type": "event",
					"event": map[string]any{
						"a": map[string]any{
							"sensor.solis_power": map[string]any{"s": "3.42", "lu": "2026-10-22T10:00:00Z"},
						},
					},
				})
			case "recorder/statistics_during_period":
				conn.WriteJSON(map[string]any{
					"id": id, "type": "result", "success": true,
					"result": map[string]any{
						"sensor.solis_energy": []map[string]any{
							{"start": "2026-10-22T00:00:00Z", "sum": 12.5},
						},
					},
				})
			default:
				conn.WriteJSON(map[string]any{"id": id, "type": "result", "success": true, "result": nil})
			}
		}
	}))
}

func waitFor(t *testing.T, cond func() bool, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestClientAuthAndSubscribe(t *testing.T) {
	srv := fakeHA(t)
	defer srv.Close()

	client := NewClient(srv.URL, "good-token")

	entityCh := make(chan string, 8)
	client.OnEntity = func(id string) { entityCh <- id }

	client.Start()
	defer client.Stop()

	if !waitFor(t, client.IsConnected, 3*time.Second) {
		t.Fatal("no conectó al HA falso")
	}

	if err := client.SubscribeEntities([]string{"sensor.solis_power"}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	select {
	case id := <-entityCh:
		if id != "sensor.solis_power" {
			t.Fatalf("entidad inesperada: %s", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no llegó evento de entidad")
	}

	state, ok := client.GetState("sensor.solis_power")
	if !ok || state.State != "3.42" {
		t.Fatalf("estado incorrecto: %+v (ok=%v)", state, ok)
	}
}

func TestClientStatistics(t *testing.T) {
	srv := fakeHA(t)
	defer srv.Close()

	client := NewClient(srv.URL, "good-token")
	client.Start()
	defer client.Stop()

	if !waitFor(t, client.IsConnected, 3*time.Second) {
		t.Fatal("no conectó")
	}

	stats, err := client.StatisticsDuringPeriod(
		"2026-10-22T00:00:00Z", "2026-10-23T00:00:00Z",
		[]string{"sensor.solis_energy"}, "day", []string{"sum"},
	)
	if err != nil {
		t.Fatalf("statistics: %v", err)
	}
	rows, ok := stats["sensor.solis_energy"]
	if !ok || len(rows) != 1 {
		t.Fatalf("stats inesperadas: %+v", stats)
	}
	if rows[0].Sum == nil || *rows[0].Sum != 12.5 {
		t.Fatalf("sum incorrecta: %+v", rows[0])
	}
}

func TestClientRejectsBadToken(t *testing.T) {
	srv := fakeHA(t)
	defer srv.Close()

	client := NewClient(srv.URL, "bad-token")
	fatalCh := make(chan string, 1)
	client.OnFatal = func(msg string) { fatalCh <- msg }

	client.Start()
	defer client.Stop()

	select {
	case msg := <-fatalCh:
		if !strings.Contains(msg, "auth_invalid") {
			t.Fatalf("mensaje inesperado: %s", msg)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no se reportó auth_invalid")
	}
}

func TestEntityDiffApplication(t *testing.T) {
	c := NewClient("http://localhost:8123", "t")
	plus := &entityChange{S: strPtr("5.0")}

	ev := entityEvent{
		C: map[string]struct {
			Plus  *entityChange `json:"+"`
			Minus any           `json:"-"`
		}{
			"sensor.x": {Plus: plus},
		},
	}
	c.applyEntityEvent(ev)

	st, ok := c.GetState("sensor.x")
	if !ok || st.State != "5.0" {
		t.Fatalf("diff no aplicado: %+v", st)
	}

	// sanity: el mensaje JSON de results se decodifica
	var msg haMessage
	if err := json.Unmarshal([]byte(`{"id":1,"type":"result","success":true,"result":{"a":1}}`), &msg); err != nil {
		t.Fatal(err)
	}
	if !msg.Success || len(msg.Result) == 0 {
		t.Fatalf("decode result falló: %+v", msg)
	}
}

func strPtr(s string) *string { return &s }

// TestDiffAttributesMerge: un diff "c" con atributos PARCIALES no debe perder
// el resto del mapa. Regresión del bug de producción 22-Oct-2026: el scraper
// perdía gridDirection/stationName/inverterOnline y el grid caía al fallback.
func TestDiffAttributesMerge(t *testing.T) {
	c := NewClient("http://x", "t")
	c.entities["sensor.solis_scraper"] = EntityState{
		State: "Mandor 40",
		Attributes: map[string]any{
			"currentGridPower": float64(1.1),
			"gridDirection":    "import",
			"inverterOnline":   float64(1),
			"stationName":      "Mandor 40",
		},
	}

	// Diff parcial: solo cambia currentGridPower.
	s := "Mandor 40"
	lu := []byte("1792692300.5")
	c.applyEntityEvent(entityEvent{
		C: map[string]struct {
			Plus  *entityChange `json:"+"`
			Minus any           `json:"-"`
		}{
			"sensor.solis_scraper": {Plus: &entityChange{
				S:  &s,
				A:  map[string]any{"currentGridPower": float64(0.55)},
				LU: lu,
			}},
		},
	})

	st, ok := c.GetState("sensor.solis_scraper")
	if !ok {
		t.Fatal("entidad perdida")
	}
	if got, _ := st.Attributes["currentGridPower"].(float64); got != 0.55 {
		t.Fatalf("currentGridPower no actualizado: %v", st.Attributes["currentGridPower"])
	}
	if st.Attributes["gridDirection"] != "import" {
		t.Fatalf("gridDirection perdido: %v", st.Attributes)
	}
	if st.Attributes["inverterOnline"] != float64(1) {
		t.Fatalf("inverterOnline perdido: %v", st.Attributes)
	}
	if st.Attributes["stationName"] != "Mandor 40" {
		t.Fatalf("stationName perdido: %v", st.Attributes)
	}
}

// TestUnmarshalSnapshotWithNumericLU: HAOS envía lu como epoch numérico en el
// snapshot y en los diffs "+". Regresión del bug de producción 22-Oct-2026:
// el unmarshal fallaba (lu *string) y TODO el snapshot se descartaba → todos
// los datos a 0.
func TestUnmarshalSnapshotWithNumericLU(t *testing.T) {
	raw := []byte(`{"id":1,"type":"event","event":{"a":{` +
		`"input_select.feyree_amperaje":{"s":"16","a":{"options":["8","16"]},"lu":1792692300.3598962},` +
		`"sensor.solis_bateria_soc":{"s":"20","a":{},"lu":1792692300}` +
		`},"c":{"sensor.solis_scraper":{"+":{"lu":1792692300.3598962,"c":"01M5","a":{"inverterOnline":1}}}}}}`)

	var msg haMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if msg.Event == nil {
		t.Fatal("event no parseado")
	}

	c := NewClient("http://x", "t")
	c.applyEntityEvent(*msg.Event)

	if st, ok := c.GetState("sensor.solis_bateria_soc"); !ok || st.State != "20" {
		t.Fatalf("soc no en cache: %+v ok=%v", st, ok)
	}
	if st, ok := c.GetState("input_select.feyree_amperaje"); !ok || st.State != "16" {
		t.Fatalf("amperaje no en cache: %+v ok=%v", st, ok)
	}
	if st, ok := c.GetState("sensor.solis_scraper"); !ok {
		t.Fatal("scraper no en cache tras diff +")
	} else if len(st.Attributes) == 0 || st.Attributes["inverterOnline"] != float64(1) {
		t.Fatalf("attrs scraper = %v", st.Attributes)
	}

	// luToString normaliza epoch (s) y epoch (ms)
	if got := luToString(json.RawMessage(`1792692300`)); !strings.Contains(got, "T") {
		t.Fatalf("lu epoch s -> %q", got)
	}
	if got := luToString(json.RawMessage(`1792692300359`)); !strings.Contains(got, "T") {
		t.Fatalf("lu epoch ms -> %q", got)
	}
	if got := luToString(json.RawMessage(`"2026-10-22T10:00:00Z"`)); got != "2026-10-22T10:00:00Z" {
		t.Fatalf("lu ISO -> %q", got)
	}
}

// TestSubscribeFromOnConnectedNoDeadlock: suscribir dentro de OnConnected
// (patrón de producción) no debe bloquear el readLoop. Regresión del deadlock
// de producción (22-Oct-2026): Call desde el readLoop esperaba un resultado
// que solo el readLoop procesa.
func TestSubscribeFromOnConnectedNoDeadlock(t *testing.T) {
	srv := fakeHA(t)
	defer srv.Close()

	client := NewClient(srv.URL, "good-token")
	subscribed := make(chan struct{}, 1)
	client.OnConnected = func() {
		_ = client.SubscribeEntities([]string{"sensor.solis_power"})
		subscribed <- struct{}{}
	}
	client.Start()
	defer client.Stop()

	select {
	case <-subscribed:
		// bien: el callback completó (recibió el result)
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: OnConnected nunca completó el subscribe")
	}

	if !waitFor(t, func() bool {
		_, ok := client.GetState("sensor.solis_power")
		return ok
	}, 3*time.Second) {
		t.Fatal("las entidades no llegaron tras el subscribe")
	}
}

// TestCallReturnsErrorWhenWSCloses: una llamada pendiente cuando el WS muere
// debe devolver error (NO paniquear). Regresión del panic de producción
// (22-Oct-2026): res.result nil + type assertion sin comprobar err.
func TestCallReturnsErrorWhenWSCloses(t *testing.T) {
	// HA falso que cierra la conexión al recibir subscribe.
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.WriteJSON(map[string]any{"type": "auth_required"})
		var authMsg map[string]any
		if err := conn.ReadJSON(&authMsg); err != nil {
			return
		}
		conn.WriteJSON(map[string]any{"type": "auth_ok"})
		// lee la suscripción y CIERRA sin responder.
		var msg map[string]any
		conn.ReadJSON(&msg)
	}))

	client := NewClient(srv.URL, "tok")
	client.Start()
	defer client.Stop()

	if !waitFor(t, client.IsConnected, 3*time.Second) {
		t.Fatal("no conectó")
	}

	done := make(chan error, 1)
	go func() {
		done <- client.SubscribeEntities([]string{"sensor.x"})
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("debería devolver error al cerrarse el WS")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Call colgado (no recibió el cierre)")
	}
}

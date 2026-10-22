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

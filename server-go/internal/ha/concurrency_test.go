package ha

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeHAConcurrent responde a cualquier call con un result, sin demora.
func fakeHAConcurrent(t *testing.T) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		var writeMu sync.Mutex
		for {
			var msg map[string]any
			if err := conn.ReadJSON(&msg); err != nil {
				return
			}
			writeMu.Lock()
			conn.WriteJSON(map[string]any{
				"id": msg["id"], "type": "result", "success": true,
				"result": map[string]any{"ok": true},
			})
			writeMu.Unlock()
		}
	}))
}

// TestConcurrentCallsNoPanic: varias goroutines llamando Call a la vez no deben
// provocar "concurrent write to websocket connection" (panic de gorilla que
// tumba el proceso). En producción hay al menos: ticks de alertas, acumulación
// de cargadores, backfill y handlers HTTP concurrentes.
func TestConcurrentCallsNoPanic(t *testing.T) {
	srv := fakeHAConcurrent(t)
	defer srv.Close()

	client := NewClient(srv.URL, "tok")
	client.Start()
	defer client.Stop()

	if !waitFor(t, client.IsConnected, 3*time.Second) {
		t.Fatal("no conectó")
	}

	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			// recover explícito: si gorilla paniquea, lo convertimos en fallo legible
			defer func() {
				if r := recover(); r != nil {
					errs <- &panicErr{msg: r}
				}
			}()
			_, err := client.Call(map[string]any{"type": "ping_test"})
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	var panics int
	for err := range errs {
		if _, ok := err.(*panicErr); ok {
			panics++
			t.Logf("PANIC: %v", err)
		}
	}
	if panics > 0 {
		t.Fatalf("%d llamadas concurrentes paniquearon (escritura concurrente al WS)", panics)
	}
}

type panicErr struct{ msg any }

func (p *panicErr) Error() string {
	b, _ := json.Marshal(p.msg)
	return "panic: " + string(b)
}

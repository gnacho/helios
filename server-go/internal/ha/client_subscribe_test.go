package ha

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeHACounting simula HA y cuenta los comandos subscribe_entities recibidos.
func fakeHACounting(t *testing.T, frames *atomic.Int32) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !endsWithWebSocket(r.URL.Path) {
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
			if msg["type"] == "subscribe_entities" {
				frames.Add(1)
			}
			conn.WriteJSON(map[string]any{"id": id, "type": "result", "success": true, "result": nil})
		}
	}))
}

func endsWithWebSocket(path string) bool {
	return len(path) >= len("/api/websocket") && path[len(path)-len("/api/websocket"):] == "/api/websocket"
}

func TestSubscribeEntitiesSkipsDuplicate(t *testing.T) {
	var frames atomic.Int32
	srv := fakeHACounting(t, &frames)
	defer srv.Close()

	client := NewClient(srv.URL, "good-token")
	client.Start()
	defer client.Stop()

	if !waitFor(t, client.IsConnected, 2*time.Second) {
		t.Fatal("el cliente no conectó")
	}
	if err := client.SubscribeEntities([]string{"sensor.a"}); err != nil {
		t.Fatalf("primera suscripción: %v", err)
	}
	if !waitFor(t, func() bool { return frames.Load() == 1 }, 2*time.Second) {
		t.Fatalf("frames = %d, want 1 tras la primera suscripción", frames.Load())
	}

	// La misma lista no debe reenviar el comando: HA apilaría suscripciones.
	if err := client.SubscribeEntities([]string{"sensor.a"}); err != nil {
		t.Fatalf("resuscripción idéntica: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if frames.Load() != 1 {
		t.Fatalf("frames = %d, want 1: la lista idéntica no debe reenviarse", frames.Load())
	}

	// Una lista distinta sí debe enviarse (entidades nuevas de config).
	if err := client.SubscribeEntities([]string{"sensor.a", "timer.new"}); err != nil {
		t.Fatalf("suscripción ampliada: %v", err)
	}
	if !waitFor(t, func() bool { return frames.Load() == 2 }, 2*time.Second) {
		t.Fatalf("frames = %d, want 2 tras añadir una entidad nueva", frames.Load())
	}
}

func TestSubscribeEntitiesStoresWhenDisconnected(t *testing.T) {
	client := NewClient("http://127.0.0.1:1", "good-token")

	// Sin conexión no debe fallar: las IDs quedan almacenadas y auth_ok las
	// reenvía al reconectar.
	if err := client.SubscribeEntities([]string{"sensor.a"}); err != nil {
		t.Fatalf("suscripción sin conexión: %v", err)
	}
	client.mu.RLock()
	defer client.mu.RUnlock()
	if len(client.subscribed) != 1 || client.subscribed[0] != "sensor.a" {
		t.Fatalf("subscribed = %v, want [sensor.a]", client.subscribed)
	}
}

func TestSubscribeEntitiesOrderInsensitive(t *testing.T) {
	var frames atomic.Int32
	srv := fakeHACounting(t, &frames)
	defer srv.Close()

	client := NewClient(srv.URL, "good-token")
	client.Start()
	defer client.Stop()

	if !waitFor(t, client.IsConnected, 2*time.Second) {
		t.Fatal("el cliente no conectó")
	}
	if err := client.SubscribeEntities([]string{"a", "b", "c"}); err != nil {
		t.Fatalf("primera suscripción: %v", err)
	}
	if !waitFor(t, func() bool { return frames.Load() == 1 }, 2*time.Second) {
		t.Fatalf("frames = %d, want 1", frames.Load())
	}
	// El mismo conjunto en otro orden no debe reenviarse.
	if err := client.SubscribeEntities([]string{"c", "a", "b"}); err != nil {
		t.Fatalf("resuscripción reordenada: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if frames.Load() != 1 {
		t.Fatalf("frames = %d, want 1: el orden no debe importar", frames.Load())
	}
}

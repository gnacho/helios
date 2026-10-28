package ha

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestReadLimitIsConfigured(t *testing.T) {
	if maxWSMessageBytes <= 0 {
		t.Fatal("maxWSMessageBytes debe ser positivo")
	}
	if maxWSMessageBytes > 8<<20 {
		t.Fatalf("maxWSMessageBytes = %d, demasiado grande para un mensaje de estado", maxWSMessageBytes)
	}
}

func TestRESTResponseSizeIsBounded(t *testing.T) {
	if maxRESTResponseBytes <= 0 {
		t.Fatal("maxRESTResponseBytes debe ser positivo")
	}
}

func TestHistoryDuringPeriodRejectsOversizedBody(t *testing.T) {
	huge := strings.Repeat("x", maxRESTResponseBytes+4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `[[{"state":"%s"}]]`, huge)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "token")
	if _, err := c.HistoryDuringPeriod("2026-10-01T00:00:00", "2026-10-02T00:00:00", "sensor.x"); err == nil {
		t.Fatal("se acepto una respuesta REST por encima del limite")
	}
}

func TestHistoryDuringPeriodCapsRows(t *testing.T) {
	rows := make([]map[string]string, maxHistoryRows+50)
	for i := range rows {
		rows[i] = map[string]string{"state": "1"}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([][]map[string]string{rows})
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "token")
	got, err := c.HistoryDuringPeriod("2026-10-01T00:00:00", "2026-10-02T00:00:00", "sensor.x")
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
	if len(got) > maxHistoryRows {
		t.Fatalf("filas = %d, maximo %d", len(got), maxHistoryRows)
	}
}

func TestReadLoopClosesOnOversizedMessage(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	closed := make(chan struct{}, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"auth_required"}`))
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"auth_ok"}`))
		big := []byte(`{"type":"event","payload":"` + strings.Repeat("a", maxWSMessageBytes+1024) + `"}`)
		_ = conn.WriteMessage(websocket.TextMessage, big)
		select {
		case <-closed:
		case <-time.After(5 * time.Second):
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "token")
	c.OnDisconnect = func() {
		select {
		case closed <- struct{}{}:
		default:
		}
	}
	c.Start()
	defer c.Stop()

	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("el cliente no cerro la conexion ante un mensaje por encima del limite")
	}
}

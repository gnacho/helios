package sse

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func payloadFn() []byte { return []byte(`{"type":"live","data":{"production":1.5}}`) }

func TestStreamSendsInitialSnapshot(t *testing.T) {
	hub := NewHub(payloadFn)
	srv := httptest.NewServer(hub)
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}
	if resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("falta X-Accel-Buffering")
	}

	reader := bufio.NewReader(resp.Body)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.HasPrefix(line, "data: ") {
		t.Fatalf("línea inicial = %q", line)
	}
	if !strings.Contains(line, `"production":1.5`) {
		t.Fatalf("payload inesperado: %q", line)
	}
}

func TestBroadcastReachesClients(t *testing.T) {
	hub := NewHub(payloadFn)
	srv := httptest.NewServer(hub)
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	// consumir snapshot inicial
	reader.ReadString('\n')
	reader.ReadString('\n')

	// Esperar a que el throttle permita emitir.
	time.Sleep(1100 * time.Millisecond)
	hub.Broadcast()

	done := make(chan string, 1)
	go func() {
		line, _ := reader.ReadString('\n')
		done <- line
	}()

	select {
	case line := <-done:
		if !strings.HasPrefix(line, "data: ") {
			t.Fatalf("broadcast inesperado: %q", line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no llegó el broadcast")
	}
}

func TestThrottleDropsRapidBroadcasts(t *testing.T) {
	hub := NewHub(payloadFn)
	srv := httptest.NewServer(hub)
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	reader.ReadString('\n')
	reader.ReadString('\n')

	// Dos broadcasts seguidos: el segundo debe descartarse por throttle.
	time.Sleep(1100 * time.Millisecond)
	hub.Broadcast()
	hub.Broadcast()

	// Debe llegar exactamente uno.
	line1, _ := reader.ReadString('\n')
	if !strings.HasPrefix(line1, "data: ") {
		t.Fatalf("primero = %q", line1)
	}
	reader.ReadString('\n')

	// No debe haber un segundo mensaje inmediato.
	gotSecond := make(chan string, 1)
	go func() {
		l, _ := reader.ReadString('\n')
		gotSecond <- l
	}()
	select {
	case l := <-gotSecond:
		if strings.HasPrefix(l, "data: ") {
			t.Fatalf("segundo broadcast no debería emitirse por throttle: %q", l)
		}
	case <-time.After(500 * time.Millisecond):
		// correcto: no llegó nada
	}
}

func TestMaxClientsRejected(t *testing.T) {
	hub := NewHub(payloadFn)
	srv := httptest.NewServer(hub)
	defer srv.Close()

	var conns []*http.Response
	defer func() {
		for _, c := range conns {
			c.Body.Close()
		}
	}()

	for i := 0; i < MaxClients; i++ {
		resp, err := http.Get(srv.URL)
		if err != nil {
			t.Fatalf("get %d: %v", i, err)
		}
		conns = append(conns, resp)
	}

	// El siguiente debe rechazarse.
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("get extra: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d (esperado 429)", resp.StatusCode)
	}
}

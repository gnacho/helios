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

// TestMaxClientsPerPool: los dos pools tienen presupuesto independiente (#206).
// Llenar la pool legacy (API externa) no debe bloquear la UI templ, y llenar la
// pool Datastar no debe bloquear los consumidores del endpoint legacy.
func TestMaxClientsPerPool(t *testing.T) {
	hub := NewHub(payloadFn)
	srv := httptest.NewServer(hub)
	defer srv.Close()

	var conns []*http.Response
	defer func() {
		for _, c := range conns {
			c.Body.Close()
		}
	}()

	// Pool legacy llena.
	for i := 0; i < MaxClients; i++ {
		resp, err := http.Get(srv.URL)
		if err != nil {
			t.Fatalf("legacy get %d: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("legacy get %d: status = %d", i, resp.StatusCode)
		}
		conns = append(conns, resp)
	}

	// Otra conexión legacy se rechaza.
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("legacy extra: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("legacy extra: status = %d (esperado 429)", resp.StatusCode)
	}

	// La pool Datastar sigue aceptando: la legacy no puede hambrunarla.
	ch := hub.SubscribeDatastar()
	if ch == nil {
		t.Fatal("Datastar devolvió nil con la pool legacy llena (presupuesto compartido)")
	}
	hub.UnsubscribeDatastar(ch)

	// Pool Datastar llena: la legacy sigue aceptando conexiones.
	var dsChans []chan []byte
	for i := 0; i < MaxClients; i++ {
		c := hub.SubscribeDatastar()
		if c == nil {
			t.Fatalf("datastar subscribe %d devolvió nil", i)
		}
		dsChans = append(dsChans, c)
	}
	if hub.SubscribeDatastar() != nil {
		t.Fatal("datastar extra debería devolver nil")
	}
	for _, c := range dsChans {
		hub.UnsubscribeDatastar(c)
	}

	// Libero la pool legacy para comprobar que vuelve a aceptar. El servidor
	// da de baja al cliente cuando su handler sale (ctx.Done), no al cerrar
	// el body desde el cliente: espero acotadamente a que la pool se vacíe.
	for _, c := range conns {
		c.Body.Close()
	}
	conns = nil
	deadline := time.Now().Add(2 * time.Second)
	for hub.legacyCount() > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if hub.legacyCount() > 0 {
		t.Fatal("la pool legacy no se vació al cerrar las conexiones")
	}

	respOK, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("legacy tras datastar llena: %v", err)
	}
	respOK.Body.Close()
	if respOK.StatusCode != http.StatusOK {
		t.Fatalf("legacy tras datastar llena: status = %d (esperado 200)", respOK.StatusCode)
	}
}

// TestBroadcastReachesDatastar: la UI templ solo suscribe en el pool Datastar,
// sin clientes legacy. Broadcast salía antes de recorrer esa pool (early
// return si h.clients estaba vacío), así que el stream se quedaba congelado en
// el snapshot inicial y la página no se actualizaba nunca.
func TestBroadcastReachesDatastar(t *testing.T) {
	h := NewHub(func() []byte { return []byte(`{"n":1}`) })
	ch := h.SubscribeDatastar()
	if ch == nil {
		t.Fatal("sin suscripción Datastar")
	}
	defer h.UnsubscribeDatastar(ch)

	h.Broadcast()
	select {
	case payload := <-ch:
		if string(payload) != `{"n":1}` {
			t.Fatalf("payload = %q", payload)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("el pool Datastar no recibió el broadcast")
	}
}

// TestBroadcastDatastarConcurrentSubscribe: el broadcast no debe iterar el mapa
// fuera del lock (carrera con subscribe/unsubscribe).
func TestBroadcastDatastarConcurrentSubscribe(t *testing.T) {
	h := NewHub(func() []byte { return []byte(`{}`) })
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			ch := h.SubscribeDatastar()
			if ch != nil {
				h.UnsubscribeDatastar(ch)
			}
		}
	}()
	for i := 0; i < 200; i++ {
		// Sin throttle: cada vuelta recorre las dos pools.
		h.mu.Lock()
		h.last = time.Time{}
		h.mu.Unlock()
		h.Broadcast()
	}
	<-done
}

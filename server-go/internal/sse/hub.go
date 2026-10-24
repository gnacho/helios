package sse

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	MaxClients    = 10
	throttleDelay = time.Second
	heartbeatEach = 25 * time.Second
)

type client struct {
	ch chan []byte
}

// Hub gestiona los clientes SSE y el throttle de emisión.
// Dos pools: `clients` recibe el mensaje SSE plano ya formateado (API React)
// y `dsClients` recibe el payload CRUDO para endpoints Datastar (UI templ).
type Hub struct {
	mu        sync.Mutex
	clients   map[*client]struct{}
	dsClients map[*client]struct{}
	last      time.Time
	// PayloadFn construye el mensaje live completo.
	PayloadFn func() []byte
}

func NewHub(payloadFn func() []byte) *Hub {
	return &Hub{
		clients:   map[*client]struct{}{},
		dsClients: map[*client]struct{}{},
		PayloadFn: payloadFn,
	}
}

func (h *Hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients) + len(h.dsClients)
}

// Count expone el total de clientes conectados (ambas pools).
func (h *Hub) Count() int { return h.count() }

// SubscribeDatastar registra un receptor del payload crudo (sin prefijo
// "data: "). Devuelve nil si se alcanzó el máximo de clientes.
func (h *Hub) SubscribeDatastar() chan []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.clients)+len(h.dsClients) >= MaxClients {
		return nil
	}
	c := &client{ch: make(chan []byte, 8)}
	h.dsClients[c] = struct{}{}
	return c.ch
}

// UnsubscribeDatastar da de baja un receptor Datastar.
func (h *Hub) UnsubscribeDatastar(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.dsClients {
		if c.ch == ch {
			delete(h.dsClients, c)
			return
		}
	}
}

// Broadcast envía a todos los clientes respetando el throttle de 1 s.
func (h *Hub) Broadcast() {
	h.mu.Lock()
	// Las dos pools cuentan: la UI templ solo usa dsClients, así que salir
	// cuando no hay clientes legacy dejaba el stream de la UI congelado en el
	// snapshot inicial (bug real: la página no se actualizaba nunca).
	if len(h.clients) == 0 && len(h.dsClients) == 0 {
		h.mu.Unlock()
		return
	}
	now := time.Now()
	if now.Sub(h.last) < throttleDelay {
		h.mu.Unlock()
		return
	}
	h.last = now
	clients := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	// Copia de la pool Datastar bajo el mismo lock: iterar el mapa fuera de él
	// es una carrera con Subscribe/Unsubscribe (fatal error, no recuperable).
	dsClients := make([]*client, 0, len(h.dsClients))
	for c := range h.dsClients {
		dsClients = append(dsClients, c)
	}
	h.mu.Unlock()

	payload := h.PayloadFn()
	msg := append([]byte("data: "), payload...)
	msg = append(msg, '\n', '\n')

	for _, c := range clients {
		select {
		case c.ch <- msg:
		default:
			// Cliente lento: se descarta (se limpiará al cerrar).
		}
	}

	// Pool Datastar: payload crudo (el endpoint lo convierte en señales).
	for _, c := range dsClients {
		select {
		case c.ch <- payload:
		default:
		}
	}
}

// ServeHTTP implementa GET /api/solar/stream.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.count() >= MaxClients {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"demasiados clientes conectados"}`))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	c := &client{ch: make(chan []byte, 8)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
	}()

	// Snapshot inicial.
	initial := append([]byte("data: "), h.PayloadFn()...)
	initial = append(initial, '\n', '\n')
	fmt.Fprint(w, string(initial))
	flusher.Flush()

	heartbeat := time.NewTicker(heartbeatEach)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-c.ch:
			if _, err := w.Write(msg); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, `data: {"type":"ping"}`+"\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// LogStart deja constancia de la puesta en marcha del hub.
func (h *Hub) LogStart() {
	log.Printf("[sse] hub listo (max %d clientes)", MaxClients)
}

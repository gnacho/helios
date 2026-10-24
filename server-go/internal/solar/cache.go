package solar

import (
	"fmt"
	"sync"
	"time"
)

// seriesCache replica el cachedCollector del original: TTL por clave y
// single-flight (N peticiones concurrentes de la misma fecha = UNA consulta a
// HAOS). Sin esto, cada carga de la página lanzaba una query de statistics de
// ~140 KB contra HAOS (regresión detectada en la auditoría 22-Oct-2026).
type seriesCache struct {
	mu       sync.Mutex
	entries  map[string]cacheEntry
	inflight map[string]*cacheCall
	max      int
}

type cacheEntry struct {
	val DaySeries
	at  time.Time
	ttl time.Duration
}

type cacheCall struct {
	done chan struct{}
	val  DaySeries
	err  error
}

func newSeriesCache() *seriesCache {
	return &seriesCache{
		entries:  map[string]cacheEntry{},
		inflight: map[string]*cacheCall{},
		max:      300,
	}
}

func (c *seriesCache) get(key string) (DaySeries, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return DaySeries{}, false
	}
	if time.Since(e.at) >= e.ttl {
		delete(c.entries, key)
		return DaySeries{}, false
	}
	return e.val, true
}

func (c *seriesCache) put(key string, val DaySeries, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.max {
		// Desaloja la entrada más antigua (suficiente: las series no cambian
		// salvo al consolidarse el día).
		var oldestKey string
		var oldest time.Time
		for k, e := range c.entries {
			if oldestKey == "" || e.at.Before(oldest) {
				oldestKey, oldest = k, e.at
			}
		}
		if oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}
	c.entries[key] = cacheEntry{val: val, at: time.Now(), ttl: ttl}
}

// do ejecuta fn con single-flight por clave.
func (c *seriesCache) do(key string, ttl time.Duration, fn func() (DaySeries, error)) (DaySeries, error) {
	if v, ok := c.get(key); ok {
		return v, nil
	}

	c.mu.Lock()
	if call, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-call.done
		return call.val, call.err
	}
	call := &cacheCall{done: make(chan struct{})}
	c.inflight[key] = call
	c.mu.Unlock()

	// La liberación va en un defer: si fn entra en panic, sin esto la entrada
	// se queda para siempre en inflight y todo el que pida esa clave se bloquea
	// en <-call.done (servicio colgado hasta reiniciar).
	defer func() {
		p := recover()
		if p != nil {
			call.err = fmt.Errorf("cache %s: panic: %v", key, p)
		}
		c.mu.Lock()
		delete(c.inflight, key)
		c.mu.Unlock()
		close(call.done)
		if p != nil {
			panic(p)
		}
	}()

	call.val, call.err = fn()

	if call.err == nil && len(call.val.Points) > 0 {
		c.put(key, call.val, ttl)
	}
	return call.val, call.err
}

// ttlFor: hoy 60 s (cambia con cada bucket de 5 min), días pasados 6 h.
func ttlFor(dateKey string) time.Duration {
	if dateKey == TodayStr() {
		return 60 * time.Second
	}
	return 6 * time.Hour
}

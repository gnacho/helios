package solar

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"helios-server/internal/ha"
	"helios-server/internal/install"
)

// TestSeriesCacheSingleFlight: N peticiones concurrentes de la misma fecha
// deben provocar UNA sola consulta a HAOS (cachedCollector del original).
func TestSeriesCacheSingleFlight(t *testing.T) {
	var calls int32
	slow := &slowHA{
		fakeHA: newFake(),
		delay:  80 * time.Millisecond,
		calls:  &calls,
	}
	slow.set("sensor.solis_potencia_actual", "2")
	slow.set("sensor.almacen_pinza_power_b", "0")
	slow.set("sensor.medidor_respaldo_power", "100")
	slow.set("sensor.vivienda_medidor_power", "100")
	slow.set("sensor.almacen_pinza_power_a", "0")
	slow.set("sensor.solis_bateria_potencia", "0")
	slow.set("sensor.solis_bateria_soc", "50")

	s := New(install.NewStore(), nil)

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = s.GetDaySeries(slow, "2026-10-20")
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("consultas a HAOS = %d (esperado 1 con single-flight)", got)
	}
}

// TestSeriesCacheTTL: la segunda llamada dentro del TTL no consulta HAOS.
func TestSeriesCacheTTL(t *testing.T) {
	var calls int32
	slow := &slowHA{fakeHA: newFake(), delay: 5 * time.Millisecond, calls: &calls}
	slow.set("sensor.solis_potencia_actual", "1")
	// Con datos en la serie (si sale vacía, la caché no la guarda y es correcto).
	slow.stats = ha.Stats{
		"sensor.solis_potencia_actual":  {{Start: flexT(2026, 10, 19, 12), Mean: floatPtr(1.0)}},
		"sensor.almacen_pinza_power_b":  {{Start: flexT(2026, 10, 19, 12), Mean: floatPtr(0)}},
		"sensor.medidor_respaldo_power": {{Start: flexT(2026, 10, 19, 12), Mean: floatPtr(100)}},
		"sensor.vivienda_medidor_power": {{Start: flexT(2026, 10, 19, 12), Mean: floatPtr(100)}},
		"sensor.almacen_pinza_power_a":  {{Start: flexT(2026, 10, 19, 12), Mean: floatPtr(0)}},
		"sensor.solis_bateria_potencia": {{Start: flexT(2026, 10, 19, 12), Mean: floatPtr(0)}},
		"sensor.solis_bateria_soc":      {{Start: flexT(2026, 10, 19, 12), Mean: floatPtr(50)}},
	}

	s := New(install.NewStore(), nil)
	_, _ = s.GetDaySeries(slow, "2026-10-19")
	_, _ = s.GetDaySeries(slow, "2026-10-19")

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("consultas = %d (esperado 1 por TTL)", got)
	}
}

func flexT(y int, m time.Month, d, h int) ha.FlexTime {
	return ha.FlexTime{Time: time.Date(y, m, d, h, 0, 0, 0, time.Local)}
}

type slowHA struct {
	*fakeHA
	delay time.Duration
	calls *int32
}

func (s *slowHA) StatisticsDuringPeriod(start, end string, ids []string, period string, types []string) (ha.Stats, error) {
	atomic.AddInt32(s.calls, 1)
	time.Sleep(s.delay)
	return s.fakeHA.StatisticsDuringPeriod(start, end, ids, period, types)
}

// TestCachePanicDoesNotPoisonKey: un panic dentro de fn no debe dejar la clave
// bloqueada para siempre (los waiters se quedaban en <-call.done).
func TestCachePanicDoesNotPoisonKey(t *testing.T) {
	c := newSeriesCache()
	func() {
		defer func() { _ = recover() }()
		_, _ = c.do("k", time.Minute, func() (DaySeries, error) { panic("boom") })
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := c.do("k", time.Minute, func() (DaySeries, error) { return DaySeries{}, nil }); err != nil {
			t.Errorf("segunda llamada: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("la clave quedó bloqueada tras el panic")
	}
}

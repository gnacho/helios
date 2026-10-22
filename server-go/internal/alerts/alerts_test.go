package alerts

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/ha"
	"helios-server/internal/install"
	"helios-server/internal/push"
)

type fakeHA struct {
	states map[string]ha.EntityState
}

func newFake() *fakeHA {
	return &fakeHA{states: map[string]ha.EntityState{}}
}

func (f *fakeHA) set(id, state string) {
	f.states[id] = ha.EntityState{State: state, Attributes: map[string]any{}}
}

func (f *fakeHA) setAttrs(id string, attrs map[string]any) {
	st := f.states[id]
	st.Attributes = attrs
	f.states[id] = st
}

func (f *fakeHA) GetState(id string) (ha.EntityState, bool) {
	st, ok := f.states[id]
	return st, ok
}

func (f *fakeHA) IsConnected() bool { return true }

type recorder struct {
	mu     sync.Mutex
	events []string
	datos  []push.Datos
}

func (r *recorder) notify(tipo string, datos push.Datos, opts push.Options) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, tipo)
	r.datos = append(r.datos, datos)
}

func (r *recorder) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.events...)
}

func newEngine(t *testing.T, live LiveSnapshot) (*Engine, *fakeHA, *recorder) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "alerts.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	f := newFake()
	f.set("sun.sun", "above_horizon")
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(30)})
	f.setAttrs("sensor.solis_scraper", map[string]any{
		"inverterOnline": float64(1),
		"lastUpdate":     time.Now().UTC().Format(time.RFC3339),
	})
	f.set("sensor.solis_potencia_actual", "2")
	f.set("sensor.almacen_pinza_power_b", "1")

	rec := &recorder{}
	e := New(d, f, install.NewStore(), rec.notify)
	e.LiveFn = func(HA) LiveSnapshot { return live }
	return e, f, rec
}

func TestInversorOfflineNeedsTwoTicks(t *testing.T) {
	e, f, rec := newEngine(t, LiveSnapshot{Soc: 50})
	f.setAttrs("sensor.solis_scraper", map[string]any{
		"inverterOnline": float64(0),
		"lastUpdate":     time.Now().UTC().Format(time.RFC3339),
	})

	e.Tick()
	if len(rec.list()) != 0 {
		t.Fatalf("no debería alertar al primer tick: %v", rec.list())
	}
	e.Tick()
	if len(rec.list()) != 1 || rec.list()[0] != "inversor_offline" {
		t.Fatalf("eventos = %v", rec.list())
	}
	// Persistente: no repite.
	e.Tick()
	if len(rec.list()) != 1 {
		t.Fatalf("no debería repetir: %v", rec.list())
	}
	// Recuperación.
	f.setAttrs("sensor.solis_scraper", map[string]any{
		"inverterOnline": float64(1),
		"lastUpdate":     time.Now().UTC().Format(time.RFC3339),
	})
	e.Tick()
	events := rec.list()
	if len(events) != 2 || events[1] != "inversor_ok" {
		t.Fatalf("eventos = %v", events)
	}
}

func TestInversorOfflineNotAtNight(t *testing.T) {
	e, f, rec := newEngine(t, LiveSnapshot{Soc: 50})
	f.set("sun.sun", "below_horizon")
	f.setAttrs("sensor.solis_scraper", map[string]any{
		"inverterOnline": float64(0),
		"lastUpdate":     time.Now().UTC().Format(time.RFC3339),
	})

	for i := 0; i < 5; i++ {
		e.Tick()
	}
	if len(rec.list()) != 0 {
		t.Fatalf("de noche no debe alertar: %v", rec.list())
	}
}

func TestFoxOfflineNeedsThreeTicks(t *testing.T) {
	e, f, rec := newEngine(t, LiveSnapshot{Soc: 50})
	f.set("sensor.almacen_pinza_power_b", "unavailable")

	e.Tick()
	e.Tick()
	if len(rec.list()) != 0 {
		t.Fatalf("no debería alertar con 2 ticks: %v", rec.list())
	}
	e.Tick()
	events := rec.list()
	if len(events) != 1 || events[0] != "fox_offline" {
		t.Fatalf("eventos = %v", events)
	}
	if rec.datos[0]["nombre"] != "Fox" {
		t.Fatalf("nombre = %v", rec.datos[0]["nombre"])
	}

	// Recuperación.
	f.set("sensor.almacen_pinza_power_b", "1200")
	e.Tick()
	events = rec.list()
	if len(events) != 2 || events[1] != "fox_ok" {
		t.Fatalf("eventos = %v", events)
	}
}

func TestBateriaBajaHysteresis(t *testing.T) {
	e, f, rec := newEngine(t, LiveSnapshot{Soc: 15})

	e.Tick()
	if len(rec.list()) != 1 || rec.list()[0] != "bateria_baja" {
		t.Fatalf("eventos = %v", rec.list())
	}

	// Persistente: no repite.
	e.Tick()
	if len(rec.list()) != 1 {
		t.Fatalf("no debería repetir: %v", rec.list())
	}

	// Sube poco (dentro de histéresis 20+5=25): sigue alertado.
	e.LiveFn = func(HA) LiveSnapshot { return LiveSnapshot{Soc: 24} }
	e.Tick()
	if len(rec.list()) != 1 {
		t.Fatalf("no debería rearmar con SOC 24: %v", rec.list())
	}

	// Por encima de la histéresis: rearma (sin notificar).
	e.LiveFn = func(HA) LiveSnapshot { return LiveSnapshot{Soc: 30} }
	e.Tick()

	// Vuelve a bajar: nueva alerta.
	e.LiveFn = func(HA) LiveSnapshot { return LiveSnapshot{Soc: 18} }
	e.Tick()
	if len(rec.list()) != 2 {
		t.Fatalf("debería volver a alertar: %v", rec.list())
	}
	_ = f
}

func TestBateriaLlenaEdge(t *testing.T) {
	e, _, rec := newEngine(t, LiveSnapshot{Soc: 100})

	e.Tick()
	if len(rec.list()) != 1 || rec.list()[0] != "bateria_llena" {
		t.Fatalf("eventos = %v", rec.list())
	}

	// Oscila en 99: no repite.
	e.LiveFn = func(HA) LiveSnapshot { return LiveSnapshot{Soc: 99} }
	e.Tick()
	if len(rec.list()) != 1 {
		t.Fatalf("no debería repetir en 99: %v", rec.list())
	}

	// Baja de 95: rearma y vuelve a avisar al llegar a 100.
	e.LiveFn = func(HA) LiveSnapshot { return LiveSnapshot{Soc: 90} }
	e.Tick()
	e.LiveFn = func(HA) LiveSnapshot { return LiveSnapshot{Soc: 100} }
	e.Tick()
	if len(rec.list()) != 2 {
		t.Fatalf("debería avisar de nuevo: %v", rec.list())
	}
}

func TestProximoEnvioResumenUsesSunset(t *testing.T) {
	e, f, _ := newEngine(t, LiveSnapshot{})
	sunset := time.Now().Add(90 * time.Minute)
	f.setAttrs("sun.sun", map[string]any{
		"elevation":    float64(10),
		"next_setting": sunset.UTC().Format(time.RFC3339),
	})

	next := e.ProximoEnvioResumen(time.Now())
	diff := next.Sub(sunset)
	if diff < 9*time.Minute || diff > 11*time.Minute {
		t.Fatalf("offset = %v (esperado ~10 min)", diff)
	}
}

func TestProximoEnvioResumenFallback(t *testing.T) {
	e, f, _ := newEngine(t, LiveSnapshot{})
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(10)}) // sin next_setting

	ahora := time.Date(2026, 10, 22, 12, 0, 0, 0, time.Local)
	next := e.ProximoEnvioResumen(ahora)
	if next.Hour() != HoraResumenDiario {
		t.Fatalf("hora = %d (esperado %d)", next.Hour(), HoraResumenDiario)
	}
	if !next.After(ahora) {
		t.Fatal("debería ser futuro")
	}

	// Si ya pasó la hora, salta al día siguiente.
	late := time.Date(2026, 10, 22, 22, 0, 0, 0, time.Local)
	next2 := e.ProximoEnvioResumen(late)
	if next2.Day() != 23 {
		t.Fatalf("día = %d (esperado 23)", next2.Day())
	}
}

func TestEjecutarResumenSiTocaNoDuplica(t *testing.T) {
	e, _, rec := newEngine(t, LiveSnapshot{})
	kpis := func() (KpisResumen, error) {
		return KpisResumen{ProductionKwh: 20, ConsumptionKwh: 15, AutoconsumoPct: 75}, nil
	}

	enviado, err := e.EjecutarResumenSiToca(kpis)
	if err != nil || !enviado {
		t.Fatalf("primero: enviado=%v err=%v", enviado, err)
	}
	if len(rec.list()) != 1 || rec.list()[0] != "resumen_diario" {
		t.Fatalf("eventos = %v", rec.list())
	}

	enviado2, err := e.EjecutarResumenSiToca(kpis)
	if err != nil || enviado2 {
		t.Fatalf("segundo: no debería enviar (enviado=%v err=%v)", enviado2, err)
	}
	if len(rec.list()) != 1 {
		t.Fatalf("no debería duplicar: %v", rec.list())
	}
}

func TestTickSkipsWhenDisconnected(t *testing.T) {
	e, _, rec := newEngine(t, LiveSnapshot{Soc: 10})
	// HA desconectado
	e.HA = &disconnectedHA{}
	e.Tick()
	if len(rec.list()) != 0 {
		t.Fatalf("sin HAOS no debe evaluar: %v", rec.list())
	}
}

type disconnectedHA struct{}

func (d *disconnectedHA) GetState(string) (ha.EntityState, bool) { return ha.EntityState{}, false }
func (d *disconnectedHA) IsConnected() bool                      { return false }

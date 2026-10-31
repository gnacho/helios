package alerts

import (
	"testing"
	"time"

	"helios-server/internal/extensions"
)

// TestChargerScheduledNotifications: la carga programada avisa una vez al
// empezar (carga_inicio) y una vez al acabar (carga_fin). Una carga manual
// (sin armado ni timer) no avisa.
func TestChargerScheduledNotifications(t *testing.T) {
	e, f, rec := newEngine(t, LiveSnapshot{})
	charger := extensions.Charger{
		Key: "main", Name: "Pared",
		PowerID:         "sensor.p",
		ScheduleArmID:   "input_boolean.coche_programado",
		ScheduleTimerID: "timer.coche_carga",
	}
	e.ChargersFn = func() []extensions.Charger { return []extensions.Charger{charger} }

	// 1. Programación armada, sin carga: sin avisos.
	f.set("sensor.p", "0")
	f.set("input_boolean.coche_programado", "on")
	f.set("timer.coche_carga", "idle")
	e.Tick()
	if got := rec.list(); len(got) != 0 {
		t.Fatalf("armado sin carga no debería avisar, avisó: %v", got)
	}

	// 2. Empieza la carga (timer active): un solo carga_inicio con la hora de fin.
	f.set("sensor.p", "1.5")
	f.set("timer.coche_carga", "active")
	f.setAttrs("timer.coche_carga", map[string]any{
		"finishes_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
	})
	e.Tick()
	got := rec.list()
	if len(got) != 1 || got[0] != "carga_inicio" {
		t.Fatalf("esperaba [carga_inicio], hay: %v", got)
	}
	if len(rec.datos) != 1 || rec.datos[0]["nombre"] != "Pared" || rec.datos[0]["fin"] == "" {
		t.Fatalf("datos de carga_inicio incompletos: %+v", rec.datos)
	}

	// 3. Sigue cargando en el siguiente tick: sin duplicado.
	e.Tick()
	if got := rec.list(); len(got) != 1 {
		t.Fatalf("la carga en marcha no debería avisar otra vez, avisó: %v", got)
	}

	// 4. Termina la carga: un solo carga_fin.
	f.set("sensor.p", "0")
	f.set("timer.coche_carga", "idle")
	f.set("input_boolean.coche_programado", "off")
	e.Tick()
	got = rec.list()
	if len(got) != 2 || got[1] != "carga_fin" {
		t.Fatalf("esperaba [carga_inicio carga_fin], hay: %v", got)
	}

	// 5. Carga manual después (sin armado ni timer): sin avisos.
	f.set("sensor.p", "1.5")
	e.Tick()
	f.set("sensor.p", "0")
	e.Tick()
	if got := rec.list(); len(got) != 2 {
		t.Fatalf("una carga manual no debería avisar, avisó: %v", got)
	}
}

// TestChargerTickWithoutChargersFn: sin ChargersFn el tick no toca nada.
func TestChargerTickWithoutChargersFn(t *testing.T) {
	e, f, rec := newEngine(t, LiveSnapshot{})
	f.set("sensor.p", "1.5")
	e.Tick()
	if got := rec.list(); len(got) != 0 {
		t.Fatalf("sin ChargersFn no debería haber avisos de cargador: %v", got)
	}
}

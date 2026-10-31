package extensions

import (
	"errors"
	"testing"
	"time"
)

// TestApplyScheduleBlockedWhenActive checks that a live schedule (arm on, or
// the charge running under the HA timer) blocks scheduling another window:
// the current one must be stopped first.
func TestApplyScheduleBlockedWhenActive(t *testing.T) {
	ch := Charger{
		Key: "main", Name: "Coche",
		ScheduleStartID:    "input_datetime.i",
		ScheduleDurationID: "input_number.d",
		ScheduleArmID:      "input_boolean.a",
		ScheduleTimerID:    "timer.t",
		SwitchID:           "switch.c",
	}
	at := time.Now().Add(2 * time.Hour).Format("15:04")

	// Armado: la ventana planeada sigue vigente.
	f := newFake()
	f.set("input_boolean.a", "on")
	if err := ApplySchedule(f, ch, at, 2); !errors.Is(err, ErrChargerScheduleActive) {
		t.Fatalf("armado: err = %v (esperado ErrChargerScheduleActive)", err)
	}

	// Carga en marcha por timer (arm ya apagado al arrancar la carga).
	f = newFake()
	f.set("timer.t", "active")
	if err := ApplySchedule(f, ch, at, 2); !errors.Is(err, ErrChargerScheduleActive) {
		t.Fatalf("timer activo: err = %v (esperado ErrChargerScheduleActive)", err)
	}

	// Sin programación vigente: la ventana futura se arma sin error.
	f = newFake()
	if err := ApplySchedule(f, ch, at, 2); err != nil {
		t.Fatalf("libre: err = %v (esperado nil)", err)
	}
}

// TestApplyActionOffClearsActiveSchedule checks that an explicit off ends the
// whole schedule: the running timer (planned end disappears) and the armed
// future window (nothing reconnects at its start). Chargers without schedule
// entities just switch off; "on" never touches the plan.
func TestApplyActionOffClearsActiveSchedule(t *testing.T) {
	ch := Charger{
		Key: "main", Name: "Coche",
		SwitchID:        "switch.c",
		ScheduleArmID:   "input_boolean.a",
		ScheduleTimerID: "timer.t",
	}

	// Carga programada en marcha: apagar corta la sesion y limpia el plan.
	f := newFake()
	f.set("switch.c", "on")
	f.set("timer.t", "active")
	if _, err := ApplyAction(f, ch, "off", nil); err != nil {
		t.Fatalf("off en marcha: err = %v", err)
	}
	if !f.called("switch/turn_off") || !f.called("input_boolean/turn_off") || !f.called("timer/cancel") {
		t.Fatalf("off debe apagar switch, arm y timer: %v", f.calls)
	}

	// Ventana futura armada: apagar tambien la desarma (no reconecta a su hora).
	f = newFake()
	f.set("switch.c", "off")
	f.set("input_boolean.a", "on")
	if _, err := ApplyAction(f, ch, "off", nil); err != nil {
		t.Fatalf("off armado: err = %v", err)
	}
	if !f.called("input_boolean/turn_off") {
		t.Fatalf("off debe desarmar la ventana futura: %v", f.calls)
	}

	// Sin programacion configurada (portatil): solo el switch, sin error.
	f = newFake()
	f.set("switch.c", "on")
	bare := Charger{Key: "c2", Name: "Portatil", SwitchID: "switch.p"}
	if _, err := ApplyAction(f, bare, "off", nil); err != nil {
		t.Fatalf("off sin schedule: err = %v", err)
	}
	if len(f.calls) != 1 || !f.called("switch/turn_off") {
		t.Fatalf("off sin schedule debe tocar solo el switch: %v", f.calls)
	}

	// "on" no toca el plan.
	f = newFake()
	f.set("input_boolean.a", "on")
	if _, err := ApplyAction(f, ch, "on", nil); err != nil {
		t.Fatalf("on: err = %v", err)
	}
	if f.called("input_boolean/turn_off") || f.called("timer/cancel") {
		t.Fatalf("on no debe tocar el plan: %v", f.calls)
	}
}

// TestChargerEntitiesIncludeTimer checks that the HA timer of the schedule is
// subscribed: without it, GetState never sees the timer and the live view
// cannot show the expected end while the scheduled charge runs.
func TestChargerEntitiesIncludeTimer(t *testing.T) {
	c := GenericExtensions
	c.Enabled = true
	c.CarCharger.Enabled = true
	c.CarCharger.Chargers = []Charger{{Key: "main", ScheduleTimerID: "timer.coche_carga"}}
	var has bool
	for _, id := range ChargerEntities(c) {
		if id == "timer.coche_carga" {
			has = true
		}
	}
	if !has {
		t.Fatalf("timer ausente de ChargerEntities: %v", ChargerEntities(c))
	}
}

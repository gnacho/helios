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

package extensions

import (
	"testing"
	"time"

	"helios-server/internal/ha"
)

// TestComputeOneChargerLiveScheduleTimer: el timer HA de la carga programada
// expone los segundos restantes solo mientras está active. Preferimos
// finishes_at (exacto mientras corre) y caemos a remaining si no viene.
func TestComputeOneChargerLiveScheduleTimer(t *testing.T) {
	base := Charger{
		Key: "main", Name: "Pared", PowerID: "sensor.p",
		ScheduleTimerID: "timer.coche_carga",
	}

	t.Run("finishes_at", func(t *testing.T) {
		f := newFake()
		f.set("sensor.p", "1.5")
		f.set("timer.coche_carga", "active")
		f.setAttrs("timer.coche_carga", map[string]any{
			"finishes_at": time.Now().Add(90 * time.Second).UTC().Format(time.RFC3339),
		})

		live := ComputeOneChargerLive(f, base)
		if live.TimerRemainingS == nil {
			t.Fatal("TimerRemainingS debería estar presente con el timer active")
		}
		if *live.TimerRemainingS <= 0 || *live.TimerRemainingS > 95 {
			t.Fatalf("remaining = %d s, esperaba ~90 s", *live.TimerRemainingS)
		}
	})

	t.Run("remaining_hh_mm_ss", func(t *testing.T) {
		f := newFake()
		f.set("sensor.p", "1.5")
		f.set("timer.coche_carga", "active")
		f.setAttrs("timer.coche_carga", map[string]any{"remaining": "01:05:00"})

		live := ComputeOneChargerLive(f, base)
		if live.TimerRemainingS == nil || *live.TimerRemainingS != 3900 {
			t.Fatalf("remaining = %v, esperaba 3900 s", live.TimerRemainingS)
		}
	})

	t.Run("remaining_segundos_planos", func(t *testing.T) {
		f := newFake()
		f.set("sensor.p", "1.5")
		f.set("timer.coche_carga", "active")
		f.setAttrs("timer.coche_carga", map[string]any{"remaining": "120"})

		live := ComputeOneChargerLive(f, base)
		if live.TimerRemainingS == nil || *live.TimerRemainingS != 120 {
			t.Fatalf("remaining = %v, esperaba 120 s", live.TimerRemainingS)
		}
	})

	t.Run("timer_idle_sin_dato", func(t *testing.T) {
		f := newFake()
		f.set("sensor.p", "0")
		f.set("timer.coche_carga", "idle")

		live := ComputeOneChargerLive(f, base)
		if live.TimerRemainingS != nil {
			t.Fatalf("con el timer idle no debería haber remaining, hay %v", *live.TimerRemainingS)
		}
	})

	t.Run("sin_timer_configurado", func(t *testing.T) {
		f := newFake()
		f.set("sensor.p", "0")
		ch := base
		ch.ScheduleTimerID = ""

		live := ComputeOneChargerLive(f, ch)
		if live.TimerRemainingS != nil {
			t.Fatal("sin ScheduleTimerID no debería haber remaining")
		}
	})
}

// TestTimerRemainingSeconds cubre el parser directamente con los bordes.
func TestTimerRemainingSeconds(t *testing.T) {
	cases := []struct {
		name string
		st   ha.EntityState
		want int
		ok   bool
	}{
		{"vacio", ha.EntityState{State: "active", Attributes: map[string]any{}}, 0, false},
		{"remaining_mm_ss", ha.EntityState{State: "active", Attributes: map[string]any{"remaining": "05:00"}}, 300, true},
		{"remaining_hms_espacios", ha.EntityState{State: "active", Attributes: map[string]any{"remaining": " 00:02:30 "}}, 150, true},
		{"remaining_no_numerico", ha.EntityState{State: "active", Attributes: map[string]any{"remaining": "xx"}}, 0, false},
		{"finishes_at_pasado", ha.EntityState{State: "active", Attributes: map[string]any{"finishes_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)}}, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := timerRemainingSeconds(c.st)
			if ok != c.ok {
				t.Fatalf("ok = %v, esperaba %v", ok, c.ok)
			}
			if ok && got != c.want {
				t.Fatalf("secs = %d, esperaba %d", got, c.want)
			}
		})
	}
}

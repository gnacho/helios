package extensions

import (
	"testing"
	"time"
)

// The schedule end is always start + duration, even when the start time has
// already passed: at 12:30 a 12:00 + 4 h schedule must end at 16:00 today.
func TestScheduleWindow(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 30, 0, 0, time.Local)
	start, end, err := scheduleWindow("12:00", 4, now)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if start.Hour() != 12 || start.Day() != 26 {
		t.Fatalf("start = %v", start)
	}
	if end.Hour() != 16 || end.Day() != 26 {
		t.Fatalf("end = %v, want 16:00 hoy", end)
	}
	if got := end.Sub(now).Hours(); got != 3.5 {
		t.Fatalf("remaining = %v h, want 3.5", got)
	}

	// Hora futura: la ventana es la elegida, sin arranque inmediato.
	start, end, err = scheduleWindow("22:30", 4, now)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if start.Day() != 26 || start.Hour() != 22 || !start.After(now) {
		t.Fatalf("start = %v", start)
	}
	if end.Day() != 27 || end.Hour() != 2 {
		t.Fatalf("end = %v, want 02:30 de madrugada", end)
	}

	// Ventana ya terminada: se programa para mañana.
	start, end, err = scheduleWindow("12:00", 4, now.Add(6*time.Hour))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if start.Day() != 27 || end.Day() != 27 {
		t.Fatalf("start/end = %v/%v, want mañana", start, end)
	}

	if _, _, err := scheduleWindow("12:00", 0, now); err == nil {
		t.Fatal("duración 0 debe fallar")
	}
	if _, _, err := scheduleWindow("bad", 4, now); err == nil {
		t.Fatal("hora inválida debe fallar")
	}
}

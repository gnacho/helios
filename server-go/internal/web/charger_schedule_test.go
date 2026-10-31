package web

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"helios-server/internal/auth"
	"helios-server/internal/db"
	"helios-server/internal/extensions"
	"helios-server/internal/ha"
	"helios-server/internal/solar"
)

// TestChgScheduleSignalTexts: la señal scheduleText muestra la ventana
// planeada mientras está armada y la hora prevista de fin mientras el timer
// corre; sin programación queda vacía.
func TestChgScheduleSignalTexts(t *testing.T) {
	hours := 3.0

	t.Run("armada", func(t *testing.T) {
		live := extensions.LiveCharger{
			Scheduled:     true,
			ScheduleStart: "02:30:00",
			ScheduleHours: &hours,
		}
		sig := chgSignalOf("es", live)
		if sig["scheduleArmed"] != true {
			t.Fatalf("scheduleArmed = %v, esperaba true", sig["scheduleArmed"])
		}
		if sig["scheduleText"] != "Programado 02:30 · 3 h" {
			t.Fatalf("scheduleText = %q", sig["scheduleText"])
		}
	})

	t.Run("cargando_con_timer", func(t *testing.T) {
		remaining := 5400 // 1.5 h
		live := extensions.LiveCharger{Charging: true, TimerRemainingS: &remaining}
		sig := chgSignalOf("es", live)
		if sig["scheduleArmed"] != false {
			t.Fatalf("scheduleArmed = %v, esperaba false", sig["scheduleArmed"])
		}
		texto, _ := sig["scheduleText"].(string)
		if !strings.HasPrefix(texto, "Cargando hasta las ") || !strings.Contains(texto, ":") {
			t.Fatalf("scheduleText = %q, esperaba hora de fin", texto)
		}
	})

	t.Run("sin_programacion", func(t *testing.T) {
		sig := chgSignalOf("es", extensions.LiveCharger{})
		if sig["scheduleArmed"] != false || sig["scheduleText"] != "" {
			t.Fatalf("sin programación: armed=%v text=%q", sig["scheduleArmed"], sig["scheduleText"])
		}
	})
}

// TestChargerChipAndScheduleBindings: el chip y la fila de programación van
// atados a las señales $chg con la sintaxis real de Datastar, para que el
// estado cambie en vivo (un cargador cargando no se queda "En reposo").
func TestChargerChipAndScheduleBindings(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "chgbind2.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	cfg := extensions.Config{Enabled: true}
	cfg.CarCharger.Enabled = true
	cfg.CarCharger.Chargers = []extensions.Charger{{
		Key: "main", Name: "Pared", PowerID: "sensor.p",
		EnergyTotalID:      "sensor.et",
		ScheduleStartID:    "input_datetime.coche_inicio",
		ScheduleDurationID: "input_number.coche_duracion",
		ScheduleArmID:      "input_boolean.coche_programado",
		ScheduleTimerID:    "timer.coche_carga",
	}}
	extStore := extensions.NewStore()
	extStore.Set(cfg)
	ext := extensions.New(extStore, d, nil)

	a := &auth.Auth{DB: d, SessionTTL: time.Hour, SessionSecret: "test-secret"}
	if _, err := a.RegisterUser("nacho", "password1", "es", "admin"); err != nil {
		t.Fatalf("register: %v", err)
	}
	fa := stubChargerHA{states: map[string]ha.EntityState{
		"sensor.p":                       {State: "1.5", Attributes: map[string]any{}},
		"sensor.et":                      {State: "10", Attributes: map[string]any{}},
		"input_datetime.coche_inicio":    {State: "02:30:00", Attributes: map[string]any{}},
		"input_number.coche_duracion":    {State: "3", Attributes: map[string]any{}},
		"input_boolean.coche_programado": {State: "on", Attributes: map[string]any{}},
		"timer.coche_carga":              {State: "idle", Attributes: map[string]any{}},
	}}
	u := New(a, d, nil, nil, nil, extStore, ext, fa, solar.PriceConfig{}, nil)
	r := chi.NewRouter()
	u.Mount(r)
	r.NotFound(u.NotFound)

	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))
	req := httptest.NewRequest("GET", "/cargador", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()

	// Chip: texto, clase e iconos siguen a $chg.main.state (templ escapa las
	// comillas simples como &#39;).
	for _, want := range []string{
		`data-text="$chg.main.stateText"`,
		`data-attr:class="$chg.main.state == &#39;charging&#39;`,
		`$chg.main.state == &#39;connected&#39;`,
		`data-show="$chg.main.state == &#39;charging&#39;"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("al chip le falta %q", want)
		}
	}
	// Fila de programación: formulario condicional y estado en vivo.
	for _, want := range []string{
		`data-show="$chg.main.scheduleText != &#39;&#39;"`,
		`$chg.main.scheduleArmed`,
		"Detener programación",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("a la fila de programación le falta %q", want)
		}
	}
}

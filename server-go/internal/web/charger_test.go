package web

import (
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"helios-server/internal/auth"
	"helios-server/internal/db"
	"helios-server/internal/extensions"
	"helios-server/internal/solar"
)

func chgHistRowOf(date string, kwh, pv float64) extensions.ChargerHistoryRow {
	k, p := kwh, pv
	return extensions.ChargerHistoryRow{Date: date, Kwh: &k, PvKwh: &p}
}

func TestChargerHistoryGeometry(t *testing.T) {
	// Semana anclada a HOY (incluye el día en curso).
	today := time.Date(2026, 10, 23, 0, 0, 0, 0, time.Local)
	anchor := today
	var hist []extensions.ChargerHistoryRow
	for i := 6; i >= 0; i-- {
		d := today.AddDate(0, 0, -i)
		hist = append(hist, chgHistRowOf(d.Format("2006-01-02"), 10, 4))
	}
	pal := newPalette(false)
	m, ok := chgHistoryChart("es", PeriodSemana, anchor, today, hist, nil, pal)
	if !ok {
		t.Fatal("ok = false")
	}
	if len(m.Rows) != 7 {
		t.Fatalf("filas = %d (esperado 7, semana incluye hoy)", len(m.Rows))
	}
	// yMax: ceil(10) = 10.
	if m.YMax != 10 {
		t.Fatalf("yMax = %d (esperado 10)", m.YMax)
	}
	// Barra apilada: pv abajo (verde) y rest encima (azul).
	r := m.Rows[3]
	plotH := m.PlotBottomY - 10
	if math.Abs(r.PvH-(4.0/10*plotH)) > 0.01 {
		t.Fatalf("pvH = %v", r.PvH)
	}
	if math.Abs(r.RestH-(6.0/10*plotH)) > 0.01 {
		t.Fatalf("restH = %v", r.RestH)
	}
	// El rest descansa sobre el pv (stack cerrado, tolerancia flotante).
	if math.Abs((r.RestY+r.RestH)-r.PvY) > 0.01 {
		t.Fatalf("stack abierto: top %v + h %v vs pv bottom %v", r.RestY, r.RestH, r.PvY)
	}
	// Ticks Y: ~5 ticks con paso redondo (0,2,..,10 para yMax 10).
	if len(m.TicksY) != 6 || m.TicksY[len(m.TicksY)-1].Label != "10" {
		t.Fatalf("ticksY = %d, último = %q", len(m.TicksY), m.TicksY[len(m.TicksY)-1].Label)
	}
}

func TestChargerHistoryYearAggregates(t *testing.T) {
	today := time.Date(2026, 10, 23, 0, 0, 0, 0, time.Local)
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	// Dos días en enero con 5 kWh (4 pv), dos en marzo con 2 kWh.
	hist := []extensions.ChargerHistoryRow{
		chgHistRowOf("2026-01-05", 5, 4),
		chgHistRowOf("2026-01-20", 5, 4),
		chgHistRowOf("2026-03-10", 2, 1),
		chgHistRowOf("2026-03-11", 2, 1),
	}
	pal := newPalette(false)
	m, _ := chgHistoryChart("es", PeriodAno, anchor, today, hist, nil, pal)
	if len(m.Rows) != 12 {
		t.Fatalf("filas = %d (esperado 12 meses)", len(m.Rows))
	}
	// yMax = ceil(10) = 10; la barra de enero es la más alta.
	if m.YMax != 10 {
		t.Fatalf("yMax = %d", m.YMax)
	}
	if m.Rows[0].PvH+m.Rows[0].RestH <= m.Rows[2].PvH+m.Rows[2].RestH {
		t.Fatal("enero debería ser más alto que marzo")
	}
	// Etiquetas de mes con nombres cortos.
	if m.Rows[0].Label != "ene" || m.Rows[11].Label != "dic" {
		t.Fatalf("labels = %q / %q", m.Rows[0].Label, m.Rows[11].Label)
	}
}

func TestChargerAmpChipsAndPowerDisabled(t *testing.T) {
	_, r := chargerTestUI(t)
	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))

	req := httptest.NewRequest("GET", "/cargador", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String()[:min(400, len(rec.Body.String()))])
	}
	body := rec.Body.String()
	// Sin HA: switchOn undefined → botón power disabled (lección omitempty).
	if !strings.Contains(body, "disabled") {
		t.Fatal("el botón power debería estar disabled sin switchOn")
	}
	if !strings.Contains(body, "aria-pressed=\"false\"") {
		t.Fatal("falta aria-pressed=false")
	}
	// AmpID configurado → 8 chips (AMP_STEPS).
	for _, a := range []string{">6<", ">8<", ">32<"} {
		if !strings.Contains(body, a) {
			t.Fatalf("falta el chip de amperaje %s", a)
		}
	}
	// La acción viaja por Datastar al endpoint JSON existente (templ escapa
	// las comillas simples del atributo: el navegador las decodifica).
	if !strings.Contains(body, "@post") || !strings.Contains(body, "charger/action") {
		t.Fatal("falta el @post al endpoint JSON")
	}
	// Un solo cargador → sin tabs.
	if strings.Contains(body, "Portátil") {
		t.Fatal("con un solo cargador no debería haber tabs")
	}
}

func TestChargerTabsAndSelection(t *testing.T) {
	_, r := chargerTestUI2(t)
	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))

	// Dos cargadores → segmented control visible.
	req := httptest.NewRequest("GET", "/cargador", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "Pared") || !strings.Contains(body, "Portátil") {
		t.Fatal("faltan los nombres de los cargadores en el tablist")
	}
	if !strings.Contains(body, "aria-selected=\"true\"") {
		t.Fatal("falta el tab activo")
	}

	// ?tab=c2 selecciona el segundo.
	req2 := httptest.NewRequest("GET", "/cargador?tab=c2", nil)
	req2.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != 200 {
		t.Fatalf("tab c2 status = %d", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "aria-selected=\"true\"") {
		t.Fatal("falta el tab c2 activo")
	}
}

func TestCargadorRedirectWithoutExtension(t *testing.T) {
	_, r, _ := testUI(t) // sin ExtStore
	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))
	req := httptest.NewRequest("GET", "/cargador", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("status = %d (esperado redirect a /)", rec.Code)
	}
}

// chargerTestUI builds a UI with the extension enabled (1 charger) and two
// days of charger history in the DB.
func chargerTestUI(t *testing.T) (*UI, *chi.Mux) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "chg.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	cfg := extensions.Config{Enabled: true}
	cfg.CarCharger.Enabled = true
	cfg.CarCharger.Chargers = []extensions.Charger{{
		Key: "main", Name: "Pared", PowerID: "sensor.p",
		EnergyTotalID: "sensor.et", AmpID: "number.amp", SwitchID: "switch.pared",
	}}
	extStore := extensions.NewStore()
	extStore.Set(cfg)
	ext := extensions.New(extStore, d, nil)

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if err := d.AddChargerKwh(yesterday, "main", 7.5, 3.0); err != nil {
		t.Fatalf("seed charger: %v", err)
	}

	return buildChargerUI(t, d, extStore, ext)
}

// chargerTestUI2 is the two-charger instance (main "Pared" + c2 "Portátil").
func chargerTestUI2(t *testing.T) (*UI, *chi.Mux) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "chg2.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	cfg := extensions.Config{Enabled: true}
	cfg.CarCharger.Enabled = true
	cfg.CarCharger.Chargers = []extensions.Charger{
		{Key: "main", Name: "Pared", PowerID: "sensor.p", EnergyTotalID: "sensor.et"},
		{Key: "c2", Name: "Portátil", PowerID: "sensor.p2", EnergyTotalID: "sensor.et2"},
	}
	extStore := extensions.NewStore()
	extStore.Set(cfg)
	ext := extensions.New(extStore, d, nil)
	return buildChargerUI(t, d, extStore, ext)
}

// buildChargerUI wires the auth + UI + router for the charger page tests.
func buildChargerUI(t *testing.T, d *db.DB, extStore *extensions.Store, ext *extensions.Ext) (*UI, *chi.Mux) {
	t.Helper()
	a := &auth.Auth{DB: d, SessionTTL: time.Hour, SessionSecret: "test-secret"}
	if _, err := a.RegisterUser("nacho", "password1", "es", "admin"); err != nil {
		t.Fatalf("register: %v", err)
	}
	u := New(a, d, nil, nil, nil, extStore, ext, nil, solar.PriceConfig{}, nil)
	u.RootFiles = func(w http.ResponseWriter, r *http.Request) bool { return false }

	r := chi.NewRouter()
	u.Mount(r)
	r.NotFound(u.NotFound)
	return u, r
}

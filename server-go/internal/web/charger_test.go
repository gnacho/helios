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
	"helios-server/internal/ha"
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
	m, ok := chgHistoryChart("es", PeriodSemana, anchor, today, hist, nil, nil, pal)
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
	m, _ := chgHistoryChart("es", PeriodAno, anchor, today, hist, nil, nil, pal)
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
	// Live: la pagina abre el stream de senales (data-init es el plugin real
	// de Datastar; data-on-load no existe y no hacia nada).
	for _, want := range []string{"data-init=", "/ui/stream"} {
		if !strings.Contains(body, want) {
			t.Fatalf("falta %q en la pagina del cargador", want)
		}
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

	r := chi.NewRouter()
	u.Mount(r)
	r.NotFound(u.NotFound)
	return u, r
}

// stubChargerHA devuelve estados fijos para el cargador (sin tocar HA real).
type stubChargerHA struct{ states map[string]ha.EntityState }

func (s stubChargerHA) GetState(id string) (ha.EntityState, bool) {
	v, ok := s.states[id]
	return v, ok
}
func (s stubChargerHA) IsConnected() bool { return true }
func (s stubChargerHA) StatisticsDuringPeriod(string, string, []string, string, []string) (ha.Stats, error) {
	return ha.Stats{}, nil
}
func (s stubChargerHA) HistoryDuringPeriod(string, string, string) ([]ha.HistoryRow, error) {
	return nil, nil
}
func (s stubChargerHA) CallService(string, string, map[string]any) error { return nil }

// TestChargerPowerButtonBindsSignals: con el switch conocido el boton se ata a
// las senales con la sintaxis REAL de Datastar. data-attr-<name> (con guion) no
// existe en esta version: el plugin es data-attr:<name>; con guion los enlaces
// quedaban inertes (aria-pressed, title y color no cambiaban nunca).
func TestChargerPowerButtonBindsSignals(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "chgbind.db"))
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

	a := &auth.Auth{DB: d, SessionTTL: time.Hour, SessionSecret: "test-secret"}
	if _, err := a.RegisterUser("nacho", "password1", "es", "admin"); err != nil {
		t.Fatalf("register: %v", err)
	}
	fa := stubChargerHA{states: map[string]ha.EntityState{
		"switch.pared": {State: "on", Attributes: map[string]any{}},
		"sensor.p":     {State: "1.5", Attributes: map[string]any{}},
		"sensor.et":    {State: "10", Attributes: map[string]any{}},
		"number.amp":   {State: "10", Attributes: map[string]any{}},
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
	body := rec.Body.String()

	for _, want := range []string{
		"data-attr:disabled", "data-attr:aria-pressed", "data-attr:title", "data-attr:class",
		"data-attr-class", // no debe quedar la sintaxis inerte
		"$ui.pending", "data-on-interval__duration.500ms",
	} {
		got := strings.Contains(body, want)
		if strings.HasPrefix(want, "data-attr-class") {
			if got {
				t.Fatalf("%q no existe en Datastar v1 (usa data-attr:<name>)", want)
			}
			continue
		}
		if !got {
			t.Fatalf("falta %q en el boton de encendido", want)
		}
	}
}

// TestChargerAmpChipsFilteredByRange: los atributos min/max del number de
// amperaje recortan los chips al rango real de la entidad (regresion: un
// number 8-32 no debe ofrecer el chip 6, un 6-16 no debe ofrecer 20/25/32).
// buildHourSelect must render 24 options and preselect the hour of day that the
// live delay (hours from now) points at.
func TestBuildStartSelects(t *testing.T) {
	now := time.Date(2026, 10, 26, 8, 30, 0, 0, time.Local)
	delay := 2.0
	hourSel, minSel := buildStartSelects("c2", &delay, now)
	if len(hourSel.Options) != 24 {
		t.Fatalf("hour options = %d, want 24", len(hourSel.Options))
	}
	if hourSel.Options[0].Label != "00" || hourSel.Options[23].Label != "23" {
		t.Fatalf("hour labels = %q .. %q", hourSel.Options[0].Label, hourSel.Options[23].Label)
	}
	var selected []string
	for _, o := range hourSel.Options {
		if o.Selected {
			selected = append(selected, o.Value)
		}
	}
	// 08:30 + 2 h = 10:30 -> hour 10, minutes 30.
	if len(selected) != 1 || selected[0] != "10" {
		t.Fatalf("selected hour = %v, want [10]", selected)
	}
	if len(minSel.Options) != 2 || minSel.Options[0].Value != "0" || minSel.Options[1].Value != "30" {
		t.Fatalf("minute options = %+v", minSel.Options)
	}
	if !minSel.Options[1].Selected {
		t.Fatal("minute 30 must be preselected for 10:30")
	}
	if hourSel.ID != "chg-h-c2" || minSel.ID != "chg-m-c2" {
		t.Fatalf("ids = %q, %q", hourSel.ID, minSel.ID)
	}
}

// buildDurationSelect must offer 1..12 h and preselect the live value.
func TestBuildDurationSelect(t *testing.T) {
	dur := 4.0
	sel := buildDurationSelect("c2", &dur)
	if len(sel.Options) != 12 {
		t.Fatalf("options = %d, want 12", len(sel.Options))
	}
	if sel.Options[0].Value != "1" || sel.Options[11].Value != "12" {
		t.Fatalf("values = %q .. %q", sel.Options[0].Value, sel.Options[11].Value)
	}
	var selected []string
	for _, o := range sel.Options {
		if o.Selected {
			selected = append(selected, o.Value)
		}
	}
	if len(selected) != 1 || selected[0] != "4" {
		t.Fatalf("selected = %v, want [4]", selected)
	}
	if sel.ID != "chg-d-c2" {
		t.Fatalf("id = %q", sel.ID)
	}
}

// chgTimedClickExpr must read the three portable pickers and post both values
// in a single action.
func TestChgTimedClickExpr(t *testing.T) {
	expr := chgTimedClickExpr("c2")
	for _, want := range []string{"$action = 'timed'", "chg-h-c2", "chg-m-c2", "chg-d-c2", "$schedHours", "$schedTime", "@post('/ui/charger/action')"} {
		if !strings.Contains(expr, want) {
			t.Fatalf("missing %q in %q", want, expr)
		}
	}
}

// The single-charger panel must render the delay/duration chip rows when the
// charger config has those entities (portable scheduling dps via helpers).
func TestChargerDelayDurationChips(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "chgdelay.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	cfg := extensions.Config{Enabled: true}
	cfg.CarCharger.Enabled = true
	cfg.CarCharger.Chargers = []extensions.Charger{{
		Key: "c2", Name: "Portátil", PowerID: "sensor.p",
		AmpID: "input_select.amps", DelayID: "input_number.feyree_delay_h",
		DurationID: "input_select.feyree_duration_h",
	}}
	extStore := extensions.NewStore()
	extStore.Set(cfg)
	ext := extensions.New(extStore, d, nil)

	a := &auth.Auth{DB: d, SessionTTL: time.Hour, SessionSecret: "test-secret"}
	if _, err := a.RegisterUser("nacho", "delay-pass-1", "es", "admin"); err != nil {
		t.Fatalf("register: %v", err)
	}
	fa := stubChargerHA{states: map[string]ha.EntityState{
		"input_number.feyree_delay_h":    {State: "2.0", Attributes: map[string]any{"min": 0.0, "max": 24.0, "step": 0.5}},
		"input_select.feyree_duration_h": {State: "2", Attributes: map[string]any{"options": []any{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.0, 12.0}}},
		"input_select.amps":              {State: "10", Attributes: map[string]any{"options": []any{8.0, 10.0, 13.0, 16.0}}},
		"sensor.p":                       {State: "0.0", Attributes: map[string]any{}},
	}}
	u := New(a, d, nil, nil, nil, extStore, ext, fa, solar.PriceConfig{}, nil)
	r := chi.NewRouter()
	u.Mount(r)
	r.NotFound(u.NotFound)

	cookie := sessionCookie(t, doLogin(t, r, "nacho", "delay-pass-1"))
	req := httptest.NewRequest("GET", "/cargador?tab=c2", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"Inicio", "Min", "Duración", "chg-h-c2", "chg-m-c2", "chg-d-c2", "12 h", "getElementById", "h-11", "gap-[26px]", "Programar", "timed"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in page", want)
		}
	}
}

func TestChargerAmpChipsFilteredByRange(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "chgrange.db"))
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

	a := &auth.Auth{DB: d, SessionTTL: time.Hour, SessionSecret: "test-secret"}
	if _, err := a.RegisterUser("nacho", "range-pass-1", "es", "admin"); err != nil {
		t.Fatalf("register: %v", err)
	}
	fa := stubChargerHA{states: map[string]ha.EntityState{
		"switch.pared": {State: "off", Attributes: map[string]any{}},
		"sensor.p":     {State: "0.0", Attributes: map[string]any{}},
		"sensor.et":    {State: "10", Attributes: map[string]any{}},
		"number.amp":   {State: "13", Attributes: map[string]any{"min": 8.0, "max": 32.0}},
	}}
	u := New(a, d, nil, nil, nil, extStore, ext, fa, solar.PriceConfig{}, nil)
	r := chi.NewRouter()
	u.Mount(r)
	r.NotFound(u.NotFound)

	cookie := sessionCookie(t, doLogin(t, r, "nacho", "range-pass-1"))
	req := httptest.NewRequest("GET", "/cargador", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()

	// Dentro de rango 8-32: presentes.
	for _, want := range []string{">8<", ">10<", ">13<", ">16<", ">32<"} {
		if !strings.Contains(body, want) {
			t.Fatalf("falta el chip %s (rango 8-32)", want)
		}
	}
	// Fuera de rango: el 6 no debe pintarse.
	if strings.Contains(body, ">6<") {
		t.Fatal("el chip 6 deberia filtrarse con min 8")
	}
	// El chip activo (13) va marcado y el aria-pressed es reactivo al signal.
	if !strings.Contains(body, "aria-pressed=\"true\"") {
		t.Fatal("falta el chip activo con aria-pressed=true")
	}
}

// flakyChargerHA cambia el estado tras N lecturas, como HA cuando aplica la
// accion de forma asincrona.
type flakyChargerHA struct {
	stubChargerHA
	pending string
	reads   int
	after   int
}

func (f *flakyChargerHA) GetState(id string) (ha.EntityState, bool) {
	if f.pending != "" {
		f.reads++
		if f.reads > f.after {
			f.states[id] = ha.EntityState{State: f.pending, Attributes: map[string]any{}}
			f.pending = ""
		}
	}
	return f.stubChargerHA.GetState(id)
}

// TestWaitChargerStateWaitsForHA: la respuesta de la accion espera a que HA
// reporte el estado pedido, para no devolver el viejo (el boton se quedaba con
// el estado anterior y parecia no responder).
func TestWaitChargerStateWaitsForHA(t *testing.T) {
	fa := &flakyChargerHA{stubChargerHA: stubChargerHA{states: map[string]ha.EntityState{
		"switch.pared": {State: "off", Attributes: map[string]any{}},
	}}, pending: "on", after: 2}
	u := &UI{ExtHA: fa}
	charger := extensions.Charger{Key: "main", SwitchID: "switch.pared"}

	start := time.Now()
	u.waitChargerState(charger, "on", nil)
	elapsed := time.Since(start)

	if st, _ := fa.GetState("switch.pared"); st.State != "on" {
		t.Fatalf("no esperó al estado nuevo: %q", st.State)
	}
	if elapsed < 150*time.Millisecond {
		t.Fatalf("no esperó lo suficiente: %v", elapsed)
	}
}

// TestWaitChargerStateGivesUp: sin confirmacion no se queda colgado.
func TestWaitChargerStateGivesUp(t *testing.T) {
	fa := &stubChargerHA{states: map[string]ha.EntityState{
		"switch.pared": {State: "off", Attributes: map[string]any{}},
	}}
	u := &UI{ExtHA: fa}
	start := time.Now()
	u.waitChargerState(extensions.Charger{Key: "main", SwitchID: "switch.pared"}, "on", nil)
	if elapsed := time.Since(start); elapsed > 2500*time.Millisecond {
		t.Fatalf("tardó demasiado en desistir: %v", elapsed)
	}
}

// TestStateMatches cubre los tres tipos de acción.
func TestStateMatches(t *testing.T) {
	amps := 10.0
	cases := []struct {
		state, action string
		amps          *float64
		want          bool
	}{
		{"on", "on", nil, true},
		{"off", "on", nil, false},
		{"off", "off", nil, true},
		{"10", "amp", &amps, true},
		{"9.4", "amp", &amps, false},
		{"unavailable", "amp", &amps, false},
	}
	for _, c := range cases {
		if got := stateMatches(c.state, c.action, c.amps); got != c.want {
			t.Fatalf("stateMatches(%q,%q) = %v, quería %v", c.state, c.action, got, c.want)
		}
	}
}

// The software schedule pickers (wall charger) start from the HA
// input_datetime state and fall back to the current hour, 00 minutes and 6 h.
func TestBuildSchedSelects(t *testing.T) {
	now := time.Date(2026, 10, 26, 8, 40, 0, 0, time.Local)
	h, m := buildSchedStartSelects("main", "22:30:00", now)
	if h.ID != "chg-sh-main" || m.ID != "chg-sm-main" {
		t.Fatalf("ids = %q %q", h.ID, m.ID)
	}
	if len(h.Options) != 24 || len(m.Options) != 2 {
		t.Fatalf("options = %d/%d", len(h.Options), len(m.Options))
	}
	var hour string
	for _, o := range h.Options {
		if o.Selected {
			hour = o.Value
		}
	}
	if hour != "22" {
		t.Fatalf("selected hour = %q, want 22", hour)
	}
	if !m.Options[1].Selected {
		t.Fatal("minutes 30 must be selected for 22:30")
	}

	h2, m2 := buildSchedStartSelects("main", "", now)
	sel := ""
	for _, o := range h2.Options {
		if o.Selected {
			sel = o.Value
		}
	}
	// Sin hora guardada, el picker arranca en la hora actual con minutos 00.
	if sel != "8" {
		t.Fatalf("fallback hour = %q, want 8 (hora actual)", sel)
	}
	if !m2.Options[0].Selected {
		t.Fatal("fallback minutes = 00 esperado por defecto")
	}
	d := buildSchedDurSelect("main", nil)
	if d.ID != "chg-sd-main" || len(d.Options) != 12 || d.Options[5].Value != "6" || !d.Options[5].Selected {
		t.Fatalf("duration select = %+v", d)
	}
	if chgClock("22:30:00") != "22:30" || chgClock("") != "--:--" {
		t.Fatal("chgClock")
	}
}

// The wall charger has no scheduling dps: the schedule row is driven by the
// software helpers (input_datetime + input_number + input_boolean + timer).
func TestChargerScheduleRow(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "chgsched.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	cfg := extensions.Config{Enabled: true}
	cfg.CarCharger.Enabled = true
	cfg.CarCharger.Chargers = []extensions.Charger{{
		Key: "main", Name: "Pared", PowerID: "sensor.p", SwitchID: "switch.pared",
		ScheduleStartID: "input_datetime.coche_inicio", ScheduleDurationID: "input_number.coche_duracion",
		ScheduleArmID: "input_boolean.coche_programado", ScheduleTimerID: "timer.coche_carga",
	}}
	extStore := extensions.NewStore()
	extStore.Set(cfg)
	ext := extensions.New(extStore, d, nil)

	a := &auth.Auth{DB: d, SessionTTL: time.Hour, SessionSecret: "test-secret"}
	if _, err := a.RegisterUser("nacho", "sched-pass-1", "es", "admin"); err != nil {
		t.Fatalf("register: %v", err)
	}
	fa := stubChargerHA{states: map[string]ha.EntityState{
		"input_datetime.coche_inicio":    {State: "07:30:00", Attributes: map[string]any{}},
		"input_number.coche_duracion":    {State: "4.0", Attributes: map[string]any{}},
		"input_boolean.coche_programado": {State: "off", Attributes: map[string]any{}},
		"switch.pared":                   {State: "off", Attributes: map[string]any{}},
		"sensor.p":                       {State: "0.0", Attributes: map[string]any{}},
	}}
	u := New(a, d, nil, nil, nil, extStore, ext, fa, solar.PriceConfig{}, nil)
	r := chi.NewRouter()
	u.Mount(r)
	r.NotFound(u.NotFound)

	cookie := sessionCookie(t, doLogin(t, r, "nacho", "sched-pass-1"))
	req := httptest.NewRequest("GET", "/cargador?tab=main", nil)
	req.AddCookie(cookie)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"chg-sh-main", "chg-sm-main", "chg-sd-main", "Programar"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in page", want)
		}
	}
}

// TestChargerHidesUnreportedStats: el firmware del cargador portátil no
// publica energía, ni de sesión ni acumulada. La cabecera solo pinta lo que el
// aparato reporta de verdad, así que no debe rellenar el hueco con "Sesión —"
// ni con un "Total acumulado 0,0 kWh" que es falso.
func TestChargerHidesUnreportedStats(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "chgstats.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	cfg := extensions.Config{Enabled: true}
	cfg.CarCharger.Enabled = true
	cfg.CarCharger.Chargers = []extensions.Charger{{
		Key: "c2", Name: "Portátil", PowerID: "sensor.p", PowerUnit: "W",
		StateID: "sensor.st", TempID: "sensor.t", SwitchID: "switch.s",
		AmpID: "number.amp",
	}}
	extStore := extensions.NewStore()
	extStore.Set(cfg)
	ext := extensions.New(extStore, d, nil)

	a := &auth.Auth{DB: d, SessionTTL: time.Hour, SessionSecret: "test-secret"}
	if _, err := a.RegisterUser("nacho", "stats-pass-1", "es", "admin"); err != nil {
		t.Fatalf("register: %v", err)
	}
	fa := stubChargerHA{states: map[string]ha.EntityState{
		"sensor.p":   {State: "1500", Attributes: map[string]any{}},
		"sensor.st":  {State: "charing", Attributes: map[string]any{}},
		"sensor.t":   {State: "30", Attributes: map[string]any{}},
		"switch.s":   {State: "on", Attributes: map[string]any{}},
		"number.amp": {State: "12", Attributes: map[string]any{"min": 6.0, "max": 16.0}},
	}}
	u := New(a, d, nil, nil, nil, extStore, ext, fa, solar.PriceConfig{}, nil)
	r := chi.NewRouter()
	u.Mount(r)
	r.NotFound(u.NotFound)

	cookie := sessionCookie(t, doLogin(t, r, "nacho", "stats-pass-1"))
	req := httptest.NewRequest("GET", "/cargador", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()

	if !strings.Contains(body, "Potencia actual") {
		t.Fatal("falta la potencia actual, que sí se publica")
	}
	for _, unwanted := range []string{"Total acumulado", "Sesión"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("%q no debería pintarse: el cargador no lo publica", unwanted)
		}
	}
}

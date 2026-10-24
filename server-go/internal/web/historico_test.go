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

// ── Geometry tests ──────────────────────────────────────────────────────

func TestHistoryBarsGeometry(t *testing.T) {
	today := time.Date(2026, 10, 23, 0, 0, 0, 0, time.Local)
	var days []db.DailyRow
	for i := 6; i >= 0; i-- {
		d := today.AddDate(0, 0, -i)
		prod, cons := 10.0, 6.0
		days = append(days, db.DailyRow{
			Date:           d.Format("2006-01-02"),
			ProductionKwh:  f64Ptr(prod),
			ConsumptionKwh: f64Ptr(cons),
			GridExportKwh:  f64Ptr(2.5), // autoconsumo 75 %
		})
	}
	pal := newPalette(false)
	m := buildHistoryBars("es", PeriodSemana, today, days, pal)
	if len(m.Rows) != 7 {
		t.Fatalf("filas = %d (esperado 7)", len(m.Rows))
	}
	if m.YMax != 10 {
		t.Fatalf("yMax = %d (esperado 10)", m.YMax)
	}
	plotH := histH - histTop - histBottom
	if math.Abs(m.Rows[0].ProdH-(10.0/10*plotH)) > 0.01 {
		t.Fatalf("prodH = %v (esperado %v)", m.Rows[0].ProdH, plotH)
	}
	if math.Abs(m.Rows[0].ConsH-(6.0/10*plotH)) > 0.01 {
		t.Fatalf("consH = %v", m.Rows[0].ConsH)
	}
	// Línea de autoconsumo al 75 % del eje 0..100.
	if !m.HasAuto || m.AutoPath == "" {
		t.Fatal("falta la línea de autoconsumo")
	}
	wantY := histTop + (1-75.0/100)*plotH
	if math.Abs(m.Rows[0].AutoY-wantY) > 0.01 {
		t.Fatalf("autoY = %v (esperado %v)", m.Rows[0].AutoY, wantY)
	}
	// Barra de producción + consumo adyacentes con gap de 3.
	gap := m.Rows[0].XCons - (m.Rows[0].XProd + m.Rows[0].BarW)
	if math.Abs(gap-3) > 0.01 {
		t.Fatalf("gap entre barras = %v", gap)
	}
	// Etiquetas en semana: todas.
	for _, r := range m.Rows {
		if !r.ShowLabel {
			t.Fatal("en semana se muestran todas las etiquetas")
		}
	}
}

func TestHistoryBarsMonthLabelInterval(t *testing.T) {
	today := time.Date(2026, 10, 23, 0, 0, 0, 0, time.Local)
	var days []db.DailyRow
	for d := 1; d <= 23; d++ {
		days = append(days, db.DailyRow{
			Date:           time.Date(2026, 10, d, 0, 0, 0, 0, time.Local).Format("2006-01-02"),
			ProductionKwh:  f64Ptr(5),
			ConsumptionKwh: f64Ptr(4),
		})
	}
	m := buildHistoryBars("es", PeriodMes, today, days, newPalette(false))
	shown := 0
	for _, r := range m.Rows {
		if r.ShowLabel {
			shown++
		}
	}
	// interval=2 de recharts: 1 de cada 3.
	if shown != 8 {
		t.Fatalf("labels mostrados = %d (esperado 8: índices 0,3,...,21)", shown)
	}
}

func TestHistoryBarsYearByMonth(t *testing.T) {
	today := time.Date(2026, 10, 23, 0, 0, 0, 0, time.Local)
	days := []db.DailyRow{
		{Date: "2026-01-10", ProductionKwh: f64Ptr(30), ConsumptionKwh: f64Ptr(25)},
		{Date: "2026-01-11", ProductionKwh: f64Ptr(30), ConsumptionKwh: f64Ptr(25)},
		{Date: "2026-06-15", ProductionKwh: f64Ptr(40), ConsumptionKwh: f64Ptr(30)},
	}
	m := buildHistoryBars("es", PeriodAno, today, days, newPalette(false))
	// Un grupo por mes con datos, ordenado (el handler pasa la ventana ya
	// filtrada; aquí solo hay días de 2026).
	if len(m.Rows) != 2 {
		t.Fatalf("filas = %d (esperado 2)", len(m.Rows))
	}
	if m.Rows[0].Label != "ene" || m.Rows[1].Label != "jun" {
		t.Fatalf("labels = %q, %q", m.Rows[0].Label, m.Rows[1].Label)
	}
	// yMax = ceil(60) = 60.
	if m.YMax != 60 {
		t.Fatalf("yMax = %d (esperado 60)", m.YMax)
	}
}

func TestYearHeatmapGeometry(t *testing.T) {
	today := time.Date(2026, 10, 23, 0, 0, 0, 0, time.Local)
	// 40 días terminando hoy: el primero con producción máxima.
	var rows []db.DailyRow
	for i := 39; i >= 0; i-- {
		d := today.AddDate(0, 0, -i)
		rows = append(rows, db.DailyRow{Date: d.Format("2006-01-02"), ProductionKwh: f64Ptr(40)})
	}
	// Un día con 0 y uno con la mitad.
	rows[10].ProductionKwh = f64Ptr(0)
	rows[20].ProductionKwh = f64Ptr(20)

	h := buildYearHeatmap("es", rows, today, newPalette(false), heatProd)
	if len(h.Cells) != 40 {
		t.Fatalf("celdas = %d (esperado 40)", len(h.Cells))
	}
	// Niveles: max=40 → 40 kWh nivel 4, 20 kWh nivel 2, 0 kWh nivel 1 (mínimo).
	if h.Cells[0].Level != 4 {
		t.Fatalf("nivel max = %d (esperado 4)", h.Cells[0].Level)
	}
	if h.Cells[20].Level != 2 {
		t.Fatalf("nivel mitad = %d (esperado 2)", h.Cells[20].Level)
	}
	if h.Cells[10].Level != 1 {
		t.Fatalf("nivel cero = %d (esperado 1)", h.Cells[10].Level)
	}
	// Geometría de celda: pitch 15, celda 12.
	if h.Cells[0].X != heatLeft || h.Cells[0].Y != heatTop {
		t.Fatalf("primera celda en %v,%v", h.Cells[0].X, h.Cells[0].Y)
	}
	// El último día es hoy → IsToday.
	if !h.Cells[len(h.Cells)-1].IsToday {
		t.Fatal("el último día debería marcarse como hoy")
	}
	// Dimensiones del lienzo 53 columnas.
	if h.Width != heatLeft+heatCols*heatPitch {
		t.Fatalf("width = %v", h.Width)
	}
	// Etiquetas de fila L/X/V.
	if len(h.Rows) != 3 {
		t.Fatalf("row labels = %d", len(h.Rows))
	}
}

// TestHeatMetricValues: el filtro cambia el valor de cada día y el color de la
// rejilla, y el nivel se escala contra el máximo de esa misma métrica.
func TestHeatMetricValues(t *testing.T) {
	today := time.Date(2026, 10, 23, 0, 0, 0, 0, time.Local)
	day := db.DailyRow{
		Date:           today.Format("2006-01-02"),
		ProductionKwh:  f64Ptr(40),
		ConsumptionKwh: f64Ptr(50),
		GridExportKwh:  f64Ptr(10),
	}
	rows := []db.DailyRow{day}
	if got := heatProd.value(day); got != 40 {
		t.Fatalf("prod = %v (esperado 40)", got)
	}
	if got := heatCons.value(day); got != 50 {
		t.Fatalf("cons = %v (esperado 50)", got)
	}
	// Autoconsumo = producción - vertido.
	if got := heatAuto.value(day); got != 30 {
		t.Fatalf("auto = %v (esperado 30)", got)
	}
	// Verter más de lo producido no da autoconsumo negativo.
	neg := db.DailyRow{Date: day.Date, ProductionKwh: f64Ptr(5), GridExportKwh: f64Ptr(9)}
	if got := heatAuto.value(neg); got != 0 {
		t.Fatalf("auto negativo = %v (esperado 0)", got)
	}
	if heatProd.color(newPalette(false)) != newPalette(false).Solar {
		t.Fatal("producción debe usar el color solar")
	}
	if heatCons.color(newPalette(false)) != newPalette(false).Consumo {
		t.Fatal("consumo debe usar el color de consumo")
	}
	if heatAuto.color(newPalette(false)) != newPalette(false).Autoconsumo {
		t.Fatal("autoconsumo debe usar su color propio")
	}
	// Con una sola fila y el valor de cada métrica, el nivel es el máximo.
	for _, m := range []heatMetric{heatProd, heatCons, heatAuto} {
		h := buildYearHeatmap("es", rows, today, newPalette(false), m)
		if h.Metric != m {
			t.Fatalf("metric = %q", h.Metric)
		}
		if len(h.Cells) != 1 || h.Cells[0].Level != 4 {
			t.Fatalf("%s: nivel = %d (esperado 4)", m, h.Cells[0].Level)
		}
		if h.Cells[0].Date != day.Date {
			t.Fatalf("%s: date = %q", m, h.Cells[0].Date)
		}
	}
}

func TestYearHeatmapEmpty(t *testing.T) {
	h := buildYearHeatmap("es", nil, time.Date(2026, 10, 23, 0, 0, 0, 0, time.Local), newPalette(false), heatProd)
	if len(h.Cells) != 0 {
		t.Fatal("sin filas no hay celdas")
	}
}

// ── Page tests ──────────────────────────────────────────────────────────

func TestHistoricoPageEmpty(t *testing.T) {
	_, r, _ := testUI(t)
	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))

	req := httptest.NewRequest("GET", "/historico", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Sin datos para este periodo", "Volver a hoy", "Día", "Semana", "Mes", "Año"} {
		if !strings.Contains(body, want) {
			t.Fatalf("falta %q en el estado vacío", want)
		}
	}
}

func TestHistoricoPageWithData(t *testing.T) {
	_, r := historicoTestUI(t)
	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))

	req := httptest.NewRequest("GET", "/historico", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String()[:min(500, len(rec.Body.String()))])
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Producción",             // KPI
		"Autoconsumo",            // KPI
		"Ahorro",                 // KPI
		"Producción vs consumo",  // título de la gráfica
		"esta semana",            // chartTitle con periodo actual
		"hist-grad-prod",         // NO: es la curva del día... en semana no aparece
		"Mejor día de la semana", // récord
		"Pulsa una barra",        // hint de drill-down
	} {
		if !strings.Contains(body, want) {
			if want == "hist-grad-prod" {
				if strings.Contains(body, want) {
					t.Fatal("la curva del día no debería renderizar en periodo semana")
				}
				continue
			}
			t.Fatalf("falta %q en /historico con datos", want)
		}
	}
	// Los enlaces de navegación apuntan a /historico con period/anchor.
	if !strings.Contains(body, "/historico?period=semana&amp;anchor=") {
		t.Fatal("faltan los links del navegador de periodo")
	}
}

func TestHistoricoYearHeatmap(t *testing.T) {
	_, r := historicoTestUI(t)
	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))

	req := httptest.NewRequest("GET", "/historico?period=ano", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Año completo", "Mapa de calor", "Menos", "Más"} {
		if !strings.Contains(body, want) {
			t.Fatalf("falta %q en el heatmap anual", want)
		}
	}
	// Las celdas son enlaces de drill-down al día.
	if !strings.Contains(body, "/historico?period=dia&amp;anchor=") {
		t.Fatal("las celdas del heatmap deberían enlazar al día")
	}
}

func TestHistoricoDayCurves(t *testing.T) {
	_, r := historicoTestUI(t)
	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	req := httptest.NewRequest("GET", "/historico?period=dia&anchor="+yesterday, nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "hist-grad-prod") {
		t.Fatal("falta la curva de producción del día")
	}
	if !strings.Contains(body, "hist-grad-cons") {
		t.Fatal("falta la curva de consumo del día")
	}
	// Sin HA no hay marcador AHORA.
	if strings.Contains(body, "AHORA") {
		t.Fatal("sin HA no debería haber marcador AHORA")
	}
}

// historicoTestUI builds a UI with 10 daily rows ending yesterday (the
// default semana anchor) plus a couple of charger-independent rows.
func historicoTestUI(t *testing.T) (*UI, *chi.Mux) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "hist.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	a := &auth.Auth{DB: d, SessionTTL: time.Hour, SessionSecret: "test-secret"}
	if _, err := a.RegisterUser("nacho", "password1", "es", "admin"); err != nil {
		t.Fatalf("register: %v", err)
	}

	// 10 días terminando AYER: la semana por defecto (16..22 oct style) tiene datos.
	for i := 10; i >= 1; i-- {
		day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		prod, cons, imp, exp := 12.5, 8.0, 2.0, 4.0
		co2 := 0.0
		if err := d.UpsertDaily(db.DailyRow{
			Date:              day,
			ProductionKwh:     &prod,
			ConsumptionKwh:    &cons,
			GridImportKwh:     &imp,
			GridExportKwh:     &exp,
			BatteryChargedKwh: &co2,
		}); err != nil {
			t.Fatalf("seed daily: %v", err)
		}
	}

	extStore := extensions.NewStore()
	u := New(a, d, nil, nil, nil, extStore, nil, nil, solar.PriceConfig{}, nil)
	u.RootFiles = func(w http.ResponseWriter, r *http.Request) bool { return false }

	r := chi.NewRouter()
	u.Mount(r)
	r.NotFound(u.NotFound)
	return u, r
}

// TestPeriodPatchBumpsRevision: cada parche lleva la revisión siguiente para
// que el bloque alterno de clase (rev-a/rev-b) y las animaciones se reinicien.
func TestPeriodPatchBumpsRevision(t *testing.T) {
	today := startOfDay(time.Now())
	sel := buildPeriodSelector("es", PeriodMes, today, today, 3)
	if len(sel.Tabs) != 4 {
		t.Fatalf("tabs = %d", len(sel.Tabs))
	}
	for _, tab := range sel.Tabs {
		if !strings.Contains(tab.Patch, "rev=4") {
			t.Fatalf("el parche de %s no lleva rev=4: %s", tab.Key, tab.Patch)
		}
	}
	if !strings.Contains(sel.PatchPrev, "rev=4") || !strings.Contains(sel.PatchToday, "rev=4") {
		t.Fatalf("flechas/hoy sin revisión: %q %q", sel.PatchPrev, sel.PatchToday)
	}
	if revClass(0) != "rev-a" || revClass(1) != "rev-b" || revClass(2) != "rev-a" {
		t.Fatal("revClass no alterna")
	}
}

// TestNavPatchRendersFragments: la navegación sin recargar pide los fragmentos
// (contenido, cabecera y estado del menú) en un solo SSE.
func TestNavPatchRendersFragments(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "nav.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	_, router := buildChargerUI(t, d, extensions.NewStore(), extensions.New(extensions.NewStore(), d, nil))
	cookie := sessionCookie(t, doLogin(t, router, "nacho", "password1"))

	req := httptest.NewRequest("GET", "/ui/nav?path=%2Fajustes", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"selector #page", "selector #page-head", "selector #nav-primary", "selector #nav-rail", "selector #nav-bottom", "Ajustes"} {
		if !strings.Contains(body, want) {
			t.Fatalf("falta %q en el parche de navegación", want)
		}
	}
	// El ítem activo tiene que ser Ajustes, no Hoy.
	if !strings.Contains(body, "bg-brand/15 text-brand") {
		t.Fatal("el menú no marca Ajustes como activo")
	}
}

// TestNavPatchUnknownPath: una ruta que no es del menú se rechaza (el cliente
// cae a recarga completa).
func TestNavPatchUnknownPath(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "nav2.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	_, router := buildChargerUI(t, d, extensions.NewStore(), extensions.New(extensions.NewStore(), d, nil))
	cookie := sessionCookie(t, doLogin(t, router, "nacho", "password1"))
	req := httptest.NewRequest("GET", "/ui/nav?path=%2Fno-existe", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, quería 400", rec.Code)
	}
}

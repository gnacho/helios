package web

import (
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/install"
	"helios-server/internal/solar"
)

func TestDonutArcs(t *testing.T) {
	m := buildDonut("es", []donutSliceIn{
		{Label: "Solis", Value: 10, Color: "#111"},
		{Label: "Fox", Value: 10, Color: "#222"},
	}, 3)
	if len(m.Slices) != 2 {
		t.Fatalf("slices = %d", len(m.Slices))
	}
	if m.Slices[0].Pct != 50 || m.Slices[1].Pct != 50 {
		t.Fatalf("pct = %v / %v (esperado 50/50)", m.Slices[0].Pct, m.Slices[1].Pct)
	}
	if m.Slices[0].Path == "" || strings.Contains(m.Slices[0].Path, "NaN") {
		t.Fatalf("path inválido: %q", m.Slices[0].Path)
	}
	// Cada arco de 180-3 grados usa large=0 (span <= 180).
	if !strings.Contains(m.Slices[0].Path, " 0 1 ") {
		t.Fatalf("arco de medio círculo debería ser large=0: %q", m.Slices[0].Path)
	}
	// Total con formato es.
	if m.TotalText != "20,0" {
		t.Fatalf("total = %q", m.TotalText)
	}
	// Determinista.
	m2 := buildDonut("es", []donutSliceIn{
		{Label: "Solis", Value: 10, Color: "#111"},
		{Label: "Fox", Value: 10, Color: "#222"},
	}, 3)
	if m.Slices[0].Path != m2.Slices[0].Path {
		t.Fatal("paths no deterministas")
	}
}

func TestDonutSingleSliceFullRing(t *testing.T) {
	m := buildDonut("es", []donutSliceIn{{Label: "Solo", Value: 5, Color: "#111"}}, 3)
	if len(m.Slices) != 1 {
		t.Fatalf("slices = %d", len(m.Slices))
	}
	// Un único valor debe cubrir todo el anillo (clamp 359.9) con large=1.
	if !strings.Contains(m.Slices[0].Path, " 1 1 ") {
		t.Fatalf("anillo completo debería ser large=1: %q", m.Slices[0].Path)
	}
	if m.Slices[0].Pct != 100 {
		t.Fatalf("pct = %v", m.Slices[0].Pct)
	}
}

func TestDonutFiltersAndEmpty(t *testing.T) {
	// Valores <= 0.005 se filtran como en el React.
	m := buildDonut("es", []donutSliceIn{
		{Label: "A", Value: 0.001, Color: "#111"},
		{Label: "B", Value: 4, Color: "#222"},
	}, 2)
	if len(m.Slices) != 1 || m.Slices[0].Label != "B" {
		t.Fatalf("filtrado de slices: %+v", m.Slices)
	}
	// Todo vacío: donut sin slices, con total 0,0.
	empty := buildDonut("es", []donutSliceIn{{Label: "A", Value: 0, Color: "#111"}}, 2)
	if len(empty.Slices) != 0 || empty.TotalText != "0,0" {
		t.Fatalf("donut vacío: %+v total %q", empty.Slices, empty.TotalText)
	}
}

func TestDonutPolarAnchors(t *testing.T) {
	// Convención donutPolar (la polar() del React): 0 = 12 en punto,
	// 90 = 3 en punto, creciendo en sentido horario.
	x, y := donutPolar(100, 100, 50, 0)
	if math.Abs(x-100) > 0.01 || math.Abs(y-50) > 0.01 {
		t.Fatalf("polar 0 = (%v, %v)", x, y)
	}
	x, y = donutPolar(100, 100, 50, 90)
	if math.Abs(x-150) > 0.01 || math.Abs(y-100) > 0.01 {
		t.Fatalf("polar 90 = (%v, %v)", x, y)
	}
	x, y = donutPolar(100, 100, 50, 180)
	if math.Abs(x-100) > 0.01 || math.Abs(y-150) > 0.01 {
		t.Fatalf("polar 180 = (%v, %v)", x, y)
	}
}

func testDailyRows() []db.DailyRow {
	return []db.DailyRow{
		{Date: "2026-10-01", ProductionKwh: f64Ptr(10), BatteryChargedKwh: f64Ptr(2), BatteryDischargedKwh: f64Ptr(1.5), InvertersKwh: jsonPtr(`{"solis":6,"fox":4}`)},
		{Date: "2026-10-02", ProductionKwh: f64Ptr(20), BatteryChargedKwh: f64Ptr(3), BatteryDischargedKwh: f64Ptr(2), InvertersKwh: jsonPtr(`{"solis":12,"fox":8}`)},
		{Date: "2026-10-03", ProductionKwh: f64Ptr(5), InvertersKwh: jsonPtr(`{"solis":3,"fox":2}`)},
	}
}

func TestBuildLast14(t *testing.T) {
	m := buildLast14("es", testDailyRows(), 0.5, "#F59E0B")
	if len(m.Bars) != 3 {
		t.Fatalf("bars = %d", len(m.Bars))
	}
	// Barra 1: 20 kWh * 0.5 = 10 kWh sobre yMax = 20*0.5+4 = 14.
	b := m.Bars[1]
	plotH := last14H - last14Top - last14Bottom
	wantH := 10.0 / 14.0 * plotH
	if math.Abs(b.H-wantH) > 0.01 {
		t.Fatalf("h = %v (esperado %v)", b.H, wantH)
	}
	if !m.Bars[2].IsLast || m.Bars[0].IsLast {
		t.Fatal("solo la última barra es destacada")
	}
	// interval=1: impares etiquetadas.
	if m.Bars[0].ShowLabel || !m.Bars[1].ShowLabel {
		t.Fatal("etiquetas cada 2 barras")
	}
	if m.Bars[1].Label != "2" {
		t.Fatalf("label = %q", m.Bars[1].Label)
	}
	// Vacío.
	if e := buildLast14("es", nil, 1, "#000"); len(e.Bars) != 0 {
		t.Fatal("sin filas no debería haber barras")
	}
}

func TestTopRoundedBarPath(t *testing.T) {
	p := topRoundedBarPath(10, 20, 30, 40, 4)
	if !strings.HasPrefix(p, "M 10.00 60.00") || !strings.Contains(p, "Q") || strings.HasSuffix(p, "Z") == false {
		t.Fatalf("path de barra: %q", p)
	}
	// Barras imposibles: vacías.
	if topRoundedBarPath(0, 0, 10, 0, 4) != "" {
		t.Fatal("h=0 debería dar path vacío")
	}
	if bottomRoundedBarPath(0, 0, 0, 10, 4) != "" {
		t.Fatal("w=0 debería dar path vacío")
	}
}

func TestBuildInverterDayChart(t *testing.T) {
	m := BuildInverterDayChart("es", testSeries(), "solis", "#0EA5E9", 720)
	if m.YMax != 3 { // max solis 2.2 + 0.4 → ceil 3
		t.Fatalf("yMax = %d", m.YMax)
	}
	if !strings.HasSuffix(m.PastPath, "Z") {
		t.Fatal("el pasado debe cerrarse como área")
	}
	if strings.HasSuffix(m.FuturePath, "Z") {
		t.Fatal("el futuro es línea, no área")
	}
	if len(m.TicksX) != 9 || len(m.TicksY) != m.YMax+1 {
		t.Fatalf("ticks %d/%d", len(m.TicksX), len(m.TicksY))
	}
}

func TestBuildCompareChart(t *testing.T) {
	colors := []string{"#0EA5E9", "#8B5CF6"}
	m := BuildCompareChart("es", testSeries(), 720, []string{"solis", "fox"},
		map[string]string{"solis": "Solis", "fox": "Fox"}, colors, nil, false)
	if len(m.InvPaths) != 2 {
		t.Fatalf("paths = %d", len(m.InvPaths))
	}
	if m.InvPaths[0].Var != "#0EA5E9" || m.InvPaths[1].Var != "#8B5CF6" {
		t.Fatalf("colores = %v / %v", m.InvPaths[0].Var, m.InvPaths[1].Var)
	}
	if m.YMax != 3 { // max 2.2 + 0.4 → ceil 3
		t.Fatalf("yMax = %d", m.YMax)
	}
	if strings.Contains(m.InvPaths[0].Path, "Z") {
		t.Fatal("las curvas de comparación son líneas")
	}
}

func TestBestIdx(t *testing.T) {
	if got := bestIdx([]float64{1, 5, 2}); got != 1 {
		t.Fatalf("best = %d", got)
	}
	if got := bestIdx([]float64{3, 3}); got != -1 {
		t.Fatalf("empate = %d", got)
	}
	if got := bestIdx([]float64{7}); got != -1 {
		t.Fatalf("single = %d", got)
	}
}

// TestDailyHistoryPaginates: with more than 1000 daily rows the window must
// still include the most recent days (LIMIT would silently truncate them).
func TestDailyHistoryPaginates(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "hist.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	// La ventana acaba en solar.TodayStr(): ancla el extremo nuevo a HOY.
	today, err := time.ParseInLocation("2006-01-02", solar.TodayStr(), time.Local)
	if err != nil {
		t.Fatalf("today: %v", err)
	}
	base := today
	const n = 2500
	for i := 0; i < n; i++ {
		date := base.AddDate(0, 0, -i).Format("2006-01-02")
		if err := d.UpsertDaily(db.DailyRow{Date: date, ProductionKwh: f64Ptr(float64(i))}); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	u := &UI{DB: d}
	rows := u.dailyHistory()
	if len(rows) != n {
		t.Fatalf("filas = %d (esperado %d)", len(rows), n)
	}
	last := rows[len(rows)-1]
	wantLast := base.Format("2006-01-02")
	if last.Date != wantLast {
		t.Fatalf("última fila = %s (esperado %s)", last.Date, wantLast)
	}
	first := rows[0]
	wantFirst := base.AddDate(0, 0, -(n - 1)).Format("2006-01-02")
	if first.Date != wantFirst {
		t.Fatalf("primera fila = %s (esperado %s)", first.Date, wantFirst)
	}
}

func TestInvMetasOf(t *testing.T) {
	hist := testDailyRows()
	inst := []install.Inverter{
		{Key: "solis", Name: "Solis", Model: "M1", Kwp: 4.4, Panels: "10 × 440 W", HasBattery: true, BatteryKwh: 5},
		{Key: "fox", Name: "Fox", Model: "M2", Kwp: 2.7},
	}
	metas := invMetasOf(inst, hist, time.Date(2026, 10, 5, 0, 0, 0, 0, time.Local))
	if len(metas) != 2 {
		t.Fatalf("metas = %d", len(metas))
	}
	if metas[0].MonthKwh != 21 || metas[0].TotalMwh != 0.021 {
		t.Fatalf("solis month/total = %v / %v", metas[0].MonthKwh, metas[0].TotalMwh)
	}
	if metas[1].MonthKwh != 14 || metas[1].TotalMwh != 0.014 {
		t.Fatalf("fox month/total = %v / %v", metas[1].MonthKwh, metas[1].TotalMwh)
	}
	// Fallback sin topología.
	fb := invMetasOf(nil, nil, time.Now())
	if len(fb) != 2 || fb[0].Key != "solis" || fb[1].Key != "fox" {
		t.Fatalf("fallback = %+v", fb)
	}
}

func TestInvDayKwhAndPeaks(t *testing.T) {
	points := []solar.DayPoint{
		{T: 0, Solis: 1, Fox: 0.5},
		{T: 5, Solis: 2, Fox: 1},
		{T: 10, Solis: 0.5, Fox: 2},
	}
	kwhs, peaks := invDayKwhAndPeaks(points, []string{"solis", "fox"})
	// 5 min = 1/12 h: solis (1+2+0.5)/12; fox (0.5+1+2)/12.
	if math.Abs(kwhs[0]-3.5/12) > 1e-9 || math.Abs(kwhs[1]-3.5/12) > 1e-9 {
		t.Fatalf("kwh = %v / %v", kwhs[0], kwhs[1])
	}
	if peaks[0].V != 2 || peaks[0].T != 5 {
		t.Fatalf("peak solis = %+v", peaks[0])
	}
	if peaks[1].V != 2 || peaks[1].T != 10 {
		t.Fatalf("peak fox = %+v", peaks[1])
	}
}

func TestTotalShareOf(t *testing.T) {
	if got := totalShareOf([]float64{3, 1}, 0); math.Abs(got-75) > 1e-9 {
		t.Fatalf("share 0 = %v", got)
	}
	// Sin totales: todo al primero.
	if got := totalShareOf([]float64{0, 0}, 0); got != 100 {
		t.Fatalf("share vacío 0 = %v", got)
	}
	if got := totalShareOf([]float64{0, 0}, 1); got != 0 {
		t.Fatalf("share vacío 1 = %v", got)
	}
}

func TestInvSignalsOf(t *testing.T) {
	m := dashboardModel{
		Live:   &solar.LiveData{Solis: 1.5, Fox: 0.5, At: 721},
		Series: solar.DaySeries{Points: testSeries()},
	}
	sig := invSignalsOf("es", m, []string{"solis", "fox"})
	inv := sig["inv"].(map[string]any)
	s := inv["solis"].(map[string]any)
	if s["nowText"] != "1,50" {
		t.Fatalf("nowText = %v", s["nowText"])
	}
	// kWh hasta untilMin=721 (incluye T=720): solis 3.7/12; fox 2.28/12 → 62/38.
	if s["shareText"] != "62" {
		t.Fatalf("shareText = %v (esperado 62)", s["shareText"])
	}
	f := inv["fox"].(map[string]any)
	if f["shareText"] != "38" {
		t.Fatalf("shareText fox = %v (esperado 38)", f["shareText"])
	}
}

func TestBuildCompareChartNormalized(t *testing.T) {
	colors := []string{"#0EA5E9", "#8B5CF6"}
	kwps := []float64{4.4, 2.7}
	m := BuildCompareChart("es", testSeries(), 720, []string{"solis", "fox"},
		map[string]string{"solis": "Solis", "fox": "Fox"}, colors, kwps, true)
	if m.YMax != 1 {
		t.Fatalf("yMax normalizado = %d (esperado 1)", m.YMax)
	}
	// Ticks decimales 0..1 (recharts toFixed(1)).
	if len(m.TicksY) != 5 || m.TicksY[2].Label != "0.5" || m.TicksY[4].Label != "1.0" {
		t.Fatalf("ticks normalizados = %+v", m.TicksY)
	}
	if m.W != geomCompare.W || m.H != geomCompare.H {
		t.Fatalf("viewBox = %v x %v", m.W, m.H)
	}
}

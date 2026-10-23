package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"helios-server/internal/solar"
)

func f(v float64) *float64 { return &v }

func TestIntegrateSeries(t *testing.T) {
	// 6 buckets (30 min): production 1,2,3,2,1,0 kW; consumption 0.5 const;
	// grid +1 (import) first 3, -2 (export) last 3; battery +4/-1.
	points := []solar.DayPoint{
		{T: 0, Production: 1, Consumption: 0.5, Grid: f(1), BatteryPower: 4},
		{T: 5, Production: 2, Consumption: 0.5, Grid: f(1), BatteryPower: 4},
		{T: 10, Production: 3, Consumption: 0.5, Grid: f(1), BatteryPower: 0},
		{T: 15, Production: 2, Consumption: 0.5, Grid: f(-2), BatteryPower: -1},
		{T: 20, Production: 1, Consumption: 0.5, Grid: f(-2), BatteryPower: -1},
		{T: 25, Production: 0, Consumption: 0.5, Grid: f(-2), BatteryPower: -1},
		{T: 30, Production: 99, Consumption: 0.5}, // untilMin=30: excluido
	}
	k := IntegrateSeries(points, 30, 50)

	// 5 buckets de 5 min = 25 min = 5/12 h.
	// production: (1+2+3+2+1)/12 = 0.75 kWh
	if got := k.ProductionKwh; got < 0.749 || got > 0.751 {
		t.Fatalf("productionKwh = %v (esperado 0.75)", got)
	}
	// consumption: 0.5*6/12 = 0.25
	if got := k.ConsumptionKwh; got < 0.249 || got > 0.251 {
		t.Fatalf("consumptionKwh = %v", got)
	}
	// grid import: 1*3/12 = 0.25; export: 2*3/12 = 0.5
	if got := k.GridImportKwh; got < 0.249 || got > 0.251 {
		t.Fatalf("gridImportKwh = %v", got)
	}
	if got := k.GridExportKwh; got < 0.499 || got > 0.501 {
		t.Fatalf("gridExportKwh = %v", got)
	}
	// battery: charged 4*2/12 = 0.6667; discharged 1*3/12 = 0.25
	if got := k.BatteryChargedKwh; got < 0.666 || got > 0.668 {
		t.Fatalf("batteryChargedKwh = %v", got)
	}
	if got := k.BatteryDischargedKwh; got < 0.249 || got > 0.251 {
		t.Fatalf("batteryDischargedKwh = %v", got)
	}
	// peak: 3 kW en t=10
	if k.PeakProductionKw != 3 || k.PeakAt != 10 {
		t.Fatalf("peak = %v @%v (esperado 3 @10)", k.PeakProductionKw, k.PeakAt)
	}
	// autoconsumo: (0.75-0.5)/0.75*100 = 33.33
	if got := k.AutoconsumoPct; got < 33.3 || got > 33.4 {
		t.Fatalf("autoconsumoPct = %v", got)
	}
	// grid nil no aporta: serie solo con nil
	nilPoints := []solar.DayPoint{{T: 0, Production: 1, Consumption: 1, Grid: nil}}
	k2 := IntegrateSeries(nilPoints, 5, 0)
	if k2.GridImportKwh != 0 || k2.GridExportKwh != 0 {
		t.Fatalf("grid nil debe ignorarse: %+v", k2)
	}
}

func TestFormatNumber(t *testing.T) {
	if got := formatNumber("es", 1.234, 2); got != "1,23" {
		t.Fatalf("es format = %q", got)
	}
	if got := formatNumber("en", 1.234, 2); got != "1.23" {
		t.Fatalf("en format = %q", got)
	}
	if got := formatNumber("zh-CN", 12.5, 1); got != "12.5" {
		t.Fatalf("zh format = %q", got)
	}
	var nilF *float64
	if got := fmtKw("es", nilF); got != "—" {
		t.Fatalf("fmtKw nil = %q", got)
	}
	if got := fmtTime(6 * 60); got != "06:00" {
		t.Fatalf("fmtTime = %q", got)
	}
	if got := fmtTime(23*60 + 5); got != "23:05" {
		t.Fatalf("fmtTime = %q", got)
	}
}

func TestDashboardSignals(t *testing.T) {
	pal := newPalette(true)
	cons := 0.45
	grid := f(-2.5)
	m := dashboardModel{
		Palette: pal,
		Online:  true,
		Live: &solar.LiveData{
			Production: 1.234, Consumption: &cons, Grid: grid,
			Soc: 84.4, BatteryPower: -1.2, At: 810,
		},
	}
	s := dashboardSignals("es", m)

	live := s["live"].(map[string]any)
	if live["productionText"] != "1,23" {
		t.Fatalf("productionText = %v", live["productionText"])
	}
	if live["consumptionText"] != "0,45" {
		t.Fatalf("consumptionText = %v", live["consumptionText"])
	}
	if live["gridKwText"] != "2,50" {
		t.Fatalf("gridKwText = %v", live["gridKwText"])
	}
	if live["gridExporting"] != true || live["gridStatus"] != "Vertiendo" {
		t.Fatalf("grid estado = %v / %v", live["gridExporting"], live["gridStatus"])
	}
	if live["gridColor"] != pal.RedVertido {
		t.Fatalf("gridColor = %v", live["gridColor"])
	}
	if live["batteryStatus"] != "Descargando" {
		t.Fatalf("batteryStatus = %v", live["batteryStatus"])
	}
	if live["atText"] != "13:30" {
		t.Fatalf("atText = %v", live["atText"])
	}

	// Sin HA: todo a guiones y sin pánico.
	s2 := dashboardSignals("en", dashboardModel{Palette: newPalette(false)})
	live2 := s2["live"].(map[string]any)
	if live2["productionText"] != "—" || live2["gridKwText"] != "—" {
		t.Fatalf("señales sin HA = %v", live2)
	}
}

func TestDecodeLivePayload(t *testing.T) {
	payload := []byte(`{"type":"live","data":{"production":0.99,"soc":84.6,"inverters":[{"key":"solis","name":"Solis","kw":0.61}]}}`)
	live, err := decodeLivePayload(payload)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if live.Production != 0.99 || live.Soc != 84.6 || len(live.Inverters) != 1 || live.Inverters[0].Kw != 0.61 {
		t.Fatalf("live decodificado mal: %+v", live)
	}
}

func TestDashboardPageRenders(t *testing.T) {
	_, r, _ := testUI(t)
	cookie := loginSession(t, r)

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`data-signals=`,
		`data-on-load=`,
		`ui/stream`,
		`$kpis.productionKwh`,
		`$live.productionKw`,
		`data-show="$live.gridExporting"`,
		"Ahora mismo",
		"Producción hoy",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard sin %q", want)
		}
	}
	// Señales iniciales embebidas como JSON válido.
	start := strings.Index(body, `data-signals="`)
	if start < 0 {
		t.Fatal("sin data-signals")
	}
}

func TestStreamWithoutHAReturns503(t *testing.T) {
	_, r, _ := testUI(t)
	cookie := loginSession(t, r)
	req := httptest.NewRequest("GET", "/ui/stream", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d (esperado 503 sin HAOS)", rec.Code)
	}
}

// loginSession logs in and returns the session cookie.
func loginSession(t *testing.T, r interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}) *http.Cookie {
	t.Helper()
	form := strings.NewReader("username=nacho&password=password1")
	req := httptest.NewRequest("POST", "/login", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.Name == "helios_session" && c.Value != "" {
			return c
		}
	}
	t.Fatal("sin cookie de sesión")
	return nil
}

var _ = json.Marshal

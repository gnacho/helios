package solar

import (
	"path/filepath"
	"testing"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/ha"
	"helios-server/internal/install"
)

type countingHA struct {
	fakeHA
	statsCalls int
}

func (c *countingHA) StatisticsDuringPeriod(start, end string, ids []string, period string, types []string) (ha.Stats, error) {
	c.statsCalls++
	return c.stats, c.statsErr
}

func newCounting() *countingHA {
	return &countingHA{fakeHA: fakeHA{states: map[string]ha.EntityState{}, connected: true}}
}

func TestGetKpisPastDayFromDailyRow(t *testing.T) {
	d := testDB(t)
	s := New(install.NewStore(), d)

	if err := d.UpsertDaily(db.DailyRow{
		Date:          "2026-10-20",
		ProductionKwh: floatPtr(20), SolisKwh: floatPtr(12), FoxKwh: floatPtr(8),
		ConsumptionKwh: floatPtr(15), GridImportKwh: floatPtr(3), GridExportKwh: floatPtr(5),
		BatteryChargedKwh: floatPtr(2), BatteryDischargedKwh: floatPtr(1),
		InvertersKwh: db.InvertersJSON(map[string]float64{"solis": 12, "fox": 8}),
	}); err != nil {
		t.Fatalf("daily: %v", err)
	}

	f := newCounting()
	pc := PriceConfig{PriceImport: 0.15, PriceExport: 0.08, Co2PerKwh: 0.25}

	k, err := s.GetKpis(f, "2026-10-20", pc)
	if err != nil {
		t.Fatalf("GetKpis: %v", err)
	}

	if k.ProductionKwh != 20 || k.SolisKwh != 12 || k.FoxKwh != 8 {
		t.Fatalf("producción = %v/%v/%v", k.ProductionKwh, k.SolisKwh, k.FoxKwh)
	}
	if k.ConsumptionKwh != 15 {
		t.Fatalf("consumption = %v", k.ConsumptionKwh)
	}
	// autoconsumo = (20-5)/20 = 75%
	if k.AutoconsumoPct != 75 {
		t.Fatalf("autoconsumo = %v (esperado 75)", k.AutoconsumoPct)
	}
	// autosuficiencia = (15-3)/15 = 80%
	if k.AutosuficienciaPct != 80 {
		t.Fatalf("autosuficiencia = %v (esperado 80)", k.AutosuficienciaPct)
	}
	// ahorro = (min(20,15)+1)*0.15 + 5*0.08 = 16*0.15 + 0.4 = 2.4+0.4 = 2.8
	if k.AhorroEur != 2.8 {
		t.Fatalf("ahorro = %v (esperado 2.8)", k.AhorroEur)
	}
	if k.Co2EvitadoKg != 5 {
		t.Fatalf("co2 = %v (esperado 5)", k.Co2EvitadoKg)
	}
	if k.InvertersKwh["solis"] != 12 || k.InvertersKwh["fox"] != 8 {
		t.Fatalf("invertersKwh = %v", k.InvertersKwh)
	}
}

func TestGetKpisPastDayWithoutDailyRow(t *testing.T) {
	d := testDB(t)
	s := New(install.NewStore(), d)

	f := newCounting()
	f.stats = ha.Stats{
		"sensor.solis_energia_hoy":               {{Start: "2026-10-19T00:00:00Z", State: floatPtr(4.0)}},
		"sensor.energia_fox_diaria":              {{Start: "2026-10-19T00:00:00Z", State: floatPtr(2.0)}},
		"sensor.consumo_total_diario":            {{Start: "2026-10-19T00:00:00Z", State: floatPtr(5.0)}},
		"sensor.energia_red_importada_solis":     {{Start: "2026-10-19T00:00:00Z", State: floatPtr(1.0)}},
		"sensor.energia_red_exportada_solis":     {{Start: "2026-10-19T00:00:00Z", State: floatPtr(2.0)}},
		"sensor.energia_bateria_carga_diaria":    {{Start: "2026-10-19T00:00:00Z", State: floatPtr(0.5)}},
		"sensor.energia_bateria_descarga_diaria": {{Start: "2026-10-19T00:00:00Z", State: floatPtr(0.25)}},
	}

	k, err := s.GetKpis(f, "2026-10-19", PriceConfig{PriceImport: 0.15, PriceExport: 0.08, Co2PerKwh: 0.25})
	if err != nil {
		t.Fatalf("GetKpis: %v", err)
	}
	if k.ProductionKwh != 6 {
		t.Fatalf("production = %v (esperado 6)", k.ProductionKwh)
	}
	if k.ConsumptionKwh != 5 {
		t.Fatalf("consumption = %v", k.ConsumptionKwh)
	}
	if k.GridImportKwh != 1 || k.GridExportKwh != 2 {
		t.Fatalf("grid = %v/%v", k.GridImportKwh, k.GridExportKwh)
	}
	if k.BatteryChargedKwh != 0.5 || k.BatteryDischargedKwh != 0.25 {
		t.Fatalf("batería = %v/%v", k.BatteryChargedKwh, k.BatteryDischargedKwh)
	}
}

func TestGetKpisToday(t *testing.T) {
	d := testDB(t)
	s := New(install.NewStore(), d)

	f := newCounting()
	f.set("sensor.solis_energia_hoy", "8")
	f.set("sensor.energia_fox_diaria", "4")
	f.set("sensor.energia_red_importada_solis", "2")
	f.set("sensor.energia_red_exportada_solis", "1")
	f.set("sensor.energia_bateria_carga_diaria", "1")
	f.set("sensor.energia_bateria_descarga_diaria", "0.5")
	f.set("sensor.consumo_total_diario", "9")
	// live
	f.set("sensor.solis_potencia_actual", "3")
	f.set("sensor.almacen_pinza_power_b", "0")
	f.set("sensor.solis_bateria_potencia", "0")
	f.set("sensor.solis_bateria_estado", "idle")
	f.set("sensor.solis_bateria_soc", "88")
	f.set("sensor.medidor_respaldo_power", "100")
	f.set("sensor.vivienda_medidor_power", "100")
	f.set("sensor.almacen_pinza_power_a", "0")
	f.setAttrs("sensor.solis_scraper", map[string]any{"currentGridPower": 0.0, "gridDirection": "none"})
	f.set("sun.sun", "above_horizon")
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(30)})

	k, err := s.GetKpis(f, "", PriceConfig{PriceImport: 0.15, PriceExport: 0.08, Co2PerKwh: 0.25})
	if err != nil {
		t.Fatalf("GetKpis: %v", err)
	}
	if k.ProductionKwh != 12 {
		t.Fatalf("production = %v (esperado 12)", k.ProductionKwh)
	}
	if k.Soc != 88 {
		t.Fatalf("soc = %v (esperado 88 del live)", k.Soc)
	}
}

func TestGetDaySeriesPersistsAndCaches(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	defer d.Close()

	s := New(install.NewStore(), d)
	f := newCounting()

	// Stats vacíos: la serie sale sin puntos, pero el test de caché usa un día
	// con datos para forzar cómputo.
	f.stats = ha.Stats{
		"sensor.solis_potencia_actual":  {{Start: "2026-10-18T12:00:00Z", Mean: floatPtr(2.0)}},
		"sensor.almacen_pinza_power_b":  {{Start: "2026-10-18T12:00:00Z", Mean: floatPtr(0)}},
		"sensor.medidor_respaldo_power": {{Start: "2026-10-18T12:00:00Z", Mean: floatPtr(0)}},
		"sensor.vivienda_medidor_power": {{Start: "2026-10-18T12:00:00Z", Mean: floatPtr(0)}},
		"sensor.almacen_pinza_power_a":  {{Start: "2026-10-18T12:00:00Z", Mean: floatPtr(0)}},
		"sensor.solis_bateria_potencia": {{Start: "2026-10-18T12:00:00Z", Mean: floatPtr(0)}},
		"sensor.solis_bateria_soc":      {{Start: "2026-10-18T12:00:00Z", Mean: floatPtr(50)}},
	}

	first, err := s.GetDaySeries(f, "2026-10-18")
	if err != nil {
		t.Fatalf("primera: %v", err)
	}
	if len(first.Points) != 1 {
		t.Fatalf("points = %d", len(first.Points))
	}
	callsAfterFirst := f.statsCalls
	if callsAfterFirst == 0 {
		t.Fatal("debería haber consultado HAOS la primera vez")
	}

	second, err := s.GetDaySeries(f, "2026-10-18")
	if err != nil {
		t.Fatalf("segunda: %v", err)
	}
	if f.statsCalls != callsAfterFirst {
		t.Fatalf("segunda llamada no debería tocar HAOS (calls=%d)", f.statsCalls)
	}
	if len(second.Points) != 1 || second.Points[0].Production != first.Points[0].Production {
		t.Fatalf("serie persistida distinta: %+v vs %+v", first.Points, second.Points)
	}
}

func TestEachDayKey(t *testing.T) {
	start := time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local)
	end := time.Date(2026, 10, 23, 0, 0, 0, 0, time.Local)
	keys := eachDayKey(start, end)
	if len(keys) != 3 {
		t.Fatalf("keys = %v", keys)
	}
	if keys[0] != "2026-10-20" || keys[2] != "2026-10-22" {
		t.Fatalf("keys = %v", keys)
	}
}

func TestSliceStatsByDay(t *testing.T) {
	local := func(day, hour int) string {
		return time.Date(2026, 10, day, hour, 0, 0, 0, time.Local).Format(time.RFC3339)
	}
	stats := ha.Stats{
		"sensor.x": {
			{Start: local(20, 10), Mean: floatPtr(1)},
			{Start: local(21, 10), Mean: floatPtr(2)},
		},
	}
	out := sliceStatsByDay(stats, "2026-10-21")
	if len(out["sensor.x"]) != 1 {
		t.Fatalf("filas = %d", len(out["sensor.x"]))
	}
	if *out["sensor.x"][0].Mean != 2 {
		t.Fatalf("mean = %v", *out["sensor.x"][0].Mean)
	}
}

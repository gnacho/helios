package solar

import (
	"encoding/json"
	"testing"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/ha"
	"helios-server/internal/install"
)

func dbRow(date string) db.DailyRow {
	return db.DailyRow{Date: date, ProductionKwh: floatPtr(1)}
}

func jsonUnmarshal(s string, v any) error { return json.Unmarshal([]byte(s), v) }

func dayRow(start string, sum, state *float64) ha.StatsRow {
	t, _ := time.Parse(time.RFC3339, start)
	return ha.StatsRow{Start: ha.FlexTime{Time: t}, Sum: sum, State: state}
}

func TestBackfillHistoryBasic(t *testing.T) {
	d := testDB(t)
	s := New(install.NewStore(), d)
	f := newCounting()

	f.stats = ha.Stats{
		// Solis: acumulador (acc sum) -> deltas
		"sensor.energia_solis_diaria_um": {
			dayRow("2026-10-20T00:00:00Z", floatPtr(10), nil),
			dayRow("2026-10-21T00:00:00Z", floatPtr(25), nil),
		},
		// Fox: state creciente -> delta
		"sensor.almacen_pinza_energy_produced_b": {
			dayRow("2026-10-20T00:00:00Z", nil, floatPtr(100)),
			dayRow("2026-10-21T00:00:00Z", nil, floatPtr(110)),
		},
		// Consumo: 3 contadores, requireAll
		"sensor.medidor_respaldo_energy": {
			dayRow("2026-10-20T00:00:00Z", nil, floatPtr(50)),
			dayRow("2026-10-21T00:00:00Z", nil, floatPtr(60)),
		},
		"sensor.vivienda_medidor_energy": {
			dayRow("2026-10-20T00:00:00Z", nil, floatPtr(70)),
			dayRow("2026-10-21T00:00:00Z", nil, floatPtr(85)),
		},
		"sensor.almacen_pinza_energy_a": {
			dayRow("2026-10-20T00:00:00Z", nil, floatPtr(30)),
			dayRow("2026-10-21T00:00:00Z", nil, floatPtr(35)),
		},
		// Red
		"sensor.energia_red_importada_solis": {
			dayRow("2026-10-20T00:00:00Z", floatPtr(2), nil),
			dayRow("2026-10-21T00:00:00Z", floatPtr(5), nil),
		},
		"sensor.energia_red_exportada_solis": {
			dayRow("2026-10-20T00:00:00Z", floatPtr(2), nil),
			dayRow("2026-10-21T00:00:00Z", floatPtr(4), nil),
		},
		"sensor.energia_bateria_carga_diaria": {
			dayRow("2026-10-20T00:00:00Z", floatPtr(1), nil),
			dayRow("2026-10-21T00:00:00Z", floatPtr(2), nil),
		},
		"sensor.energia_bateria_descarga_diaria": {
			dayRow("2026-10-20T00:00:00Z", floatPtr(1), nil),
			dayRow("2026-10-21T00:00:00Z", floatPtr(1.5), nil),
		},
	}

	n, err := s.BackfillHistory(f)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if n != 2 {
		t.Fatalf("filas = %d (esperado 2)", n)
	}

	row, err := d.DailyRowByDate("2026-10-21")
	if err != nil || row == nil {
		t.Fatalf("fila 21: %v", err)
	}
	if row.SolisKwh == nil || *row.SolisKwh != 15 {
		t.Fatalf("solis_kwh = %v (esperado 15)", row.SolisKwh)
	}
	if row.FoxKwh == nil || *row.FoxKwh != 10 {
		t.Fatalf("fox_kwh = %v (esperado 10)", row.FoxKwh)
	}
	if row.ProductionKwh == nil || *row.ProductionKwh != 25 {
		t.Fatalf("production = %v (esperado 25)", row.ProductionKwh)
	}
	if row.GridImportKwh == nil || *row.GridImportKwh != 3 {
		t.Fatalf("grid_import = %v (esperado 3)", row.GridImportKwh)
	}
	if row.GridExportKwh == nil || *row.GridExportKwh != 2 {
		t.Fatalf("grid_export = %v (esperado 2)", row.GridExportKwh)
	}
	if row.BatteryChargedKwh == nil || *row.BatteryChargedKwh != 1 {
		t.Fatalf("bat_charge = %v", row.BatteryChargedKwh)
	}
	if row.BatteryDischargedKwh == nil || *row.BatteryDischargedKwh != 0.5 {
		t.Fatalf("bat_discharge = %v", row.BatteryDischargedKwh)
	}

	// inverters_kwh por clave real
	if row.InvertersKwh == nil {
		t.Fatal("falta inverters_kwh")
	}
	var ik map[string]float64
	if err := jsonUnmarshal(*row.InvertersKwh, &ik); err != nil {
		t.Fatalf("inverters json: %v", err)
	}
	if ik["solis"] != 15 || ik["fox"] != 10 {
		t.Fatalf("inverters_kwh = %v", ik)
	}
}

func TestBackfillHistorySpreadsGap(t *testing.T) {
	d := testDB(t)
	s := New(install.NewStore(), d)
	f := newCounting()

	// Fox salta del 18 al 21 (huecos 19 y 20). delta 60 -> repartido en 3 días.
	f.stats = ha.Stats{
		"sensor.almacen_pinza_energy_produced_b": {
			dayRow("2026-10-18T00:00:00Z", nil, floatPtr(100)),
			dayRow("2026-10-21T00:00:00Z", nil, floatPtr(160)),
		},
		// resto vacío: solo nos interesa la columna fox
		"sensor.energia_solis_diaria_um": {},
	}

	if _, err := s.BackfillHistory(f); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	for _, date := range []string{"2026-10-19", "2026-10-20", "2026-10-21"} {
		row, _ := d.DailyRowByDate(date)
		if row == nil || row.FoxKwh == nil {
			t.Fatalf("%s sin fila/valor", date)
		}
		if *row.FoxKwh < 19.9 || *row.FoxKwh > 20.1 {
			t.Fatalf("%s fox = %v (esperado ~20 repartido)", date, *row.FoxKwh)
		}
	}
}

func TestBackfillHistoryAppliesGlitchOffset(t *testing.T) {
	d := testDB(t)
	s := New(install.NewStore(), d)
	f := newCounting()

	// 2025-06-15 tiene glitchOffset 5.46 para Fox: el delta bruto 20 debe
	// quedar en 20 - 5.46 = 14.54.
	f.stats = ha.Stats{
		"sensor.almacen_pinza_energy_produced_b": {
			dayRow("2025-06-14T00:00:00Z", nil, floatPtr(100)),
			dayRow("2025-06-15T00:00:00Z", nil, floatPtr(120)),
		},
		"sensor.energia_solis_diaria_um": {},
	}

	if _, err := s.BackfillHistory(f); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	row, _ := d.DailyRowByDate("2025-06-15")
	if row == nil || row.FoxKwh == nil {
		t.Fatal("sin fila para 2025-06-15")
	}
	if *row.FoxKwh < 14.5 || *row.FoxKwh > 14.6 {
		t.Fatalf("fox = %v (esperado ~14.54 tras glitch)", *row.FoxKwh)
	}
}

func TestBackfillHistoryCapsAnomaly(t *testing.T) {
	d := testDB(t)
	s := New(install.NewStore(), d)
	f := newCounting()

	// Fox con salto absurdo (1000 kWh) -> cap 20.
	f.stats = ha.Stats{
		"sensor.almacen_pinza_energy_produced_b": {
			dayRow("2026-10-20T00:00:00Z", nil, floatPtr(100)),
			dayRow("2026-10-21T00:00:00Z", nil, floatPtr(1100)),
		},
		"sensor.energia_solis_diaria_um": {},
	}

	if _, err := s.BackfillHistory(f); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	row, _ := d.DailyRowByDate("2026-10-21")
	if row == nil || row.FoxKwh == nil {
		t.Fatal("sin fila")
	}
	if *row.FoxKwh != 20 {
		t.Fatalf("fox = %v (esperado cap 20)", *row.FoxKwh)
	}
}

func TestMaybeBackfillOnlyWhenEmpty(t *testing.T) {
	d := testDB(t)
	s := New(install.NewStore(), d)
	f := newCounting()
	f.stats = ha.Stats{}

	ran, _, err := s.MaybeBackfill(f)
	if err != nil {
		t.Fatalf("maybe: %v", err)
	}
	if !ran {
		t.Fatal("con daily vacío debería ejecutarse")
	}

	// Insertamos una fila: ya no debe correr.
	if err := d.UpsertDaily(dbRow("2026-10-21")); err != nil {
		t.Fatalf("daily: %v", err)
	}
	ran2, _, err := s.MaybeBackfill(f)
	if err != nil {
		t.Fatalf("maybe2: %v", err)
	}
	if ran2 {
		t.Fatal("con daily con datos NO debería ejecutarse")
	}
}

func TestGapBetween(t *testing.T) {
	cases := []struct {
		prev, key string
		want      int
	}{
		{"", "2026-10-21", 0},
		{"2026-10-21", "2026-10-21", 0},
		{"2026-10-20", "2026-10-21", 0},
		{"2026-10-18", "2026-10-21", 2},
	}
	for _, c := range cases {
		if got := gapBetween(c.prev, c.key); got != c.want {
			t.Fatalf("gapBetween(%q,%q) = %d (esperado %d)", c.prev, c.key, got, c.want)
		}
	}
}

func TestBackfillDaySeriesSkipsOldWithoutBackup(t *testing.T) {
	d := testDB(t)
	s := New(install.NewStore(), d)
	f := newCounting()

	if err := d.UpsertDaily(dbRow("2024-05-01")); err != nil {
		t.Fatalf("daily: %v", err)
	}

	ran, rows := s.BackfillDaySeries(f, ha.Stats{})
	if ran || rows != 0 {
		t.Fatalf("no debería tocar días fuera de la ventana sin backup (ran=%v rows=%d)", ran, rows)
	}
}

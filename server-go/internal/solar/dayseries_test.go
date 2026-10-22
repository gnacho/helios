package solar

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/ha"
	"helios-server/internal/install"
)

func testDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "helios.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func statsRow(minute int, mean float64) ha.StatsRow {
	// Bucket en hora local para que bucketMinute devuelva el minuto esperado.
	base := time.Date(2026, 10, 22, minute/60, minute%60, 0, 0, time.Local)
	m := mean
	return ha.StatsRow{Start: base.Format(time.RFC3339), Mean: &m}
}

func mkStats(m map[string][]ha.StatsRow) ha.Stats { return ha.Stats(m) }

func TestBuildDaySeriesBasic(t *testing.T) {
	d := testDB(t)
	store := install.NewStore()
	s := New(store, d)

	stats := mkStats(map[string][]ha.StatsRow{
		"sensor.solis_potencia_actual":  {statsRow(720, 2.0)},
		"sensor.almacen_pinza_power_b":  {statsRow(720, 1000)}, // W
		"sensor.medidor_respaldo_power": {statsRow(720, 200)},
		"sensor.vivienda_medidor_power": {statsRow(720, 300)},
		"sensor.almacen_pinza_power_a":  {statsRow(720, 100)},
		"sensor.solis_bateria_potencia": {statsRow(720, 0)},
		"sensor.solis_bateria_soc":      {statsRow(720, 60)},
	})

	res := s.BuildDaySeries(stats, "2026-10-22", nil)

	if len(res.Points) != 1 {
		t.Fatalf("points = %d", len(res.Points))
	}
	p := res.Points[0]
	if p.T != 720 || p.Label != "12:00" {
		t.Fatalf("t/label = %d/%q", p.T, p.Label)
	}
	if p.Solis != 2.0 {
		t.Fatalf("solis = %v", p.Solis)
	}
	if p.Fox != 1.0 {
		t.Fatalf("fox = %v (1000W -> 1kW)", p.Fox)
	}
	if p.Production != 3.0 {
		t.Fatalf("production = %v", p.Production)
	}
	if p.Consumption != 0.6 {
		t.Fatalf("consumption = %v (600W -> 0.6kW)", p.Consumption)
	}
	if p.Grid == nil {
		t.Fatal("grid debería calcularse (todos los sensores presentes)")
	}
	// grid = consumo - producción + batería = 0.6 - 3.0 + 0
	if *p.Grid != -2.4 {
		t.Fatalf("grid = %v (esperado -2.4 export)", *p.Grid)
	}
	if res.Estimated {
		t.Fatal("no debería marcarse estimated")
	}
}

func TestBuildDaySeriesMissingConsumptionGivesNullGrid(t *testing.T) {
	d := testDB(t)
	s := New(install.NewStore(), d)

	// Sin filas de los medidores de consumo → grid no fiable → null.
	stats := mkStats(map[string][]ha.StatsRow{
		"sensor.solis_potencia_actual":  {statsRow(720, 2.0)},
		"sensor.almacen_pinza_power_b":  {statsRow(720, 1000)},
		"sensor.solis_bateria_potencia": {statsRow(720, 0)},
		"sensor.solis_bateria_soc":      {statsRow(720, 60)},
	})

	res := s.BuildDaySeries(stats, "2026-10-22", nil)
	if len(res.Points) != 1 {
		t.Fatalf("points = %d", len(res.Points))
	}
	if res.Points[0].Grid != nil {
		t.Fatalf("grid debería ser null, es %v", *res.Points[0].Grid)
	}
}

func TestBuildDaySeriesBackupScaling(t *testing.T) {
	d := testDB(t)
	s := New(install.NewStore(), d)

	// El Fox (inv1) no reporta su pinza en el bucket, pero sí su respaldo en la
	// nube (0.5 kW). El día tiene 3 kWh reales de Fox → debe reescalar el tramo.
	stats := mkStats(map[string][]ha.StatsRow{
		"sensor.solis_potencia_actual":  {statsRow(720, 2.0)},
		"sensor.foxess_solar_power":     {statsRow(720, 0.5)},
		"sensor.medidor_respaldo_power": {statsRow(720, 200)},
		"sensor.vivienda_medidor_power": {statsRow(720, 300)},
		"sensor.almacen_pinza_power_a":  {statsRow(720, 100)},
		"sensor.solis_bateria_potencia": {statsRow(720, 0)},
		"sensor.solis_bateria_soc":      {statsRow(720, 60)},
	})

	if err := d.UpsertDaily(db.DailyRow{
		Date: "2026-10-22", FoxKwh: floatPtr(3.0), SolisKwh: floatPtr(2.0),
	}); err != nil {
		t.Fatalf("daily: %v", err)
	}

	res := s.BuildDaySeries(stats, "2026-10-22", nil)

	if !res.Estimated {
		t.Fatal("debería marcarse estimated (usó respaldo)")
	}
	if len(res.Points) != 1 {
		t.Fatalf("points = %d", len(res.Points))
	}
	p := res.Points[0]
	// target 3.0 kWh en un único bucket de 5 min => 3.0/(5/60) = 36 kW
	if p.Fox <= 0.5 {
		t.Fatalf("fox no se reescaló: %v", p.Fox)
	}
	// El tramo con respaldo debe integrar el total diario de Fox.
	if got := p.Fox * bucketHours; got < 2.9 || got > 3.1 {
		t.Fatalf("integración fox = %v kWh (esperado ~3.0)", got)
	}
}

func TestBuildDaySeriesConsumptionGapFilledFromPrevDay(t *testing.T) {
	d := testDB(t)
	s := New(install.NewStore(), d)

	// Día anterior con curva de consumo conocida.
	prev := []DayPoint{{T: 720, Consumption: 1.5, Production: 0, BatteryPower: 0}}
	prevJSON := mustJSON(t, prev)
	if err := d.UpsertDaySeries("2026-10-21", prevJSON, false, "live", time.Now().UnixMilli()); err != nil {
		t.Fatalf("dayseries: %v", err)
	}

	// Hoy los medidores de consumo no reportan, pero los inversores sí.
	stats := mkStats(map[string][]ha.StatsRow{
		"sensor.solis_potencia_actual":  {statsRow(720, 2.0)},
		"sensor.almacen_pinza_power_b":  {statsRow(720, 0)},
		"sensor.solis_bateria_potencia": {statsRow(720, 0)},
		"sensor.solis_bateria_soc":      {statsRow(720, 60)},
	})

	res := s.BuildDaySeries(stats, "2026-10-22", nil)

	if !res.Estimated {
		t.Fatal("debería marcarse estimated (hueco de consumo)")
	}
	if len(res.Points) != 1 {
		t.Fatalf("points = %d", len(res.Points))
	}
	if res.Points[0].Consumption != 1.5 {
		t.Fatalf("consumption = %v (esperado patrón 1.5 del día anterior)", res.Points[0].Consumption)
	}
}

func TestBuildDaySeriesSolisHeuristic(t *testing.T) {
	d := testDB(t)
	s := New(install.NewStore(), d)

	// inv0 (Solis) sin statistics; inv1 (Fox) con 1.2 kW durante 13 buckets
	// (13 * 5/60 * 1.2 = 1.3 kWh, por encima del umbral sum1 > 1).
	var foxRows, cons1, cons2, cons3, batRows, socRows []ha.StatsRow
	for i := 0; i < 13; i++ {
		min := 720 + i*5
		foxRows = append(foxRows, statsRow(min, 1200))
		cons1 = append(cons1, statsRow(min, 200))
		cons2 = append(cons2, statsRow(min, 300))
		cons3 = append(cons3, statsRow(min, 100))
		batRows = append(batRows, statsRow(min, 0))
		socRows = append(socRows, statsRow(min, 60))
	}
	stats := mkStats(map[string][]ha.StatsRow{
		"sensor.almacen_pinza_power_b":  foxRows,
		"sensor.medidor_respaldo_power": cons1,
		"sensor.vivienda_medidor_power": cons2,
		"sensor.almacen_pinza_power_a":  cons3,
		"sensor.solis_bateria_potencia": batRows,
		"sensor.solis_bateria_soc":      socRows,
	})

	if err := d.UpsertDaily(db.DailyRow{
		Date: "2026-10-22", SolisKwh: floatPtr(10.0), FoxKwh: floatPtr(3.0),
	}); err != nil {
		t.Fatalf("daily: %v", err)
	}

	res := s.BuildDaySeries(stats, "2026-10-22", nil)

	if !res.Estimated {
		t.Fatal("debería marcarse estimated (heurística Solis)")
	}
	if len(res.Points) != 13 {
		t.Fatalf("points = %d", len(res.Points))
	}
	p := res.Points[0]
	if p.Solis <= 0 {
		t.Fatalf("solis debería estimarse desde fox, es %v", p.Solis)
	}
	// ratio = 10 kWh / (13 * 5/60 * 1.2 = 1.3 kWh) ≈ 7.69 -> solis ≈ 9.23 kW
	if p.Solis < 5 {
		t.Fatalf("solis estimado demasiado bajo: %v", p.Solis)
	}
	// El total integrado de Solis debe acercarse a los 10 kWh de daily.
	sum := 0.0
	for _, pt := range res.Points {
		sum += pt.Solis * bucketHours
	}
	if sum < 9.0 || sum > 11.0 {
		t.Fatalf("integración solis = %v kWh (esperado ~10)", sum)
	}
}

func TestBuildDaySeriesBatterySign(t *testing.T) {
	d := testDB(t)
	s := New(install.NewStore(), d)

	// SOC sube entre dos buckets -> batería cargando (positivo).
	stats := mkStats(map[string][]ha.StatsRow{
		"sensor.solis_potencia_actual":  {statsRow(720, 1.0), statsRow(725, 1.0)},
		"sensor.almacen_pinza_power_b":  {statsRow(720, 0), statsRow(725, 0)},
		"sensor.medidor_respaldo_power": {statsRow(720, 200), statsRow(725, 200)},
		"sensor.vivienda_medidor_power": {statsRow(720, 300), statsRow(725, 300)},
		"sensor.almacen_pinza_power_a":  {statsRow(720, 100), statsRow(725, 100)},
		"sensor.solis_bateria_potencia": {statsRow(720, 0.5), statsRow(725, 0.5)},
		"sensor.solis_bateria_soc":      {statsRow(720, 60), statsRow(725, 62)},
	})

	res := s.BuildDaySeries(stats, "2026-10-22", nil)
	if len(res.Points) != 2 {
		t.Fatalf("points = %d", len(res.Points))
	}
	// Primer bucket: sin prevSoc -> 0
	if res.Points[0].BatteryPower != 0 {
		t.Fatalf("batería primer bucket = %v (esperado 0)", res.Points[0].BatteryPower)
	}
	// Segundo bucket: SOC subió -> +0.5
	if res.Points[1].BatteryPower != 0.5 {
		t.Fatalf("batería segundo bucket = %v (esperado +0.5 cargando)", res.Points[1].BatteryPower)
	}
}

func TestBucketMinuteParsesLocal(t *testing.T) {
	base := time.Date(2026, 10, 22, 14, 35, 0, 0, time.Local)
	min, ok := bucketMinute(base.Format(time.RFC3339))
	if !ok {
		t.Fatal("no parseó")
	}
	if min != 14*60+35 {
		t.Fatalf("minuto = %d (esperado 875)", min)
	}
	if _, ok := bucketMinute("no-es-fecha"); ok {
		t.Fatal("no debería parsear basura")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	return string(b)
}

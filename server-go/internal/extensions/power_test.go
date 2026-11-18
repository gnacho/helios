package extensions

import (
	"math"
	"testing"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/ha"
	"helios-server/internal/install"
)

func powerExt(t *testing.T, cfg Charger) (*Ext, *db.DB, *fakeHA) {
	t.Helper()
	d := testDB(t)
	store := NewStore()
	store.Set(Config{Enabled: true, CarCharger: CarChargerConfig{
		Enabled:  true,
		Chargers: []Charger{cfg},
	}})
	ext := New(store, d, install.NewStore())
	f := newFake()
	return ext, d, f
}

func setPower(f *fakeHA, id, state string, updated time.Time) {
	f.states[id] = ha.EntityState{
		State:       state,
		Attributes:  map[string]any{},
		LastUpdated: updated.Format(time.RFC3339),
	}
}

func chargerKwh(t *testing.T, d *db.DB, day, key string) float64 {
	t.Helper()
	rows, err := d.ChargerHistoryRange(day, day, key)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	for _, r := range rows {
		if r.Kwh != nil {
			return *r.Kwh
		}
	}
	return 0
}

// TestAccumulateFromPowerAddsEnergy: un cargador sin contador acumula la
// energía del intervalo entre ticks a partir de la potencia.
func TestAccumulateFromPowerAddsEnergy(t *testing.T) {
	c := Charger{Key: "c2", Name: "Portátil", PowerID: "sensor.p", PowerUnit: UnitW}
	ext, d, f := powerExt(t, c)

	t0 := time.Now()
	setPower(f, "sensor.p", "2400", t0)
	ext.accumulateFromPower(f, c, t0) // primera muestra: solo base
	if got := chargerKwh(t, d, todayStr(), "c2"); got != 0 {
		t.Fatalf("primer tick no debe sumar: %v", got)
	}

	ext.accumulateFromPower(f, c, t0.Add(time.Minute))
	want := round3(2.4 * (time.Minute).Hours())
	got := chargerKwh(t, d, todayStr(), "c2")
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("kwh = %v (esperado %v)", got, want)
	}
}

// TestAccumulateFromPowerSkipsHoles: un hueco de datos no se rellena con
// energía inventada, solo se re-basa el punto de partida.
func TestAccumulateFromPowerSkipsHoles(t *testing.T) {
	c := Charger{Key: "c2", PowerID: "sensor.p", PowerUnit: UnitW}
	ext, d, f := powerExt(t, c)

	t0 := time.Now()
	setPower(f, "sensor.p", "2400", t0)
	ext.accumulateFromPower(f, c, t0)
	// Tick 30 min después con el dato fresco: hueco, no suma energía.
	setPower(f, "sensor.p", "2400", t0.Add(30*time.Minute))
	ext.accumulateFromPower(f, c, t0.Add(30*time.Minute))
	if got := chargerKwh(t, d, todayStr(), "c2"); got != 0 {
		t.Fatalf("el hueco no puede sumar energía: %v", got)
	}
	// Tras el hueco el acumulador sigue vivo desde el nuevo punto.
	setPower(f, "sensor.p", "2400", t0.Add(31*time.Minute))
	ext.accumulateFromPower(f, c, t0.Add(31*time.Minute))
	if got := chargerKwh(t, d, todayStr(), "c2"); got <= 0 {
		t.Fatalf("tras re-basar debe acumular: %v", got)
	}
}

// TestAccumulateFromPowerSkipsStale: con el dato de potencia congelado (socket
// abierto sin frames) no se acumula nada.
func TestAccumulateFromPowerSkipsStale(t *testing.T) {
	c := Charger{Key: "c2", PowerID: "sensor.p", PowerUnit: UnitW}
	ext, d, f := powerExt(t, c)

	t0 := time.Now()
	old := t0.Add(-30 * time.Minute)
	setPower(f, "sensor.p", "2400", old)
	ext.accumulateFromPower(f, c, t0.Add(-20*time.Minute))
	ext.accumulateFromPower(f, c, t0)
	if got := chargerKwh(t, d, todayStr(), "c2"); got != 0 {
		t.Fatalf("dato viejo no acumula: %v", got)
	}
}

// TestPowerKwhByDaySplitsMidnight: un intervalo que cruza medianoche reparte
// la energía entre los dos días.
func TestPowerKwhByDaySplitsMidnight(t *testing.T) {
	loc := time.Local
	base := time.Date(2026, 11, 17, 23, 58, 0, 0, loc)
	pts := []PowerPoint{
		{At: base, Kw: 1},
		{At: base.Add(4 * time.Minute), Kw: 1},
	}
	got := PowerKwhByDay(pts)
	want := round3(1 * (2 * time.Minute).Hours())
	if math.Abs(got["2026-11-17"]-want) > 1e-9 {
		t.Fatalf("día 17 = %v (esperado %v)", got["2026-11-17"], want)
	}
	if math.Abs(got["2026-11-18"]-want) > 1e-9 {
		t.Fatalf("día 18 = %v (esperado %v)", got["2026-11-18"], want)
	}
}

// TestPowerKwhByDayHoldsStepAcrossSparseSamples: la potencia constante genera
// pocas filas en el recorder (sin cambio de estado), y ese tramo SÍ es energía.
func TestPowerKwhByDayHoldsStepAcrossSparseSamples(t *testing.T) {
	loc := time.Local
	base := time.Date(2026, 11, 17, 10, 0, 0, 0, loc)
	pts := []PowerPoint{
		{At: base, Kw: 2},
		{At: base.Add(30 * time.Minute), Kw: 2},
	}
	got := PowerKwhByDay(pts)["2026-11-17"]
	if math.Abs(got-1) > 1e-9 {
		t.Fatalf("kwh = %v (esperado 1.0: 2 kW durante 30 min)", got)
	}
}

// TestPowerKwhByDaySkipsRecorderOutages: un corte largo del recorder no se
// rellena con energía inventada.
func TestPowerKwhByDaySkipsRecorderOutages(t *testing.T) {
	loc := time.Local
	base := time.Date(2026, 11, 17, 10, 0, 0, 0, loc)
	pts := []PowerPoint{
		{At: base, Kw: 2},
		{At: base.Add(4 * time.Hour), Kw: 2},
	}
	if got := PowerKwhByDay(pts); len(got) != 0 {
		t.Fatalf("un corte de 4 h no debe sumar: %v", got)
	}
}

// TestBackfillChargerPowerFillsMissingDays: los días pasados sin fila se
// rellenan integrando el historial de potencia del recorder.
func TestBackfillChargerPowerFillsMissingDays(t *testing.T) {
	c := Charger{Key: "c2", Name: "Portátil", PowerID: "sensor.p", PowerUnit: UnitW}
	ext, d, f := powerExt(t, c)

	today := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	for _, date := range []string{"2026-11-16", "2026-11-17"} {
		if err := d.UpsertDaily(db.DailyRow{Date: date}); err != nil {
			t.Fatalf("daily: %v", err)
		}
	}
	// Carga real de 1 h a 2 kW el 17: puntos cada minuto.
	var rows []ha.HistoryRow
	start := time.Date(2026, 11, 17, 10, 0, 0, 0, time.Local)
	for i := 0; i <= 60; i++ {
		rows = append(rows, ha.HistoryRow{
			State:       "2000",
			LastChanged: start.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
		})
	}
	f.history["sensor.p"] = rows

	n := ext.BackfillChargerPower(f, c, today)
	if n != 1 {
		t.Fatalf("días escritos = %d (esperado 1, el 16 no tiene muestras)", n)
	}
	got := chargerKwh(t, d, "2026-11-17", "c2")
	if math.Abs(got-2.0) > 1e-6 {
		t.Fatalf("kwh 17 = %v (esperado 2.0)", got)
	}
	if v := chargerKwh(t, d, "2026-11-16", "c2"); v != 0 {
		t.Fatalf("el día sin muestras no se escribe: %v", v)
	}
}

// TestAccumulateCounterWinsOverPower: con contador de energía no se usa la
// potencia, así que no hay doble contabilidad.
func TestAccumulateCounterWinsOverPower(t *testing.T) {
	c := Charger{Key: "main", PowerID: "sensor.p", PowerUnit: UnitW, EnergyTotalID: "sensor.e"}
	ext, d, f := powerExt(t, c)

	now := time.Now()
	setPower(f, "sensor.p", "3000", now)
	ext.accumulateOne(f, c)
	ext.accumulateOne(f, c)
	if raw, _ := d.KVGet(powerCounterKey("main")); raw != "" {
		t.Fatalf("con contador no se debe usar la base de potencia: %q", raw)
	}
}

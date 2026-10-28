package extensions

import (
	"math"
	"testing"
	"time"

	"helios-server/internal/ha"
	"helios-server/internal/install"
)

func mkPtr(v float64) *float64 { return &v }

// realHouseFA reproduce el caso del 28-Oct-2026 a las 16:03: las pinzas suman
// 407 W mientras el balance da 3115 W (producción 2,9 kW + red 0,6 kW - carga de
// batería 0,385 kW).
func realHouseFA() *fakeHA {
	f := newFake()
	f.set("sensor.solis_potencia_actual", "2")
	f.set("sensor.almacen_pinza_power_b", "900") // W -> 0,9 kW
	f.set("sensor.solis_bateria_potencia", "0.385")
	f.set("sensor.solis_bateria_estado", "Cargando")
	f.set("sensor.medidor_respaldo_power", "394")
	f.set("sensor.vivienda_medidor_power", "0")
	f.set("sensor.almacen_pinza_power_a", "12.8")
	f.setAttrs("sensor.solis_scraper", map[string]any{
		"currentGridPower": 0.6,
		"gridDirection":    "import",
	})
	f.set("sun.sun", "above_horizon")
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(30)})
	return f
}

// El reparto sale del balance. Con las pinzas (0,407 kW) el resto de casa parece
// vacío y todo el coche quedaría como solar.
func TestPVShareNowUsesBalanceInsteadOfClamps(t *testing.T) {
	f := realHouseFA()
	power := 2.0
	charger := Charger{Key: "main", ChargerInHouseMeters: false}

	share, ok := PVShareNow(f, install.NewStore(), &power, charger)
	if !ok {
		t.Fatal("con el balance en pie el reparto debe estar disponible")
	}
	// resto = 3,115 - 2 = 1,115 kW y solar al cargador = min(2, 2,9 - 1,115)
	want := 1.785 / 2.0
	if math.Abs(share-want) > 1e-9 {
		t.Fatalf("share = %v (esperado %v del balance, no 1,0 de las pinzas)", share, want)
	}
}

// Una fuente de red configurada y caída deja el reparto sin base. No se puede
// devolver un porcentaje calculado sobre las pinzas.
func TestPVShareNowUnavailableWhenGridSourceIsDown(t *testing.T) {
	f := newFake()
	f.set("sensor.solis_potencia_actual", "2")
	f.set("sensor.almacen_pinza_power_b", "900")
	f.set("sensor.solis_bateria_potencia", "0")
	f.set("sensor.solis_bateria_estado", "idle")
	f.set("sensor.medidor_respaldo_power", "394")
	f.set("sensor.vivienda_medidor_power", "0")
	f.set("sensor.almacen_pinza_power_a", "12.8")
	f.set("sensor.solis_scraper", "unavailable")
	f.set("sun.sun", "above_horizon")
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(30)})

	power := 2.0
	share, ok := PVShareNow(f, install.NewStore(), &power, Charger{Key: "main"})
	if ok {
		t.Fatalf("sin balance el reparto debe ser no disponible, devolvió %v", share)
	}
	if share != 0 {
		t.Fatalf("share = %v (debe ser 0 cuando no está disponible)", share)
	}
}

// La batería cubriendo parte de la carga entra en el balance como generación.
func TestPVShareNowBatteryCoveringLoad(t *testing.T) {
	f := newFake()
	f.set("sensor.solis_potencia_actual", "1")
	f.set("sensor.almacen_pinza_power_b", "0")
	f.set("sensor.solis_bateria_potencia", "0.5")
	f.set("sensor.solis_bateria_estado", "Descargando")
	f.set("sensor.medidor_respaldo_power", "0")
	f.set("sensor.vivienda_medidor_power", "0")
	f.set("sensor.almacen_pinza_power_a", "0")
	f.setAttrs("sensor.solis_scraper", map[string]any{
		"currentGridPower": 0.0,
		"gridDirection":    "none",
	})
	f.set("sun.sun", "above_horizon")
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(30)})

	power := 1.0
	// balance = 1 + 0 - (-0,5) = 1,5 kW, resto = 0,5 y solar = min(1, 1-0,5)
	share, ok := PVShareNow(f, install.NewStore(), &power, Charger{Key: "main"})
	if !ok {
		t.Fatal("el reparto debe estar disponible")
	}
	if math.Abs(share-0.5) > 1e-9 {
		t.Fatalf("share = %v (esperado 0,5)", share)
	}
}

// Cargando batería de noche el balance puede salir negativo. Nunca negativo y
// nunca solar para el coche.
func TestPVShareNowClampsNegativeHouseToZero(t *testing.T) {
	f := newFake()
	f.set("sensor.solis_potencia_actual", "0")
	f.set("sensor.almacen_pinza_power_b", "0")
	f.set("sensor.solis_bateria_potencia", "1")
	f.set("sensor.solis_bateria_estado", "Cargando")
	f.set("sensor.medidor_respaldo_power", "0")
	f.set("sensor.vivienda_medidor_power", "0")
	f.set("sensor.almacen_pinza_power_a", "0")
	f.setAttrs("sensor.solis_scraper", map[string]any{
		"currentGridPower": 0.0,
		"gridDirection":    "none",
	})
	f.set("sun.sun", "below_horizon")
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(-10)})

	power := 1.0
	share, ok := PVShareNow(f, install.NewStore(), &power, Charger{Key: "main"})
	if !ok {
		t.Fatal("el reparto debe estar disponible")
	}
	if share != 0 {
		t.Fatalf("share = %v (esperado 0, el balance da -1 kW)", share)
	}
}

// Ni más FV que la energía del cargador ni negativa.
func TestPVShareNowBoundedByChargerEnergy(t *testing.T) {
	f := newFake()
	f.set("sensor.solis_potencia_actual", "10")
	f.set("sensor.almacen_pinza_power_b", "0")
	f.set("sensor.solis_bateria_potencia", "0")
	f.set("sensor.solis_bateria_estado", "idle")
	f.set("sensor.medidor_respaldo_power", "0")
	f.set("sensor.vivienda_medidor_power", "0")
	f.set("sensor.almacen_pinza_power_a", "0")
	f.setAttrs("sensor.solis_scraper", map[string]any{
		"currentGridPower": 0.0,
		"gridDirection":    "none",
	})
	f.set("sun.sun", "above_horizon")
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(30)})

	// El cargador consume 3 kW y sobra producción: como mucho, toda su energía.
	power := 3.0
	share, ok := PVShareNow(f, install.NewStore(), &power, Charger{Key: "main"})
	if !ok {
		t.Fatal("el reparto debe estar disponible")
	}
	if share != 1 {
		t.Fatalf("share = %v (esperado 1)", share)
	}

	// Y al revés, el cargador tira más que la producción: solo lo que hay.
	power = 20.0
	share, _ = PVShareNow(f, install.NewStore(), &power, Charger{Key: "main"})
	if math.Abs(share-0.5) > 1e-9 {
		t.Fatalf("share = %v (esperado 0,5: 10 kW de 20)", share)
	}
}

// Cargador sin energía: se mantiene la fracción de autoconsumo sobre el balance.
func TestPVShareNowWithoutChargerEnergy(t *testing.T) {
	f := realHouseFA()
	// casa 3,115 kW y producción 2,9 kW -> 93,1 % de la casa cubierta por FV
	share, ok := PVShareNow(f, install.NewStore(), nil, Charger{Key: "main"})
	if !ok {
		t.Fatal("el reparto debe estar disponible")
	}
	want := 2.9 / 3.115
	if math.Abs(share-want) > 1e-9 {
		t.Fatalf("share = %v (esperado %v)", share, want)
	}
}

// Sin fuente de red configurada no hay balance y se mantiene el respaldo de las
// pinzas, igual que hace el KPI de consumo en vivo.
func TestPVShareNowMetersFallbackWithoutGridSource(t *testing.T) {
	store := install.NewStore()
	topo := install.LegacyTopology
	topo.Grid = install.GridConfig{}
	store.Set(topo)

	f := newFake()
	f.set("sensor.solis_potencia_actual", "3")
	f.set("sensor.almacen_pinza_power_b", "0")
	f.set("sensor.solis_bateria_potencia", "0")
	f.set("sensor.solis_bateria_estado", "idle")
	f.set("sensor.medidor_respaldo_power", "500")
	f.set("sensor.vivienda_medidor_power", "300")
	f.set("sensor.almacen_pinza_power_a", "0")
	f.set("sun.sun", "above_horizon")
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(30)})

	power := 1.0
	// resto = 0,8 kW de pinzas, solar = min(1, 3-0,8) -> todo el cargador
	share, ok := PVShareNow(f, store, &power, Charger{Key: "main", ChargerInHouseMeters: false})
	if !ok {
		t.Fatal("sin fuente de red el respaldo de pinzas sigue disponible")
	}
	if share != 1 {
		t.Fatalf("share = %v (esperado 1)", share)
	}
}

// Sin fuente de red, el flag del cargador sigue diciendo si está detrás de las
// pinzas: si lo está, se descuenta del resto.
func TestPVShareNowMetersFallbackHonoursChargerInHouseMeters(t *testing.T) {
	store := install.NewStore()
	topo := install.LegacyTopology
	topo.Grid = install.GridConfig{}
	store.Set(topo)

	f := newFake()
	f.set("sensor.solis_potencia_actual", "2")
	f.set("sensor.almacen_pinza_power_b", "0")
	f.set("sensor.solis_bateria_potencia", "0")
	f.set("sensor.solis_bateria_estado", "idle")
	f.set("sensor.medidor_respaldo_power", "2500")
	f.set("sensor.vivienda_medidor_power", "0")
	f.set("sensor.almacen_pinza_power_a", "0")
	f.set("sun.sun", "above_horizon")
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(30)})

	power := 2.0
	// dentro de las pinzas: resto = 2,5 - 2 = 0,5 y solar = min(2, 1,5)
	share, _ := PVShareNow(f, store, &power, Charger{Key: "main", ChargerInHouseMeters: true})
	if math.Abs(share-0.75) > 1e-9 {
		t.Fatalf("share = %v (esperado 0,75)", share)
	}
	// fuera de las pinzas: resto = 2,5 y no queda excedente
	share, _ = PVShareNow(f, store, &power, Charger{Key: "main", ChargerInHouseMeters: false})
	if share != 0 {
		t.Fatalf("share = %v (esperado 0)", share)
	}
}

// El acumulador no inventa FV cuando no hay balance: guarda la energía y deja el
// reparto sin dato.
func TestAccumulateOneLeavesPVUnknownWithoutBalance(t *testing.T) {
	d := testDB(t)
	e := New(NewStore(), d, install.NewStore())

	f := realHouseFA()
	f.set("sensor.solis_scraper", "unavailable")
	f.set("sensor.cargador_energia_total", "12")
	f.set("sensor.cargador_potencia", "2000")

	c := Charger{
		Key: "main", PowerID: "sensor.cargador_potencia", PowerUnit: UnitW,
		EnergyTotalID: "sensor.cargador_energia_total", EnergyDivisor: 1,
	}
	e.writeCounterBase(c.Key, 10)
	e.accumulateOne(f, c)

	rows, err := d.ChargerHistoryRange(todayStr(), todayStr(), c.Key)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %d, err = %v", len(rows), err)
	}
	if rows[0].Kwh == nil || *rows[0].Kwh != 2 {
		t.Fatalf("kwh = %v (esperado 2, la energía se guarda igual)", rows[0].Kwh)
	}
	if rows[0].PvKwh != nil {
		t.Fatalf("pv = %v (sin balance el reparto queda sin dato)", *rows[0].PvKwh)
	}
}

// Con balance, el acumulador guarda el reparto calculado sobre él.
func TestAccumulateOneUsesBalanceForPV(t *testing.T) {
	d := testDB(t)
	e := New(NewStore(), d, install.NewStore())

	f := realHouseFA()
	f.set("sensor.cargador_energia_total", "12")
	f.set("sensor.cargador_potencia", "2000")

	c := Charger{
		Key: "main", PowerID: "sensor.cargador_potencia", PowerUnit: UnitW,
		EnergyTotalID: "sensor.cargador_energia_total", EnergyDivisor: 1,
	}
	e.writeCounterBase(c.Key, 10)
	e.accumulateOne(f, c)

	rows, err := d.ChargerHistoryRange(todayStr(), todayStr(), c.Key)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %d, err = %v", len(rows), err)
	}
	if rows[0].Kwh == nil || *rows[0].Kwh != 2 {
		t.Fatalf("kwh = %v (esperado 2)", rows[0].Kwh)
	}
	// 2 kWh * 0,8925 = 1,785 kWh de FV
	if rows[0].PvKwh == nil || math.Abs(*rows[0].PvKwh-1.785) > 1e-9 {
		t.Fatalf("pv = %v (esperado 1,785 del balance)", rows[0].PvKwh)
	}
}

// seriesStats construye los buckets de 5 minutos de una tarde de producción
// 2,9 kW con las pinzas a 0,407 kW y un contador de consumo dado.
func seriesStats(base time.Time, buckets int, counterStep *float64) ha.Stats {
	stats := ha.Stats{}
	for i := -1; i < buckets; i++ {
		ts := ha.FlexTime{Time: base.Add(time.Duration(i) * 5 * time.Minute)}
		stats["sensor.solis_potencia_actual"] = append(stats["sensor.solis_potencia_actual"], ha.StatsRow{Start: ts, Mean: mkPtr(2)})
		stats["sensor.almacen_pinza_power_b"] = append(stats["sensor.almacen_pinza_power_b"], ha.StatsRow{Start: ts, Mean: mkPtr(900)})
		stats["sensor.medidor_respaldo_power"] = append(stats["sensor.medidor_respaldo_power"], ha.StatsRow{Start: ts, Mean: mkPtr(394)})
		stats["sensor.vivienda_medidor_power"] = append(stats["sensor.vivienda_medidor_power"], ha.StatsRow{Start: ts, Mean: mkPtr(0)})
		stats["sensor.almacen_pinza_power_a"] = append(stats["sensor.almacen_pinza_power_a"], ha.StatsRow{Start: ts, Mean: mkPtr(12.8)})
		if counterStep != nil {
			// kWh acumulados del contador de consumo (3,115 kW por bucket).
			v := 10 + float64(i+1)*(*counterStep)
			stats["sensor.consumo_total_diario"] = append(stats["sensor.consumo_total_diario"], ha.StatsRow{Start: ts, Sum: mkPtr(v)})
		}
	}
	return stats
}

func chargerCurvesFixture(t *testing.T, counterStep *float64, buckets int) (*Ext, *fakeHA, time.Time) {
	t.Helper()
	d := testDB(t)
	e := New(NewStore(), d, install.NewStore())

	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.Local)
	f := newFake()
	f.stats = seriesStats(base, buckets, counterStep)
	f.history["sensor.cargador_potencia"] = []ha.HistoryRow{
		{State: "2000", LastChanged: base.Add(-30 * time.Minute).Format(time.RFC3339)},
	}
	return e, f, base
}

// El reparto consolidado sale del contador de consumo del balance y nunca de las
// pinzas.
func TestChargerPVFromCurvesUsesBalanceCounter(t *testing.T) {
	// 3,115 kW por bucket de 5 minutos = 0,2595833 kWh
	step := 3.115 * 5.0 / 60.0
	e, f, base := chargerCurvesFixture(t, &step, 4)

	c := Charger{Key: "main", PowerID: "sensor.cargador_potencia", PowerUnit: UnitW, ChargerInHouseMeters: false}
	got := e.ChargerPVFromCurves(f, base.Format(time.RFC3339), base.Add(20*time.Minute).Format(time.RFC3339), c)

	// 4 buckets x min(2, 2,9 - (3,115 - 2)) x 5/60 = 0,595 kWh
	const want = 0.595
	if v := got["2026-10-27"]; math.Abs(v-want) > 0.002 {
		t.Fatalf("pv = %v (esperado %v del balance, no 0,667 de las pinzas)", v, want)
	}
}

// Sin el contador del balance no hay base y el reparto no se inventa.
func TestChargerPVFromCurvesUnavailableWithoutBalance(t *testing.T) {
	e, f, base := chargerCurvesFixture(t, nil, 4)

	c := Charger{Key: "main", PowerID: "sensor.cargador_potencia", PowerUnit: UnitW}
	got := e.ChargerPVFromCurves(f, base.Format(time.RFC3339), base.Add(20*time.Minute).Format(time.RFC3339), c)

	if len(got) != 0 {
		t.Fatalf("pv = %v (sin balance el reparto debe quedar sin dato)", got)
	}
}

// Un contador que retrocede (reset) no aporta delta y ese bucket se descarta.
func TestChargerPVFromCurvesSkipsCounterReset(t *testing.T) {
	step := 3.115 * 5.0 / 60.0
	e, f, base := chargerCurvesFixture(t, &step, 4)

	// El bucket de las 12:10 vuelve a 5 kWh: reset del contador.
	for i := range f.stats["sensor.consumo_total_diario"] {
		if f.stats["sensor.consumo_total_diario"][i].Start.Time.Equal(base.Add(10 * time.Minute)) {
			f.stats["sensor.consumo_total_diario"][i].Sum = mkPtr(5)
		}
		if f.stats["sensor.consumo_total_diario"][i].Start.Time.Equal(base.Add(15 * time.Minute)) {
			f.stats["sensor.consumo_total_diario"][i].Sum = mkPtr(5 + step)
		}
	}

	c := Charger{Key: "main", PowerID: "sensor.cargador_potencia", PowerUnit: UnitW}
	got := e.ChargerPVFromCurves(f, base.Format(time.RFC3339), base.Add(20*time.Minute).Format(time.RFC3339), c)

	// Se descarta el bucket del reset: 3 buckets x 0,14875 = 0,44625 kWh
	const want = 0.44625
	if v := got["2026-10-27"]; math.Abs(v-want) > 0.002 {
		t.Fatalf("pv = %v (esperado %v sin el bucket del reset)", v, want)
	}
}

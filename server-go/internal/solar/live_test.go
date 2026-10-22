package solar

import (
	"testing"

	"helios-server/internal/ha"
	"helios-server/internal/install"
)

type fakeHA struct {
	states    map[string]ha.EntityState
	connected bool
	stats     ha.Stats
	statsErr  error
}

func newFake() *fakeHA {
	return &fakeHA{states: map[string]ha.EntityState{}, connected: true}
}

func (f *fakeHA) set(id, state string) {
	f.states[id] = ha.EntityState{State: state, Attributes: map[string]any{}}
}

func (f *fakeHA) setAttrs(id string, attrs map[string]any) {
	st := f.states[id]
	st.Attributes = attrs
	f.states[id] = st
}

func (f *fakeHA) GetState(id string) (ha.EntityState, bool) {
	st, ok := f.states[id]
	return st, ok
}

func (f *fakeHA) IsConnected() bool { return f.connected }

func (f *fakeHA) StatisticsDuringPeriod(start, end string, ids []string, period string, types []string) (ha.Stats, error) {
	return f.stats, f.statsErr
}

func newSolar() *Solar {
	store := install.NewStore()
	return New(store, nil)
}

func TestComputeLiveNominal(t *testing.T) {
	f := newFake()
	f.set("sensor.solis_potencia_actual", "2.5")
	f.set("sensor.almacen_pinza_power_b", "1200") // W -> 1.2 kW
	f.set("sensor.solis_bateria_potencia", "0.5")
	f.set("sensor.solis_bateria_estado", "Cargando")
	f.set("sensor.solis_bateria_soc", "75")
	f.setAttrs("sensor.solis_scraper", map[string]any{
		"currentGridPower": 0.4,
		"gridDirection":    "import",
		"inverterOnline":   float64(1),
		"stationName":      "Casa",
	})
	f.set("sun.sun", "above_horizon")
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(35)})
	f.set("weather.forecast_casa", "sunny")
	f.set("sensor.sensor_temp_ext_temperature", "18.5")
	f.set("sensor.medidor_respaldo_power", "200")
	f.set("sensor.vivienda_medidor_power", "300")
	f.set("sensor.almacen_pinza_power_a", "100")

	s := newSolar()
	live := s.ComputeLive(f)

	if live.Production != 3.7 {
		t.Fatalf("production = %v (esperado 3.7)", live.Production)
	}
	if live.Solis != 2.5 || live.Fox != 1.2 {
		t.Fatalf("solis/fox = %v/%v", live.Solis, live.Fox)
	}
	if live.Consumption == nil || *live.Consumption != 0.6 {
		t.Fatalf("consumption = %v (esperado 0.6)", live.Consumption)
	}
	if live.BatteryPower != 0.5 {
		t.Fatalf("batteryPower = %v (esperado 0.5 cargando)", live.BatteryPower)
	}
	if live.Grid == nil || *live.Grid != 0.4 {
		t.Fatalf("grid = %v (esperado 0.4 import)", live.Grid)
	}
	if live.Soc != 75 {
		t.Fatalf("soc = %v", live.Soc)
	}
	if live.Station != "Casa" || !live.InverterOnline {
		t.Fatalf("station/online = %q/%v", live.Station, live.InverterOnline)
	}
	if len(live.Inverters) != 2 {
		t.Fatalf("inverters = %d", len(live.Inverters))
	}
	if len(live.Alerts) != 0 {
		t.Fatalf("alerts inesperadas: %+v", live.Alerts)
	}
}

func TestComputeLiveDischargingBattery(t *testing.T) {
	f := newFake()
	f.set("sensor.solis_potencia_actual", "0")
	f.set("sensor.almacen_pinza_power_b", "0")
	f.set("sensor.solis_bateria_potencia", "0.8")
	f.set("sensor.solis_bateria_estado", "Descargando")
	f.set("sensor.solis_bateria_soc", "15")
	f.setAttrs("sensor.solis_scraper", map[string]any{"currentGridPower": 0.0, "gridDirection": "none"})
	f.set("sun.sun", "below_horizon")
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(-10)})

	s := newSolar()
	live := s.ComputeLive(f)

	if live.BatteryPower != -0.8 {
		t.Fatalf("batteryPower = %v (esperado -0.8 descargando)", live.BatteryPower)
	}
	// alerta de batería en reserva
	found := false
	for _, a := range live.Alerts {
		if a.ID == "bateria" {
			found = true
		}
	}
	if !found {
		t.Fatalf("falta alerta de batería en reserva: %+v", live.Alerts)
	}
}

func TestComputeLiveConsumptionDown(t *testing.T) {
	f := newFake()
	f.set("sensor.solis_potencia_actual", "2")
	f.set("sensor.almacen_pinza_power_b", "0")
	f.set("sensor.solis_bateria_potencia", "0")
	f.set("sensor.solis_bateria_estado", "idle")
	f.set("sensor.solis_bateria_soc", "50")
	f.set("sensor.medidor_respaldo_power", "unavailable")
	f.set("sensor.vivienda_medidor_power", "unknown")
	f.set("sensor.almacen_pinza_power_a", "0")
	f.setAttrs("sensor.solis_scraper", map[string]any{"currentGridPower": 0.1, "gridDirection": "import"})
	f.set("sun.sun", "above_horizon")
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(40)})

	s := newSolar()
	live := s.ComputeLive(f)

	if live.Consumption != nil {
		t.Fatalf("consumption debería ser null con medidores caídos, es %v", *live.Consumption)
	}
	found := false
	for _, a := range live.Alerts {
		if a.ID == "consumo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("falta alerta de consumo: %+v", live.Alerts)
	}
}

func TestComputeLiveGridDown(t *testing.T) {
	f := newFake()
	f.set("sensor.solis_potencia_actual", "2")
	f.set("sensor.almacen_pinza_power_b", "0")
	f.set("sensor.solis_bateria_potencia", "0")
	f.set("sensor.solis_bateria_estado", "idle")
	f.set("sensor.solis_bateria_soc", "50")
	f.set("sensor.medidor_respaldo_power", "100")
	f.set("sensor.vivienda_medidor_power", "100")
	f.set("sensor.almacen_pinza_power_a", "0")
	f.set("sensor.solis_scraper", "unavailable")
	f.set("sun.sun", "above_horizon")
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(40)})

	s := newSolar()
	live := s.ComputeLive(f)

	if live.Grid != nil {
		t.Fatalf("grid debería ser null con fuente caída, es %v", *live.Grid)
	}
}

func TestComputeLiveNotConnected(t *testing.T) {
	f := newFake()
	f.connected = false

	s := newSolar()
	live := s.ComputeLive(f)

	found := false
	for _, a := range live.Alerts {
		if a.ID == "haos" {
			found = true
		}
	}
	if !found {
		t.Fatalf("falta alerta haos: %+v", live.Alerts)
	}
}

func TestComputeTodayKpis(t *testing.T) {
	f := newFake()
	f.set("sensor.solis_energia_hoy", "12.5")
	f.set("sensor.energia_fox_diaria", "6.25")
	f.set("sensor.energia_red_importada_solis", "3.0")
	f.set("sensor.energia_red_exportada_solis", "5.0")
	f.set("sensor.energia_bateria_carga_diaria", "2.0")
	f.set("sensor.energia_bateria_descarga_diaria", "1.5")
	f.set("sensor.consumo_total_diario", "10.0")

	s := newSolar()
	k := s.ComputeTodayKpis(f, nil)

	if k.ProductionKwh != 18.75 {
		t.Fatalf("productionKwh = %v (esperado 18.75)", k.ProductionKwh)
	}
	if k.SolisKwh != 12.5 || k.FoxKwh != 6.25 {
		t.Fatalf("solis/fox = %v/%v", k.SolisKwh, k.FoxKwh)
	}
	if k.GridImportKwh != 3 || k.GridExportKwh != 5 {
		t.Fatalf("grid = %v/%v", k.GridImportKwh, k.GridExportKwh)
	}
	if k.BatteryChargedKwh != 2 || k.BatteryDischargedKwh != 1.5 {
		t.Fatalf("battery = %v/%v", k.BatteryChargedKwh, k.BatteryDischargedKwh)
	}
	// derived = 18.75 + 3 - 5 - 2 + 1.5 = 16.25 ; medidores=0 -> g(consumption)=10
	if k.ConsumptionKwh != 16.25 {
		t.Fatalf("consumptionKwh = %v (esperado 16.25 derivado)", k.ConsumptionKwh)
	}
}

func TestComputeTodayKpisConsumptionOverrideWins(t *testing.T) {
	f := newFake()
	f.set("sensor.solis_energia_hoy", "1")
	f.set("sensor.energia_fox_diaria", "0")
	f.set("sensor.energia_red_importada_solis", "0")
	f.set("sensor.energia_red_exportada_solis", "0")
	f.set("sensor.energia_bateria_carga_diaria", "0")
	f.set("sensor.energia_bateria_descarga_diaria", "0")
	f.set("sensor.consumo_total_diario", "0.5")

	s := newSolar()
	override := 4.2
	k := s.ComputeTodayKpis(f, &override)

	if k.ConsumptionKwh != 4.2 {
		t.Fatalf("consumptionKwh = %v (esperado 4.2, medidores mandan)", k.ConsumptionKwh)
	}
}

func TestComputeTodayKpisCapsAt100(t *testing.T) {
	f := newFake()
	f.set("sensor.solis_energia_hoy", "50")
	f.set("sensor.energia_fox_diaria", "50")
	f.set("sensor.energia_red_importada_solis", "80")
	f.set("sensor.energia_red_exportada_solis", "0")
	f.set("sensor.energia_bateria_carga_diaria", "0")
	f.set("sensor.energia_bateria_descarga_diaria", "0")
	f.set("sensor.consumo_total_diario", "0")

	s := newSolar()
	k := s.ComputeTodayKpis(f, nil)

	if k.ConsumptionKwh != 100 {
		t.Fatalf("consumptionKwh = %v (debería tope 100)", k.ConsumptionKwh)
	}
}

func TestConsumptionTodayFromCounters(t *testing.T) {
	f := newFake()
	f.set("sensor.medidor_respaldo_energy", "150")
	f.set("sensor.vivienda_medidor_energy", "260")
	f.set("sensor.almacen_pinza_energy_a", "90")

	s := newSolar()
	base := &ConsumptionBaseline{
		Date: TodayStr(),
		Values: map[string]*float64{
			"sensor.medidor_respaldo_energy": floatPtr(100),
			"sensor.vivienda_medidor_energy": floatPtr(200),
			"sensor.almacen_pinza_energy_a":  floatPtr(40),
		},
	}

	got := s.ConsumptionTodayFromCounters(f, base)
	if got == nil {
		t.Fatal("esperaba valor, got nil")
	}
	// (150-100) + (260-200) + (90-40) = 50 + 60 + 50 = 160
	if *got != 160 {
		t.Fatalf("consumo = %v (esperado 160)", *got)
	}
}

func TestConsumptionTodayFromCountersNoBaseline(t *testing.T) {
	f := newFake()
	f.set("sensor.medidor_respaldo_energy", "150")

	s := newSolar()
	if got := s.ConsumptionTodayFromCounters(f, nil); got != nil {
		t.Fatalf("sin baseline debería ser nil, es %v", *got)
	}
}

func TestConsumptionTodayFromCountersCounterReset(t *testing.T) {
	f := newFake()
	f.set("sensor.medidor_respaldo_energy", "10") // contador reseteado

	s := newSolar()
	base := &ConsumptionBaseline{
		Date:   TodayStr(),
		Values: map[string]*float64{"sensor.medidor_respaldo_energy": floatPtr(100)},
	}

	if got := s.ConsumptionTodayFromCounters(f, base); got != nil {
		t.Fatalf("con reset no debería sumar (nil), es %v", *got)
	}
}

func TestNumParsesStrings(t *testing.T) {
	cases := map[any]float64{
		"12.5":   12.5,
		" 3.0 ":  3.0,
		"abc":    0,
		nil:      0,
		2.5:      2.5,
	}
	for in, want := range cases {
		if got := Num(in); got != want {
			t.Fatalf("Num(%v) = %v (esperado %v)", in, got, want)
		}
	}
}

func TestRoundHelpers(t *testing.T) {
	if Round3(1.23456) != 1.235 {
		t.Fatalf("Round3 = %v", Round3(1.23456))
	}
	if Round2(1.005) != 1.01 && Round2(1.005) != 1.0 {
		t.Fatalf("Round2 inesperado: %v", Round2(1.005))
	}
	if Round1(19.96) != 20 {
		t.Fatalf("Round1 = %v", Round1(19.96))
	}
}

func floatPtr(f float64) *float64 { return &f }

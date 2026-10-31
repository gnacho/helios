package extensions

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"path/filepath"
	"testing"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/ha"
	"helios-server/internal/install"
)

func pngEncodeTiny(buf *bytes.Buffer) error {
	return png.Encode(buf, image.NewRGBA(image.Rect(0, 0, 2, 2)))
}

type fakeHA struct {
	states  map[string]ha.EntityState
	history map[string][]ha.HistoryRow
	stats   ha.Stats
	calls   []string
}

func newFake() *fakeHA {
	return &fakeHA{states: map[string]ha.EntityState{}, history: map[string][]ha.HistoryRow{}}
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

func (f *fakeHA) IsConnected() bool { return true }

func (f *fakeHA) StatisticsDuringPeriod(start, end string, ids []string, period string, types []string) (ha.Stats, error) {
	if f.stats == nil {
		return ha.Stats{}, nil
	}
	return f.stats, nil
}

func (f *fakeHA) HistoryDuringPeriod(start, end, entityID string) ([]ha.HistoryRow, error) {
	return f.history[entityID], nil
}

func (f *fakeHA) CallService(domain, service string, data map[string]any) error {
	f.calls = append(f.calls, domain+"/"+service)
	return nil
}

func (f *fakeHA) called(s string) bool {
	for _, c := range f.calls {
		if c == s {
			return true
		}
	}
	return false
}

func testDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "ext.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestNormalizeChargerDefaults(t *testing.T) {
	// key ausente en índice 0 -> 'main'
	c0 := NormalizeCharger(map[string]any{}, GenericCharger, 0)
	if c0.Key != "main" {
		t.Fatalf("key[0] = %q", c0.Key)
	}
	// índice 1 -> c2
	c1 := NormalizeCharger(map[string]any{}, GenericCharger, 1)
	if c1.Key != "c2" {
		t.Fatalf("key[1] = %q", c1.Key)
	}
	// key inválida explícita -> también cae al fallback (no hereda 'main')
	c2 := NormalizeCharger(map[string]any{"key": "MAYÚSCULAS!!"}, GenericCharger, 0)
	if c2.Key != "main" {
		t.Fatalf("key inválida = %q", c2.Key)
	}
	if c2.Name != "Car charger" {
		t.Fatalf("name = %q", c2.Name)
	}
	if c2.PowerUnit != UnitKW {
		t.Fatalf("powerUnit = %q", c2.PowerUnit)
	}
	if c2.EnergyDivisor != 1 {
		t.Fatalf("energyDivisor = %d", c2.EnergyDivisor)
	}
	if !c2.ChargerInHouseMeters {
		t.Fatal("chargerInHouseMeters debería heredar true")
	}
}

func TestNormalizeChargerOverrides(t *testing.T) {
	c := NormalizeCharger(map[string]any{
		"key":                  "coche-2",
		"name":                 "  Coche 2  ",
		"powerUnit":            "W",
		"energyDivisor":        float64(100),
		"chargerInHouseMeters": false,
		"chargingStates":       []any{"charging", 42, "on"},
	}, GenericCharger, 1)

	if c.Key != "coche-2" {
		t.Fatalf("key = %q", c.Key)
	}
	if c.Name != "Coche 2" {
		t.Fatalf("name = %q", c.Name)
	}
	if c.PowerUnit != UnitW {
		t.Fatalf("powerUnit = %q", c.PowerUnit)
	}
	if c.EnergyDivisor != 100 {
		t.Fatalf("divisor = %d", c.EnergyDivisor)
	}
	if c.ChargerInHouseMeters {
		t.Fatal("chargerInHouseMeters debería ser false")
	}
	if len(c.ChargingStates) != 2 || c.ChargingStates[0] != "charging" || c.ChargingStates[1] != "on" {
		t.Fatalf("chargingStates = %v (debe filtrar no-strings)", c.ChargingStates)
	}
}

func TestNormalizeConfigMigratesLegacyFlatShape(t *testing.T) {
	raw := map[string]any{
		"enabled": true,
		"carCharger": map[string]any{
			"enabled":       true,
			"powerId":       "sensor.viejo",
			"name":          "Viejo",
			"energyDivisor": float64(10),
		},
	}
	c := NormalizeConfig(raw, GenericExtensions)
	if !c.Enabled || !c.CarCharger.Enabled {
		t.Fatal("enabled no propagado")
	}
	if len(c.CarCharger.Chargers) != 1 {
		t.Fatalf("chargers = %d (forma plana debe migrar a 1)", len(c.CarCharger.Chargers))
	}
	ch := c.CarCharger.Chargers[0]
	if ch.PowerID != "sensor.viejo" || ch.Name != "Viejo" || ch.EnergyDivisor != 10 {
		t.Fatalf("migración incompleta: %+v", ch)
	}
}

func TestChargerActiveAndEntities(t *testing.T) {
	off := GenericExtensions
	if ChargerActive(off) {
		t.Fatal("no debería estar activo (master off)")
	}
	if ids := ChargerEntities(off); len(ids) != 0 {
		t.Fatalf("entities = %v", ids)
	}

	on := GenericExtensions
	on.Enabled = true
	on.CarCharger.Enabled = true
	on.CarCharger.Chargers = []Charger{
		{Key: "a", PowerID: "sensor.p1", EnergyTotalID: "sensor.e1"},
		{Key: "b", PowerID: "sensor.p1", EnergyTotalID: "sensor.e2"},
	}
	if !ChargerActive(on) {
		t.Fatal("debería estar activo")
	}
	ids := ChargerEntities(on)
	// dedup: sensor.p1 aparece una sola vez
	count := 0
	for _, id := range ids {
		if id == "sensor.p1" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("sensor.p1 aparece %d veces", count)
	}
	if len(ids) != 3 {
		t.Fatalf("entities = %v (esperado p1, e1, e2)", ids)
	}
}

func TestComputeOneChargerLive(t *testing.T) {
	f := newFake()
	ch := Charger{
		Key: "main", Name: "Coche",
		PowerID: "sensor.pot", PowerUnit: UnitW,
		StateID: "sensor.estado", SwitchID: "switch.c",
		AmpID:         "number.a",
		EnergyTotalID: "sensor.total", EnergyDivisor: 100,
		EnergySessionID: "sensor.sesion",
		TempID:          "sensor.temp",
		ChargingStates:  []string{"charger_charging"},
		ConnectedStates: []string{"charger_insert"},
	}
	f.set("sensor.pot", "2605") // W -> 2.605 kW
	f.set("sensor.estado", "charger_charging")
	f.set("switch.c", "on")
	f.set("sensor.total", "469")  // /100 = 4.69
	f.set("sensor.sesion", "120") // /100 = 1.2
	f.set("sensor.temp", "32")
	f.set("number.a", "16")
	f.setAttrs("number.a", map[string]any{"options": []any{"6", "10", "16"}})

	live := ComputeOneChargerLive(f, ch)

	if !live.Charging || !live.Connected {
		t.Fatalf("charging/connected = %v/%v", live.Charging, live.Connected)
	}
	if live.PowerKw != 2.605 {
		t.Fatalf("powerKw = %v", live.PowerKw)
	}
	if live.TotalKwh == nil || *live.TotalKwh != 4.69 {
		t.Fatalf("totalKwh = %v", live.TotalKwh)
	}
	if live.SessionKwh == nil || *live.SessionKwh != 1.2 {
		t.Fatalf("sessionKwh = %v", live.SessionKwh)
	}
	if live.SwitchOn == nil || !*live.SwitchOn {
		t.Fatalf("switchOn = %v", live.SwitchOn)
	}
	if len(live.AmpOptions) != 3 || live.AmpOptions[2] != 16 {
		t.Fatalf("ampOptions = %v", live.AmpOptions)
	}
}

func TestComputeOneChargerLiveChargingByPower(t *testing.T) {
	f := newFake()
	ch := Charger{
		Key: "main", PowerID: "sensor.pot", PowerUnit: UnitKW,
		StateID:        "sensor.estado",
		ChargingStates: []string{"charging"},
	}
	f.set("sensor.pot", "3.5")
	f.set("sensor.estado", "idle")

	live := ComputeOneChargerLive(f, ch)
	if !live.Charging {
		t.Fatal("potencia > 0.05 kW debería marcar charging")
	}
}

func TestComputeOneChargerLiveUnavailable(t *testing.T) {
	f := newFake()
	ch := Charger{Key: "main", PowerID: "sensor.pot", StateID: "sensor.e"}
	f.set("sensor.pot", "unavailable")
	f.set("sensor.e", "unknown")

	live := ComputeOneChargerLive(f, ch)
	if live.PowerKw != 0 {
		t.Fatalf("powerKw = %v (esperado 0)", live.PowerKw)
	}
	if live.State != nil {
		t.Fatalf("state debería ser nil, es %v", *live.State)
	}
	if live.Charging || live.Connected {
		t.Fatal("unavailable no puede estar cargando")
	}
}

func TestPVShareNow(t *testing.T) {
	f := newFake()
	inst := install.NewStore()

	// Producción 5 kW, casa 1 kW (exportando 4), cargador 2 kW en circuito aparte.
	f.set("sensor.solis_potencia_actual", "3")
	f.set("sensor.almacen_pinza_power_b", "2000") // W
	f.set("sensor.medidor_respaldo_power", "500")
	f.set("sensor.vivienda_medidor_power", "500")
	f.set("sensor.almacen_pinza_power_a", "0")
	f.setAttrs("sensor.solis_scraper", map[string]any{
		"currentGridPower": 4.0,
		"gridDirection":    "export",
	})
	f.set("sun.sun", "above_horizon")
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(30)})

	power := 2.0
	charger := Charger{Key: "main", ChargerInHouseMeters: false}
	share, ok := PVShareNow(f, inst, &power, charger)
	if !ok {
		t.Fatal("el reparto debe estar disponible")
	}

	// balance = 5 + (-4) - 0 = 1 kW, solar al cargador = min(2, 5-1) = 2 -> share 1
	if share != 1 {
		t.Fatalf("share = %v (esperado 1)", share)
	}
}

func TestPVShareNowInHouseMeters(t *testing.T) {
	f := newFake()
	inst := install.NewStore()
	f.set("sensor.solis_potencia_actual", "3")
	f.set("sensor.almacen_pinza_power_b", "0")
	// El balance es de casa entera y siempre incluye el cargador: 3 kW.
	f.set("sensor.medidor_respaldo_power", "1500")
	f.set("sensor.vivienda_medidor_power", "1500")
	f.set("sensor.almacen_pinza_power_a", "0")
	f.setAttrs("sensor.solis_scraper", map[string]any{
		"currentGridPower": 0.0,
		"gridDirection":    "none",
	})
	f.set("sun.sun", "above_horizon")
	f.setAttrs("sun.sun", map[string]any{"elevation": float64(30)})

	power := 2.0
	charger := Charger{Key: "main", ChargerInHouseMeters: true}
	share, ok := PVShareNow(f, inst, &power, charger)
	if !ok {
		t.Fatal("el reparto debe estar disponible")
	}
	// production 3, other = 3-2 = 1, solarToCharger = min(2, 2) = 2 -> 1
	if share != 1 {
		t.Fatalf("share = %v (esperado 1)", share)
	}
}

func TestAddChargerKwhClampsPV(t *testing.T) {
	d := testDB(t)

	if err := d.AddChargerKwh("2026-10-21", "main", 2.0, 5.0); err != nil {
		t.Fatalf("add: %v", err)
	}
	rows, err := d.ChargerHistoryRange("2026-10-21", "2026-10-21", "main")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows: %v", err)
	}
	if rows[0].PvKwh == nil || *rows[0].PvKwh != 2.0 {
		t.Fatalf("pv = %v (debe clampear a kwh=2)", rows[0].PvKwh)
	}

	// segundo add suma
	if err := d.AddChargerKwh("2026-10-21", "main", 1.0, 0.5); err != nil {
		t.Fatalf("add2: %v", err)
	}
	rows, _ = d.ChargerHistoryRange("2026-10-21", "2026-10-21", "main")
	if rows[0].Kwh == nil || *rows[0].Kwh != 3.0 {
		t.Fatalf("kwh acumulado = %v (esperado 3)", rows[0].Kwh)
	}

}

func TestSetChargerKwhIfNull(t *testing.T) {
	d := testDB(t)

	pv := 1.0
	if err := d.SetChargerKwhIfNull("2026-10-21", "main", 4.0, &pv); err != nil {
		t.Fatalf("set: %v", err)
	}
	rows, _ := d.ChargerHistoryRange("2026-10-21", "2026-10-21", "main")
	if rows[0].Kwh == nil || *rows[0].Kwh != 4.0 {
		t.Fatalf("kwh = %v", rows[0].Kwh)
	}

	// segundo intento NO debe sobrescribir
	if err := d.SetChargerKwhIfNull("2026-10-21", "main", 9.0, nil); err != nil {
		t.Fatalf("set2: %v", err)
	}
	rows, _ = d.ChargerHistoryRange("2026-10-21", "2026-10-21", "main")
	if *rows[0].Kwh != 4.0 {
		t.Fatalf("kwh sobrescrito = %v (debe respetar el existente)", *rows[0].Kwh)
	}
}

func TestAccumulateChargerDaily(t *testing.T) {
	d := testDB(t)
	store := NewStore()
	cfg := GenericExtensions
	cfg.Enabled = true
	cfg.CarCharger.Enabled = true
	cfg.CarCharger.Chargers = []Charger{{
		Key: "main", PowerID: "sensor.pot", PowerUnit: UnitKW,
		EnergyTotalID: "sensor.total", EnergyDivisor: 100,
		ChargerInHouseMeters: false,
	}}
	store.Set(cfg)
	e := New(store, d, install.NewStore())

	f := newFake()
	f.set("sensor.pot", "0")
	f.set("sensor.solis_potencia_actual", "0")
	f.set("sensor.almacen_pinza_power_b", "0")
	f.set("sensor.medidor_respaldo_power", "0")
	f.set("sensor.vivienda_medidor_power", "0")
	f.set("sensor.almacen_pinza_power_a", "0")

	// Primer tick: solo siembra la base (469 -> 4.69 kWh), sin sumar.
	f.set("sensor.total", "469")
	e.AccumulateChargerDaily(f)
	today := todayStr()
	rows, _ := d.ChargerHistoryRange(today, today, "main")
	if len(rows) != 0 {
		t.Fatalf("primer tick no debería sumar, filas=%v", rows)
	}

	// Segundo tick: 480 -> delta 0.11 kWh.
	f.set("sensor.total", "480")
	e.AccumulateChargerDaily(f)
	rows, _ = d.ChargerHistoryRange(today, today, "main")
	if len(rows) != 1 || rows[0].Kwh == nil {
		t.Fatalf("debería haber 1 fila, hay %v", rows)
	}
	if *rows[0].Kwh < 0.1 || *rows[0].Kwh > 0.12 {
		t.Fatalf("kwh = %v (esperado ~0.11)", *rows[0].Kwh)
	}

	// Reset del contador: 300 (3.0 kWh < 4.80) -> nueva base, sin sumar.
	f.set("sensor.total", "300")
	e.AccumulateChargerDaily(f)
	rows, _ = d.ChargerHistoryRange(today, today, "main")
	if *rows[0].Kwh < 0.1 || *rows[0].Kwh > 0.12 {
		t.Fatalf("reset no debería sumar, kwh = %v", *rows[0].Kwh)
	}

	// Glitch: salto de 100 kWh -> ignorado (no suma).
	f.set("sensor.total", "13000") // 130 kWh
	e.AccumulateChargerDaily(f)
	rows, _ = d.ChargerHistoryRange(today, today, "main")
	if *rows[0].Kwh > 1 {
		t.Fatalf("glitch sumó: kwh = %v", *rows[0].Kwh)
	}
}

func TestDailyDeltasFromHistory(t *testing.T) {
	rows := []HistoryPoint{
		{State: "100", LastChanged: "2026-10-20T10:00:00Z"},
		{State: "110", LastChanged: "2026-10-20T12:00:00Z"}, // +10
		{State: "50", LastChanged: "2026-10-20T14:00:00Z"},  // reset: no resta
		{State: "60", LastChanged: "2026-10-20T16:00:00Z"},  // +10
		{State: "70", LastChanged: "2026-10-21T10:00:00Z"},  // nuevo día: base
		{State: "80", LastChanged: "2026-10-21T12:00:00Z"},  // +10
	}
	got := DailyDeltasFromHistory(rows, 1)

	// Ojo: la clave depende de la zona local; usamos la misma lógica.
	d20 := HistoryPoint{LastChanged: "2026-10-20T10:00:00Z"}.lastChangedDay()
	d21 := HistoryPoint{LastChanged: "2026-10-21T10:00:00Z"}.lastChangedDay()

	if got[d20] != 20 {
		t.Fatalf("%s = %v (esperado 20)", d20, got[d20])
	}
	if got[d21] != 10 {
		t.Fatalf("%s = %v (esperado 10)", d21, got[d21])
	}
}

func TestChargerHistoryEstimatesPV(t *testing.T) {
	d := testDB(t)
	store := NewStore()
	cfg := GenericExtensions
	cfg.Enabled = true
	cfg.CarCharger.Enabled = true
	cfg.CarCharger.Chargers = []Charger{{Key: "main", ChargerInHouseMeters: false}}
	store.Set(cfg)
	e := New(store, d, install.NewStore())

	if err := d.UpsertDaily(db.DailyRow{
		Date: "2026-10-20", ProductionKwh: floatPtr(20), ConsumptionKwh: floatPtr(10),
	}); err != nil {
		t.Fatalf("daily: %v", err)
	}
	// kwh sin pv_kwh -> se estima: min(5, max(0, 20 - 10)) = 5
	if err := d.AddChargerKwh("2026-10-20", "main", 5, 5); err != nil {
		t.Fatalf("add: %v", err)
	}

	rows, err := e.ChargerHistory("2026-10-20", "2026-10-20", "main")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0].PvKwh == nil {
		t.Fatal("pvKwh debería estimarse")
	}
}

func TestChargerDayCurveSteps(t *testing.T) {
	d := testDB(t)
	e := New(NewStore(), d, install.NewStore())

	f := newFake()
	day := "2026-10-20"
	base := time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local)
	f.history["sensor.pot"] = []ha.HistoryRow{
		{State: "0", LastChanged: base.Format(time.RFC3339)},
		{State: "3000", LastChanged: base.Add(10 * time.Minute).Format(time.RFC3339)},
		{State: "0", LastChanged: base.Add(30 * time.Minute).Format(time.RFC3339)},
	}

	ch := &Charger{Key: "main", PowerID: "sensor.pot", PowerUnit: UnitW}
	curve := e.ChargerDayCurve(f, day, ch)

	if len(curve.Points) != 1440/stepMin {
		t.Fatalf("points = %d (esperado 288 para un día completo)", len(curve.Points))
	}
	// minuto 0 -> 0 kW; minuto 10 -> 3 kW; minuto 30 -> 0
	for _, p := range curve.Points {
		switch p.T {
		case 0:
			if p.Kw != 0 {
				t.Fatalf("t=0 kw = %v", p.Kw)
			}
		case 10:
			if p.Kw != 3 {
				t.Fatalf("t=10 kw = %v (esperado 3)", p.Kw)
			}
		case 30:
			if p.Kw != 0 {
				t.Fatalf("t=30 kw = %v", p.Kw)
			}
		}
	}
}

func TestSaveAndClearChargerImage(t *testing.T) {
	d := testDB(t)
	store := NewStore()
	cfg := GenericExtensions
	cfg.Enabled = true
	cfg.CarCharger.Enabled = true
	cfg.CarCharger.Chargers = []Charger{{Key: "main", Name: "Coche"}}
	store.Set(cfg)
	// config en kv para que SetChargerImageURL pueda escribir
	if err := d.KVSet("extensions_config", `{"enabled":true,"carCharger":{"enabled":true,"chargers":[{"key":"main","name":"Coche"}]}}`); err != nil {
		t.Fatalf("kv: %v", err)
	}

	dataDir := t.TempDir()
	e := New(store, d, install.NewStore())

	var png bytes.Buffer
	if err := pngEncodeTiny(&png); err != nil {
		t.Fatalf("fixture png: %v", err)
	}
	ok, msg := e.SaveChargerImage(dataDir, "main", "data:image/png;base64,"+base64.StdEncoding.EncodeToString(png.Bytes()))
	if !ok {
		t.Fatalf("save falló: %s", msg)
	}
	path, mime := FindChargerImage(dataDir, "main")
	if path == "" || mime != "image/png" {
		t.Fatalf("find = %q / %q", path, mime)
	}

	e.ClearChargerImage(dataDir, "main")
	if p, _ := FindChargerImage(dataDir, "main"); p != "" {
		t.Fatalf("imagen no borrada: %q", p)
	}
}

func TestSaveChargerImageRejectsBadFormat(t *testing.T) {
	d := testDB(t)
	e := New(NewStore(), d, install.NewStore())
	ok, msg := e.SaveChargerImage(t.TempDir(), "main", "no-es-una-imagen")
	if ok || msg == "" {
		t.Fatalf("debería rechazar, ok=%v msg=%q", ok, msg)
	}
}

func floatPtr(f float64) *float64 { return &f }

// TestLiveChargerOmitsUndefined: el server Node omitía las claves undefined y
// el front distingue "sin dato" (undefined) de null (regresión 22-Oct-2026:
// switchOn null habilitaba el botón power).
func TestLiveChargerOmitsUndefined(t *testing.T) {
	f := newFake()
	ch := Charger{Key: "main", PowerID: "sensor.pot", PowerUnit: UnitKW, StateID: "sensor.e"}

	f.set("sensor.pot", "3.5")
	f.set("sensor.e", "unavailable")

	live := ComputeOneChargerLive(f, ch)
	raw, err := json.Marshal(live)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"state", "sessionKwh", "totalKwh", "tempC", "switchOn", "amps", "ampOptions"} {
		if _, ok := m[k]; ok {
			t.Fatalf("la clave %q debería omitirse (undefined en el original), payload: %s", k, raw)
		}
	}
	if !live.Charging {
		t.Fatal("potencia > 0.05 debería marcar charging")
	}
}

// Regression: delayId/durationId must survive the real config load path
// (kv extensions_config -> ResolveConfig -> NormalizeCharger). The whitelist
// in NormalizeCharger dropped them, so the portable charger never rendered
// its delay/duration chips even though the stored config had the entities.
func TestResolveConfigKeepsDelayDuration(t *testing.T) {
	d := testDB(t)
	raw := `{"enabled":true,"carCharger":{"enabled":true,"chargers":[
		{"key":"c2","name":"Portatil","powerId":"sensor.p","ampId":"input_select.amps",
		 "delayId":"input_select.feyree_delay_h","durationId":"input_select.feyree_duration_h"}]}}`
	if err := d.KVSet("extensions_config", raw); err != nil {
		t.Fatalf("kv: %v", err)
	}

	cfg := ResolveConfig(d)
	ch := GetChargerByKey(cfg, "c2")
	if ch == nil {
		t.Fatalf("cargador c2 no encontrado: %+v", cfg.CarCharger.Chargers)
	}
	if ch.DelayID != "input_select.feyree_delay_h" {
		t.Fatalf("delayId = %q (perdido al normalizar)", ch.DelayID)
	}
	if ch.DurationID != "input_select.feyree_duration_h" {
		t.Fatalf("durationId = %q (perdido al normalizar)", ch.DurationID)
	}

	// The scheduling entities must be subscribed so the live value arrives.
	var hasDelay, hasDur bool
	for _, id := range ChargerEntities(cfg) {
		if id == "input_select.feyree_delay_h" {
			hasDelay = true
		}
		if id == "input_select.feyree_duration_h" {
			hasDur = true
		}
	}
	if !hasDelay || !hasDur {
		t.Fatalf("delay/duration ausentes de ChargerEntities: %v", ChargerEntities(cfg))
	}
}

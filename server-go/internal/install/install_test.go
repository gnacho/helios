package install

import (
	"reflect"
	"testing"
)

func TestLegacyProfileShape(t *testing.T) {
	s := NewStore()
	e := s.Entities()

	if e.PvSolis != "sensor.solis_potencia_actual" {
		t.Fatalf("pvSolis = %q", e.PvSolis)
	}
	if e.PvFox != "sensor.almacen_pinza_power_b" {
		t.Fatalf("pvFox = %q", e.PvFox)
	}
	if e.BatterySoc != "sensor.solis_bateria_soc" {
		t.Fatalf("batterySoc = %q", e.BatterySoc)
	}
	if e.Scraper == nil || *e.Scraper != "sensor.solis_scraper" {
		t.Fatalf("scraper = %v", e.Scraper)
	}
	if e.EGridImport != "sensor.energia_red_importada_solis" {
		t.Fatalf("eGridImport = %q", e.EGridImport)
	}
}

func TestLiveEntitiesDedupAndContent(t *testing.T) {
	s := NewStore()
	ids := s.LiveEntities()

	set := map[string]bool{}
	for _, id := range ids {
		if set[id] {
			t.Fatalf("ID duplicado: %s", id)
		}
		set[id] = true
	}

	must := []string{
		"sensor.solis_potencia_actual",
		"sensor.almacen_pinza_power_b",
		"sensor.solis_bateria_soc",
		"sensor.solis_scraper",
		"sensor.consumo_total_diario",
		"sun.sun",
	}
	for _, id := range must {
		if !set[id] {
			t.Fatalf("falta %s en liveEntities", id)
		}
	}
}

func TestDeepSourcesMapping(t *testing.T) {
	s := NewStore()
	srcs, mapping := s.DeepSourceDailyKeys()

	if _, ok := srcs["inv0"]; !ok {
		t.Fatal("falta inv0")
	}
	if mapping["inv0"] != "solis" {
		t.Fatalf("inv0 -> %q", mapping["inv0"])
	}
	if mapping["inv1"] != "fox" {
		t.Fatalf("inv1 -> %q", mapping["inv1"])
	}
	// gridExport cap = 25 en el original
	if srcs["gridExport"].Cap != 25 {
		t.Fatalf("gridExport cap = %v", srcs["gridExport"].Cap)
	}
	if !srcs["consumption"].RequireAll {
		t.Fatal("consumption debe requireAll")
	}
}

func TestNormalizeOverridesAndFills(t *testing.T) {
	cfg := TopologyInput{
		Inverters: []InverterInput{
			{
				Key: "nuevo", Name: "Mi inversor",
				PowerID: "sensor.x_power", PowerUnit: UnitKW,
				EnergyID: "sensor.x_energy", EnergyAcc: AccSum,
			},
		},
		Sun: "sun.sun",
	}

	out := Normalize(cfg, GenericTopology)

	if len(out.Inverters) != 1 {
		t.Fatalf("inverters = %d", len(out.Inverters))
	}
	inv := out.Inverters[0]
	if inv.Key != "nuevo" || inv.Name != "Mi inversor" {
		t.Fatalf("key/name = %q/%q", inv.Key, inv.Name)
	}
	if inv.PowerID != "sensor.x_power" {
		t.Fatalf("powerId = %q", inv.PowerID)
	}
	// heredado del base genérico
	if inv.EnergyCap != 100 {
		t.Fatalf("energyCap = %v (debería heredar 100)", inv.EnergyCap)
	}
	// deepIds derivado del energyId
	if len(inv.DeepIDs) != 1 || inv.DeepIDs[0] != "sensor.x_energy" {
		t.Fatalf("deepIds = %v", inv.DeepIDs)
	}
	if out.Battery.Enabled {
		t.Fatal("battery debería seguir heredada (disabled)")
	}
}

func TestNormalizeKeepsBaseWhenEmpty(t *testing.T) {
	out := Normalize(TopologyInput{}, LegacyTopology)
	if len(out.Inverters) != 2 {
		t.Fatalf("inverters = %d (esperado 2 del legacy)", len(out.Inverters))
	}
	if out.Sun != "sun.sun" {
		t.Fatalf("sun = %q", out.Sun)
	}
	if !out.Battery.Enabled {
		t.Fatal("battery legacy debería estar enabled")
	}
}

func TestNormalizeSanitizesUnsafeInverterKey(t *testing.T) {
	payload := "x[(console.info(1),'x')]"
	out := Normalize(TopologyInput{Inverters: []InverterInput{{Key: payload}}}, GenericTopology)
	if len(out.Inverters) != 1 {
		t.Fatalf("inverters = %d", len(out.Inverters))
	}
	if got := out.Inverters[0].Key; got != "inv1" {
		t.Fatalf("key = %q, want inv1", got)
	}
}

// Una config guardada con la batería única (blob antiguo sin "batteries") debe
// cargar sin intervención: Normalize la convierte en lista de una (#197).
func TestNormalizeLegacySingleBatteryBecomesList(t *testing.T) {
	out := Normalize(TopologyInput{}, LegacyTopology)
	if len(out.Batteries) != 1 {
		t.Fatalf("batteries = %d (esperado 1 desde el perfil legacy)", len(out.Batteries))
	}
	if !reflect.DeepEqual(out.Batteries[0], out.Battery) {
		t.Fatalf("batteries[0] != battery: %+v vs %+v", out.Batteries[0], out.Battery)
	}
	if !out.Batteries[0].Enabled || out.Batteries[0].SocID != "sensor.solis_bateria_soc" {
		t.Fatalf("batería legacy mal migrada: %+v", out.Batteries[0])
	}

	cfg := TopologyInput{Battery: &BatteryConfig{
		Enabled: true, PowerID: "sensor.bat_potencia", SocID: "sensor.bat_soc", CapacityKwh: 10,
	}}
	out = Normalize(cfg, GenericTopology)
	if len(out.Batteries) != 1 || out.Batteries[0].PowerID != "sensor.bat_potencia" {
		t.Fatalf("battery única del input no llegó a la lista: %+v", out.Batteries)
	}
	if !reflect.DeepEqual(out.Batteries[0], out.Battery) {
		t.Fatal("battery debe seguir siendo la primera de la lista")
	}
}

// Normalize poda las baterías extra vacías pero conserva las deshabilitadas con
// datos (una batería apagada a propósito no puede desaparecer del editor).
func TestNormalizeBatteriesListPrunesEmptyExtras(t *testing.T) {
	filled := BatteryConfig{
		Enabled: true, PowerID: "sensor.bat_potencia", SocID: "sensor.bat_soc", CapacityKwh: 5,
	}
	empty := BatteryConfig{}
	disabledWithValues := BatteryConfig{
		Enabled: false, PowerID: "sensor.bat2_potencia", SocID: "sensor.bat2_soc", CapacityKwh: 10,
	}
	out := Normalize(TopologyInput{Batteries: []BatteryConfig{filled, empty, disabledWithValues}}, GenericTopology)

	if len(out.Batteries) != 2 {
		t.Fatalf("batteries = %d (la vacía debe podarse, la disabled con datos no)", len(out.Batteries))
	}
	if out.Batteries[0].PowerID != "sensor.bat_potencia" {
		t.Fatalf("primera batería = %+v", out.Batteries[0])
	}
	if out.Batteries[1].PowerID != "sensor.bat2_potencia" || out.Batteries[1].Enabled {
		t.Fatalf("segunda batería = %+v", out.Batteries[1])
	}
	if !reflect.DeepEqual(out.Battery, out.Batteries[0]) {
		t.Fatal("battery debe ser la primera de la lista")
	}
}

// BatteryList unifica el consumo: blob antiguo (solo Battery) y config nueva
// (Batteries) devuelven la lista efectiva por la misma vía.
func TestBatteryListFallsBackToSingle(t *testing.T) {
	old := Topology{Battery: LegacyTopology.Battery}
	list := old.BatteryList()
	if len(list) != 1 || list[0].SocID != "sensor.solis_bateria_soc" {
		t.Fatalf("batteryList = %+v", list)
	}

	fresh := Normalize(TopologyInput{Batteries: []BatteryConfig{
		{Enabled: true, PowerID: "a"}, {Enabled: true, PowerID: "b"},
	}}, GenericTopology)
	list = fresh.BatteryList()
	if len(list) != 2 || list[1].PowerID != "b" {
		t.Fatalf("batteryList = %+v", list)
	}
}

func TestStoreIsolatesMutations(t *testing.T) {
	s := NewStore()
	before := s.Entities().PvSolis
	s.Set(GenericTopology)
	after := s.Entities().PvSolis
	if before == after {
		t.Fatal("Set no cambió la topología")
	}
	if after != "sensor.inverter_power" {
		t.Fatalf("pvSolis genérico = %q", after)
	}
}

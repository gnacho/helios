package install

import (
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

package main

import (
	"slices"
	"testing"

	"helios-server/internal/extensions"
	"helios-server/internal/install"
)

func TestHASubscribeIDsUnion(t *testing.T) {
	store := install.NewStore()
	cfg := extensions.GenericExtensions
	cfg.Enabled = true
	cfg.CarCharger.Enabled = true
	cfg.CarCharger.Chargers = []extensions.Charger{{
		Key:             "main",
		SwitchID:        "switch.wallbox",
		ScheduleTimerID: "timer.coche_carga",
	}}

	ids := haSubscribeIDs(store, cfg)

	if !slices.Contains(ids, "switch.wallbox") {
		t.Fatalf("falta switch.wallbox en %v", ids)
	}
	if !slices.Contains(ids, "timer.coche_carga") {
		t.Fatalf("falta timer.coche_carga en %v", ids)
	}

	// Sin duplicados: la misma ID en live y en el cargador aparece una vez.
	cfg.CarCharger.Chargers[0].PowerID = "sensor.duplicado"
	cfg2 := cfg
	cfg2.CarCharger.Chargers[0].EnergyTotalID = "sensor.duplicado"
	ids2 := haSubscribeIDs(store, cfg2)
	count := 0
	for _, id := range ids2 {
		if id == "sensor.duplicado" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("sensor.duplicado aparece %d veces en %v, want 1", count, ids2)
	}
}

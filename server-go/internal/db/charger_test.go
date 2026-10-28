package db

import (
	"path/filepath"
	"testing"
)

func openChargerTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "charger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// SetChargerPVKwh reescribe el reparto sin tocar la energía y lo clampea a lo
// medido.
func TestSetChargerPVKwhRewritesAndClamps(t *testing.T) {
	d := openChargerTestDB(t)

	if err := d.AddChargerKwh("2026-10-21", "main", 4, 4); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := d.SetChargerPVKwh("2026-10-21", "main", 0.595); err != nil {
		t.Fatalf("set: %v", err)
	}
	rows, err := d.ChargerHistoryRange("2026-10-21", "2026-10-21", "main")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows: %v, %v", len(rows), err)
	}
	if rows[0].Kwh == nil || *rows[0].Kwh != 4 {
		t.Fatalf("kwh = %v (el recálculo no puede tocar la energía)", rows[0].Kwh)
	}
	if rows[0].PvKwh == nil || *rows[0].PvKwh != 0.595 {
		t.Fatalf("pv = %v (esperado 0,595)", rows[0].PvKwh)
	}

	// Por encima de la energía medida se queda en la energía medida.
	if err := d.SetChargerPVKwh("2026-10-21", "main", 9.5); err != nil {
		t.Fatalf("set2: %v", err)
	}
	rows, _ = d.ChargerHistoryRange("2026-10-21", "2026-10-21", "main")
	if rows[0].PvKwh == nil || *rows[0].PvKwh != 4 {
		t.Fatalf("pv = %v (debe clampear a kwh)", rows[0].PvKwh)
	}

	// Negativo se queda en cero.
	if err := d.SetChargerPVKwh("2026-10-21", "main", -1); err != nil {
		t.Fatalf("set3: %v", err)
	}
	rows, _ = d.ChargerHistoryRange("2026-10-21", "2026-10-21", "main")
	if rows[0].PvKwh == nil || *rows[0].PvKwh != 0 {
		t.Fatalf("pv = %v (nunca negativo)", rows[0].PvKwh)
	}
}

// Un día sin energía (fila sembrada a NULL) no se puede repartir y no se crea.
func TestSetChargerPVKwhIgnoresRowsWithoutEnergy(t *testing.T) {
	d := openChargerTestDB(t)

	// Fila sembrada sin energía (es la que deja el backfill a la espera de datos).
	if _, err := d.db.Exec(
		`INSERT INTO daily_charger (date, charger, kwh, pv_kwh) VALUES (?, ?, NULL, NULL)`,
		"2026-10-22", "main",
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := d.SetChargerPVKwh("2026-10-22", "main", 1.2); err != nil {
		t.Fatalf("set: %v", err)
	}
	rows, _ := d.ChargerHistoryRange("2026-10-22", "2026-10-22", "main")
	if len(rows) != 1 || rows[0].PvKwh != nil {
		t.Fatalf("rows = %+v (sin energía el reparto queda intacto)", rows)
	}

	// Y no inventa filas para días que no existen.
	if err := d.SetChargerPVKwh("2026-10-23", "main", 1.2); err != nil {
		t.Fatalf("set2: %v", err)
	}
	if rows, _ := d.ChargerHistoryRange("2026-10-23", "2026-10-23", "main"); len(rows) != 0 {
		t.Fatalf("rows = %+v (no debe crear filas)", rows)
	}
}

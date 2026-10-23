package web

import (
	"testing"
	"time"

	"helios-server/internal/db"
)

func jsonPtr(s string) *string  { return &s }
func f64Ptr(v float64) *float64 { return &v }

func TestDailyInvKwh(t *testing.T) {
	row := db.DailyRow{
		InvertersKwh: jsonPtr(`{"solis":9.0,"fox":8.1}`),
		SolisKwh:     f64Ptr(7),
		FoxKwh:       f64Ptr(6),
	}
	if got := dailyInvKwh(row, "solis"); got != 9.0 {
		t.Fatalf("solis = %v", got)
	}
	if got := dailyInvKwh(row, "fox"); got != 8.1 {
		t.Fatalf("fox = %v", got)
	}
	if got := dailyInvKwh(row, "otro"); got != 0 {
		t.Fatalf("inversor ausente = %v", got)
	}
	// Sin JSON: columnas legacy.
	legacy := db.DailyRow{SolisKwh: f64Ptr(7), FoxKwh: f64Ptr(6)}
	if got := dailyInvKwh(legacy, "solis"); got != 7 {
		t.Fatalf("legacy solis = %v", got)
	}
	if got := dailyInvKwh(legacy, "fox"); got != 6 {
		t.Fatalf("legacy fox = %v", got)
	}
	// JSON corrupto: cae a las columnas legacy.
	bad := db.DailyRow{InvertersKwh: jsonPtr("{nope"), SolisKwh: f64Ptr(3)}
	if got := dailyInvKwh(bad, "solis"); got != 3 {
		t.Fatalf("json corrupto = %v", got)
	}
}

func TestKwhSumAndWindows(t *testing.T) {
	rows := []db.DailyRow{
		{Date: "2026-09-30", InvertersKwh: jsonPtr(`{"solis":9}`)},
		{Date: "2026-10-01", InvertersKwh: jsonPtr(`{"solis":10,"fox":8}`)},
		{Date: "2026-10-02", InvertersKwh: jsonPtr(`{"solis":5,"fox":4}`)},
		{Date: "2026-10-03", InvertersKwh: jsonPtr(`{"solis":6,"fox":3}`)},
	}
	now := time.Date(2026, 10, 5, 10, 0, 0, 0, time.Local)

	if got := kwhSum(rows, "solis"); got != 30 {
		t.Fatalf("suma solis = %v", got)
	}
	m := monthRows(rows, now)
	if len(m) != 3 {
		t.Fatalf("mes = %d filas (esperado 3)", len(m))
	}
	if got := kwhSum(m, "solis"); got != 21 {
		t.Fatalf("mes solis = %v", got)
	}
	l := lastN(rows, 14)
	if len(l) != 4 {
		t.Fatalf("last14 = %d filas", len(l))
	}
	l2 := lastN(rows, 2)
	if len(l2) != 2 || l2[0].Date != "2026-10-02" || l2[1].Date != "2026-10-03" {
		t.Fatalf("lastN recorta mal: %+v", l2)
	}
}

func TestDailyAutoconsumo(t *testing.T) {
	row := db.DailyRow{ProductionKwh: f64Ptr(10), GridExportKwh: f64Ptr(2.5)}
	if got := dailyAutoconsumo(row); got != 75 {
		t.Fatalf("autoconsumo = %v", got)
	}
	// Clamp a 100.
	row2 := db.DailyRow{ProductionKwh: f64Ptr(10), GridExportKwh: f64Ptr(0)}
	if got := dailyAutoconsumo(row2); got != 100 {
		t.Fatalf("autoconsumo clamp = %v", got)
	}
	// Sin producción: 0.
	if got := dailyAutoconsumo(db.DailyRow{}); got != 0 {
		t.Fatalf("autoconsumo sin producción = %v", got)
	}
	// Media semanal redondeada: (75+75+100)/3 = 83.33 → 83.
	week := []db.DailyRow{row, row, row2}
	if got := avgDailyAutoconsumo(week); got != 83 {
		t.Fatalf("media = %v (esperado 83)", got)
	}
	if got := avgDailyAutoconsumo(nil); got != 0 {
		t.Fatalf("media sin filas = %v", got)
	}
}

func TestWeekdayNarrow(t *testing.T) {
	// 2026-10-21 es miércoles.
	d := time.Date(2026, 10, 21, 0, 0, 0, 0, time.Local)
	if got := weekdayNarrow("es", d.Weekday()); got != "X" {
		t.Fatalf("es = %q", got)
	}
	if got := weekdayNarrow("en", d.Weekday()); got != "W" {
		t.Fatalf("en = %q", got)
	}
	if got := weekdayNarrow("zh-CN", d.Weekday()); got != "三" {
		t.Fatalf("zh = %q", got)
	}
}

func TestSourceSplit(t *testing.T) {
	// Fórmula del periodo día en Historico.tsx.
	k := DayKpis{
		ConsumptionKwh:       10,
		GridImportKwh:        3,
		BatteryDischargedKwh: 4,
	}
	s := sourceSplitFromKpis(k)
	if s.Red != 3 || s.Bateria != 4 || s.Solar != 3 {
		t.Fatalf("split = %+v", s)
	}
	// La descarga no puede superar el consumo no-red.
	k2 := DayKpis{ConsumptionKwh: 5, GridImportKwh: 1, BatteryDischargedKwh: 9}
	s2 := sourceSplitFromKpis(k2)
	if s2.Bateria != 4 || s2.Solar != 0 {
		t.Fatalf("split con exceso de batería = %+v", s2)
	}
}

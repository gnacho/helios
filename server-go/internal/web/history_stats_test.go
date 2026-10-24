package web

import (
	"math"
	"testing"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/solar"
)

// Tests replicate the TS behavior of app/src/lib/historyStats.ts for the same
// inputs (anchored to a fixed date so the results are stable).

func hsToday() time.Time { return time.Date(2026, time.July, 15, 10, 30, 0, 0, time.Local) }

func solarPrice() solar.PriceConfig {
	return solar.PriceConfig{PriceImport: 0.25, PriceExport: 0.10, Co2PerKwh: 0.3}
}

func hsRow(date string, prod, cons, imp, exp float64) db.DailyRow {
	p, c, i, e := prod, cons, imp, exp
	return db.DailyRow{
		Date:          date,
		ProductionKwh: &p, ConsumptionKwh: &c,
		GridImportKwh: &i, GridExportKwh: &e,
	}
}

func TestPeriodWindow(t *testing.T) {
	anchor := time.Date(2026, 7, 14, 23, 59, 0, 0, time.Local)

	start, end := periodWindow(PeriodSemana, anchor)
	if !start.Equal(time.Date(2026, 7, 8, 0, 0, 0, 0, time.Local)) || !end.Equal(time.Date(2026, 7, 14, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("semana: %v..%v", start, end)
	}

	start, end = periodWindow(PeriodMes, anchor)
	if start.Day() != 1 || start.Month() != time.July || end.Day() != 31 {
		t.Fatalf("mes: %v..%v", start, end)
	}

	// Febrero bisiesto y años de 365: el día 0 del mes siguiente.
	start, end = periodWindow(PeriodMes, time.Date(2028, 2, 10, 0, 0, 0, 0, time.Local))
	if start.Day() != 1 || end.Day() != 29 || end.Month() != time.February {
		t.Fatalf("febrero 2028: %v..%v", start, end)
	}

	start, end = periodWindow(PeriodAno, anchor)
	if start.Month() != time.January || start.Day() != 1 || end.Month() != time.December || end.Day() != 31 {
		t.Fatalf("año: %v..%v", start, end)
	}
}

func TestInitialAnchorAndShift(t *testing.T) {
	today := hsToday()
	if a := initialAnchor(PeriodDia, today); !a.Equal(time.Date(2026, 7, 14, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("dia anchor = %v (ayer)", a)
	}
	if a := initialAnchor(PeriodSemana, today); !a.Equal(time.Date(2026, 7, 14, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("semana anchor = %v (ayer)", a)
	}
	if a := initialAnchor(PeriodMes, today); !a.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("mes anchor = %v", a)
	}
	if a := initialAnchor(PeriodAno, today); !a.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("año anchor = %v", a)
	}

	// shift ±1 mes cruza el cambio de año.
	if s := shiftAnchor(PeriodMes, time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local), -1); !s.Equal(time.Date(2025, 12, 1, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("shift mes -1 = %v", s)
	}
	// La semana se desplaza de 7 en 7 días.
	if s := shiftAnchor(PeriodSemana, time.Date(2026, 7, 14, 0, 0, 0, 0, time.Local), 1); !s.Equal(time.Date(2026, 7, 21, 0, 0, 0, 0, time.Local)) {
		t.Fatalf("shift semana +1 = %v", s)
	}
}

func TestCanGoNextAndIsCurrent(t *testing.T) {
	today := hsToday()
	a := initialAnchor(PeriodSemana, today)
	if !isCurrentPeriod(PeriodSemana, a, today) || canGoNext(PeriodSemana, a, today) {
		t.Fatal("el periodo por defecto es actual y no puede avanzar")
	}
	// Una semana hacia atrás SÍ puede volver a la actual.
	prev := shiftAnchor(PeriodSemana, a, -1)
	if isCurrentPeriod(PeriodSemana, prev, today) {
		t.Fatal("la semana anterior no es la actual")
	}
	if !canGoNext(PeriodSemana, prev, today) {
		t.Fatal("desde la semana anterior se puede avanzar")
	}
	// Un mes cuyo periodo siguiente aún no empezó no puede avanzar.
	if canGoNext(PeriodMes, time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local), today) {
		t.Fatal("agosto 2026 no puede avanzar desde julio 2026")
	}
	// Año anterior: sí.
	if !canGoNext(PeriodAno, time.Date(2025, 1, 1, 0, 0, 0, 0, time.Local), today) {
		t.Fatal("desde 2025 se puede avanzar a 2026")
	}
}

func TestNavLabel(t *testing.T) {
	es := "es"
	if got := periodNavLabel(PeriodDia, time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local), es); got != "15 jul" {
		t.Fatalf("navLabel dia = %q", got)
	}
	if got := periodNavLabel(PeriodSemana, time.Date(2026, 7, 14, 0, 0, 0, 0, time.Local), es); got != "8 - 14 jul" {
		t.Fatalf("navLabel semana = %q", got)
	}
	// Semana que cruza mes: `29 jun - 5 jul`.
	if got := periodNavLabel(PeriodSemana, time.Date(2026, 7, 5, 0, 0, 0, 0, time.Local), es); got != "29 jun - 5 jul" {
		t.Fatalf("navLabel semana cross-month = %q", got)
	}
	if got := periodNavLabel(PeriodMes, time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local), es); got != "julio 2026" {
		t.Fatalf("navLabel mes = %q", got)
	}
	if got := periodNavLabel(PeriodAno, time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local), es); got != "2026" {
		t.Fatalf("navLabel año = %q", got)
	}
	// Inglés y chino.
	if got := periodNavLabel(PeriodDia, time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local), "en"); got != "Jul 15" {
		t.Fatalf("navLabel dia en = %q", got)
	}
	if got := periodNavLabel(PeriodMes, time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local), "en"); got != "July 2026" {
		t.Fatalf("navLabel mes en = %q", got)
	}
	if got := periodNavLabel(PeriodMes, time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local), "zh-CN"); got != "2026年7月" {
		t.Fatalf("navLabel mes zh = %q", got)
	}
}

func TestPeriodSubtitle(t *testing.T) {
	if got := periodSubtitle(PeriodSemana, time.Date(2026, 7, 14, 0, 0, 0, 0, time.Local), "es"); got != "Semana del 8 al 14 de julio" {
		t.Fatalf("subtitle semana = %q", got)
	}
	if got := periodSubtitle(PeriodMes, time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local), "es"); got != "Mes de julio 2026" {
		t.Fatalf("subtitle mes = %q", got)
	}
	if got := periodSubtitle(PeriodAno, time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local), "es"); got != "Año 2026" {
		t.Fatalf("subtitle año = %q", got)
	}
	if got := periodSubtitle(PeriodDia, time.Date(2026, 7, 15, 0, 0, 0, 0, time.Local), "es"); got != "miércoles, 15 de julio" {
		t.Fatalf("subtitle dia = %q", got)
	}
}

func TestRowsInWindowAndTotals(t *testing.T) {
	rows := []db.DailyRow{
		hsRow("2026-07-07", 10, 8, 2, 4),  // fuera (semana del 8 al 14)
		hsRow("2026-07-08", 20, 15, 3, 8), // autoconsumo (20-8)/20 = 60 %
		hsRow("2026-07-09", 30, 25, 5, 9), // autoconsumo (30-9)/30 = 70 %
		hsRow("2026-07-15", 40, 30, 4, 0), // fuera
	}
	days := rowsInWindow(PeriodSemana, time.Date(2026, 7, 14, 0, 0, 0, 0, time.Local), rows)
	if len(days) != 2 {
		t.Fatalf("rowsInWindow = %d filas (esperado 2)", len(days))
	}

	tot := totalsFor(days, solarPrice())
	if tot.ProductionKwh != 50 || tot.ConsumptionKwh != 40 || tot.GridImportKwh != 8 || tot.GridExportKwh != 17 {
		t.Fatalf("totals = %+v", tot)
	}
	// Media ponderada: (20*60 + 30*70)/50 = 66.
	if math.Abs(tot.AutoconsumoPct-66) > 0.001 {
		t.Fatalf("autoconsumo = %v (esperado 66)", tot.AutoconsumoPct)
	}
	// Ahorro: (40-8)*0.25 + 17*0.10 = 9,7.
	if math.Abs(tot.AhorroEur-9.7) > 0.001 {
		t.Fatalf("ahorro = %v (esperado 9.7)", tot.AhorroEur)
	}
	// CO2: 50 * 0.3 = 15.
	if math.Abs(tot.Co2Kg-15) > 0.001 {
		t.Fatalf("co2 = %v (esperado 15)", tot.Co2Kg)
	}
	if tot.DayCount != 2 {
		t.Fatalf("dayCount = %d", tot.DayCount)
	}

	// Sin producción: autoconsumo 0 (no NaN).
	empty := totalsFor(nil, solarPrice())
	if empty.AutoconsumoPct != 0 || math.IsNaN(empty.AutoconsumoPct) {
		t.Fatalf("autoconsumo sin datos = %v", empty.AutoconsumoPct)
	}
}

func TestDeltaPct(t *testing.T) {
	if v := deltaPct(110, 100); v == nil || math.Abs(*v-10) > 0.001 {
		t.Fatalf("deltaPct(110,100) = %v", v)
	}
	if v := deltaPct(90, 100); v == nil || math.Abs(*v+10) > 0.001 {
		t.Fatalf("deltaPct(90,100) = %v", v)
	}
	if v := deltaPct(10, 0); v != nil {
		t.Fatalf("deltaPct(10,0) = %v (esperado nil)", v)
	}
	if v := deltaPct(10, -5); v != nil {
		t.Fatalf("deltaPct(10,-5) = %v (esperado nil)", v)
	}
}

func TestPluralTP(t *testing.T) {
	// es: 1 árbol / 2 árboles.
	if got := TP("es", "historico.trees", 1, map[string]string{"count": "1"}); got != "≈ 1 árbol plantado" {
		t.Fatalf("trees 1 = %q", got)
	}
	if got := TP("es", "historico.trees", 3, map[string]string{"count": "3"}); got != "≈ 3 árboles plantados" {
		t.Fatalf("trees 3 = %q", got)
	}
	// zh-CN: sin distinción de plural.
	if got := TP("zh-CN", "historico.trees", 1, nil); got == "historico.trees_one" {
		t.Fatalf("zh no debería usar _one: %q", got)
	}
	// streakDays.
	if got := TP("es", "historico.records.streakDays", 1, map[string]string{"count": "1"}); got != "1 día" {
		t.Fatalf("streakDays 1 = %q", got)
	}
}

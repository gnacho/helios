package extensions

import (
	"math"
	"strings"
	"testing"
	"time"

	"helios-server/internal/ha"
	"helios-server/internal/install"
)

// recomputeNow es el instante de referencia: 28-Oct-2026 a las 09:00 locales.
func recomputeNow() time.Time {
	return time.Date(2026, 10, 28, 9, 0, 0, 0, time.Local)
}

// recomputeDay es el día cerrado que se recalcula: dos días atrás.
func recomputeDay() time.Time {
	return time.Date(2026, 10, 26, 0, 0, 0, 0, time.Local)
}

func recomputeDayKey() string { return recomputeDay().Format("2006-01-02") }

// recomputeFixture monta un cargador de 2 kW con el contador del balance
// consumiendo 3,115 kW por bucket: el reparto por curvas de una tarde de cuatro
// buckets es 0,595 kWh, no los 4 kWh que dejó el reparto viejo de las pinzas.
func recomputeFixture(t *testing.T) (*Ext, *fakeHA) {
	t.Helper()
	d := testDB(t)
	store := NewStore()
	cfg := GenericExtensions
	cfg.Enabled = true
	cfg.CarCharger.Enabled = true
	cfg.CarCharger.Chargers = []Charger{{
		Key: "main", PowerID: "sensor.cargador_potencia", PowerUnit: UnitW,
		EnergyTotalID: "sensor.cargador_energia_total", EnergyDivisor: 1,
	}}
	store.Set(cfg)
	e := New(store, d, install.NewStore())

	day := recomputeDay()
	base := day.Add(12 * time.Hour)
	step := 3.115 * 5.0 / 60.0
	f := newFake()
	f.stats = seriesStats(base, 4, &step)
	f.history["sensor.cargador_potencia"] = []ha.HistoryRow{
		{State: "2000", LastChanged: day.Add(-30 * time.Minute).Format(time.RFC3339)},
	}
	return e, f
}

func chargerPV(t *testing.T, e *Ext, date string) *float64 {
	t.Helper()
	rows, err := e.DB.ChargerHistoryRange(date, date, "main")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("filas del %s = %d (esperada 1)", date, len(rows))
	}
	return rows[0].PvKwh
}

// El recálculo reescribe el reparto inflado del histórico con el criterio del
// balance.
func TestRecomputeChargerPVShareLowersInflatedSplit(t *testing.T) {
	e, f := recomputeFixture(t)
	day := recomputeDayKey()
	// Reparto viejo: 4 kWh de carga al 100 % de sol.
	if err := e.DB.AddChargerKwh(day, "main", 4, 4); err != nil {
		t.Fatalf("add: %v", err)
	}

	rep, err := e.recomputeChargerPVShareAt(f, recomputeNow(), 9)
	if err != nil {
		t.Fatalf("recompute: %v", err)
	}

	if rep.Days != 1 || rep.Updated != 1 || rep.Unchanged != 0 || len(rep.Skipped) != 0 {
		t.Fatalf("resumen = %+v (esperado 1 día recorrido y actualizado)", rep)
	}
	pv := chargerPV(t, e, day)
	if pv == nil {
		t.Fatal("pv no debería quedar a NULL")
	}
	const want = 0.595
	if math.Abs(*pv-want) > 0.002 {
		t.Fatalf("pv = %v (esperado %v del balance, no 4 del reparto viejo)", *pv, want)
	}

	summary := rep.String()
	for _, frag := range []string{"dias recorridos: 1", "dias actualizados: 1", "dias sin cambios: 0"} {
		if !strings.Contains(summary, frag) {
			t.Fatalf("resumen sin %q:\n%s", frag, summary)
		}
	}
}

// Una segunda pasada no cambia nada.
func TestRecomputeChargerPVShareIsIdempotent(t *testing.T) {
	e, f := recomputeFixture(t)
	day := recomputeDayKey()
	if err := e.DB.AddChargerKwh(day, "main", 4, 4); err != nil {
		t.Fatalf("add: %v", err)
	}

	first, err := e.recomputeChargerPVShareAt(f, recomputeNow(), 9)
	if err != nil {
		t.Fatalf("primera pasada: %v", err)
	}
	if first.Updated != 1 {
		t.Fatalf("primera pasada: %+v (esperado 1 actualizado)", first)
	}
	before := chargerPV(t, e, day)

	second, err := e.recomputeChargerPVShareAt(f, recomputeNow(), 9)
	if err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}
	if second.Updated != 0 || second.Unchanged != 1 {
		t.Fatalf("segunda pasada: %+v (no debe reescribir nada)", second)
	}
	after := chargerPV(t, e, day)
	if after == nil || before == nil || *after != *before {
		t.Fatalf("pv cambió entre pasadas: %v -> %v", before, after)
	}
}

// Sin balance ese día no hay base: el día queda intacto y se cuenta como omitido.
func TestRecomputeChargerPVShareSkipsDayWithoutBalance(t *testing.T) {
	e, f := recomputeFixture(t)
	f.stats = nil
	day := recomputeDayKey()
	if err := e.DB.AddChargerKwh(day, "main", 4, 4); err != nil {
		t.Fatalf("add: %v", err)
	}

	rep, err := e.recomputeChargerPVShareAt(f, recomputeNow(), 9)
	if err != nil {
		t.Fatalf("recompute: %v", err)
	}

	if rep.Days != 1 || rep.Updated != 0 {
		t.Fatalf("resumen = %+v (no debe escribir nada)", rep)
	}
	if len(rep.Skipped) != 1 || rep.Skipped[0].Date != day {
		t.Fatalf("omitidos = %+v (esperado el %s)", rep.Skipped, day)
	}
	if !strings.Contains(rep.Skipped[0].Reason, "balance") {
		t.Fatalf("motivo = %q (debe hablar del balance)", rep.Skipped[0].Reason)
	}
	if pv := chargerPV(t, e, day); pv == nil || *pv != 4 {
		t.Fatalf("pv = %v (el día debe quedar intacto)", pv)
	}
	if !strings.Contains(rep.String(), "dias omitidos: 1") {
		t.Fatalf("resumen sin el día omitido:\n%s", rep.String())
	}
}

// Un contador de consumo que retrocede dentro del día invalida ese día: no se
// estima un reparto sobre una base rota.
func TestRecomputeChargerPVShareSkipsDayWithCounterReset(t *testing.T) {
	e, f := recomputeFixture(t)
	day := recomputeDayKey()
	if err := e.DB.AddChargerKwh(day, "main", 4, 4); err != nil {
		t.Fatalf("add: %v", err)
	}

	// El bucket de las 12:05 vuelve a 5 kWh: reset del contador dentro del día.
	reset := recomputeDay().Add(12*time.Hour + 5*time.Minute)
	for i := range f.stats["sensor.consumo_total_diario"] {
		if f.stats["sensor.consumo_total_diario"][i].Start.Time.Equal(reset) {
			f.stats["sensor.consumo_total_diario"][i].Sum = mkPtr(5)
		}
	}

	rep, err := e.recomputeChargerPVShareAt(f, recomputeNow(), 9)
	if err != nil {
		t.Fatalf("recompute: %v", err)
	}

	if rep.Updated != 0 || len(rep.Skipped) != 1 {
		t.Fatalf("resumen = %+v (el día debe omitirse)", rep)
	}
	if !strings.Contains(rep.Skipped[0].Reason, "retrocede") {
		t.Fatalf("motivo = %q (debe decir que el contador retrocede)", rep.Skipped[0].Reason)
	}
	if pv := chargerPV(t, e, day); pv == nil || *pv != 4 {
		t.Fatalf("pv = %v (el día debe quedar intacto)", pv)
	}
}

// Solo se toca la ventana cerrada: hoy lo sigue acumulando el tick y los días
// fuera de la ventana no tienen curvas disponibles.
func TestRecomputeChargerPVShareLeavesDaysOutsideWindow(t *testing.T) {
	e, f := recomputeFixture(t)
	today := recomputeNow().Format("2006-01-02")
	old := recomputeNow().AddDate(0, 0, -20).Format("2006-01-02")
	for _, date := range []string{today, old} {
		if err := e.DB.AddChargerKwh(date, "main", 3, 3); err != nil {
			t.Fatalf("add %s: %v", date, err)
		}
	}

	rep, err := e.recomputeChargerPVShareAt(f, recomputeNow(), 9)
	if err != nil {
		t.Fatalf("recompute: %v", err)
	}
	if rep.Days != 0 || rep.Updated != 0 {
		t.Fatalf("resumen = %+v (no hay días cerrados en la ventana)", rep)
	}
	for _, date := range []string{today, old} {
		if pv := chargerPV(t, e, date); pv == nil || *pv != 3 {
			t.Fatalf("pv del %s = %v (fuera de la ventana debe quedar intacto)", date, pv)
		}
	}
}

// El reparto recalculado nunca supera la energía medida del cargador.
func TestRecomputeChargerPVShareClampsToEnergy(t *testing.T) {
	e, f := recomputeFixture(t)
	day := recomputeDayKey()
	// Día con 0,2 kWh medidos y sin reparto previo (el acumulador no tenía
	// balance cuando lo escribió).
	if err := e.DB.AddChargerKwhUnknownPV(day, "main", 0.2); err != nil {
		t.Fatalf("add: %v", err)
	}

	if _, err := e.recomputeChargerPVShareAt(f, recomputeNow(), 9); err != nil {
		t.Fatalf("recompute: %v", err)
	}
	pv := chargerPV(t, e, day)
	if pv == nil || math.Abs(*pv-0.2) > 1e-9 {
		t.Fatalf("pv = %v (debe clampear a los 0,2 kWh medidos)", pv)
	}
}

package extensions

import (
	"sync/atomic"
	"testing"
	"time"

	"helios-server/internal/ha"
	"helios-server/internal/install"
)

type countingHistoryHA struct {
	*fakeHA
	calls *int32
}

func (c *countingHistoryHA) HistoryDuringPeriod(start, end, entityID string) ([]ha.HistoryRow, error) {
	atomic.AddInt32(c.calls, 1)
	return c.fakeHA.HistoryDuringPeriod(start, end, entityID)
}

// TestChargerDayCurveCached: dos peticiones de la misma curva solo deben
// consultar HAOS una vez dentro del TTL (regresión de rendimiento 22-Oct-2026).
func TestChargerDayCurveCached(t *testing.T) {
	d := testDB(t)
	e := New(NewStore(), d, install.NewStore())

	var calls int32
	f := &countingHistoryHA{fakeHA: newFake(), calls: &calls}
	base := time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local)
	f.history["sensor.pot"] = []ha.HistoryRow{
		{State: "0", LastChanged: base.Format(time.RFC3339)},
		{State: "3000", LastChanged: base.Add(10 * time.Minute).Format(time.RFC3339)},
	}

	ch := &Charger{Key: "main", PowerID: "sensor.pot", PowerUnit: UnitW}

	c1 := e.ChargerDayCurve(f, "2026-10-20", ch)
	c2 := e.ChargerDayCurve(f, "2026-10-20", ch)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("consultas a HAOS = %d (esperado 1 por caché)", got)
	}
	if len(c1.Points) != len(c2.Points) {
		t.Fatalf("curvas distintas: %d vs %d", len(c1.Points), len(c2.Points))
	}
}

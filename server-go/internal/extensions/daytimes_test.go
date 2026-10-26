package extensions

import (
	"sync/atomic"
	"testing"
	"time"

	"helios-server/internal/ha"
	"helios-server/internal/install"
)

// TestChargerDayTimes derives minutes and start/end span from the recorder
// history of the charger state entity (issue 173).
func TestChargerDayTimes(t *testing.T) {
	d := testDB(t)
	e := New(NewStore(), d, install.NewStore())

	f := newFake()
	base := time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local)
	f.history["sensor.st"] = []ha.HistoryRow{
		{State: "connect", LastChanged: base.Format(time.RFC3339)},
		{State: "charing", LastChanged: base.Add(1 * time.Hour).Format(time.RFC3339)},
		{State: "connect", LastChanged: base.Add(90 * time.Minute).Format(time.RFC3339)},
	}

	ch := &Charger{Key: "c2", StateID: "sensor.st", ChargingStates: []string{"charing"}}
	tm := e.ChargerDayTimes(f, "2026-10-20", "2026-10-20", ch)

	got, ok := tm["2026-10-20"]
	if !ok {
		t.Fatalf("sin datos para 2026-10-20: %v", tm)
	}
	if got.Minutes != 30 {
		t.Fatalf("minutes = %d (esperado 30)", got.Minutes)
	}
	if got.Start != "01:00" || got.End != "01:30" {
		t.Fatalf("span = %s-%s (esperado 01:00-01:30)", got.Start, got.End)
	}
}

// TestChargerDayTimesCached: repeated calls for the same window must hit the
// cache and query HA only once.
func TestChargerDayTimesCached(t *testing.T) {
	d := testDB(t)
	e := New(NewStore(), d, install.NewStore())

	var calls int32
	f := &countingHistoryHA{fakeHA: newFake(), calls: &calls}
	base := time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local)
	f.history["sensor.st"] = []ha.HistoryRow{
		{State: "charing", LastChanged: base.Format(time.RFC3339)},
		{State: "connect", LastChanged: base.Add(30 * time.Minute).Format(time.RFC3339)},
	}

	ch := &Charger{Key: "c2", StateID: "sensor.st", ChargingStates: []string{"charing"}}
	e.ChargerDayTimes(f, "2026-10-20", "2026-10-20", ch)
	e.ChargerDayTimes(f, "2026-10-20", "2026-10-20", ch)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("consultas a HAOS = %d (esperado 1 por caché)", got)
	}
}

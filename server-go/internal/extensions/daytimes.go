package extensions

import (
	"strings"
	"time"
)

// ChargerDayTimes aggregates, for one day, how long the charger was in a
// charging state plus the start/end times of that activity (issue 173).
type ChargerDayTimes struct {
	Minutes int
	Start   string // HH:MM of the first charging segment start
	End     string // HH:MM of the last charging segment end
}

type timesEntry struct {
	times map[string]ChargerDayTimes
	at    time.Time
	ttl   time.Duration
}

// ChargerDayTimes derives, from the history of the charger state entity, the
// minutes in a charging state and the start/end span per day in [from, to]
// (local dates). Cached the same way as the day curve.
func (e *Ext) ChargerDayTimes(h HA, from, to string, c *Charger) map[string]ChargerDayTimes {
	if c == nil || c.StateID == "" || len(c.ChargingStates) == 0 {
		return map[string]ChargerDayTimes{}
	}

	cacheKey := "times:" + c.Key + ":" + from + ":" + to
	e.timesMu.Lock()
	if ent, ok := e.timesCache[cacheKey]; ok && time.Since(ent.at) < ent.ttl {
		times := ent.times
		e.timesMu.Unlock()
		return times
	}
	e.timesMu.Unlock()

	times := e.chargerDayTimesUncached(h, from, to, c)

	ttl := 6 * time.Hour
	if to >= todayStr() {
		ttl = 60 * time.Second
	}
	e.timesMu.Lock()
	if e.timesCache == nil {
		e.timesCache = map[string]timesEntry{}
	}
	e.timesCache[cacheKey] = timesEntry{times: times, at: time.Now(), ttl: ttl}
	e.timesMu.Unlock()
	return times
}

func (e *Ext) chargerDayTimesUncached(h HA, from, to string, c *Charger) map[string]ChargerDayTimes {
	out := map[string]ChargerDayTimes{}
	start, err1 := time.ParseInLocation("2006-01-02", from, time.Local)
	end, err2 := time.ParseInLocation("2006-01-02", to, time.Local)
	if err1 != nil || err2 != nil {
		return out
	}
	end = end.AddDate(0, 0, 1)

	rows, err := h.HistoryDuringPeriod(start.Format(time.RFC3339), end.Format(time.RFC3339), c.StateID)
	if err != nil || len(rows) == 0 {
		return out
	}

	charging := map[string]bool{}
	for _, s := range c.ChargingStates {
		charging[strings.ToLower(strings.TrimSpace(s))] = true
	}

	// Each recorder row opens a segment: the previous row's state lasted until
	// this transition. Segments crossing midnight are split per day.
	var prev time.Time
	prevCharging := false
	for _, r := range rows {
		t, err := time.Parse(time.RFC3339, r.LastChanged)
		if err != nil {
			continue
		}
		t = t.Local()
		if !prev.IsZero() && prevCharging {
			for segEnd := t; prev.Before(segEnd); {
				dayEnd := time.Date(prev.Year(), prev.Month(), prev.Day(), 0, 0, 0, 0, prev.Location()).AddDate(0, 0, 1)
				cut := segEnd
				if cut.After(dayEnd) {
					cut = dayEnd
				}
				day := prev.Format("2006-01-02")
				d := out[day]
				d.Minutes += int(cut.Sub(prev).Minutes())
				if d.Start == "" {
					d.Start = prev.Format("15:04")
				}
				d.End = cut.Format("15:04")
				out[day] = d
				prev = cut
			}
		}
		prev = t
		prevCharging = charging[strings.ToLower(strings.TrimSpace(r.State))]
	}
	// Open segment: still charging (or the history ends charging), close the
	// span at "now".
	if !prev.IsZero() && prevCharging {
		now := time.Now().Local()
		if now.Before(prev) {
			now = prev
		}
		day := prev.Format("2006-01-02")
		d := out[day]
		d.Minutes += int(now.Sub(prev).Minutes())
		if d.Start == "" {
			d.Start = prev.Format("15:04")
		}
		d.End = now.Format("15:04")
		out[day] = d
	}
	return out
}

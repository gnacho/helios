package extensions

import (
	"fmt"
	"math"
	"time"
)

type CurvePoint struct {
	T     int     `json:"t"`
	Label string  `json:"label"`
	Kw    float64 `json:"kw"`
}

type ChargerCurve struct {
	Points []CurvePoint `json:"points"`
}

const stepMin = 5

// ChargerDayCurve construye la curva del día (kW cada 5 min) por interpolación
// escalonada del historial de estados del sensor de potencia.
func (e *Ext) ChargerDayCurve(h HA, dateStr string, c *Charger) ChargerCurve {
	if c == nil || c.PowerID == "" {
		return ChargerCurve{Points: []CurvePoint{}}
	}

	key := dateStr
	if key == "" {
		key = todayStr()
	}

	start, err := time.ParseInLocation("2006-01-02", key, time.Local)
	if err != nil {
		return ChargerCurve{Points: []CurvePoint{}}
	}
	end := start.AddDate(0, 0, 1)

	rows, err := h.HistoryDuringPeriod(start.Format(time.RFC3339), end.Format(time.RFC3339), c.PowerID)
	if err != nil || len(rows) == 0 {
		return ChargerCurve{Points: []CurvePoint{}}
	}

	type step struct {
		min int
		kw  float64
	}
	steps := make([]step, 0, len(rows))
	for _, r := range rows {
		v := parseFloatStrict(r.State)
		ts, err := time.Parse(time.RFC3339, r.LastChanged)
		if err != nil {
			continue
		}
		minutes := int(math.Round(float64(ts.Sub(start).Minutes())))
		if minutes < 0 {
			minutes = 0
		}
		kw := 0.0
		if v != nil {
			if c.PowerUnit == UnitW {
				kw = *v / 1000
			} else {
				kw = *v
			}
		}
		steps = append(steps, step{min: minutes, kw: kw})
	}
	if len(steps) == 0 {
		return ChargerCurve{Points: []CurvePoint{}}
	}

	// HOY: la curva termina en el minuto actual. Días pasados: completo.
	isToday := key == todayStr()
	limitMin := 1440
	if isToday {
		limitMin = int(math.Max(0, math.Floor(float64(time.Since(start).Minutes()))))
		if limitMin > 1440 {
			limitMin = 1440
		}
	}

	points := make([]CurvePoint, 0, limitMin/stepMin+1)
	idx := 0
	for min := 0; min < limitMin; min += stepMin {
		for idx < len(steps)-1 && steps[idx+1].min <= min {
			idx++
		}
		if steps[idx].min > min {
			break
		}
		kw := steps[idx].kw
		if kw < 0 {
			kw = 0
		}
		points = append(points, CurvePoint{
			T:     min,
			Label: fmt.Sprintf("%02d:%02d", min/60, min%60),
			Kw:    round3(kw),
		})
	}
	return ChargerCurve{Points: points}
}

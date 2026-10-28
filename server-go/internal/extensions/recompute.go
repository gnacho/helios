package extensions

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// pvRecomputeDays es la ventana por defecto: los mismos nueve días que rellena
// el backfill del cargador, que son los que caben con holgura en el histórico de
// cinco minutos que guarda HAOS.
const pvRecomputeDays = 9

// pvRecomputeEpsilon: dos repartos del mismo día que difieren menos que esto se
// consideran el mismo valor y no se reescriben (segunda pasada sin cambios).
const pvRecomputeEpsilon = 0.0005

// PVRecomputeSkip es un día que no se pudo recalcular y el motivo.
type PVRecomputeSkip struct {
	Date   string
	Reason string
}

// PVRecomputeReport resume la pasada de recálculo del reparto FV del cargador.
type PVRecomputeReport struct {
	From      string
	To        string
	Days      int
	Updated   int
	Unchanged int
	Skipped   []PVRecomputeSkip
}

func (r *PVRecomputeReport) skip(date, reason string) {
	r.Skipped = append(r.Skipped, PVRecomputeSkip{Date: date, Reason: reason})
}

// String devuelve el resumen legible que imprime el comando.
func (r PVRecomputeReport) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "reparto FV del cargador: %s..%s\n", r.From, r.To)
	fmt.Fprintf(&b, "  dias recorridos: %d\n", r.Days)
	fmt.Fprintf(&b, "  dias actualizados: %d\n", r.Updated)
	fmt.Fprintf(&b, "  dias sin cambios: %d\n", r.Unchanged)
	fmt.Fprintf(&b, "  dias omitidos: %d\n", len(r.Skipped))
	for _, s := range r.Skipped {
		fmt.Fprintf(&b, "    - %s: %s\n", s.Date, s.Reason)
	}
	return b.String()
}

// RecomputeChargerPVShare reescribe daily_charger.pv_kwh de los días ya cerrados
// con el reparto del balance: es el arreglo del histórico escrito con las pinzas,
// que se quedó con fracciones infladas. Solo toca el reparto, nunca la energía
// acumulada, y los días sin balance utilizable quedan intactos.
func (e *Ext) RecomputeChargerPVShare(h HA, days int) (PVRecomputeReport, error) {
	return e.recomputeChargerPVShareAt(h, time.Now(), days)
}

// recomputeChargerPVShareAt es RecomputeChargerPVShare con el reloj inyectado.
func (e *Ext) recomputeChargerPVShareAt(h HA, now time.Time, days int) (PVRecomputeReport, error) {
	if days <= 0 {
		days = pvRecomputeDays
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	from := today.AddDate(0, 0, -days)
	to := today // extremo exclusivo: hoy lo sigue acumulando el tick
	report := PVRecomputeReport{
		From: from.Format("2006-01-02"),
		To:   to.AddDate(0, 0, -1).Format("2006-01-02"),
	}
	if !to.After(from) {
		return report, nil
	}

	c := e.Store.Get()
	if !ChargerActive(c) {
		return report, fmt.Errorf("el cargador de coche está desactivado en los ajustes")
	}

	statuses, err := e.balanceDayStatuses(h, from, to)
	if err != nil {
		return report, fmt.Errorf("balance: %w", err)
	}

	for _, ch := range c.CarCharger.Chargers {
		if err := e.recomputeOneCharger(h, ch, from, to, statuses, &report); err != nil {
			return report, err
		}
	}
	return report, nil
}

// recomputeOneCharger recalcula el reparto de un cargador día a día.
func (e *Ext) recomputeOneCharger(h HA, ch Charger, from, to time.Time, statuses map[string]balanceDayStatus, report *PVRecomputeReport) error {
	fromKey := from.Format("2006-01-02")
	toKey := to.AddDate(0, 0, -1).Format("2006-01-02")

	rows, err := e.DB.ChargerHistoryRange(fromKey, toKey, ch.Key)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	pvByDay := map[string]float64{}
	if ch.PowerID != "" {
		pvByDay = e.ChargerPVFromCurves(h, from.Format(time.RFC3339), to.Format(time.RFC3339), ch)
	}

	for _, r := range rows {
		report.Days++
		if r.Kwh == nil || *r.Kwh <= 0 {
			report.skip(r.Date, "sin energía acumulada que repartir")
			continue
		}
		st, ok := statuses[r.Date]
		switch {
		case !ok || st.Buckets == 0:
			report.skip(r.Date, "sin balance de consumo ese día")
			continue
		case st.Rewound || st.Total < 0:
			report.skip(r.Date, "el contador de consumo retrocede")
			continue
		}
		pv, ok := pvByDay[r.Date]
		if !ok {
			report.skip(r.Date, "las curvas no dan reparto ese día")
			continue
		}
		if pv > *r.Kwh {
			pv = *r.Kwh
		}
		if pv < 0 {
			pv = 0
		}
		pv = round3(pv)
		if r.PvKwh != nil && math.Abs(*r.PvKwh-pv) <= pvRecomputeEpsilon {
			report.Unchanged++
			continue
		}
		if err := e.DB.SetChargerPVKwh(r.Date, ch.Key, pv); err != nil {
			return err
		}
		report.Updated++
	}
	return nil
}

// balanceDayStatus es la salud del contador de consumo del balance en un día.
type balanceDayStatus struct {
	Buckets int
	Total   float64
	Rewound bool
}

// balanceDayStatuses resume el contador de consumo del balance por día: buckets
// de cinco minutos disponibles, energía que sumó y si retrocedió dentro del día.
// El reinicio del contador diario al cambiar de día es normal: solo se mira la
// variación entre buckets del mismo día.
func (e *Ext) balanceDayStatuses(h HA, from, to time.Time) (map[string]balanceDayStatus, error) {
	out := map[string]balanceDayStatus{}
	consID := e.Inst.Get().Energy.ConsumptionID
	if consID == "" {
		return out, nil
	}

	stats, err := h.StatisticsDuringPeriod(
		from.Format(time.RFC3339), to.Format(time.RFC3339),
		[]string{consID}, "5minute", []string{"sum"},
	)
	if err != nil {
		return nil, err
	}

	type point struct {
		ms  int64
		sum float64
	}
	byDay := map[string][]point{}
	for _, r := range stats[consID] {
		if r.Sum == nil || r.Start.IsZero() {
			continue
		}
		key := r.Start.In(time.Local).Format("2006-01-02")
		byDay[key] = append(byDay[key], point{ms: r.Start.UnixMilli(), sum: *r.Sum})
	}

	for date, points := range byDay {
		sort.Slice(points, func(i, j int) bool { return points[i].ms < points[j].ms })
		st := balanceDayStatus{
			Buckets: len(points),
			Total:   points[len(points)-1].sum - points[0].sum,
		}
		for i := 1; i < len(points); i++ {
			if points[i].sum < points[i-1].sum {
				st.Rewound = true
				break
			}
		}
		out[date] = st
	}
	return out, nil
}

package extensions

import (
	"log"
	"math"
	"sort"
	"time"

	"helios-server/internal/ha"
	"helios-server/internal/install"
)

// DailyDeltasFromHistory: incrementos positivos por día de un contador creciente.
// Un reset (bajada) no suma ni resta; los saltos > glitch se ignoran.
func DailyDeltasFromHistory(rows []HistoryPoint, divisor int) map[string]float64 {
	perDay := map[string]float64{}
	prevKey := ""
	prevV := 0.0

	for _, r := range rows {
		v := parseFloatStrict(r.State)
		if v == nil {
			continue
		}
		key := r.lastChangedDay()
		if key == "" {
			continue
		}
		if key != prevKey {
			prevKey = key
			prevV = *v
			if _, ok := perDay[key]; !ok {
				perDay[key] = 0
			}
			continue
		}
		div := float64(divisor)
		if div == 0 {
			div = 1
		}
		diff := (*v - prevV) / div
		if diff > 0 {
			if diff <= chargerGlitchKwh {
				perDay[key] = round3(perDay[key] + diff)
			} else {
				log.Printf("[extensions] cargador: salto anomalo %v kWh ignorado en backfill", diff)
			}
		}
		prevV = *v
	}
	return perDay
}

// HistoryPoint normaliza una fila de historial con su fecha de cambio.
type HistoryPoint struct {
	State       string
	LastChanged string
}

func (p HistoryPoint) lastChangedDay() string {
	t, err := time.Parse(time.RFC3339, p.LastChanged)
	if err != nil {
		if t2, err2 := time.Parse("2006-01-02T15:04:05", p.LastChanged); err2 == nil {
			return t2.In(time.Local).Format("2006-01-02")
		}
		return ""
	}
	return t.In(time.Local).Format("2006-01-02")
}

func toHistoryPoints(rows []ha.HistoryRow) []HistoryPoint {
	out := make([]HistoryPoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, HistoryPoint{State: r.State, LastChanged: r.LastChanged})
	}
	return out
}

// ChargerPVFromCurves integra min(cargador, max(0, producción - resto casa))
// sobre buckets de 5 min. Devuelve kWh por día.
func (e *Ext) ChargerPVFromCurves(h HA, startTime, endTime string, c Charger) map[string]float64 {
	t := e.Inst.Get()

	idSet := map[string]struct{}{}
	for _, inv := range t.Inverters {
		if inv.PowerID != "" {
			idSet[inv.PowerID] = struct{}{}
		}
	}
	for _, id := range t.Consumption.PowerIDs {
		if id != "" {
			idSet[id] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return map[string]float64{}
	}
	statIDs := make([]string, 0, len(idSet))
	for id := range idSet {
		statIDs = append(statIDs, id)
	}
	sort.Strings(statIDs)

	stats, err := h.StatisticsDuringPeriod(startTime, endTime, statIDs, "5minute", []string{"mean"})
	if err != nil {
		return map[string]float64{}
	}

	// bucketStartMs -> mean, por sensor
	bySensor := map[string]map[int64]float64{}
	for id, rows := range stats {
		m := map[int64]float64{}
		for _, r := range rows {
			if r.Mean == nil {
				continue
			}
			ts := r.Start.Time
			err := error(nil)
			_ = err
			if err != nil {
				continue
			}
			m[ts.UnixMilli()] = *r.Mean
		}
		bySensor[id] = m
	}
	if len(bySensor) == 0 {
		return map[string]float64{}
	}

	// Escalón de potencia del cargador desde historial REST.
	// unavailable/unknown = 0 kW (nunca hold-last: fabrica cargas fantasma).
	type step struct {
		t  int64
		kw float64
	}
	var steps []step
	if c.PowerID != "" {
		rows, err := h.HistoryDuringPeriod(startTime, endTime, c.PowerID)
		if err == nil {
			for _, r := range rows {
				v := parseFloatStrict(r.State)
				kw := 0.0
				if v != nil {
					if c.PowerUnit == UnitW {
						kw = *v / 1000
					} else {
						kw = *v
					}
				}
				ts, err := time.Parse(time.RFC3339, r.LastChanged)
				if err != nil {
					continue
				}
				steps = append(steps, step{t: ts.UnixMilli(), kw: kw})
			}
			sort.SliceStable(steps, func(i, j int) bool { return steps[i].t < steps[j].t })
		}
	}
	if len(steps) == 0 {
		return map[string]float64{}
	}
	chgAt := func(ms int64) float64 {
		v := 0.0
		for _, st := range steps {
			if st.t > ms {
				break
			}
			v = st.kw
		}
		return v
	}

	consDiv := 1.0
	if t.Consumption.PowerUnit == install.UnitW {
		consDiv = 1000
	}

	pvByDay := map[string]float64{}
	bucket := int64(5 * 60 * 1000)

	startParsed, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		return pvByDay
	}
	endParsed, err := time.Parse(time.RFC3339, endTime)
	if err != nil {
		return pvByDay
	}
	startMs := ((startParsed.UnixMilli() + bucket - 1) / bucket) * bucket
	endMs := endParsed.UnixMilli()
	if now := time.Now().UnixMilli(); now < endMs {
		endMs = now
	}

	for ms := startMs; ms < endMs; ms += bucket {
		k := chgAt(ms)
		if !(k > 0.05) {
			continue
		}
		prod := 0.0
		for _, inv := range t.Inverters {
			if v, ok := bySensor[inv.PowerID][ms]; ok {
				if inv.PowerUnit == install.UnitW {
					prod += v / 1000
				} else {
					prod += v
				}
			}
		}
		cons := 0.0
		haveCons := true
		for _, id := range t.Consumption.PowerIDs {
			v, ok := bySensor[id][ms]
			if !ok {
				haveCons = false
				break
			}
			cons += v / consDiv
		}
		if !haveCons {
			continue
		}
		other := cons
		if c.ChargerInHouseMeters {
			other = cons - k
		}
		if other < 0 {
			other = 0
		}
		solar := math.Min(k, math.Max(0, prod-other)) * (float64(bucket) / 3600000.0)
		if solar <= 0 {
			continue
		}
		key := time.UnixMilli(ms).In(time.Local).Format("2006-01-02")
		pvByDay[key] = round3(pvByDay[key] + solar)
	}
	return pvByDay
}

// BackfillOneCharger rellena días a NULL desde el recorder. Devuelve días escritos.
func (e *Ext) BackfillOneCharger(h HA, c Charger) int {
	if c.EnergyTotalID == "" {
		return 0
	}
	today := todayStr()
	from := time.Now().AddDate(0, 0, -9)
	fromKey := from.In(time.Local).Format("2006-01-02")

	missing, err := e.DB.MissingChargerDays(fromKey, today, c.Key)
	if err != nil {
		return 0
	}

	startTime := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.Local).Format(time.RFC3339)
	endTime := time.Now().Format(time.RFC3339)

	rows, err := h.HistoryDuringPeriod(startTime, endTime, c.EnergyTotalID)
	if err != nil {
		return 0
	}
	if len(rows) == 0 {
		return 0
	}

	perDay := DailyDeltasFromHistory(toHistoryPoints(rows), c.EnergyDivisor)

	needCurves := false
	for _, date := range missing {
		if v, ok := perDay[date]; ok && v > 0 {
			needCurves = true
			break
		}
	}
	pvByDay := map[string]float64{}
	if needCurves {
		pvByDay = e.ChargerPVFromCurves(h, startTime, endTime, c)
	}

	firstRowKey := ""
	for _, r := range rows {
		if parseFloatStrict(r.State) != nil {
			hp := HistoryPoint{State: r.State, LastChanged: r.LastChanged}
			firstRowKey = hp.lastChangedDay()
			break
		}
	}

	n := 0
	for _, date := range missing {
		if v, ok := perDay[date]; ok {
			var pv *float64
			if p, ok := pvByDay[date]; ok {
				pv = &p
			}
			if err := e.DB.SetChargerKwhIfNull(date, c.Key, v, pv); err == nil {
				n++
			}
		} else if firstRowKey != "" && date >= firstRowKey {
			// Día dentro de la ventana sin estados: contador estático -> 0.
			if err := e.DB.SetChargerKwhIfNull(date, c.Key, 0, nil); err == nil {
				n++
			}
		}
	}

	// HOY sin fila daily: sembrar la base del acumulador con el primer estado de hoy.
	hasTodayRow, _ := e.DB.HasDailyRow(today)
	_, kvErr := e.DB.KVGet(counterKey(c.Key))
	legacyRaw, _ := e.DB.KVGet("charger_counter")
	legacyBase := ""
	if c.Key == "main" {
		legacyBase = legacyRaw
	}
	if !hasTodayRow && kvErr != nil && legacyBase == "" {
		for _, r := range rows {
			if parseFloatStrict(r.State) == nil {
				continue
			}
			hp := HistoryPoint{State: r.State, LastChanged: r.LastChanged}
			if hp.lastChangedDay() != today {
				continue
			}
			v := parseFloatStrict(r.State)
			if v != nil {
				div := float64(c.EnergyDivisor)
				if div == 0 {
					div = 1
				}
				e.writeCounterBase(c.Key, round3(*v/div))
				log.Printf("[extensions] cargador %s: hoy via acumulador (base sembrada)", c.Key)
			}
			break
		}
	}

	return n
}

// BackfillChargerHistory ejecuta el backfill de todos los cargadores activos.
func (e *Ext) BackfillChargerHistory(h HA) int {
	c := e.Store.Get()
	if !ChargerActive(c) {
		return 0
	}
	n := 0
	for _, ch := range c.CarCharger.Chargers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[extensions] cargador %s: backfill fallo: %v", ch.Key, r)
				}
			}()
			n += e.BackfillOneCharger(h, ch)
		}()
	}
	return n
}

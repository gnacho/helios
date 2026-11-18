package extensions

import (
	"encoding/json"
	"log"
	"math"
	"time"
)

// Contabilidad de energía para cargadores que no publican un contador fiable.
// El Feyree portátil (c2) llegó a tener el DP de energía muerto a 0.0 y su
// contador de sesión se reinicia al terminar cada carga, así que la energía se
// integra a partir de la potencia, que sí es una medida real mientras carga.
const (
	// powerFreshAfter: un dato de potencia más viejo que esto no se integra
	// (el aparato deja el socket abierto sin mandar frames).
	powerFreshAfter = 10 * time.Minute
	// powerMaxGap: si entre dos ticks pasa más que esto (parada del servicio,
	// pérdida de datos) la energía del hueco no se inventa: solo se re-basa.
	powerMaxGap = 5 * time.Minute
	// powerHistoryMaxGap: en el historial del recorder los huecos son muestra
	// escasa (la potencia se mantiene y no hay cambio de estado que registrar),
	// así que el último valor se sostiene hasta el siguiente punto. Solo un
	// corte largo del recorder se descarta.
	powerHistoryMaxGap = 2 * time.Hour
	// powerMinKw: por debajo de esto se considera que no hay carga.
	powerMinKw = 0.05
)

type powerBase struct {
	At int64 `json:"at"` // unix ms de la última muestra integrada
}

func powerCounterKey(key string) string { return "charger_power:" + key }

func (e *Ext) readPowerBase(key string) *powerBase {
	raw, err := e.DB.KVGet(powerCounterKey(key))
	if err != nil || raw == "" {
		return nil
	}
	var pb powerBase
	if err := json.Unmarshal([]byte(raw), &pb); err != nil {
		return nil
	}
	return &pb
}

func (e *Ext) writePowerBase(key string, ms int64) {
	raw, err := json.Marshal(powerBase{At: ms})
	if err != nil {
		return
	}
	e.DB.KVSet(powerCounterKey(key), string(raw))
}

// chargerPowerKw reads the charger power in kW and reports whether the sample
// is fresh enough to integrate.
func chargerPowerKw(h HA, c Charger, now time.Time) (float64, bool) {
	if c.PowerID == "" {
		return 0, false
	}
	st, ok := h.GetState(c.PowerID)
	if !ok || st.State == "unavailable" || st.State == "unknown" {
		return 0, false
	}
	if st.LastUpdated != "" {
		if ts, err := time.Parse(time.RFC3339, st.LastUpdated); err == nil && now.Sub(ts) > powerFreshAfter {
			return 0, false
		}
	}
	raw := parseFloatStrict(st.State)
	if raw == nil {
		return 0, false
	}
	kw := *raw
	if c.PowerUnit == UnitW {
		kw /= 1000
	}
	if math.IsNaN(kw) || math.IsInf(kw, 0) {
		return 0, false
	}
	return math.Max(0, kw), true
}

// accumulateFromPower adds the energy charged since the previous tick, using
// the charger power as the only source. It is the path for chargers without an
// energy counter.
func (e *Ext) accumulateFromPower(h HA, c Charger, now time.Time) {
	kw, ok := chargerPowerKw(h, c, now)
	if !ok {
		return
	}

	ms := now.UnixMilli()
	saved := e.readPowerBase(c.Key)
	e.writePowerBase(c.Key, ms)
	if saved == nil {
		return
	}
	elapsed := time.Duration(ms-saved.At) * time.Millisecond
	if elapsed <= 0 || elapsed > powerMaxGap {
		return
	}
	if kw < powerMinKw {
		return
	}

	kwh := round3(kw * elapsed.Hours())
	if kwh <= 0 {
		return
	}

	var pv *float64
	if e.Inst != nil {
		if share, ok := PVShareNow(h, e.Inst, &kw, c); ok {
			p := round3(kwh * share)
			pv = &p
		}
	}
	var err error
	if pv != nil {
		err = e.DB.AddChargerKwh(todayStr(), c.Key, kwh, *pv)
	} else {
		// Sin balance no se inventa un reparto: la energía se acumula igual y
		// la FV de ese tramo queda sin dato.
		err = e.DB.AddChargerKwhUnknownPV(todayStr(), c.Key, kwh)
	}
	if err != nil {
		log.Printf("[extensions] addChargerKwh %s: %v", c.Key, err)
	}
}

// PowerPoint is one power sample (kW since the sample is valid).
type PowerPoint struct {
	At time.Time
	Kw float64
}

// PowerKwhByDay integrates a step series of power samples into kWh per local
// day. Each sample holds until the next one (a loader at 2.5 kW produces no
// state changes), so gaps up to powerHistoryMaxGap are integrated. A longer
// gap is a recorder outage and is skipped instead of invented.
func PowerKwhByDay(points []PowerPoint) map[string]float64 {
	out := map[string]float64{}
	for i := 0; i+1 < len(points); i++ {
		a, b := points[i], points[i+1]
		if !b.At.After(a.At) {
			continue
		}
		if b.At.Sub(a.At) > powerHistoryMaxGap {
			continue
		}
		kw := a.Kw
		if kw < powerMinKw {
			continue
		}
		from := a.At
		for from.Before(b.At) {
			next := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location()).AddDate(0, 0, 1)
			if next.After(b.At) {
				next = b.At
			}
			day := from.In(time.Local).Format("2006-01-02")
			out[day] += kw * next.Sub(from).Hours()
			from = next
		}
	}
	for day, v := range out {
		out[day] = round3(v)
	}
	return out
}

// BackfillChargerPower fills the days without a row by integrating the power
// history from the recorder. Only days with samples are written: a hole in the
// history is not a zero.
func (e *Ext) BackfillChargerPower(h HA, c Charger, now time.Time) int {
	if c.EnergyTotalID != "" || c.PowerID == "" {
		return 0
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	// Misma ventana que el contador: el recorder de HAOS no llega más atrás.
	from := today.AddDate(0, 0, -9)
	fromKey := from.Format("2006-01-02")
	toKey := today.AddDate(0, 0, -1).Format("2006-01-02")
	if toKey < fromKey {
		return 0
	}

	missing, err := e.DB.MissingChargerDays(fromKey, toKey, c.Key)
	if err != nil || len(missing) == 0 {
		return 0
	}

	rows, err := h.HistoryDuringPeriod(from.Format(time.RFC3339), today.Format(time.RFC3339), c.PowerID)
	if err != nil || len(rows) == 0 {
		return 0
	}

	points := make([]PowerPoint, 0, len(rows))
	for _, r := range rows {
		ts, err := time.Parse(time.RFC3339, r.LastChanged)
		if err != nil {
			continue
		}
		v := parseFloatStrict(r.State)
		kw := 0.0
		if v != nil {
			kw = *v
			if c.PowerUnit == UnitW {
				kw /= 1000
			}
		}
		if math.IsNaN(kw) || math.IsInf(kw, 0) || kw < 0 {
			kw = 0
		}
		points = append(points, PowerPoint{At: ts, Kw: kw})
	}
	if len(points) == 0 {
		return 0
	}

	perDay := PowerKwhByDay(points)

	needCurves := false
	for _, date := range missing {
		if v, ok := perDay[date]; ok && v > 0 {
			needCurves = true
			break
		}
	}
	pvByDay := map[string]float64{}
	if needCurves && e.Inst != nil {
		pvByDay = e.ChargerPVFromCurves(h, from.Format(time.RFC3339), today.Format(time.RFC3339), c)
	}

	n := 0
	for _, date := range missing {
		v, ok := perDay[date]
		if !ok {
			continue
		}
		var pv *float64
		if p, ok := pvByDay[date]; ok {
			pv = &p
		}
		if err := e.DB.SetChargerKwhIfNull(date, c.Key, v, pv); err == nil {
			n++
		}
	}
	if n > 0 {
		log.Printf("[extensions] cargador %s: %d días desde la potencia (sin contador)", c.Key, n)
	}
	return n
}

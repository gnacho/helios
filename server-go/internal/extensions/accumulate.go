package extensions

import (
	"encoding/json"
	"log"
	"math"
	"sync"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/install"
)

const chargerGlitchKwh = 50.0

// Ext agrupa estado resuelto + BD + topología para el acumulador.
type Ext struct {
	Store *Store
	DB    *db.DB
	Inst  *install.Store

	curveMu    sync.Mutex
	curveCache map[string]curveEntry

	timesMu    sync.Mutex
	timesCache map[string]timesEntry
}

func New(store *Store, database *db.DB, inst *install.Store) *Ext {
	return &Ext{Store: store, DB: database, Inst: inst}
}

func todayStr() string {
	return time.Now().Format("2006-01-02")
}

func counterKey(key string) string { return "charger_counter:" + key }

type counterBase struct {
	Date  string  `json:"date"`
	Total float64 `json:"total"`
}

func (e *Ext) readCounterBase(key string) *counterBase {
	if raw, err := e.DB.KVGet(counterKey(key)); err == nil && raw != "" {
		var cb counterBase
		if err := json.Unmarshal([]byte(raw), &cb); err == nil {
			return &cb
		}
	}
	// Base legacy plana heredada por 'main'.
	if key == "main" {
		if raw, err := e.DB.KVGet("charger_counter"); err == nil && raw != "" {
			var cb counterBase
			if err := json.Unmarshal([]byte(raw), &cb); err == nil {
				return &cb
			}
		}
	}
	return nil
}

func (e *Ext) writeCounterBase(key string, total float64) {
	raw, err := json.Marshal(counterBase{Date: todayStr(), Total: total})
	if err != nil {
		return
	}
	e.DB.KVSet(counterKey(key), string(raw))
}

// AccumulateChargerDaily: tick de acumulación para todos los cargadores.
func (e *Ext) AccumulateChargerDaily(h HA) {
	c := e.Store.Get()
	if !ChargerActive(c) {
		return
	}
	for _, ch := range c.CarCharger.Chargers {
		e.accumulateOne(h, ch)
	}
}

func (e *Ext) accumulateOne(h HA, c Charger) {
	if c.EnergyTotalID == "" {
		return
	}
	totalPtr := divOpt(entityNumOrUndefined(h, c.EnergyTotalID), c.EnergyDivisor)
	if totalPtr == nil {
		return
	}
	total := *totalPtr

	saved := e.readCounterBase(c.Key)
	if saved == nil || saved.Total > total {
		// Primera vez, kv perdido o reset: nueva base sin sumar.
		e.writeCounterBase(c.Key, total)
		return
	}

	delta := total - saved.Total
	if delta > 0 && delta <= chargerGlitchKwh {
		rawPower := entityNumOrUndefined(h, c.PowerID)
		var powerKw *float64
		if rawPower != nil {
			v := *rawPower
			if c.PowerUnit == UnitW {
				v = v / 1000
			}
			powerKw = &v
		}
		pv := delta * PVShareNow(h, e.Inst, powerKw, c)
		if err := e.DB.AddChargerKwh(todayStr(), c.Key, round3(delta), round3(pv)); err != nil {
			log.Printf("[extensions] addChargerKwh %s: %v", c.Key, err)
		}
	} else if delta > chargerGlitchKwh {
		log.Printf("[extensions] cargador %s: salto anomalo %v kWh, ignorado", c.Key, delta)
	}

	if delta != 0 {
		e.writeCounterBase(c.Key, total)
	}
}

type ChargerHistoryRow struct {
	Date  string   `json:"date"`
	Kwh   *float64 `json:"kwh"`
	PvKwh *float64 `json:"pvKwh"`
}

// ChargerHistory devuelve histórico diario y estima la fracción solar cuando falta.
func (e *Ext) ChargerHistory(from, to, key string) ([]ChargerHistoryRow, error) {
	if key == "" {
		key = "main"
	}
	rows, err := e.DB.ChargerHistoryRange(from, to, key)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []ChargerHistoryRow{}, nil
	}

	daily, err := e.DB.ProductionConsumptionRange(from, to)
	if err != nil {
		return nil, err
	}

	c := e.Store.Get()
	inHouse := true
	if ch := GetChargerByKey(c, key); ch != nil {
		inHouse = ch.ChargerInHouseMeters
	}

	out := make([]ChargerHistoryRow, 0, len(rows))
	for _, r := range rows {
		kwh := r.Kwh
		pv := r.PvKwh
		if kwh != nil && *kwh > 0 && pv == nil {
			d := daily[r.Date]
			other := d[1] // consumption
			if inHouse {
				other -= *kwh
			}
			if other < 0 {
				other = 0
			}
			val := math.Max(0, math.Min(*kwh, d[0]-other))
			pv = &val
		}
		out = append(out, ChargerHistoryRow{Date: r.Date, Kwh: kwh, PvKwh: pv})
	}
	return out, nil
}

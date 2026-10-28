package solar

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/ha"
	"helios-server/internal/install"
)

type DayPoint struct {
	T            int                `json:"t"`
	Label        string             `json:"label"`
	Solis        float64            `json:"solis"`
	Fox          float64            `json:"fox"`
	Inverters    map[string]float64 `json:"inverters"`
	Production   float64            `json:"production"`
	Consumption  float64            `json:"consumption"`
	BatteryPower float64            `json:"batteryPower"`
	Soc          float64            `json:"soc"`
	Grid         *float64           `json:"grid"`
}

type DaySeries struct {
	Points    []DayPoint `json:"points"`
	Estimated bool       `json:"estimated"`
}

const (
	bucketHours   = 5.0 / 60.0
	bucketMinutes = 5
	// kwPerBucket convierte el kWh de un bucket de 5 minutos a kW.
	kwPerBucket = 60.0 / bucketMinutes
)

// BuildDaySeries replica buildDaySeries() de solar.ts.
// stats: {sensorId: [{start, mean, sum}]} con buckets de 5 minutos. El consumo y
// la red salen de la suma acumulada de los contadores de energía (mismo criterio
// que los días consolidados) y las pinzas quedan como respaldo y detalle.
func (s *Solar) BuildDaySeries(stats ha.Stats, dateStr string, liveTargets map[string]float64) DaySeries {
	t := s.Store.Get()

	// Índice minuto-del-día -> valores por sensor.
	buckets := map[int]map[string]float64{}
	ids := make([]string, 0)
	for _, inv := range t.Inverters {
		if inv.PowerID != "" {
			ids = append(ids, inv.PowerID)
		}
		if inv.BackupPowerID != "" {
			ids = append(ids, inv.BackupPowerID)
		}
	}
	ids = append(ids, t.Consumption.PowerIDs...)
	if t.Battery.Enabled {
		if t.Battery.PowerID != "" {
			ids = append(ids, t.Battery.PowerID)
		}
		if t.Battery.SocID != "" {
			ids = append(ids, t.Battery.SocID)
		}
	}

	for _, id := range ids {
		for _, row := range stats[id] {
			if row.Mean == nil {
				continue
			}
			min, ok := bucketMinute(row.Start.Time)
			if !ok {
				continue
			}
			if buckets[min] == nil {
				buckets[min] = map[string]float64{}
			}
			buckets[min][id] = *row.Mean
		}
	}

	minutes := make([]int, 0, len(buckets))
	for m := range buckets {
		minutes = append(minutes, m)
	}
	sort.Ints(minutes)

	// Contadores del balance: el consumo y la red salen de su suma acumulada,
	// con el mismo criterio que la consolidación diaria. Las pinzas quedan como
	// detalle por circuito y solo se usan si el contador no da un bucket válido.
	e := s.Store.EnergyEntities()
	sums := map[string]map[int]float64{}
	for _, id := range []string{e.Consumption, e.GridImport, e.GridExport} {
		if id == "" {
			continue
		}
		for _, row := range stats[id] {
			if row.Sum == nil {
				continue
			}
			m, ok := bucketMinute(row.Start.Time)
			if !ok {
				continue
			}
			if sums[id] == nil {
				sums[id] = map[int]float64{}
			}
			sums[id][m] = *row.Sum
		}
	}

	// counterDelta devuelve el kWh del bucket usando la lectura del bucket
	// inmediatamente anterior. Un contador que retrocede (reset) da ok=false.
	counterDelta := func(id string, min int) (float64, bool) {
		rows := sums[id]
		if rows == nil {
			return 0, false
		}
		cur, ok := rows[min]
		if !ok {
			return 0, false
		}
		prev, ok := rows[min-bucketMinutes]
		if !ok {
			return 0, false
		}
		d := cur - prev
		if d < 0 {
			return 0, false
		}
		return d, true
	}

	consDiv := 1.0
	if t.Consumption.PowerUnit == install.UnitW {
		consDiv = 1000
	}
	consMeterIDs := t.Consumption.PowerIDs

	points := make([]DayPoint, 0, len(minutes))
	measuredGrid := make([]bool, 0, len(minutes))
	var prevSoc *float64
	backupIdx := map[string][]int{}
	consGapIdx := make([]int, 0)

	for pointIdx, min := range minutes {
		b := buckets[min]
		production := 0.0
		perInv := make(map[string]float64, len(t.Inverters))

		for _, inv := range t.Inverters {
			kw := 0.0
			if raw, ok := b[inv.PowerID]; ok && inv.PowerID != "" {
				kw = raw
				if inv.PowerUnit == install.UnitW {
					kw = raw / 1000
				}
			} else if inv.BackupPowerID != "" {
				if backRaw, ok := b[inv.BackupPowerID]; ok {
					kw = backRaw
					unit := inv.BackupPowerUnit
					if unit == "" {
						unit = install.UnitKW
					}
					if unit == install.UnitW {
						kw = backRaw / 1000
					}
					backupIdx[inv.Key] = append(backupIdx[inv.Key], pointIdx)
				}
			}
			if kw < 0.02 {
				kw = 0
			}
			perInv[inv.Key] = kw
			production += kw
		}

		// Consumo del bucket: contador del balance si lo hay, si no la suma de
		// las pinzas.
		consumption := 0.0
		consFromCounters := false
		if d, ok := counterDelta(e.Consumption, min); ok {
			consumption = d * kwPerBucket
			consFromCounters = true
		} else {
			for _, id := range consMeterIDs {
				if v, ok := b[id]; ok {
					consumption += v / consDiv
				}
			}
		}

		// Red del bucket: contadores de importación y exportación.
		gridFromCounters := false
		grid := 0.0
		switch {
		case e.GridImport != "" && e.GridExport != "":
			imp, okImp := counterDelta(e.GridImport, min)
			exp, okExp := counterDelta(e.GridExport, min)
			if okImp && okExp {
				grid, gridFromCounters = (imp-exp)*kwPerBucket, true
			}
		case e.GridImport != "":
			if imp, ok := counterDelta(e.GridImport, min); ok {
				grid, gridFromCounters = imp*kwPerBucket, true
			}
		case e.GridExport != "":
			if exp, ok := counterDelta(e.GridExport, min); ok {
				grid, gridFromCounters = -exp*kwPerBucket, true
			}
		}

		// Hueco de consumo: el bucket existe pero ningún medidor tiene fila.
		consGap := false
		if !consFromCounters && len(consMeterIDs) > 0 {
			consGap = true
			for _, id := range consMeterIDs {
				if _, ok := b[id]; ok {
					consGap = false
					break
				}
			}
		}
		if consGap {
			consGapIdx = append(consGapIdx, pointIdx)
		}

		batteryPower := 0.0
		if t.Battery.Enabled {
			var batMag, socPrev float64
			if v, ok := b[t.Battery.PowerID]; ok {
				batMag = v
			}
			if v, ok := b[t.Battery.SocID]; ok {
				socPrev = v
			}
			if prevSoc != nil && batMag > 0.01 {
				socDelta := socPrev - *prevSoc
				if socDelta > 0.02 {
					batteryPower = batMag
				} else if socDelta < -0.02 {
					batteryPower = -batMag
				}
			}
			if socPrev > 0 {
				v := socPrev
				prevSoc = &v
			}
		}

		soc := 0.0
		if t.Battery.Enabled {
			if v, ok := b[t.Battery.SocID]; ok {
				soc = v
			}
		}

		var gridPtr *float64
		measured := gridFromCounters
		if measured {
			v := Round3(grid)
			gridPtr = &v
		} else {
			derivedGrid := consumption - production + batteryPower
			if math.Abs(derivedGrid) < 0.03 {
				derivedGrid = 0
			}
			consMissing := false
			if !consFromCounters {
				for _, id := range consMeterIDs {
					if _, ok := b[id]; !ok {
						consMissing = true
						break
					}
				}
			}
			batMissing := false
			if t.Battery.Enabled && t.Battery.PowerID != "" {
				if _, ok := b[t.Battery.PowerID]; !ok {
					batMissing = true
				}
			}
			if !consMissing && !batMissing {
				v := Round3(derivedGrid)
				gridPtr = &v
			}
		}

		inv0, inv1 := 0.0, 0.0
		if len(t.Inverters) > 0 {
			inv0 = perInv[t.Inverters[0].Key]
		}
		if len(t.Inverters) > 1 {
			inv1 = perInv[t.Inverters[1].Key]
		}

		roundedInv := make(map[string]float64, len(perInv))
		for k, v := range perInv {
			roundedInv[k] = Round3(v)
		}

		points = append(points, DayPoint{
			T:            min,
			Label:        fmt.Sprintf("%02d:%02d", min/60, min%60),
			Solis:        Round3(inv0),
			Fox:          Round3(inv1),
			Inverters:    roundedInv,
			Production:   Round3(production),
			Consumption:  Round3(consumption),
			BatteryPower: Round3(batteryPower),
			Soc:          Round1(soc),
			Grid:         gridPtr,
		})
		measuredGrid = append(measuredGrid, measured)
	}

	// setGrid recalcula el grid derivado de un punto (consumo - producción +
	// batería) salvo que venga medido por los contadores del balance.
	setGrid := func(i int) {
		if i < len(measuredGrid) && measuredGrid[i] {
			return
		}
		p := &points[i]
		grid := p.Consumption - p.Production + p.BatteryPower
		if math.Abs(grid) < 0.03 {
			grid = 0
		}
		v := Round3(grid)
		p.Grid = &v
	}

	dayKey := dateStr
	if dayKey == "" {
		dayKey = TodayStr()
	}

	estimated := false

	// ---- Pasada 1: escalado del tramo rellenado por respaldo ----
	if len(backupIdx) > 0 && len(points) > 0 {
		estimated = true
		var row *db.DailyRow
		if s.DB != nil {
			row, _ = s.DB.DailyRowByDate(dayKey)
		}

		for invIdx, inv := range t.Inverters {
			idxs := backupIdx[inv.Key]
			if len(idxs) == 0 {
				continue
			}
			target := 0.0
			if v, ok := liveTargets[inv.Key]; ok && v > 0 {
				target = v
			}
			if target <= 0 && row != nil {
				if row.InvertersKwh != nil && *row.InvertersKwh != "" {
					var ik map[string]float64
					if err := json.Unmarshal([]byte(*row.InvertersKwh), &ik); err == nil {
						target = ik[inv.Key]
					}
				}
				if target <= 0 {
					switch invIdx {
					case 0:
						if row.SolisKwh != nil {
							target = *row.SolisKwh
						}
					case 1:
						if row.FoxKwh != nil {
							target = *row.FoxKwh
						}
					}
				}
			}
			if target <= 0 {
				continue
			}

			idxSet := make(map[int]struct{}, len(idxs))
			for _, i := range idxs {
				idxSet[i] = struct{}{}
			}

			sumReal, sumBackup := 0.0, 0.0
			for i, p := range points {
				kw := p.Inverters[inv.Key]
				if _, ok := idxSet[i]; ok {
					sumBackup += kw
				} else {
					sumReal += kw
				}
			}
			sumReal *= bucketHours
			sumBackup *= bucketHours
			if sumBackup <= 0 {
				continue
			}
			gap := target - (sumReal + sumBackup)
			if gap < 0.05 {
				continue
			}
			factor := (target - sumReal) / sumBackup

			compat := ""
			if invIdx == 0 {
				compat = "solis"
			} else if invIdx == 1 {
				compat = "fox"
			}

			for _, i := range idxs {
				p := &points[i]
				v := p.Inverters[inv.Key] * factor
				p.Inverters[inv.Key] = Round3(v)
				if compat == "solis" {
					p.Solis = p.Inverters[inv.Key]
				} else if compat == "fox" {
					p.Fox = p.Inverters[inv.Key]
				}
				prod := 0.0
				for _, x := range t.Inverters {
					prod += p.Inverters[x.Key]
				}
				p.Production = Round3(prod)
				setGrid(i)
			}
		}
	}

	// ---- Pasada 2: relleno del hueco de consumo con la forma de ayer ----
	if s.DB != nil && len(consGapIdx) > 0 && len(points) > 0 {
		consTarget := 0.0
		if rowC, _ := s.DB.DailyRowByDate(dayKey); rowC != nil && rowC.ConsumptionKwh != nil {
			if *rowC.ConsumptionKwh > 0 {
				consTarget = *rowC.ConsumptionKwh
			}
		}

		day, err := time.ParseInLocation("2006-01-02", dayKey, time.Local)
		if err == nil {
			prevKey := day.AddDate(0, 0, -1).Format("2006-01-02")
			if prevRow, _ := s.DB.GetDaySeriesRow(prevKey); prevRow != nil {
				var prevPts []DayPoint
				if err := json.Unmarshal([]byte(prevRow.PointsJSON), &prevPts); err == nil {
					prevByT := make(map[int]float64, len(prevPts))
					for _, p := range prevPts {
						prevByT[p.T] = p.Consumption
					}
					gapSet := make(map[int]struct{}, len(consGapIdx))
					for _, i := range consGapIdx {
						gapSet[i] = struct{}{}
					}

					var factor *float64
					if consTarget > 0 {
						realSum, patSum := 0.0, 0.0
						for i, p := range points {
							if _, ok := gapSet[i]; ok {
								patSum += prevByT[p.T]
							} else {
								realSum += p.Consumption
							}
						}
						realSum *= bucketHours
						patSum *= bucketHours
						deficit := consTarget - realSum
						if patSum > 0.01 && deficit > 0.05 {
							f := deficit / patSum
							factor = &f
						}
					}
					f := 1.0
					if factor != nil && *factor > 0 {
						f = *factor
					}

					for _, i := range consGapIdx {
						p := &points[i]
						pat, ok := prevByT[p.T]
						if !ok {
							continue
						}
						p.Consumption = Round3(pat * f)
						setGrid(i)
					}
					estimated = true
				}
			}
		}
	}

	// ---- Pasada 3: heurística Solis sin statistics → escalar desde Fox ----
	if s.DB != nil && len(points) > 0 && len(t.Inverters) >= 2 {
		inv0Key := t.Inverters[0].Key
		inv1Key := t.Inverters[1].Key

		sumInv := func(key string) float64 {
			total := 0.0
			for _, p := range points {
				total += p.Inverters[key] * bucketHours
			}
			return total
		}

		sum0 := sumInv(inv0Key)
		sum1 := sumInv(inv1Key)

		if sum0 < 0.5 && sum1 > 1 {
			if row, _ := s.DB.DailyRowByDate(dayKey); row != nil {
				solis := 0.0
				if row.SolisKwh != nil {
					solis = *row.SolisKwh
				}
				if solis > 1 && sum1 > 1 {
					ratio := solis / sum1
					for i := range points {
						p := &points[i]
						p.Inverters[inv0Key] = Round3(p.Inverters[inv1Key] * ratio)
						p.Solis = p.Inverters[inv0Key]
						p.Production = Round3(p.Inverters[inv0Key] + p.Inverters[inv1Key])
						setGrid(i)
					}
					estimated = true
				}
			}
		}
	}

	return DaySeries{Points: points, Estimated: estimated}
}

// bucketMinute convierte el inicio del bucket a minuto del día en hora LOCAL
// (equivalente a Date.getHours()/getMinutes() en el original).
func bucketMinute(t time.Time) (int, bool) {
	if t.IsZero() {
		return 0, false
	}
	local := t.Local()
	return local.Hour()*60 + local.Minute(), true
}

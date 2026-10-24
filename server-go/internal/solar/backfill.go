package solar

import (
	"encoding/json"
	"math"
	"sort"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/ha"
	"helios-server/internal/install"
)

// dateKey formatea una fecha como YYYY-MM-DD en hora local.
func dateKey(t time.Time) string {
	return t.In(time.Local).Format("2006-01-02")
}

// gapBetween devuelve los días intermedios sin fila entre dos claves.
func gapBetween(prevKey, key string) int {
	if prevKey == "" || prevKey >= key {
		return 0
	}
	a, err1 := time.ParseInLocation("2006-01-02", prevKey, time.Local)
	b, err2 := time.ParseInLocation("2006-01-02", key, time.Local)
	if err1 != nil || err2 != nil {
		return 0
	}
	days := int(math.Round(b.Sub(a).Hours() / 24))
	return days - 1
}

// BackfillHistory reconstruye la tabla daily desde las statistics diarias de HAOS.
// Devuelve el número de filas escritas.
// BackfillHistory recalcula todo el histórico diario desde 2024-01-01.
func (s *Solar) BackfillHistory(haReader HAReader) (int, error) {
	return s.backfillHistoryFrom(haReader, time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local))
}

// ConsolidateDaily rellena los días que falten en `daily` desde el último
// consolidado (idempotente: el upsert reescribe los mismos valores). El
// servicio no tenía ningún job que escribiera la tabla, así que solo se llenaba
// con el backfill inicial y el histórico se quedaba parado en esa fecha (bug
// real: 22 y 23 de octubre salían como "sin datos").
func (s *Solar) ConsolidateDaily(haReader HAReader) (int, error) {
	empty, err := s.DB.DailyEmpty()
	if err != nil {
		return 0, err
	}
	if empty {
		return s.BackfillHistory(haReader)
	}
	last, err := s.DB.MaxDailyDate()
	if err != nil || last == "" {
		return 0, err
	}
	d, err := time.ParseInLocation("2006-01-02", last, time.Local)
	if err != nil {
		return 0, err
	}
	// Dos días de margen: el reparto de huecos necesita contexto y el upsert
	// es idempotente.
	start := d.AddDate(0, 0, -2)
	return s.backfillHistoryFrom(haReader, start)
}

func (s *Solar) backfillHistoryFrom(haReader HAReader, start time.Time) (int, error) {
	t := s.Store.Get()

	now := time.Now()
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, 1)
	queryStart := start.AddDate(0, 0, -2)

	srcs, mapping := s.Store.DeepSourceDailyKeys()

	idSet := map[string]struct{}{}
	for _, src := range srcs {
		for _, id := range src.IDs {
			idSet[id] = struct{}{}
		}
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	stats, err := haReader.StatisticsDuringPeriod(
		queryStart.Format(time.RFC3339), end.Format(time.RFC3339),
		ids, "day", []string{"state", "sum"},
	)
	if err != nil {
		return 0, err
	}

	minKey := dateKey(start)
	today := TodayStr()

	// byDate: fecha -> columna -> valor (se queda con el máximo entre fuentes).
	byDate := map[string]map[string]float64{}
	ensure := func(key string) map[string]float64 {
		if byDate[key] == nil {
			byDate[key] = map[string]float64{}
		}
		return byDate[key]
	}

	for field, src := range srcs {
		perIDDaily := map[string]map[string]float64{}

		for _, id := range src.IDs {
			rows := make([]ha.StatsRow, 0)
			for _, r := range stats[id] {
				var v *float64
				if src.Acc == install.AccSum {
					v = r.Sum
				} else {
					v = r.State
				}
				if v != nil {
					rows = append(rows, r)
				}
			}
			// Orden cronológico (el original no ordenaba; HA ya entrega ordenado).
			sort.SliceStable(rows, func(i, j int) bool { return rows[i].Start.Before(rows[j].Start.Time) })

			daily := map[string]float64{}
			prev := 0.0
			prevKey := ""

			for _, r := range rows {
				key := rowDayKey(r.Start.Time)
				var raw float64
				if src.Acc == install.AccSum {
					if r.Sum != nil {
						raw = *r.Sum
					}
				} else if r.State != nil {
					raw = *r.State
				}

				if key == today {
					prev = raw
					prevKey = key
					continue
				}

				if src.Acc == install.AccSum {
					value := math.Max(0, raw-prev)
					prev = raw
					prevKey = key
					if key < minKey {
						continue
					}
					daily[key] = value
					continue
				}

				// acc 'state': acumulador creciente con reparto de huecos.
				outKey := mapping[field]
				var glitch float64
				if outKey != "" {
					for _, inv := range t.Inverters {
						if inv.Key == outKey {
							glitch = inv.GlitchOffsets[key]
							break
						}
					}
				}
				delta := math.Max(0, raw-prev-glitch)
				gapDays := gapBetween(prevKey, key)
				if prevKey != "" && gapDays > 0 {
					share := delta / float64(gapDays+1)
					anchor, err := time.ParseInLocation("2006-01-02", key, time.Local)
					if err == nil {
						for d := 1; d <= gapDays; d++ {
							gk := dateKey(anchor.AddDate(0, 0, -d))
							if gk >= minKey && gk != today {
								if share > daily[gk] {
									daily[gk] = share
								}
							}
						}
					}
					delta = share
				}
				prev = raw
				prevKey = key
				if key < minKey {
					continue
				}
				if delta > daily[key] {
					daily[key] = delta
				}
			}
			perIDDaily[id] = daily
		}

		allKeys := map[string]struct{}{}
		for _, daily := range perIDDaily {
			for k := range daily {
				allKeys[k] = struct{}{}
			}
		}

		outKey := mapping[field]
		if outKey == "" {
			outKey = field
		}

		for key := range allKeys {
			if src.RequireAll {
				all := true
				for _, daily := range perIDDaily {
					if _, ok := daily[key]; !ok {
						all = false
						break
					}
				}
				if !all {
					continue
				}
			}

			value := 0.0
			for _, daily := range perIDDaily {
				value += daily[key]
			}
			if src.Cap > 0 && value > src.Cap {
				value = src.Cap
			}

			target := ensure(key)
			cur, exists := target[outKey]
			if !exists {
				target[outKey] = value
			} else if value > cur {
				target[outKey] = value
			}
		}
	}

	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	n := 0
	colForIndex := func(i int) string {
		switch i {
		case 0:
			return "solis"
		case 1:
			return "fox"
		default:
			return "inv" + itoa(i+1)
		}
	}

	for _, date := range dates {
		f := byDate[date]

		perInv := map[string]float64{}
		production := 0.0
		for k := range srcs {
			if len(k) < 3 || k[:3] != "inv" {
				continue
			}
			col := mapping[k]
			perInv[col] = f[col]
			production += perInv[col]
		}

		medidores := f["consumption"]
		derived := production + f["gridImport"] - f["gridExport"] - f["batCharge"] + f["batDischarge"]
		consumption := 0.0
		if medidores > 0 {
			consumption = math.Min(100, math.Max(medidores, derived))
		}

		invertersKwh := map[string]float64{}
		for i, inv := range t.Inverters {
			invertersKwh[inv.Key] = Round2(perInv[colForIndex(i)])
		}
		invJSON := db.InvertersJSON(invertersKwh)

		row := db.DailyRow{
			Date:                 date,
			ProductionKwh:        fptr(Round2(production)),
			ConsumptionKwh:       fptr(Round2(consumption)),
			GridImportKwh:        fptr(Round2(f["gridImport"])),
			GridExportKwh:        fptr(Round2(f["gridExport"])),
			BatteryChargedKwh:    fptr(Round2(f["batCharge"])),
			BatteryDischargedKwh: fptr(Round2(f["batDischarge"])),
			SolisKwh:             fptr(Round2(perInv["solis"])),
			FoxKwh:               fptr(Round2(f["fox"])),
			InvertersKwh:         invJSON,
		}
		if err := s.DB.UpsertDaily(row); err != nil {
			return n, err
		}
		n++
	}

	return n, nil
}

// MaybeBackfill lanza el backfill solo si daily está vacío.
func (s *Solar) MaybeBackfill(haReader HAReader) (ran bool, rows int, err error) {
	empty, err := s.DB.DailyEmpty()
	if err != nil {
		return false, 0, err
	}
	if !empty {
		return false, 0, nil
	}
	n, err := s.BackfillHistory(haReader)
	if err != nil {
		return false, 0, err
	}
	return true, n, nil
}

// BackfillDaySeries rellena day_series para los días con datos disponibles.
// backup es opcional (stats de un HAOS antiguo).
func (s *Solar) BackfillDaySeries(haReader HAReader, backup ha.Stats) (ran bool, rows int) {
	firstDaily, err := s.DB.MinDailyDate()
	if err != nil || firstDaily == "" {
		return false, 0
	}

	doneList, _ := s.DB.DaySeriesDates()
	done := map[string]struct{}{}
	for _, k := range doneList {
		done[k] = struct{}{}
	}

	now := time.Now()
	haosFloor := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -15)
	yesterdayKey := dateKey(now.AddDate(0, 0, -1))

	cursor, err := time.ParseInLocation("2006-01-02", firstDaily, time.Local)
	if err != nil {
		return false, 0
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, 1)

	for _, key := range eachDayKey(cursor, end) {
		if _, ok := done[key]; ok && key != yesterdayKey {
			continue
		}
		keyDate, err := time.ParseInLocation("2006-01-02", key, time.Local)
		if err != nil {
			continue
		}

		fromBackup := len(sliceStatsByDay(backup, key)) > 0
		if !fromBackup && keyDate.Before(haosFloor) {
			continue
		}

		var points []DayPoint
		estimated := false
		source := "haos"

		if fromBackup {
			res := s.BuildDaySeries(sliceStatsByDay(backup, key), key, nil)
			points = res.Points
			estimated = res.Estimated
			source = "backup"
		}
		if len(points) == 0 {
			res, err := s.getDaySeriesUncached(haReader, key)
			if err != nil {
				continue
			}
			points = res.Points
			estimated = res.Estimated
			source = "haos"
		}
		if len(points) == 0 {
			continue
		}

		raw, err := json.Marshal(points)
		if err != nil {
			continue
		}
		if err := s.DB.UpsertDaySeries(key, string(raw), estimated, source, time.Now().UnixMilli()); err != nil {
			continue
		}
		done[key] = struct{}{}
		ran = true
		rows++
	}
	return ran, rows
}

// rowDayKey extrae la fecha (local) del start de una fila de statistics.
func rowDayKey(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return dateKey(t)
}

func fptr(v float64) *float64 { return &v }

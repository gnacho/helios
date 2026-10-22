package solar

import (
	"encoding/json"
	"math"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/ha"
)

type DayKpis struct {
	Kpis
	Soc                float64 `json:"soc"`
	PeakProductionKw   float64 `json:"peakProductionKw"`
	PeakAt             int     `json:"peakAt"`
	AutoconsumoPct     float64 `json:"autoconsumoPct"`
	AutosuficienciaPct float64 `json:"autosuficienciaPct"`
	AhorroEur          float64 `json:"ahorroEur"`
	Co2EvitadoKg       float64 `json:"co2EvitadoKg"`
}

// PriceConfig son los parámetros económicos de decorateKpis.
type PriceConfig struct {
	PriceImport float64
	PriceExport float64
	Co2PerKwh   float64
}

func (s *Solar) decorateKpis(k Kpis, soc, peakKw float64, peakAt int, pc PriceConfig) DayKpis {
	autoconsumo := 0.0
	if k.ProductionKwh > 0 {
		autoconsumo = math.Min(100, ((k.ProductionKwh-k.GridExportKwh)/k.ProductionKwh)*100)
	}
	autosuf := 0.0
	if k.ConsumptionKwh > 0 {
		autosuf = math.Min(100, ((k.ConsumptionKwh-k.GridImportKwh)/k.ConsumptionKwh)*100)
	}
	directFv := math.Max(0, math.Min(k.ProductionKwh, k.ConsumptionKwh))
	ahorro := (directFv+k.BatteryDischargedKwh)*pc.PriceImport + k.GridExportKwh*pc.PriceExport

	return DayKpis{
		Kpis:               k,
		Soc:                soc,
		PeakProductionKw:   Round3(peakKw),
		PeakAt:             peakAt,
		AutoconsumoPct:     Round1(autoconsumo),
		AutosuficienciaPct: Round1(autosuf),
		AhorroEur:          Round2(ahorro),
		Co2EvitadoKg:       Round2(k.ProductionKwh * pc.Co2PerKwh),
	}
}

// GetDaySeries devuelve la curva del día. Días pasados se sirven de la BD si
// existen; el resto se computa desde HAOS y se persiste.
func (s *Solar) GetDaySeries(haReader HAReader, dateStr string) (DaySeries, error) {
	key := dateStr
	if key == "" {
		key = TodayStr()
	}
	isToday := key == TodayStr()

	if s.DB != nil && !isToday {
		if row, err := s.DB.GetDaySeriesRow(key); err == nil && row != nil {
			var pts []DayPoint
			if err := json.Unmarshal([]byte(row.PointsJSON), &pts); err == nil {
				return DaySeries{Points: pts, Estimated: row.Estimated == 1}, nil
			}
		}
	}

	result, err := s.getDaySeriesUncached(haReader, dateStr)
	if err != nil {
		return DaySeries{}, err
	}

	if s.DB != nil && len(result.Points) > 0 {
		if raw, err := json.Marshal(result.Points); err == nil {
			source := "haos"
			if isToday {
				source = "live"
			}
			s.DB.UpsertDaySeries(key, string(raw), result.Estimated, source, time.Now().UnixMilli())
		}
	}
	return result, nil
}

func (s *Solar) getDaySeriesUncached(haReader HAReader, dateStr string) (DaySeries, error) {
	t := s.Store.Get()

	day := time.Now()
	if dateStr != "" {
		parsed, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
		if err != nil {
			return DaySeries{}, err
		}
		day = parsed
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	end := start.AddDate(0, 0, 1)

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

	stats, err := haReader.StatisticsDuringPeriod(
		start.Format(time.RFC3339), end.Format(time.RFC3339),
		ids, "5minute", []string{"mean"},
	)
	if err != nil {
		return DaySeries{}, err
	}

	liveTargets := map[string]float64{}
	isToday := dateStr == "" || dateStr == TodayStr()
	if isToday {
		for _, inv := range t.Inverters {
			if inv.BackupPowerID != "" && inv.EnergyID != "" {
				liveTargets[inv.Key] = s.entityNum(haReader, inv.EnergyID, 1)
			}
		}
	}

	return s.BuildDaySeries(stats, dateStr, liveTargets), nil
}

// GetKpis devuelve los KPIs del día (hoy en vivo, días pasados de daily o HAOS).
func (s *Solar) GetKpis(haReader HAReader, dateStr string, pc PriceConfig) (DayKpis, error) {
	today := TodayStr()

	if dateStr == "" || dateStr == today {
		baseline, err := s.EnsureConsumptionBaseline(haReader)
		if err != nil {
			baseline = nil
		}
		consToday := s.ConsumptionTodayFromCounters(haReader, baseline)
		k := s.ComputeTodayKpis(haReader, consToday)
		live := s.ComputeLive(haReader)

		series, err := s.GetDaySeries(haReader, today)
		if err != nil {
			series = DaySeries{}
		}
		peakKw := 0.0
		peakAt := 14 * 60
		for _, p := range series.Points {
			if p.Production > peakKw {
				peakKw = p.Production
				peakAt = p.T
			}
		}
		return s.decorateKpis(k, live.Soc, peakKw, peakAt, pc), nil
	}

	if _, err := time.ParseInLocation("2006-01-02", dateStr, time.Local); err != nil {
		return DayKpis{}, err
	}

	var row *db.DailyRow
	if s.DB != nil {
		row, _ = s.DB.DailyRowByDate(dateStr)
	}

	series, err := s.GetDaySeries(haReader, dateStr)
	if err != nil {
		series = DaySeries{}
	}
	peakKw := 0.0
	peakAt := 14 * 60
	soc := 0.0
	for _, p := range series.Points {
		if p.Production > peakKw {
			peakKw = p.Production
			peakAt = p.T
		}
		if p.Soc > 0 {
			soc = p.Soc
		}
	}

	if row != nil {
		k := Kpis{
			ProductionKwh:        Round2(deref(row.ProductionKwh)),
			SolisKwh:             Round2(deref(row.SolisKwh)),
			FoxKwh:               Round2(deref(row.FoxKwh)),
			InvertersKwh:         parseInvertersKwh(row),
			ConsumptionKwh:       Round2(deref(row.ConsumptionKwh)),
			GridImportKwh:        Round2(deref(row.GridImportKwh)),
			GridExportKwh:        Round2(deref(row.GridExportKwh)),
			BatteryChargedKwh:    Round2(deref(row.BatteryChargedKwh)),
			BatteryDischargedKwh: Round2(deref(row.BatteryDischargedKwh)),
		}
		return s.decorateKpis(k, soc, peakKw, peakAt, pc), nil
	}

	// Sin fila daily: calcular desde statistics diarias.
	day, _ := time.ParseInLocation("2006-01-02", dateStr, time.Local)
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	end := start.AddDate(0, 0, 1)

	E := s.Store.EnergyEntities()
	ids := []string{E.Solis, E.Fox, E.Consumption, E.GridImport, E.GridExport, E.BatCharge, E.BatDischarge}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			filtered = append(filtered, id)
		}
	}

	stats, err := haReader.StatisticsDuringPeriod(
		start.Format(time.RFC3339), end.Format(time.RFC3339),
		filtered, "day", []string{"state"},
	)
	if err != nil {
		return DayKpis{}, err
	}

	change := func(id string) float64 {
		total := 0.0
		for _, r := range stats[id] {
			if r.State != nil {
				total += *r.State
			}
		}
		return total
	}

	inv0 := change(E.Solis)
	inv1 := change(E.Fox)
	invKwh := make(map[string]float64)
	for i, inv := range s.Store.Get().Inverters {
		switch i {
		case 0:
			invKwh[inv.Key] = inv0
		case 1:
			invKwh[inv.Key] = inv1
		default:
			invKwh[inv.Key] = change(inv.EnergyID)
		}
	}

	k := Kpis{
		ProductionKwh:        Round2(inv0 + inv1),
		SolisKwh:             Round2(inv0),
		FoxKwh:               Round2(inv1),
		InvertersKwh:         invKwh,
		ConsumptionKwh:       Round2(change(E.Consumption)),
		GridImportKwh:        Round2(change(E.GridImport)),
		GridExportKwh:        Round2(change(E.GridExport)),
		BatteryChargedKwh:    Round2(change(E.BatCharge)),
		BatteryDischargedKwh: Round2(change(E.BatDischarge)),
	}
	return s.decorateKpis(k, soc, peakKw, peakAt, pc), nil
}

func parseInvertersKwh(row *db.DailyRow) map[string]float64 {
	if row.InvertersKwh != nil && *row.InvertersKwh != "" {
		var m map[string]float64
		if err := json.Unmarshal([]byte(*row.InvertersKwh), &m); err == nil {
			return m
		}
	}
	out := map[string]float64{}
	if row.SolisKwh != nil {
		out["solis"] = *row.SolisKwh
	}
	if row.FoxKwh != nil {
		out["fox"] = *row.FoxKwh
	}
	return out
}

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// sliceStatsByDay filtra stats al día indicado.
func sliceStatsByDay(stats ha.Stats, key string) ha.Stats {
	day, err := time.ParseInLocation("2006-01-02", key, time.Local)
	if err != nil {
		return ha.Stats{}
	}
	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local)
	dayEnd := dayStart.AddDate(0, 0, 1)

	out := ha.Stats{}
	for sid, rows := range stats {
		var inDay []ha.StatsRow
		for _, r := range rows {
			lt := r.Start.Local()
			if !lt.Before(dayStart) && lt.Before(dayEnd) {
				inDay = append(inDay, r)
			}
		}
		if len(inDay) > 0 {
			out[sid] = inDay
		}
	}
	return out
}

// eachDayKey itera claves YYYY-MM-DD entre start (incluido) y end (excluido).
func eachDayKey(start, end time.Time) []string {
	cur := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	out := []string{}
	for cur.Before(end) {
		out = append(out, cur.Format("2006-01-02"))
		cur = cur.AddDate(0, 0, 1)
	}
	return out
}

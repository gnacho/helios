package web

import (
	"encoding/json"
	"math"
	"time"

	"helios-server/internal/db"
)

// Daily history helpers for the Inversores and Batería pages. The React front
// loads /api/solar/history into HistoryDay rows; the templ pages read the same
// daily table in process (DB.DailyRange) and aggregate here.

func derefF(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// dailyInvKwh is the per-inverter kWh of one daily row: the inverters_kwh JSON
// column first, falling back to the legacy solis/fox columns.
func dailyInvKwh(row db.DailyRow, key string) float64 {
	if row.InvertersKwh != nil && *row.InvertersKwh != "" {
		var m map[string]float64
		if err := json.Unmarshal([]byte(*row.InvertersKwh), &m); err == nil {
			if v, ok := m[key]; ok {
				return v
			}
		}
	}
	switch key {
	case "solis":
		return derefF(row.SolisKwh)
	case "fox":
		return derefF(row.FoxKwh)
	}
	return 0
}

// kwhSum adds the per-inverter kWh across rows.
func kwhSum(rows []db.DailyRow, key string) float64 {
	var sum float64
	for _, r := range rows {
		sum += dailyInvKwh(r, key)
	}
	return sum
}

// monthRows keeps the rows of the current calendar month (including today).
func monthRows(rows []db.DailyRow, now time.Time) []db.DailyRow {
	prefix := now.Format("2006-01") + "-"
	out := make([]db.DailyRow, 0, len(rows))
	for _, r := range rows {
		if len(r.Date) >= 7 && r.Date[:8] == prefix {
			out = append(out, r)
		}
	}
	return out
}

// lastN returns up to n trailing rows (ordered ascending).
func lastN(rows []db.DailyRow, n int) []db.DailyRow {
	if len(rows) <= n {
		return rows
	}
	return rows[len(rows)-n:]
}

// dailyAutoconsumo mirrors the autoconsumoPct computed by handleSolarHistory:
// (production - gridExport) / production, clamped to 100.
func dailyAutoconsumo(row db.DailyRow) float64 {
	production := derefF(row.ProductionKwh)
	gridExport := derefF(row.GridExportKwh)
	if production <= 0 {
		return 0
	}
	pct := (production - gridExport) / production * 100
	if pct > 100 {
		pct = 100
	}
	return pct
}

// avgDailyAutoconsumo is the weekly average shown under the battery bars
// (rounded, 0 with no rows, like the React useMemo).
func avgDailyAutoconsumo(rows []db.DailyRow) int {
	if len(rows) == 0 {
		return 0
	}
	var sum float64
	for _, r := range rows {
		sum += dailyAutoconsumo(r)
	}
	return int(math.Round(sum / float64(len(rows))))
}

// parseDayDate parses a daily row date (YYYY-MM-DD) in local time.
func parseDayDate(s string) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// weekdayNarrow mirrors date-fns "EEEEE" per locale: one-letter weekday.
func weekdayNarrow(lang string, wd time.Weekday) string {
	es := []string{"D", "L", "M", "X", "J", "V", "S"}
	en := []string{"S", "M", "T", "W", "T", "F", "S"}
	zh := []string{"日", "一", "二", "三", "四", "五", "六"}
	switch normalizeLang(lang) {
	case "es":
		return es[wd]
	case "zh-CN":
		return zh[wd]
	default:
		return en[wd]
	}
}

// sourceSplit is the "where did the energy come from" breakdown of the daily
// consumption (port of the period=dia branch in Historico.tsx).
type sourceSplit struct {
	Solar, Bateria, Red float64
}

func sourceSplitFromKpis(k DayKpis) sourceSplit {
	red := k.GridImportKwh
	bateria := math.Min(k.BatteryDischargedKwh, math.Max(0, k.ConsumptionKwh-red))
	solar := math.Max(0, k.ConsumptionKwh-red-bateria)
	return sourceSplit{Solar: solar, Bateria: bateria, Red: red}
}

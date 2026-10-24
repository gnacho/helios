package web

import (
	"strconv"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/solar"
)

// Port of app/src/lib/historyStats.ts: period windows, anchors, navigation
// labels and period totals. The React page works on synthetic HistoryDay rows;
// here the same math runs on db.DailyRow (the same daily table). Dates are
// local-time days truncated to midnight, like startOfDay() in the TS.

// Period is one of the history periods (dia/semana/mes/ano).
type Period string

const (
	PeriodDia    Period = "dia"
	PeriodSemana Period = "semana"
	PeriodMes    Period = "mes"
	PeriodAno    Period = "ano"
)

func periodOf(s string) (Period, bool) {
	switch Period(s) {
	case PeriodDia, PeriodSemana, PeriodMes, PeriodAno:
		return Period(s), true
	}
	return "", false
}

// startOfDay truncates to local midnight.
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// periodWindow returns the [start, end] window (both inclusive, at midnight /
// end of day) like periodWindow() in the TS.
func periodWindow(p Period, anchor time.Time) (time.Time, time.Time) {
	a := startOfDay(anchor)
	switch p {
	case PeriodDia:
		return a, a
	case PeriodSemana:
		return a.AddDate(0, 0, -6), a
	case PeriodMes:
		return time.Date(a.Year(), a.Month(), 1, 0, 0, 0, 0, a.Location()),
			time.Date(a.Year(), a.Month()+1, 0, 0, 0, 0, 0, a.Location())
	case PeriodAno:
		return time.Date(a.Year(), time.January, 1, 0, 0, 0, 0, a.Location()),
			time.Date(a.Year(), time.December, 31, 0, 0, 0, 0, a.Location())
	}
	return a, a
}

// initialAnchor mirrors initialAnchor(): full days/weeks end yesterday; the
// month/year anchor is the first day of the current month/year.
func initialAnchor(p Period, today time.Time) time.Time {
	t := startOfDay(today)
	switch p {
	case PeriodDia, PeriodSemana:
		return t.AddDate(0, 0, -1)
	case PeriodMes:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	case PeriodAno:
		return time.Date(t.Year(), time.January, 1, 0, 0, 0, 0, t.Location())
	}
	return t
}

// shiftAnchor moves the anchor one period unit (dir = ±1).
func shiftAnchor(p Period, anchor time.Time, dir int) time.Time {
	a := startOfDay(anchor)
	switch p {
	case PeriodDia:
		return a.AddDate(0, 0, dir)
	case PeriodSemana:
		return a.AddDate(0, 0, dir*7)
	case PeriodMes:
		return time.Date(a.Year(), a.Month()+time.Month(dir), 1, 0, 0, 0, 0, a.Location())
	case PeriodAno:
		return time.Date(a.Year()+dir, time.January, 1, 0, 0, 0, 0, a.Location())
	}
	return a
}

// isCurrentPeriod mirrors isCurrentPeriod(): the anchor is the default one.
func isCurrentPeriod(p Period, anchor, today time.Time) bool {
	a, b := startOfDay(anchor), initialAnchor(p, today)
	return a.Equal(b)
}

// canGoNext mirrors canGoNext(): moving forward must not pass today.
func canGoNext(p Period, anchor, today time.Time) bool {
	if isCurrentPeriod(p, anchor, today) {
		return false
	}
	start, end := periodWindow(p, shiftAnchor(p, anchor, 1))
	todayEnd := startOfDay(today).AddDate(0, 0, 1).Add(-time.Millisecond)
	if p == PeriodDia || p == PeriodSemana {
		return !end.After(todayEnd)
	}
	return !start.After(todayEnd)
}

// sameDay compares two dates at day precision.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// monthNames are the date-fns locale month names (full and abbreviated).
var monthNames = map[string]struct{ Full, Short []string }{
	"es": {
		[]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"},
		[]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"},
	},
	"en": {
		[]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
		[]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
	},
	"zh-CN": {
		[]string{"一月", "二月", "三月", "四月", "五月", "六月", "七月", "八月", "九月", "十月", "十一月", "十二月"},
		[]string{"1月", "2月", "3月", "4月", "5月", "6月", "7月", "8月", "9月", "10月", "11月", "12月"},
	},
}

func monthName(lang string, m time.Month, short bool) string {
	n, ok := monthNames[normalizeLang(lang)]
	if !ok {
		n = monthNames["es"]
	}
	if short {
		return n.Short[int(m)-1]
	}
	return n.Full[int(m)-1]
}

// fmtDayMonth mirrors fmtDayMonth(): `31 jul` / `Jul 31` / `7月31日`.
func fmtDayMonth(lang string, t time.Time) string {
	switch normalizeLang(lang) {
	case "zh-CN":
		return strconv.Itoa(int(t.Month())) + "月" + strconv.Itoa(t.Day()) + "日"
	case "en":
		return monthName(lang, t.Month(), true) + " " + strconv.Itoa(t.Day())
	default:
		return strconv.Itoa(t.Day()) + " " + monthName(lang, t.Month(), true)
	}
}

// fmtDayMonthLong mirrors fmtDayMonthLong(): `31 de julio` / `July 31`.
func fmtDayMonthLong(lang string, t time.Time) string {
	switch normalizeLang(lang) {
	case "zh-CN":
		return strconv.Itoa(int(t.Month())) + "月" + strconv.Itoa(t.Day()) + "日"
	case "en":
		return monthName(lang, t.Month(), false) + " " + strconv.Itoa(t.Day())
	default:
		return strconv.Itoa(t.Day()) + " de " + monthName(lang, t.Month(), false)
	}
}

// fmtMonthYear mirrors fmtMonthYear(): `julio 2026` / `July 2026` / `2026年7月`.
func fmtMonthYear(lang string, t time.Time) string {
	if normalizeLang(lang) == "zh-CN" {
		return strconv.Itoa(t.Year()) + "年" + strconv.Itoa(int(t.Month())) + "月"
	}
	return monthName(lang, t.Month(), false) + " " + strconv.Itoa(t.Year())
}

// periodNavLabel mirrors navLabel(): `15 jul` · `8 - 14 jul` · `julio 2025` · `2025`.
func periodNavLabel(p Period, anchor time.Time, lang string) string {
	start, end := periodWindow(p, anchor)
	switch p {
	case PeriodDia:
		return fmtDayMonth(lang, anchor)
	case PeriodSemana:
		left := strconv.Itoa(start.Day())
		if start.Month() != end.Month() {
			left = fmtDayMonth(lang, start)
		}
		return left + " - " + fmtDayMonth(lang, end)
	case PeriodMes:
		return fmtMonthYear(lang, anchor)
	case PeriodAno:
		return strconv.Itoa(anchor.Year())
	}
	return ""
}

// periodSubtitle mirrors periodSubtitle(): `Semana del 8 al 14 de julio`, etc.
func periodSubtitle(p Period, anchor time.Time, lang string) string {
	_, end := periodWindow(p, anchor)
	switch p {
	case PeriodDia:
		return fmtWeekdayDateLower(lang, anchor)
	case PeriodSemana:
		start, _ := periodWindow(p, anchor)
		return T(lang, "historico.subtitle.week", map[string]string{
			"start": strconv.Itoa(start.Day()),
			"end":   fmtDayMonthLong(lang, end),
		})
	case PeriodMes:
		return T(lang, "historico.subtitle.month", map[string]string{
			"month": fmtMonthYear(lang, anchor),
		})
	case PeriodAno:
		return T(lang, "historico.subtitle.year", map[string]string{
			"year": strconv.Itoa(anchor.Year()),
		})
	}
	return ""
}

// fmtWeekdayDateLower mirrors fmtWeekdayDate() (lowercase, the React page
// applies capitalize via CSS).
func fmtWeekdayDateLower(lang string, t time.Time) string {
	esDays := []string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}
	enDays := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	zhDays := []string{"日", "一", "二", "三", "四", "五", "六"}
	switch normalizeLang(lang) {
	case "zh-CN":
		return strconv.Itoa(int(t.Month())) + "月" + strconv.Itoa(t.Day()) + "日 星期" + zhDays[t.Weekday()]
	case "en":
		return enDays[t.Weekday()] + ", " + monthName(lang, t.Month(), false) + " " + strconv.Itoa(t.Day())
	default:
		return esDays[t.Weekday()] + ", " + strconv.Itoa(t.Day()) + " de " + monthName(lang, t.Month(), false)
	}
}

// rowsInWindow filters daily rows inside the period window (string dates
// YYYY-MM-DD compare lexicographically like the JS Date filters).
func rowsInWindow(p Period, anchor time.Time, rows []db.DailyRow) []db.DailyRow {
	start, end := periodWindow(p, anchor)
	from, to := start.Format("2006-01-02"), end.Format("2006-01-02")
	out := make([]db.DailyRow, 0, len(rows))
	for _, r := range rows {
		if r.Date >= from && r.Date <= to {
			out = append(out, r)
		}
	}
	return out
}

// PeriodTotals mirrors PeriodTotals in the TS.
type PeriodTotals struct {
	ProductionKwh  float64
	ConsumptionKwh float64
	GridImportKwh  float64
	GridExportKwh  float64
	// AutoconsumoPct is weighted by production (media ponderada).
	AutoconsumoPct float64
	AhorroEur      float64
	Co2Kg          float64
	DayCount       int
}

// totalsFor mirrors totalsFor(): aggregate window rows into period totals
// using the configured prices.
func totalsFor(days []db.DailyRow, prices solar.PriceConfig) PeriodTotals {
	var t PeriodTotals
	var autoWeighted float64
	for _, d := range days {
		production := derefF(d.ProductionKwh)
		t.ProductionKwh += production
		t.ConsumptionKwh += derefF(d.ConsumptionKwh)
		t.GridImportKwh += derefF(d.GridImportKwh)
		t.GridExportKwh += derefF(d.GridExportKwh)
		autoWeighted += dailyAutoconsumo(d) * production
	}
	if t.ProductionKwh > 0 {
		t.AutoconsumoPct = autoWeighted / t.ProductionKwh
	}
	saved := t.ConsumptionKwh - t.GridImportKwh
	if saved < 0 {
		saved = 0
	}
	t.AhorroEur = saved*prices.PriceImport + t.GridExportKwh*prices.PriceExport
	t.Co2Kg = t.ProductionKwh * prices.Co2PerKwh
	t.DayCount = len(days)
	return t
}

// deltaPct mirrors deltaPct(): nil when there is no base to compare.
func deltaPct(current, previous float64) *float64 {
	if previous <= 0 {
		return nil
	}
	v := (current - previous) / previous * 100
	return &v
}

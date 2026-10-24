package web

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/starfederation/datastar-go/datastar"

	"helios-server/internal/auth"
	"helios-server/internal/db"
	"helios-server/internal/solar"
)

// /historico page (port of app/src/pages/Historico.tsx plus HistoryChart and
// YearHeatmap). Period and anchor travel as query params (?period=&anchor=):
// every navigation is a full server render, no client state. Hover tooltips,
// the export dropdown (a stub that only fires a toast in React) and the GSAP
// animations are not ported.

const (
	histW       = 767.0 // HistoryChart card is lg:col-span-8
	histH       = 320.0
	histTop     = 12.0
	histSideL   = 34.0
	histSideR   = 34.0 // right axis (autoconsumo %)
	histBottom  = 24.0
	histDayYMax = 7.0 // fixed domain [0,7] like the React day AreaChart

	heatCellPx = 12.0
	heatGap    = 3.0
	heatPitch  = heatCellPx + heatGap
	heatLeft   = 22.0
	heatTop    = 18.0
	heatCols   = 53
	heatDays   = 365 // heatmap window: last 365 days ending today
)

// histTicksX is the 3-hour X axis using the history box (its own side pads).
func histTicksX() []chartTick {
	var ticks []chartTick
	for _, t := range axisTicksX {
		ticks = append(ticks, chartTick{X: histX(float64(t)), Y: histH - 6, Label: fmtTime(t)})
	}
	return ticks
}

func histX(t float64) float64 {
	return histSideL + t/1440*(histW-histSideL-histSideR)
}

func histY(v float64) float64 {
	return histTop + (1-v/histDayYMax)*(histH-histTop-histBottom)
}

// ── Day curves (period = dia) ───────────────────────────────────────────

// histDayChartModel is the day-curves chart: production and consumption
// areas on the fixed 0..7 domain, AHORA line when the day is today.
type histDayChartModel struct {
	Lang        string
	ProdPath    string
	ProdLine    string
	ConsPath    string
	ConsLine    string
	TicksX      []chartTick
	TicksY      []chartTick
	PlotRightX  float64
	PlotBottomY float64
	NowX        float64
	HasNow      bool
	ColorProd   string
	ColorCons   string
}

func buildHistoryDayChart(lang string, points []solar.DayPoint, nowMin int, hasNow bool, pal Palette) histDayChartModel {
	m := histDayChartModel{
		Lang:        lang,
		TicksX:      histTicksX(),
		PlotRightX:  histW - histSideR,
		PlotBottomY: histH - histBottom,
		HasNow:      hasNow,
		NowX:        histX(float64(roundStep(nowMin))),
		ColorProd:   pal.Solar,
		ColorCons:   pal.Consumo,
	}
	for v := 0; v <= int(histDayYMax); v++ {
		m.TicksY = append(m.TicksY, chartTick{X: histSideL - 6, Y: histY(float64(v)) + 4, Label: strconv.Itoa(v)})
	}
	var prod, cons []cpoint
	for _, p := range points {
		x := histX(float64(p.T))
		prod = append(prod, cpoint{x, histY(p.Production)})
		cons = append(cons, cpoint{x, histY(p.Consumption)})
	}
	baseline := histY(0)
	m.ProdPath = areaPath(prod, baseline)
	m.ProdLine = monotoneLine(prod)
	m.ConsPath = areaPath(cons, baseline)
	m.ConsLine = monotoneLine(cons)
	return m
}

// ── Bars (period = semana / mes / ano) ──────────────────────────────────

// histBarRow is one bar group: production + consumption bars and the point
// of the dotted autoconsumo line.
type histBarRow struct {
	Label                      string
	XProd, XCons, BarW         float64
	ProdY, ProdH, ConsY, ConsH float64
	AutoX, AutoY               float64
	IsToday                    bool
	ShowLabel                  bool
	Href                       string
}

// histBarsModel is the grouped bars chart (semana/mes: per recorded day;
// ano: aggregated per month with data), autoconsumo line on the right 0..100
// axis and drill-down links on every bar group.
type histBarsModel struct {
	Lang        string
	YMax        int
	Rows        []histBarRow
	TicksYLeft  []chartTick // left axis, kWh
	TicksY      []chartTick // right axis, %
	AutoPath    string
	HasAuto     bool
	PlotRightX  float64
	PlotBottomY float64
	ColorProd   string
	ColorCons   string
	ColorAuto   string
}

func histBarY(v, max float64) float64 {
	return histTop + (1-v/max)*(histH-histTop-histBottom)
}

func histURL(p Period, anchor time.Time) string {
	// #grafico: al cambiar de periodo la página no salta arriba, aterriza en
	// la tarjeta que cambia (el navegador hace scroll al ancla).
	return "/historico?period=" + string(p) + "&anchor=" + anchor.Format("2006-01-02") + "#grafico"
}

// buildHistoryBars ports the HistoryChart branches for semana/mes/ano.
func buildHistoryBars(lang string, p Period, today time.Time, days []db.DailyRow, pal Palette) histBarsModel {
	m := histBarsModel{
		Lang:        lang,
		PlotRightX:  histW - histSideR,
		PlotBottomY: histH - histBottom,
		ColorProd:   pal.Solar,
		ColorCons:   pal.Consumo,
		ColorAuto:   pal.Bateria,
	}
	for _, v := range []int{0, 50, 100} {
		m.TicksY = append(m.TicksY, chartTick{X: histW - histSideR + 6, Y: histBarY(float64(v), 100) + 4, Label: strconv.Itoa(v) + "%"})
	}
	if len(days) == 0 {
		return m
	}

	type group struct {
		date    time.Time
		label   string
		href    string
		prod    float64
		cons    float64
		autoSum float64 // autoconsumoPct weighted by production
	}
	var groups []group
	if p == PeriodAno {
		byMonth := map[time.Time]*group{}
		for _, d := range days {
			dt, ok := parseDayDate(d.Date)
			if !ok {
				continue
			}
			first := time.Date(dt.Year(), dt.Month(), 1, 0, 0, 0, 0, time.Local)
			g := byMonth[first]
			if g == nil {
				g = &group{date: first, label: monthName(lang, dt.Month(), true), href: histURL(PeriodMes, first)}
				byMonth[first] = g
			}
			g.prod += derefF(d.ProductionKwh)
			g.cons += derefF(d.ConsumptionKwh)
			g.autoSum += dailyAutoconsumo(d) * derefF(d.ProductionKwh)
		}
		for _, g := range byMonth {
			groups = append(groups, *g)
		}
		sort.Slice(groups, func(i, j int) bool { return groups[i].date.Before(groups[j].date) })
	} else {
		for _, d := range days {
			dt, ok := parseDayDate(d.Date)
			if !ok {
				continue
			}
			prod := derefF(d.ProductionKwh)
			groups = append(groups, group{
				date:    dt,
				label:   weekdayNarrow(lang, dt.Weekday()) + " " + strconv.Itoa(dt.Day()),
				href:    histURL(PeriodDia, dt),
				prod:    prod,
				cons:    derefF(d.ConsumptionKwh),
				autoSum: dailyAutoconsumo(d) * prod,
			})
		}
	}

	yMaxF := 1.0
	for _, g := range groups {
		if g.prod > yMaxF {
			yMaxF = g.prod
		}
		if g.cons > yMaxF {
			yMaxF = g.cons
		}
	}
	yMax := math.Ceil(yMaxF)
	m.YMax = int(yMax)
	// Recharts-style ~5 ticks: multiples of a round step instead of 1-by-1.
	tickStep := math.Max(1, math.Ceil(yMax/5))
	for v := 0.0; v <= yMax; v += tickStep {
		m.TicksYLeft = append(m.TicksYLeft, chartTick{X: histSideL - 6, Y: histBarY(v, yMax) + 4, Label: strconv.Itoa(int(v))})
	}

	plotW := histW - histSideL - histSideR
	slot := plotW / float64(len(groups))
	barW := (slot*0.82 - 3) / 2
	if barW < 2 {
		barW = 2
	}
	var autoPts []cpoint
	for i, g := range groups {
		x0 := histSideL + float64(i)*slot + (slot-(barW*2+3))/2
		row := histBarRow{
			Label:   g.label,
			XProd:   x0,
			XCons:   x0 + barW + 3,
			BarW:    barW,
			IsToday: sameDay(g.date, today),
			Href:    g.href,
			// Like the React interval: all labels on semana, 1 of 3 on mes.
			ShowLabel: p != PeriodMes || i%3 == 0,
		}
		if h := g.prod / yMax * (histH - histTop - histBottom); h > 0.5 {
			row.ProdH = h
			row.ProdY = m.PlotBottomY - h
		}
		if h := g.cons / yMax * (histH - histTop - histBottom); h > 0.5 {
			row.ConsH = h
			row.ConsY = m.PlotBottomY - h
		}
		auto := 0.0
		if g.prod > 0 {
			auto = g.autoSum / g.prod
		}
		row.AutoX = x0 + barW + 1.5
		row.AutoY = histBarY(auto, 100)
		autoPts = append(autoPts, cpoint{row.AutoX, row.AutoY})
		m.Rows = append(m.Rows, row)
	}
	if len(autoPts) > 1 {
		m.AutoPath = monotoneLine(autoPts)
		m.HasAuto = true
	}
	return m
}

// ── Year heatmap (YearHeatmap.tsx) ──────────────────────────────────────

type heatCell struct {
	X, Y    float64
	Level   int // 0..4
	IsToday bool
	Href    string
	Aria    string
}

type heatMonthLabel struct {
	X     float64
	Label string
}

type heatRowLabel struct {
	Y     float64
	Label string
}

type heatModel struct {
	Cells      []heatCell
	Months     []heatMonthLabel
	Rows       []heatRowLabel
	Width      float64
	Height     float64
	SolarColor string
	LevelOp    [5]float64
}

// buildYearHeatmap ports YearHeatmap.tsx: GitHub-style 53x7 grid of the last
// 365 days. Level = min(4, max(1, ceil(kwh/max*4))), like the TS: zero days
// render at the faintest amber, never empty.
func buildYearHeatmap(lang string, rows []db.DailyRow, today time.Time, pal Palette) heatModel {
	m := heatModel{
		Width:      heatLeft + heatCols*heatPitch,
		Height:     heatTop + 7*heatPitch - heatGap,
		SolarColor: pal.Solar,
		LevelOp:    [5]float64{0, 0.28, 0.48, 0.72, 1},
	}
	for _, rl := range []struct {
		row int
		key string
	}{{0, "historico.wdMon"}, {2, "historico.wdWed"}, {4, "historico.wdFri"}} {
		m.Rows = append(m.Rows, heatRowLabel{Y: heatTop + float64(rl.row)*heatPitch + heatCellPx - 2, Label: T(lang, rl.key)})
	}
	from := startOfDay(today).AddDate(0, 0, -(heatDays - 1))
	var days []db.DailyRow
	for _, r := range rows {
		if r.Date >= from.Format("2006-01-02") && r.Date <= today.Format("2006-01-02") {
			days = append(days, r)
		}
	}
	if len(days) == 0 {
		return m
	}

	max := 1.0
	for _, d := range days {
		if v := derefF(d.ProductionKwh); v > max {
			max = v
		}
	}
	first, ok := parseDayDate(days[0].Date)
	if !ok {
		return m
	}
	firstMonday := mondayOf(first)
	prevMonth := -1
	for _, d := range days {
		dt, ok := parseDayDate(d.Date)
		if !ok {
			continue
		}
		monday := mondayOf(dt)
		col := int(math.Round(monday.Sub(firstMonday).Hours() / (7 * 24)))
		if col > heatCols-1 {
			col = heatCols - 1
		}
		row := (int(dt.Weekday()) + 6) % 7
		kwh := derefF(d.ProductionKwh)
		level := int(math.Ceil(kwh / max * 4))
		if level < 1 {
			level = 1
		}
		if level > 4 {
			level = 4
		}
		m.Cells = append(m.Cells, heatCell{
			X:       heatLeft + float64(col)*heatPitch,
			Y:       heatTop + float64(row)*heatPitch,
			Level:   level,
			IsToday: sameDay(dt, today),
			Href:    histURL(PeriodDia, dt),
			Aria:    T(lang, "historico.cellAria", map[string]string{"date": fmtDayMonthLong(lang, dt), "kwh": fmtEnergy(lang, kwh)}),
		})
		if int(monday.Month()) != prevMonth {
			m.Months = append(m.Months, heatMonthLabel{
				X:     heatLeft + float64(col)*heatPitch,
				Label: monthName(lang, monday.Month(), true),
			})
			prevMonth = int(monday.Month())
		}
	}
	return m
}

// mondayOf returns the monday of the week containing t (weeks start monday).
func mondayOf(t time.Time) time.Time {
	m := startOfDay(t)
	return m.AddDate(0, 0, -(int(m.Weekday())+6)%7)
}

// ── Records cards ───────────────────────────────────────────────────────

// recordCard is one of the "récords y curiosidades" cards.
type recordCard struct {
	IconKey string // zap | arrowUpFromLine | leaf | trophy | moon | flame | calendarCheck
	Color   string
	Title   string
	Value   string
	Text    string
}

// buildRecords ports the records useMemo of Historico.tsx.
func buildRecords(lang string, p Period, days []db.DailyRow, dayKpis solar.DayKpis, prices solar.PriceConfig, pal Palette) []recordCard {
	if len(days) == 0 {
		return nil
	}
	var list []recordCard
	if p == PeriodDia {
		list = append(list, recordCard{
			IconKey: "zap", Color: pal.Solar,
			Title: T(lang, "historico.records.peak"),
			Value: fmtKwVal(lang, dayKpis.PeakProductionKw) + " kW",
			Text: T(lang, "historico.records.peakText", map[string]string{
				"time": fmtTime(dayKpis.PeakAt),
				"kwh":  fmtEnergy(lang, dayKpis.ProductionKwh),
			}),
		})
		if dayKpis.GridExportKwh > 0 {
			list = append(list, recordCard{
				IconKey: "arrowUpFromLine", Color: pal.RedVertido,
				Title: T(lang, "historico.records.export"),
				Value: fmtEnergy(lang, dayKpis.GridExportKwh) + " kWh",
				Text:  T(lang, "historico.records.exportText", map[string]string{"price": fmtEuro(lang, prices.PriceExport)}),
			})
		}
		list = append(list, recordCard{
			IconKey: "leaf", Color: pal.Bateria,
			Title: T(lang, "common.autoconsumo"),
			Value: fmtPct(lang, dayKpis.AutoconsumoPct) + " %",
			Text: T(lang, "historico.records.autoText", map[string]string{
				"kwh": fmtEnergy(lang, dayKpis.ProductionKwh-dayKpis.GridExportKwh),
			}),
		})
		if len(list) > 3 {
			list = list[:3]
		}
		return list
	}

	best := days[0]
	for _, d := range days[1:] {
		if derefF(d.ProductionKwh) > derefF(best.ProductionKwh) {
			best = d
		}
	}
	bestTitle := T(lang, "historico.records.bestYear")
	switch p {
	case PeriodSemana:
		bestTitle = T(lang, "historico.records.bestWeek")
	case PeriodMes:
		bestTitle = T(lang, "historico.records.bestMonth")
	}
	if bd, ok := parseDayDate(best.Date); ok {
		list = append(list, recordCard{
			IconKey: "trophy", Color: pal.Solar,
			Title: bestTitle,
			Value: fmtEnergy(lang, derefF(best.ProductionKwh)) + " kWh",
			Text:  fmtWeekdayDateLower(lang, bd),
		})
	}

	var bestNight *db.DailyRow
	for i := range days {
		if derefF(days[i].ConsumptionKwh) <= 1 {
			continue
		}
		if bestNight == nil || dailyAutoconsumo(days[i]) > dailyAutoconsumo(*bestNight) {
			bestNight = &days[i]
		}
	}
	if bestNight != nil {
		pct := dailyAutoconsumo(*bestNight)
		if pct > 97 {
			pct = 97
		}
		nd, _ := parseDayDate(bestNight.Date)
		list = append(list, recordCard{
			IconKey: "moon", Color: pal.Fox,
			Title: T(lang, "historico.records.autonomous"),
			Value: fmtPct(lang, pct) + " %",
			Text:  fmtDayMonthLong(lang, nd),
		})
	}

	threshold := 25.0
	if p == PeriodAno {
		threshold = 35
	}
	bestStreak, cur := 0, 0
	for _, d := range days {
		if derefF(d.ProductionKwh) >= threshold {
			cur++
			if cur > bestStreak {
				bestStreak = cur
			}
		} else {
			cur = 0
		}
	}
	if bestStreak > 0 {
		list = append(list, recordCard{
			IconKey: "flame", Color: pal.RedCompra,
			Title: T(lang, "historico.records.streak"),
			Value: TP(lang, "historico.records.streakDays", bestStreak, map[string]string{"count": strconv.Itoa(bestStreak)}),
			Text:  T(lang, "historico.records.streakText", map[string]string{"threshold": fmtInt(lang, threshold)}),
		})
	}

	if p == PeriodAno {
		type monthAcc struct {
			date time.Time
			kwh  float64
		}
		byMonth := map[int]*monthAcc{}
		for _, d := range days {
			dt, ok := parseDayDate(d.Date)
			if !ok {
				continue
			}
			acc := byMonth[int(dt.Month())]
			if acc == nil {
				acc = &monthAcc{date: time.Date(dt.Year(), dt.Month(), 1, 0, 0, 0, 0, time.Local)}
				byMonth[int(dt.Month())] = acc
			}
			acc.kwh += derefF(d.ProductionKwh)
		}
		var bestM *monthAcc
		for _, acc := range byMonth {
			if bestM == nil || acc.kwh > bestM.kwh {
				bestM = acc
			}
		}
		if bestM != nil {
			list = append(list, recordCard{
				IconKey: "calendarCheck", Color: pal.Bateria,
				Title: T(lang, "historico.records.bestMonthTitle"),
				Value: monthName(lang, bestM.date.Month(), false) + " · " + fmtInt(lang, bestM.kwh) + " kWh",
				Text:  T(lang, "historico.records.bestMonthText"),
			})
		}
	}

	if len(list) > 4 {
		list = list[:4]
	}
	return list
}

// ── KPI cards + period selector ─────────────────────────────────────────

// histKpiView is one KPI card of the period (KpiCard.tsx port).
type histKpiView struct {
	IconKey     string // sun | house | leaf | euro
	Color       string
	Label       string
	ValueText   string
	Num         float64 // raw value for the Datastar count-up tween
	Dec         int     // fixed decimals of Num
	Unit        string
	HasProgress bool
	ProgressPct float64
	HasDelta    bool
	DeltaUp     bool
	DeltaGood   bool
	DeltaText   string
	Sub         string
}

// periodTabView is one period of the selector (Día/Semana/Mes/Año).
type periodTabView struct {
	Key, Label, URL string
	Active          bool
	Patch           string // data-on:click expression ("" = plain link)
}

// periodSelectorView is the PeriodSelector.tsx port (links, no client state).
type periodSelectorView struct {
	Lang      string
	Tabs      []periodTabView
	PrevURL   string
	NextURL   string
	TodayURL  string
	Label     string
	CanNext   bool
	IsCurrent bool
	PrevAria  string
	NextAria  string

	// Expresiones data-on:click para el parche SPA del histórico (vacías en
	// otros usos del selector, p.ej. el cargador).
	PatchPrev  string
	PatchNext  string
	PatchToday string
}

func buildPeriodSelector(lang string, period Period, anchor, today time.Time, rev int) periodSelectorView {
	v := periodSelectorView{
		Lang:      lang,
		PrevURL:   histURL(period, shiftAnchor(period, anchor, -1)),
		NextURL:   histURL(period, shiftAnchor(period, anchor, 1)),
		TodayURL:  histURL(period, initialAnchor(period, today)),
		Label:     periodNavLabel(period, anchor, lang),
		CanNext:   canGoNext(period, anchor, today),
		IsCurrent: isCurrentPeriod(period, anchor, today),
		PrevAria:  T(lang, "historico.prevAria"),
		NextAria:  T(lang, "historico.nextAria"),
	}
	// Cada parche lleva la revisión siguiente: al alternar rev-a/rev-b cambia
	// el nombre de la animación y el navegador la reinicia aunque Datastar
	// reutilice los nodos (si no, las barras nuevas salen ya crecidas).
	cur := "&rev=" + strconv.Itoa(rev+1)
	for _, p := range []Period{PeriodDia, PeriodSemana, PeriodMes, PeriodAno} {
		a := initialAnchor(p, today)
		v.Tabs = append(v.Tabs, periodTabView{
			Key:    string(p),
			Label:  T(lang, "historico.periods."+string(p)),
			URL:    histURL(p, a),
			Active: p == period,
			Patch:  historicoPatchExpr(p, a, cur),
		})
	}
	v.PatchPrev = historicoPatchExpr(period, shiftAnchor(period, anchor, -1), cur)
	v.PatchNext = historicoPatchExpr(period, shiftAnchor(period, anchor, 1), cur)
	v.PatchToday = historicoPatchExpr(period, initialAnchor(period, today), cur)
	return v
}

// revClass alternates the two animation-name sets so a Datastar patch restarts
// every chart animation even when the morph reuses the nodes.
func revClass(rev int) string {
	if rev%2 == 0 {
		return "rev-a"
	}
	return "rev-b"
}

// historicoPatchExpr is the Datastar call that swaps the body in place (no page
// reload, so the scroll never jumps); the href stays as the no-JS fallback.
func historicoPatchExpr(p Period, a time.Time, cur string) string {
	return "@get('/ui/historico?period=" + string(p) + "&anchor=" + a.Format("2006-01-02") + cur + "')"
}

// buildHistKpis ports the 5 KPI cards with their delta chips.
func buildHistKpis(lang string, period Period, totals, prev PeriodTotals, hasPrev bool, prices solar.PriceConfig, pal Palette) []histKpiView {
	prodDelta, consDelta := (*float64)(nil), (*float64)(nil)
	if hasPrev {
		prodDelta = deltaPct(totals.ProductionKwh, prev.ProductionKwh)
		consDelta = deltaPct(totals.ConsumptionKwh, prev.ConsumptionKwh)
	}
	vsText := T(lang, "historico.vs."+string(period))

	k := func(icon string) histKpiView { return histKpiView{IconKey: icon} }
	kpis := []histKpiView{
		k("sun"), k("house"), k("leaf"), k("euro"), k("leaf"),
	}
	kpis[0].Color, kpis[1].Color, kpis[2].Color = pal.Solar, pal.Consumo, pal.Bateria
	kpis[3].Color, kpis[4].Color = pal.Bateria, pal.Bateria
	kpis[0].Label, kpis[1].Label = T(lang, "common.production"), T(lang, "common.consumption")
	kpis[2].Label, kpis[3].Label, kpis[4].Label = T(lang, "common.autoconsumo"), T(lang, "historico.savings"), T(lang, "historico.co2")
	decimals := func(v float64) string {
		if v >= 100 {
			return fmtInt(lang, v)
		}
		return fmtEnergy(lang, v)
	}
	kpis[0].ValueText, kpis[0].Unit = decimals(totals.ProductionKwh), "kWh"
	kpis[1].ValueText, kpis[1].Unit = decimals(totals.ConsumptionKwh), "kWh"
	kpis[2].ValueText, kpis[2].Unit = fmtInt(lang, totals.AutoconsumoPct), "%"
	kpis[2].HasProgress, kpis[2].ProgressPct = true, clampPct(totals.AutoconsumoPct)
	kpis[3].ValueText, kpis[3].Unit = formatNumber(lang, totals.AhorroEur, 2), "€"
	// Raw values for the count-up tween (decimals must match the formatter).
	decOf := func(v float64) int {
		if v >= 100 {
			return 0
		}
		return 1
	}
	kpis[0].Num, kpis[0].Dec = totals.ProductionKwh, decOf(totals.ProductionKwh)
	kpis[1].Num, kpis[1].Dec = totals.ConsumptionKwh, decOf(totals.ConsumptionKwh)
	kpis[2].Num, kpis[2].Dec = totals.AutoconsumoPct, 0
	kpis[3].Num, kpis[3].Dec = totals.AhorroEur, 2
	kpis[3].Sub = T(lang, "historico.savingsDetail", map[string]string{
		"importPrice": fmtEuro(lang, prices.PriceImport),
		"exportPrice": fmtEuro(lang, prices.PriceExport),
	})
	kpis[4].ValueText, kpis[4].Unit = fmtInt(lang, totals.Co2Kg), "kg"
	kpis[4].Num, kpis[4].Dec = totals.Co2Kg, 0
	kpis[4].Sub = TP(lang, "historico.trees", treesOf(totals.Co2Kg), map[string]string{"count": strconv.Itoa(treesOf(totals.Co2Kg))})

	if prodDelta != nil {
		kpis[0].HasDelta = true
		kpis[0].DeltaUp = *prodDelta >= 0
		kpis[0].DeltaGood = *prodDelta >= 0
		kpis[0].DeltaText = fmtPct(lang, math.Abs(*prodDelta)) + " % " + vsText
	}
	if consDelta != nil {
		kpis[1].HasDelta = true
		kpis[1].DeltaUp = *consDelta >= 0
		kpis[1].DeltaGood = *consDelta < 0
		kpis[1].DeltaText = fmtPct(lang, math.Abs(*consDelta)) + " % " + vsText
	}
	return kpis
}

func treesOf(co2 float64) int {
	if co2 <= 0 {
		return 0
	}
	n := int(math.Round(co2 / 21))
	if n < 1 {
		n = 1
	}
	return n
}

// ── Page view + handler ─────────────────────────────────────────────────

type historicoView struct {
	pageProps
	Palette Palette

	Period     Period
	Subtitle   string
	ChartTitle string
	IsDay      bool
	Empty      bool

	Selector periodSelectorView
	Rev      string // revisión (periodo+ancla): fuerza recrear el DOM parcheado
	RevClass string // rev-a / rev-b: reinicia las animaciones al parchear
	RevNum   int    // contador de parches (se pasa al siguiente en los enlaces)
	Kpis     []histKpiView

	Day    histDayChartModel
	Bars   histBarsModel
	IsBars bool // non-day period

	Donut   donutModel
	Heat    heatModel
	HasHeat bool

	Records     []recordCard
	Estimated   bool
	DrillHint   string
	BackToToday string
}

func (u *UI) handleHistorico(w http.ResponseWriter, r *http.Request) {
	v := u.buildHistoricoView(r)
	_ = HistoricoPage(v).Render(r.Context(), w)
}

// handleHistoricoPatch answers the Datastar period navigation: it swaps
// #hist-body in place so the page scroll never moves, with a horizontal slide
// towards the direction of the new period.
//
// Mode outer (morph) keeps the nodes, so the scroll anchoring does not jump
// when the year view adds its heatmap. The chart animations still restart
// because every patch flips the rev-a/rev-b class (see revClass).
func (u *UI) handleHistoricoPatch(w http.ResponseWriter, r *http.Request) {
	v := u.buildHistoricoView(r)
	gen := datastar.NewSSE(w, r)
	_ = gen.PatchElementTempl(historicoBody(v), datastar.WithSelectorID("hist-body"), datastar.WithModeOuter())
}

// buildHistoricoView gathers everything the histórico page needs.
func (u *UI) buildHistoricoView(r *http.Request) historicoView {
	userID, _ := auth.UserIDFrom(r.Context())
	lang := u.userLang(userID)
	p := u.baseProps(r, userID, "/historico", "")
	p.Title = T(lang, "nav.historico")

	pal := newPalette(p.Theme == "dark")
	today := startOfDay(time.Now())
	period := PeriodSemana
	if ps, ok := periodOf(r.URL.Query().Get("period")); ok {
		period = ps
	}
	revNum, _ := strconv.Atoi(r.URL.Query().Get("rev"))
	anchor := initialAnchor(period, today)
	if a, err := time.ParseInLocation("2006-01-02", r.URL.Query().Get("anchor"), time.Local); err == nil {
		anchor = startOfDay(a)
	}

	v := historicoView{
		pageProps:   p,
		Palette:     pal,
		Period:      period,
		Subtitle:    periodSubtitle(period, anchor, lang),
		IsDay:       period == PeriodDia,
		Selector:    buildPeriodSelector(lang, period, anchor, today, revNum),
		BackToToday: histURL(period, initialAnchor(period, today)),
		Rev:         string(period) + "-" + anchor.Format("2006-01-02"),
		RevNum:      revNum,
		RevClass:    revClass(revNum),
	}

	var hist []db.DailyRow
	if u.DB != nil {
		hist = u.dailyHistory()
	}
	days := rowsInWindow(period, anchor, hist)
	v.Empty = len(days) == 0
	if v.Empty {
		return v
	}

	// KPIs del día: hoy para el donut de periodos largos y los récords del día.
	dayKpis := solar.DayKpis{}
	hasDayKpis := false
	if u.Solar != nil && u.HA != nil {
		if k, err := u.Solar.GetKpis(u.HA, solar.TodayStr(), u.Prices); err == nil {
			dayKpis = k
			hasDayKpis = true
		}
	}

	// Totales del periodo visible.
	var totals PeriodTotals
	seriesEst := false
	if period == PeriodDia {
		if u.Solar != nil && u.HA != nil {
			if k, err := u.Solar.GetKpis(u.HA, anchor.Format("2006-01-02"), u.Prices); err == nil {
				totals = dayTotalsFromSolar(k, u.Prices)
				dayKpis = k
				hasDayKpis = true
			}
			if s, err := u.Solar.GetDaySeries(u.HA, anchor.Format("2006-01-02")); err == nil {
				seriesEst = s.Estimated
			}
		}
	} else {
		totals = totalsFor(days, u.Prices)
	}

	// Delta vs el periodo anterior.
	prevTotals := PeriodTotals{}
	hasPrev := false
	prevDays := rowsInWindow(period, shiftAnchor(period, anchor, -1), hist)
	if len(prevDays) > 0 {
		prevTotals = totalsFor(prevDays, u.Prices)
		hasPrev = true
	}

	v.ChartTitle = T(lang, "historico.chartTitle", map[string]string{"period": chartPeriodText(period, anchor, today, lang)})
	v.Kpis = buildHistKpis(lang, period, totals, prevTotals, hasPrev, u.Prices, pal)

	// Gráfica principal.
	nowMin := time.Now().Hour()*60 + time.Now().Minute()
	if period == PeriodDia {
		var points []solar.DayPoint
		if u.Solar != nil && u.HA != nil {
			if s, err := u.Solar.GetDaySeries(u.HA, anchor.Format("2006-01-02")); err == nil {
				points = s.Points
				seriesEst = s.Estimated
			}
		}
		v.Day = buildHistoryDayChart(lang, points, nowMin, sameDay(anchor, today), pal)
	} else {
		v.IsBars = true
		v.Bars = buildHistoryBars(lang, period, today, days, pal)
		v.DrillHint = T(lang, "historico.drillDay")
		if period == PeriodAno {
			v.DrillHint = T(lang, "historico.drillMonth")
		}
	}
	v.Estimated = seriesEst

	// Donut: origen de la energía consumida.
	if totals.ConsumptionKwh > 0 {
		split := histSourceSplit(period, totals, dayKpis, hasDayKpis)
		v.Donut = buildDonut(lang, []donutSliceIn{
			{Label: T(lang, "historico.solarDirect"), Value: split.Solar, Color: pal.Solar},
			{Label: T(lang, "common.battery"), Value: split.Bateria, Color: pal.Bateria},
			{Label: T(lang, "common.grid"), Value: split.Red, Color: pal.RedCompra},
		}, 2)
		v.Donut.CenterSub = T(lang, "historico.consumed")
	}

	// Heatmap anual (solo periodo Año).
	if period == PeriodAno {
		v.Heat = buildYearHeatmap(lang, hist, today, pal)
		v.HasHeat = true
	}

	v.Records = buildRecords(lang, period, days, dayKpis, u.Prices, pal)
	return v
}

// chartPeriodText is the {{period}} of historico.chartTitle.
func chartPeriodText(period Period, anchor, today time.Time, lang string) string {
	switch period {
	case PeriodDia:
		if sameDay(anchor, today) {
			return T(lang, "historico.todayLower")
		}
		return periodNavLabel(PeriodDia, anchor, lang)
	case PeriodSemana:
		if isCurrentPeriod(PeriodSemana, anchor, today) {
			return T(lang, "historico.thisWeek")
		}
		return periodNavLabel(PeriodSemana, anchor, lang)
	case PeriodMes:
		return monthName(lang, anchor.Month(), false)
	case PeriodAno:
		return strconv.Itoa(anchor.Year())
	}
	return ""
}

// dayTotalsFromSolar mirrors totalsFromDayKpis() in Historico.tsx.
func dayTotalsFromSolar(k solar.DayKpis, prices solar.PriceConfig) PeriodTotals {
	saved := k.ConsumptionKwh - k.GridImportKwh
	if saved < 0 {
		saved = 0
	}
	return PeriodTotals{
		ProductionKwh:  k.ProductionKwh,
		ConsumptionKwh: k.ConsumptionKwh,
		GridImportKwh:  k.GridImportKwh,
		GridExportKwh:  k.GridExportKwh,
		AutoconsumoPct: k.AutoconsumoPct,
		AhorroEur:      saved*prices.PriceImport + k.GridExportKwh*prices.PriceExport,
		Co2Kg:          k.ProductionKwh * prices.Co2PerKwh,
		DayCount:       1,
	}
}

// histSourceSplit mirrors the donut split useMemo: the day uses the exact day
// KPIs; longer periods estimate the battery share with today's ratio.
func histSourceSplit(p Period, totals PeriodTotals, dayKpis solar.DayKpis, hasDayKpis bool) sourceSplit {
	if p == PeriodDia {
		red := totals.GridImportKwh
		bateria := math.Min(dayKpis.BatteryDischargedKwh, math.Max(0, totals.ConsumptionKwh-red))
		return sourceSplit{Solar: math.Max(0, totals.ConsumptionKwh-red-bateria), Bateria: bateria, Red: red}
	}
	red := totals.GridImportKwh
	nonRed := math.Max(0, totals.ConsumptionKwh-red)
	base := dayKpis.ConsumptionKwh - dayKpis.GridImportKwh
	if base < 0.001 {
		base = 0.001
	}
	batteryRatio := 0.0
	if hasDayKpis {
		batteryRatio = dayKpis.BatteryDischargedKwh / base
	}
	if batteryRatio > 0.5 {
		batteryRatio = 0.5
	}
	bateria := nonRed * batteryRatio
	return sourceSplit{Solar: nonRed - bateria, Bateria: bateria, Red: red}
}

package web

import (
	"fmt"
	"math"

	"helios-server/internal/db"
	"helios-server/internal/solar"
)

// SVG donut generator shared by the Inversores share card (reparto) and the
// battery energy-source card (port of the recharts PieChart used by
// CompareView and EnergySourceDonut: start at 12 o'clock, clockwise).

// Donut geometry: viewBox 200x200, radii replicate innerRadius 62% /
// outerRadius 88% of recharts (half size = 100).
const (
	donutCX   = 100.0
	donutCY   = 100.0
	donutRIn  = 62.0
	donutROut = 88.0
	donutView = 200
)

// donutSliceIn is one input slice.
type donutSliceIn struct {
	Label string
	Value float64
	Color string
}

// donutSliceView is one rendered slice: precomputed arc path + texts.
type donutSliceView struct {
	Label     string
	Color     string
	Path      string
	Pct       float64
	PctText   string
	ValueText string
}

// donutModel is the render model: slices + center texts.
type donutModel struct {
	Slices      []donutSliceView
	TotalText   string
	CenterSub   string
	CenterValue string
	Lang        string
}

// donutPolar mirrors the polar() helper of SocHeroGauge.tsx: 0 deg = 3
// o'clock, 90 deg = 12 o'clock, growing clockwise.
func donutPolar(cx, cy, r, deg float64) (float64, float64) {
	a := (deg - 90) * math.Pi / 180
	return cx + r*math.Cos(a), cy + r*math.Sin(a)
}

// donutArcPath builds one annular sector from a0 to a1 degrees (clockwise).
func donutArcPath(cx, cy, rOut, rIn, a0, a1 float64) string {
	// Full-ring slice (single value): an arc with start==end renders nothing,
	// so the sweep is clamped just short of a full circle.
	if a1-a0 >= 359.99 {
		a1 = a0 + 359.9
	}
	x0, y0 := donutPolar(cx, cy, rOut, a0)
	x1, y1 := donutPolar(cx, cy, rOut, a1)
	xi1, yi1 := donutPolar(cx, cy, rIn, a1)
	xi0, yi0 := donutPolar(cx, cy, rIn, a0)
	large := 0
	if a1-a0 > 180 {
		large = 1
	}
	return fmt.Sprintf("M %.2f %.2f A %.2f %.2f 0 %d 1 %.2f %.2f L %.2f %.2f A %.2f %.2f 0 %d 0 %.2f %.2f Z",
		x0, y0, rOut, rOut, large, x1, y1,
		xi1, yi1,
		rIn, rIn, large, xi0, yi0)
}

// buildDonut computes the slice geometry. padDeg is the padding angle between
// slices (recharts paddingAngle); slices at or below 0.005 kWh drop out like
// the React filter. recharts renders startAngle=90 (its own convention, 12
// o'clock) and sweeps clockwise; in donutPolar terms that is start=0.
func buildDonut(lang string, in []donutSliceIn, padDeg float64) donutModel {
	m := donutModel{Lang: lang}
	var total float64
	for _, s := range in {
		if s.Value > 0.005 {
			total += s.Value
		}
	}
	m.TotalText = fmtEnergy(lang, total)
	if total <= 0 {
		return m
	}
	const start = 0.0
	angle := start
	for _, s := range in {
		if s.Value <= 0.005 {
			continue
		}
		span := s.Value / total * 360
		a0 := angle + padDeg/2
		a1 := angle + span - padDeg/2
		if a1 < a0 {
			a1 = a0 // slice smaller than the padding: zero-width arc
		}
		pct := s.Value / total * 100
		m.Slices = append(m.Slices, donutSliceView{
			Label:     s.Label,
			Color:     s.Color,
			Path:      donutArcPath(donutCX, donutCY, donutROut, donutRIn, a0, a1),
			Pct:       pct,
			PctText:   fmtInt(lang, pct),
			ValueText: fmtEnergy(lang, s.Value),
		})
		angle += span
	}
	return m
}

// ── Inversores page models ───────────────────────────────────────────────

// invMetaView is InvMeta in Inversores.tsx (per-inverter metadata).
type invMetaView struct {
	Key, Name, Model, Panels string
	Kwp                      float64
	HasBattery               bool
	BatteryKwh               float64
	TempC                    int
	MonthKwh                 float64
	TotalMwh                 float64
}

// invTabView is one entry of the segmented control.
type invTabView struct {
	Key, Label, Color string
	Active            bool
	IsCompare         bool
}

// invHeroModel is the InverterHeroCard port.
type invHeroModel struct {
	Aria           string
	Name           string
	ModelText      string
	Color          string
	NowText        string
	NowSignal      string
	Panels         string
	HasBattery     bool
	BatteryKwhText string
	TempC          int
	TempText       string
	TempHot        bool
	OnlineText     string
	ProducingText  string
	SolarFieldText string
	CoupledText    string
}

// invKpiView is one of the 4 per-inverter KPI cards.
type invKpiView struct {
	Label     string
	ValueText string
	Unit      string
	Accent    string
	HasDelta  bool
	DeltaText string
}

// invDayChartModel is the per-inverter day curve (InverterDayChart port:
// past area + dashed forecast + AHORA line). Same frame as the dashboard
// chart; the "compare with yesterday" toggle is not ported (mock series).
type invDayChartModel struct {
	Lang        string
	Key         string
	Color       string
	YMax        int
	Now         int
	PastPath    string
	FuturePath  string
	TicksX      []chartTick
	TicksY      []chartTick
	PlotRightX  float64
	PlotBottomY float64
}

// BuildInverterDayChart ports the InverterDayChart series geometry (domain
// dataMax + 0.4, ceil, min 1).
func BuildInverterDayChart(lang string, points []solar.DayPoint, key, color string, nowMin int) invDayChartModel {
	m := invDayChartModel{Lang: lang, Key: key, Color: color, Now: roundStep(nowMin)}

	maxV := 0.5
	for _, p := range points {
		if v := seriesInvValue(p, key); v > maxV {
			maxV = v
		}
	}
	m.YMax = int(math.Max(1, math.Ceil(maxV+0.4)))

	nowIdx := m.Now / stepMin
	if nowIdx > len(points) {
		nowIdx = len(points)
	}
	past := make([]cpoint, 0, len(points))
	future := make([]cpoint, 0, len(points))
	for i, p := range points {
		x := chartX(float64(p.T))
		y := chartY(seriesInvValue(p, key), float64(m.YMax))
		if i <= nowIdx {
			past = append(past, cpoint{x, y})
		}
		if i >= nowIdx {
			future = append(future, cpoint{x, y})
		}
	}
	baseline := chartY(0, float64(m.YMax))
	m.PastPath = areaPath(past, baseline)
	m.FuturePath = monotonePath(future)

	m.TicksX = dayTicksX()
	m.TicksY = nil
	for v := 0; v <= m.YMax; v++ {
		m.TicksY = append(m.TicksY, chartTick{X: plotLeft - 6, Y: chartY(float64(v), float64(m.YMax)) + 4, Label: fmt.Sprintf("%d", v)})
	}
	m.PlotRightX = chartW - plotRight
	m.PlotBottomY = chartH - plotBottomPad
	return m
}

// dayTicksX is the shared 3-hour X axis (0..1440).
func dayTicksX() []chartTick {
	var ticks []chartTick
	for _, t := range axisTicksX {
		ticks = append(ticks, chartTick{X: chartX(float64(t)), Y: chartH - 6, Label: fmtTime(t)})
	}
	return ticks
}

// BuildCompareChart ports CompareChart: all inverter curves superposed with
// their own color (no production/consumption series). Normalization by kWp is
// not ported (interactive toggle).
func BuildCompareChart(lang string, points []solar.DayPoint, nowMin int, keys []string, names map[string]string, colors []string) DayChartModel {
	m := DayChartModel{Lang: lang, Now: roundStep(nowMin)}

	maxV := 0.5
	for _, p := range points {
		for _, k := range keys {
			if v := seriesInvValue(p, k); v > maxV {
				maxV = v
			}
		}
	}
	m.YMax = int(math.Max(1, math.Ceil(maxV+0.4)))

	pts := make(map[string][]cpoint, len(keys))
	for _, k := range keys {
		pts[k] = make([]cpoint, 0, len(points))
	}
	for _, p := range points {
		x := chartX(float64(p.T))
		for _, k := range keys {
			pts[k] = append(pts[k], cpoint{x, chartY(seriesInvValue(p, k), float64(m.YMax))})
		}
	}
	for ci, k := range keys {
		name := k
		if n, ok := names[k]; ok && n != "" {
			name = n
		}
		color := "var(--c-solar)"
		if ci < len(colors) && colors[ci] != "" {
			color = colors[ci]
		}
		m.InvPaths = append(m.InvPaths, DayChartInv{Key: k, Name: name, Var: color, Path: monotonePath(pts[k])})
	}
	m.TicksX = dayTicksX()
	for v := 0; v <= m.YMax; v++ {
		m.TicksY = append(m.TicksY, chartTick{X: plotLeft - 6, Y: chartY(float64(v), float64(m.YMax)) + 4, Label: fmt.Sprintf("%d", v)})
	}
	m.PlotRightX = chartW - plotRight
	m.PlotBottomY = chartH - plotBottomPad
	return m
}

// ── Last 14 days bars (Last14Chart port) ────────────────────────────────

const (
	last14W      = 1100.0
	last14H      = 240.0
	last14Top    = 10.0
	last14SideL  = 12.0
	last14SideR  = 12.0
	last14Bottom = 24.0
	last14BarR   = 4.0
)

// last14Bar is one daily bar.
type last14Bar struct {
	X, W, Y, H float64
	Label      string // day of month
	KwhText    string
	IsLast     bool
	ShowLabel  bool // recharts XAxis interval=1: every other tick
}

// last14Model is the whole bar chart.
type last14Model struct {
	Lang        string
	Color       string
	Bars        []last14Bar
	PlotBottomY float64
}

// topRoundedBarPath is a bar with rounded top corners (recharts radius [4,4,0,0]).
func topRoundedBarPath(x, y, w, h, r float64) string {
	if h <= 0 || w <= 0 {
		return ""
	}
	if r > h {
		r = h
	}
	if r > w/2 {
		r = w / 2
	}
	x2 := x + w
	yb := y + h
	return fmt.Sprintf("M %.2f %.2f L %.2f %.2f Q %.2f %.2f %.2f %.2f L %.2f %.2f Q %.2f %.2f %.2f %.2f L %.2f %.2f Z",
		x, yb, x, y+r, x, y, x+r, y, x2-r, y, x2, y, x2, y+r, x2, yb)
}

// bottomRoundedBarPath is a bar with rounded bottom corners (radius [0,0,3,3]).
func bottomRoundedBarPath(x, y, w, h, r float64) string {
	if h <= 0 || w <= 0 {
		return ""
	}
	if r > h {
		r = h
	}
	if r > w/2 {
		r = w / 2
	}
	x2 := x + w
	yb := y + h
	return fmt.Sprintf("M %.2f %.2f L %.2f %.2f Q %.2f %.2f %.2f %.2f L %.2f %.2f Q %.2f %.2f %.2f %.2f L %.2f %.2f Z",
		x, y, x, yb-r, x, yb, x+r, yb, x2-r, yb, x2, yb, x2, yb-r, x2, y)
}

// buildLast14 ports Last14Chart: daily production * share for the trailing 14
// days, last bar full opacity, XAxis interval=1.
func buildLast14(lang string, rows []db.DailyRow, share float64, colorHex string) last14Model {
	m := last14Model{Lang: lang, Color: colorHex, PlotBottomY: last14H - last14Bottom}
	window := lastN(rows, 14)
	if len(window) == 0 {
		return m
	}
	maxKwh := 0.0
	kwhs := make([]float64, len(window))
	for i, r := range window {
		kwhs[i] = derefF(r.ProductionKwh) * share
		if kwhs[i] > maxKwh {
			maxKwh = kwhs[i]
		}
	}
	// recharts domain [0, dataMax + 4]
	yMax := maxKwh + 4
	if yMax <= 0 {
		yMax = 4
	}
	plotW := last14W - last14SideL - last14SideR
	plotH := last14H - last14Top - last14Bottom
	slot := plotW / float64(len(window))
	barW := slot * 0.7
	for i, r := range window {
		day := 1
		if tt, ok := parseDayDate(r.Date); ok {
			day = tt.Day()
		}
		h := kwhs[i] / yMax * plotH
		x := last14SideL + float64(i)*slot + (slot-barW)/2
		m.Bars = append(m.Bars, last14Bar{
			X: x, W: barW,
			Y:       m.PlotBottomY - h,
			H:       h,
			Label:   fmtInt(lang, float64(day)),
			KwhText: fmtEnergy(lang, kwhs[i]),
			IsLast:  i == len(window)-1,
			// interval=1: first tick hidden, odd indices labeled
			ShowLabel: i%2 == 1,
		})
	}
	return m
}

// ── Compare view models ─────────────────────────────────────────────────

// invCompareHero is one cell of the comparison hero.
type invCompareHero struct {
	Key, Name, Color string
	KwpText          string
	NowText          string
	NowSignal        string
	KwhText          string
	TodayLabel       string
}

// invTableCell is one metric cell; Chip renders the online pill instead of text.
type invTableCell struct {
	Text  string
	Color string
	Best  bool
	Chip  bool
}

// invTableRow is one metrics-table row.
type invTableRow struct {
	Label string
	Cells []invTableCell
}

// invShareBar is one segment of the horizontal share bar.
type invShareBar struct {
	Name      string
	Color     string
	WidthText string
	LabelText string
}

// invCompareView is the whole compare tab content.
type invCompareView struct {
	Heroes       []invCompareHero
	Chart        DayChartModel
	HeadNames    []string
	HeadColors   []string
	Rows         []invTableRow
	Share        donutModel
	ShareBars    []invShareBar
	BestAria     string
	ShareInsight string
}

// bestIdx mirrors the best() helper of CompareView: -1 (null) with fewer than
// 2 values or when every value ties; else the index of the maximum.
func bestIdx(values []float64) int {
	if len(values) < 2 {
		return -1
	}
	max := values[0]
	for _, v := range values[1:] {
		if v > max {
			max = v
		}
	}
	for _, v := range values {
		if v != max {
			for i, v2 := range values {
				if v2 == max {
					return i
				}
			}
		}
	}
	return -1
}

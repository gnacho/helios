package web

import (
	"fmt"
	"math"
	"strings"

	"helios-server/internal/solar"
)

// Day chart model ported from app/src/components/DayChart.tsx (Recharts
// ComposedChart). Geometry and style mirror the React chart; colors reference
// CSS variables (var(--c-solar)...) so the SVG follows the theme without a
// server re-render.

const (
	chartW        = 1100.0
	chartH        = 380.0
	plotLeft      = 34.0
	plotRight     = 12.0
	plotTop       = 10.0
	plotBottomPad = 24.0 // x-axis labels
)

var axisTicksX = [9]int{0, 180, 360, 540, 720, 900, 1080, 1260, 1440}

// DayChartModel holds the precomputed geometry for the templ renderer.
type DayChartModel struct {
	Lang string
	YMax int
	Now  int // minutes, rounds to STEP_MIN

	ProdPastPath   string // area (filled) up to now
	ProdFuturePath string // dashed line from now on
	ConsPath       string // consumption area
	InvPaths       []DayChartInv

	TicksX   []chartTick
	TicksY   []chartTick
	PlotRightX float64
	PlotBottomY float64
}

type DayChartInv struct {
	Key  string
	Name string
	Var  string // CSS var, e.g. "var(--c-solis)"
	Path string
}

type chartTick struct {
	X, Y  float64
	Label string
}

func chartX(t float64) float64 { return plotLeft + t/1440*(chartW-plotLeft-plotRight) }

func chartY(v, yMax float64) float64 {
	top, bottom := plotTop, chartH-plotBottomPad
	return top + (1-v/yMax)*(bottom-top)
}

// BuildDayChart computes the SVG geometry for the day series.
func BuildDayChart(lang string, points []solar.DayPoint, nowMin int, invKeys []string, invNames map[string]string) DayChartModel {
	m := DayChartModel{Lang: lang, Now: roundStep(nowMin)}

	// yMax replica: max de producción/consumo/inversores visibles +0.3, ceil, min 1.
	maxV := 0.5
	for _, p := range points {
		if p.Production > maxV {
			maxV = p.Production
		}
		if p.Consumption > maxV {
			maxV = p.Consumption
		}
		for _, k := range invKeys {
			if v := seriesInvValue(p, k); v > maxV {
				maxV = v
			}
		}
	}
	m.YMax = int(math.Max(1, math.Ceil(maxV+0.3)))

	nowIdx := m.Now / stepMin
	if nowIdx > len(points) {
		nowIdx = len(points)
	}

	past := make([]cpoint, 0, len(points))
	future := make([]cpoint, 0, len(points))
	cons := make([]cpoint, 0, len(points))
	invPts := make(map[string][]cpoint, len(invKeys))
	for _, k := range invKeys {
		invPts[k] = make([]cpoint, 0, len(points))
	}
	for i, p := range points {
		x := chartX(float64(p.T))
		if i <= nowIdx {
			past = append(past, cpoint{x, chartY(p.Production, float64(m.YMax))})
		}
		if i >= nowIdx {
			future = append(future, cpoint{x, chartY(p.Production, float64(m.YMax))})
		}
		cons = append(cons, cpoint{x, chartY(p.Consumption, float64(m.YMax))})
		for _, k := range invKeys {
			invPts[k] = append(invPts[k], cpoint{x, chartY(seriesInvValue(p, k), float64(m.YMax))})
		}
	}

	baseline := chartY(0, float64(m.YMax))
	m.ProdPastPath = areaPath(past, baseline)
	m.ProdFuturePath = monotonePath(future)
	m.ConsPath = areaPath(cons, baseline)
	for _, k := range invKeys {
		name := k
		if n, ok := invNames[k]; ok && n != "" {
			name = n
		}
		m.InvPaths = append(m.InvPaths, DayChartInv{
			Key: k, Name: name, Var: inverterVar(k),
			Path: monotonePath(invPts[k]),
		})
	}

	for _, t := range axisTicksX {
		m.TicksX = append(m.TicksX, chartTick{X: chartX(float64(t)), Y: chartH - 6, Label: fmtTime(t)})
	}
	for v := 0; v <= m.YMax; v++ {
		m.TicksY = append(m.TicksY, chartTick{X: plotLeft - 6, Y: chartY(float64(v), float64(m.YMax)) + 4, Label: fmt.Sprintf("%d", v)})
	}
	m.PlotRightX = chartW - plotRight
	m.PlotBottomY = chartH - plotBottomPad
	return m
}

func inverterVar(key string) string {
	switch key {
	case "solis":
		return "var(--c-solis)"
	case "fox":
		return "var(--c-fox)"
	}
	return "var(--c-solar)"
}

// r2 rounds coordinates to 2 decimals for clean SVG output.
func r2(v float64) float64 { return math.Round(v*100) / 100 }

func roundStep(min int) int { return int(math.Round(float64(min)/stepMin)) * stepMin }

type cpoint struct{ x, y float64 }

// areaPath builds a closed area path (curve + baseline) from the points.
func areaPath(pts []cpoint, baseline float64) string {
	line := monotonePath(pts)
	if line == "" {
		return ""
	}
	first := pts[0]
	last := pts[len(pts)-1]
	return fmt.Sprintf("%s L %.2f %.2f L %.2f %.2f Z", line, last.x, baseline, first.x, baseline)
}

// monotonePath renders an SVG path with monotone cubic interpolation
// (Fritsch-Carlson tangents, the "monotone" curve of Recharts), splitting
// into separate M...C runs when the x gap is not exactly one step (data gaps).
func monotonePath(pts []cpoint) string {
	if len(pts) == 0 {
		return ""
	}
	if len(pts) == 1 {
		return fmt.Sprintf("M %.2f %.2f", pts[0].x, pts[0].y)
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("M %.2f %.2f", pts[0].x, pts[0].y))
	// Tangentes por tramos contiguos (hueco = salto x anómalo).
	start := 0
	for i := 1; i <= len(pts); i++ {
		if i == len(pts) || pts[i].x-pts[i-1].x > stepMin+0.01 {
			writeMonotoneRun(&b, pts[start:i])
			if i < len(pts) {
				b.WriteString(fmt.Sprintf(" M %.2f %.2f", pts[i].x, pts[i].y))
			}
			start = i
		}
	}
	return b.String()
}

func writeMonotoneRun(b *strings.Builder, pts []cpoint) {
	n := len(pts)
	if n < 2 {
		return
	}
	// Deltas y tangentes Fritsch-Carlson.
	d := make([]float64, n-1)
	m := make([]float64, n)
	for i := 0; i < n-1; i++ {
		d[i] = (pts[i+1].y - pts[i].y) / (pts[i+1].x - pts[i].x)
	}
	m[0] = d[0]
	m[n-1] = d[n-2]
	for i := 1; i < n-1; i++ {
		if d[i-1]*d[i] <= 0 {
			m[i] = 0
		} else {
			w1 := 2*(pts[i+1].x-pts[i].x) + (pts[i].x - pts[i-1].x)
			w2 := (pts[i+1].x-pts[i].x) + 2*(pts[i].x-pts[i-1].x)
			m[i] = (w1 + w2) / (w1/d[i-1] + w2/d[i])
		}
	}
	for i := 0; i < n-1; i++ {
		h := pts[i+1].x - pts[i].x
		c1x := pts[i].x + h/3
		c1y := pts[i].y + m[i]*h/3
		c2x := pts[i+1].x - h/3
		c2y := pts[i+1].y - m[i+1]*h/3
		b.WriteString(fmt.Sprintf(" C %.2f %.2f %.2f %.2f %.2f %.2f", c1x, c1y, c2x, c2y, pts[i+1].x, pts[i+1].y))
	}
}

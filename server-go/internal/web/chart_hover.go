package web

// Chart hover layer: the server-rendered port of the Recharts <Tooltip> the
// React app attached to every chart (a vertical cursor and a pill with the
// values at the hovered time). There is no client JS for charts, so each
// column of data gets an invisible band that reveals its own pill with CSS
// (:hover on the group). The bands are transparent, so a pill is never covered
// by the bands that come after it in the SVG paint order.

import (
	"math"

	"helios-server/internal/solar"
)

const (
	hoverBucketMin = 15  // tooltip granularity: 15 min in a 24 h chart
	hoverTipPad    = 8.0 // pill inner padding
	hoverLineH     = 13.0
	hoverTitleH    = 15.0
	hoverDotGap    = 6.0
	hoverGapLeft   = 17.0 // dot + gap before the label
	hoverTitleGap  = 6.0
)

// hoverRow is one series line inside a tooltip.
type hoverRow struct {
	Color string // dot color (CSS var or hex)
	Label string
	Value string
}

// hoverColRow is a laid-out row: coordinates are precomputed because templ
// renders plain SVG elements.
type hoverColRow struct {
	DotX, DotY     float64
	LabelX, ValueX float64
	TextY          float64
	Color          string
	Label, Value   string
}

// hoverCol is one hoverable column of a chart.
type hoverCol struct {
	BandX, BandW float64 // invisible hit band
	CursorX      float64 // dashed vertical line
	TipX, TipY   float64
	TipW, TipH   float64
	TitleX       float64
	TitleY       float64
	Title        string
	Rows         []hoverColRow
	// Href makes the hover band the click area too (charts with drill-down).
	Href string
}

// hoverSample is one tooltip's content before the layout is computed.
type hoverSample struct {
	X, W  float64 // band centre and width
	Title string
	Rows  []hoverRow
}

// hoverBox is the plot area the pills must stay inside.
type hoverBox struct{ Left, Right, Top, Bottom float64 }

// buildHoverCols lays the pills out at the top of the plot, on the side of the
// cursor that has room.
func buildHoverCols(samples []hoverSample, bx hoverBox) []hoverCol {
	if len(samples) == 0 {
		return nil
	}
	cols := make([]hoverCol, 0, len(samples))
	for _, s := range samples {
		if len(s.Rows) == 0 {
			continue
		}
		w := heatTipWidth(s.Title)
		for _, r := range s.Rows {
			if rw := hoverGapLeft + heatTipWidth(r.Label) + hoverDotGap + heatTipWidth(r.Value); rw > w {
				w = rw
			}
		}
		w += hoverTipPad * 2
		h := hoverTitleGap + hoverTitleH + float64(len(s.Rows))*hoverLineH + 5

		x := s.X + 12
		if x+w > bx.Right {
			x = s.X - 12 - w
		}
		x = math.Max(bx.Left, math.Min(x, bx.Right-w))
		y := bx.Top + 2

		c := hoverCol{
			BandX:   s.X - s.W/2,
			BandW:   s.W,
			CursorX: s.X,
			TipX:    x,
			TipY:    y,
			TipW:    w,
			TipH:    h,
			TitleX:  x + hoverTipPad,
			TitleY:  y + hoverTitleGap + 10,
			Title:   s.Title,
		}
		for i, r := range s.Rows {
			textY := y + hoverTitleGap + hoverTitleH + float64(i)*hoverLineH + 10
			c.Rows = append(c.Rows, hoverColRow{
				DotX:   x + hoverTipPad + 4,
				DotY:   textY - 3.5,
				LabelX: x + hoverTipPad + 10,
				ValueX: x + w - hoverTipPad,
				TextY:  textY,
				Color:  r.Color,
				Label:  r.Label,
				Value:  r.Value,
			})
		}
		cols = append(cols, c)
	}
	return cols
}

// dayBuckets averages the 5-min series into tooltip buckets, returning the
// start minute of each bucket and the averaged point.
func dayBuckets(lang string, points []solar.DayPoint, minutes int) ([]int, []solar.DayPoint) {
	if len(points) == 0 || minutes <= 0 {
		return nil, nil
	}
	starts := make([]int, 0, 1440/minutes)
	avg := make([]solar.DayPoint, 0, 1440/minutes)
	for start := 0; start < 1440; start += minutes {
		end := start + minutes
		var sum solar.DayPoint
		var n int
		invs := map[string]float64{}
		for _, p := range points {
			if p.T < start || p.T >= end {
				continue
			}
			sum.Production += p.Production
			sum.Consumption += p.Consumption
			sum.BatteryPower += p.BatteryPower
			sum.Soc += p.Soc
			for k, v := range p.Inverters {
				invs[k] += v
			}
			n++
		}
		if n == 0 {
			continue
		}
		row := solar.DayPoint{
			T:            start,
			Label:        fmtTime(start),
			Production:   sum.Production / float64(n),
			Consumption:  sum.Consumption / float64(n),
			BatteryPower: sum.BatteryPower / float64(n),
			Soc:          sum.Soc / float64(n),
		}
		if len(invs) > 0 {
			row.Inverters = map[string]float64{}
			for k, v := range invs {
				row.Inverters[k] = v / float64(n)
			}
		}
		starts = append(starts, start)
		avg = append(avg, row)
	}
	_ = lang
	return starts, avg
}

// dayHoverColumns builds the tooltip columns of a 24 h chart. val returns the
// value of one series at the averaged point of the bucket.
func dayHoverColumns(points []solar.DayPoint, xOf func(float64) float64, bx hoverBox, rowsFor func(p solar.DayPoint) []hoverRow) []hoverCol {
	starts, avg := dayBuckets("", points, hoverBucketMin)
	if len(starts) == 0 {
		return nil
	}
	step := xOf(float64(hoverBucketMin)) - xOf(0)
	samples := make([]hoverSample, 0, len(starts))
	for i, p := range avg {
		samples = append(samples, hoverSample{
			X:     xOf(float64(starts[i])),
			W:     step,
			Title: fmtTime(starts[i]),
			Rows:  rowsFor(p),
		})
	}
	return buildHoverCols(samples, bx)
}

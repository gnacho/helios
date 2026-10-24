package web

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"

	"helios-server/internal/auth"
	"helios-server/internal/db"
	"helios-server/internal/solar"
)

// /bateria page (port of app/src/pages/Bateria.tsx plus SocHeroGauge,
// SocDayChart, BatteryPowerChart and the energy-source donut of
// EnergySourceDonut.tsx). Static server render; the GSAP sweep of the gauge
// and the hover tooltips are not ported.

// ── SOC hero gauge (SocHeroGauge.tsx) ───────────────────────────────────

// Gauge geometry from SocHeroGauge.tsx: 240 deg arc with the gap at the
// bottom, from 150 deg to 390 deg in donutPolar terms.
const (
	gaugeCX, gaugeCY = 130.0, 132.0
	gaugeR           = 100.0
	gaugeStart       = 150.0
	gaugeSweep       = 240.0
	gaugeBatteryKwh  = 5.0 // battery size used by the hero subtitle (React constant)
)

type socGaugeModel struct {
	Lang       string
	Soc        float64
	Color      string
	TrackPath  string
	ValuePath  string
	TickX1     float64
	TickY1     float64
	TickX2     float64
	TickY2     float64
	LabelX     float64
	LabelY     float64
	ReserveLbl string
	SocText    string
	KwhText    string
	Aria       string
	BuyColor   string
}

// gaugeArcPath is the arcPath helper of SocHeroGauge.tsx.
func gaugeArcPath(startDeg, endDeg float64) string {
	x0, y0 := donutPolar(gaugeCX, gaugeCY, gaugeR, startDeg)
	x1, y1 := donutPolar(gaugeCX, gaugeCY, gaugeR, endDeg)
	large := 0
	if endDeg-startDeg > 180 {
		large = 1
	}
	return fmt.Sprintf("M %.2f %.2f A %.2f %.2f 0 %d 1 %.2f %.2f", x0, y0, gaugeR, gaugeR, large, x1, y1)
}

func buildSocGauge(lang string, soc float64, pal Palette) socGaugeModel {
	clamped := clampPct(soc)
	m := socGaugeModel{
		Lang:      lang,
		Soc:       clamped,
		Color:     socGaugeColor(clamped, pal),
		TrackPath: gaugeArcPath(gaugeStart, gaugeStart+gaugeSweep),
		BuyColor:  pal.RedCompra,
		SocText:   fmtInt(lang, clamped),
		KwhText: T(lang, "bateria.ofTotalKwh", map[string]string{
			"kwh":   fmtEnergy(lang, clamped/100*gaugeBatteryKwh),
			"total": fmtEnergy(lang, gaugeBatteryKwh),
		}),
		Aria:       T(lang, "bateria.gaugeAria", map[string]string{"pct": fmtInt(lang, clamped)}),
		ReserveLbl: T(lang, "bateria.reservePct", map[string]string{"pct": fmtInt(lang, BatteryReservePct)}),
	}
	if clamped > 0 {
		m.ValuePath = gaugeArcPath(gaugeStart, gaugeStart+gaugeSweep*clamped/100)
	}
	resAngle := gaugeStart + gaugeSweep*BatteryReservePct/100
	m.TickX1, m.TickY1 = donutPolar(gaugeCX, gaugeCY, gaugeR-13, resAngle)
	m.TickX2, m.TickY2 = donutPolar(gaugeCX, gaugeCY, gaugeR+9, resAngle)
	m.LabelX, m.LabelY = donutPolar(gaugeCX, gaugeCY, gaugeR-30, resAngle)
	return m
}

// ── SOC day curve (SocDayChart.tsx) ─────────────────────────────────────

type socEventDot struct {
	X, Y  float64
	Label string
	Past  bool
}

type socDayChartModel struct {
	W, H        float64 // viewBox
	Lang        string
	PastPath    string
	PastLine    string
	FuturePath  string
	Events      []socEventDot
	TicksX      []chartTick
	TicksY      []chartTick
	ReserveY    float64
	ReserveLbl  string
	NowX        float64
	PlotRightX  float64
	PlotBottomY float64
	ColorBat    string
	ColorBuy    string
	Hover       []hoverCol
}

// buildSocDayChart ports the SOC curve: fixed 0..100 domain with 20-steps,
// reserve zone below 20 %, dashed projection after "now" and the three event
// annotations (charge start, full, discharge start).
func buildSocDayChart(lang string, points []solar.DayPoint, nowMin int, pal Palette) socDayChartModel {
	g := geomBatSoc
	m := socDayChartModel{
		W:           g.W,
		H:           g.H,
		Lang:        lang,
		ColorBat:    pal.Bateria,
		ColorBuy:    pal.RedCompra,
		ReserveLbl:  T(lang, "bateria.reserve"),
		NowX:        g.x(float64(roundStep(nowMin))),
		TicksX:      dayTicksX(g),
		PlotRightX:  g.rightX(),
		PlotBottomY: g.bottomY(),
	}
	for _, v := range []int{0, 20, 40, 60, 80, 100} {
		m.TicksY = append(m.TicksY, chartTick{X: plotLeft - 6, Y: g.y(float64(v), 100) + 4, Label: fmt.Sprintf("%d", v)})
	}
	m.ReserveY = g.y(BatteryReservePct, 100)
	m.Hover = dayHoverColumns(points, g.x,
		hoverBox{Left: plotLeft, Right: g.rightX(), Top: plotTop, Bottom: g.bottomY()},
		func(p solar.DayPoint) []hoverRow {
			return []hoverRow{{Color: pal.Bateria, Label: T(lang, "bateria.socRow"), Value: fmtInt(lang, p.Soc) + " %"}}
		})

	nowIdx := roundStep(nowMin) / stepMin
	if nowIdx > len(points) {
		nowIdx = len(points)
	}
	past := make([]cpoint, 0, len(points))
	future := make([]cpoint, 0, len(points))
	for i, p := range points {
		x := g.x(float64(p.T))
		y := g.y(p.Soc, 100)
		if i <= nowIdx {
			past = append(past, cpoint{x, y})
		}
		if i >= nowIdx {
			future = append(future, cpoint{x, y})
		}
	}
	baseline := g.y(0, 100)
	m.PastPath = areaPath(past, baseline)
	m.PastLine = monotoneLine(past)
	m.FuturePath = monotonePath(future)

	addEvent := func(p solar.DayPoint, key string) {
		m.Events = append(m.Events, socEventDot{
			X:     g.x(float64(p.T)),
			Y:     g.y(p.Soc, 100),
			Label: T(lang, key, map[string]string{"time": p.Label}),
			Past:  p.T <= nowMin,
		})
	}
	for _, p := range points {
		if p.BatteryPower > 0.05 {
			addEvent(p, "bateria.chargeStart")
			break
		}
	}
	for _, p := range points {
		if p.Soc >= 99.5 {
			addEvent(p, "bateria.socFull")
			break
		}
	}
	for _, p := range points {
		if p.BatteryPower < -0.05 {
			addEvent(p, "bateria.dischargeStart")
			break
		}
	}
	return m
}

// ── Battery power bars (BatteryPowerChart.tsx) ──────────────────────────

const (
	batW         = 570.0 // geomBatPower.W (card is lg:col-span-6)
	batH         = 260.0 // geomBatPower.H
	batTop       = 10.0
	batSideL     = 34.0
	batSideR     = 12.0
	batBottom    = 32.0
	batYMax      = 2.6 // minimum domain (react [-2.6, 2.6]); grows with the data
	batBucketMin = 15
)

type batBucket struct {
	X, W                   float64
	ChargeY, ChargeH       float64
	DischargeY, DischargeH float64
}

type batPowerChartModel struct {
	YMax           float64
	Lang           string
	Buckets        []batBucket
	TicksX         []chartTick
	TicksY         []chartTick
	ZeroY          float64
	PlotRightX     float64
	ColorCharge    string
	ColorDischarge string
	Hover          []hoverCol
}

func batY(v, yMax float64) float64 {
	plotH := batH - batTop - batBottom
	return batTop + (1-(v+yMax)/(2*yMax))*plotH
}

// buildBatPowerChart ports the divergent 15-min bar chart: charge (battery
// color) up, discharge (solar color) down, axis centered on 0.
func buildBatPowerChart(lang string, points []solar.DayPoint, pal Palette) batPowerChartModel {
	// Domain grows with the data: a 2.7 kW spike must not overrun the axis
	// labels (react uses a fixed [-2.6, 2.6] and clips).
	maxAbs := 0.0
	for _, p := range points {
		if a := abs(p.BatteryPower); a > maxAbs {
			maxAbs = a
		}
	}
	yMax := math.Max(batYMax, math.Ceil(maxAbs*10)/10)
	m := batPowerChartModel{
		YMax:           yMax,
		Lang:           lang,
		ColorCharge:    pal.Bateria,
		ColorDischarge: pal.Solar,
		ZeroY:          batY(0, yMax),
		PlotRightX:     batW - batSideR,
	}
	for _, t := range []int{0, 240, 480, 720, 960, 1200, 1440} {
		m.TicksX = append(m.TicksX, chartTick{X: batX(float64(t)), Y: batH - 8, Label: fmtTime(t)})
	}
	// Fixed ticks like the React YAxis (they stay inside the grown domain).
	for _, v := range []int{-2, -1, 0, 1, 2} {
		m.TicksY = append(m.TicksY, chartTick{X: batSideL - 6, Y: batY(float64(v), yMax) + 4, Label: fmt.Sprintf("%d", v)})
	}

	plotW := batW - batSideL - batSideR
	nBuckets := 24 * 60 / batBucketMin
	slot := plotW / float64(nBuckets)

	// Tooltip: the React chart showed "Cargando/Descargando X kW" per bucket.
	// Every bar is hoverable, including the empty ones (the bars cover the day).
	samples := make([]hoverSample, 0, nBuckets)
	for b := 0; b < nBuckets; b++ {
		start := b * batBucketMin
		end := start + batBucketMin
		var sum float64
		var n int
		for _, p := range points {
			if p.T >= start && p.T < end {
				sum += p.BatteryPower
				n++
			}
		}
		value := 0.0
		if n > 0 {
			value = sum / float64(n)
		}
		label := T(lang, "common.charging")
		if value < -0.005 {
			label = T(lang, "common.discharging")
		}
		samples = append(samples, hoverSample{
			X:     batX(float64(start)),
			W:     slot,
			Title: fmtTime(start),
			Rows:  []hoverRow{{Color: pal.Bateria, Label: label, Value: fmtKwVal(lang, abs(value)) + " kW"}},
		})
	}
	m.Hover = buildHoverCols(samples, hoverBox{Left: batSideL, Right: batW - batSideR, Top: batTop, Bottom: batH - batBottom})
	barW := slot * 0.7
	for b := 0; b < nBuckets; b++ {
		start := b * batBucketMin
		end := start + batBucketMin
		var sum float64
		var n int
		for _, p := range points {
			if p.T >= start && p.T < end {
				sum += p.BatteryPower
				n++
			}
		}
		avg := 0.0
		if n > 0 {
			avg = sum / float64(n)
		}
		bucket := batBucket{X: batSideL + float64(b)*slot + (slot-barW)/2, W: barW}
		switch {
		case avg > 0.005:
			bucket.ChargeY = batY(avg, yMax)
			bucket.ChargeH = batY(0, yMax) - batY(avg, yMax)
		case avg < -0.005:
			bucket.DischargeY = batY(0, yMax)
			bucket.DischargeH = batY(avg, yMax) - batY(0, yMax)
		}
		m.Buckets = append(m.Buckets, bucket)
	}
	return m
}

func batX(t float64) float64 {
	return batSideL + t/1440*(batW-batSideL-batSideR)
}

// ── Week balance (Esta semana) ──────────────────────────────────────────

const (
	weekW      = 767.0 // card is lg:col-span-8
	weekH      = 240.0
	weekTop    = 10.0
	weekSideL  = 30.0
	weekSideR  = 12.0
	weekBottom = 24.0
	weekYMax   = 8.0
)

type weekDayBar struct {
	Label                                string
	X                                    float64
	BarW                                 float64
	CargaY, CargaH, DescargaY, DescargaH float64
}

type weekBalanceModel struct {
	Lang           string
	Days           []weekDayBar
	TicksY         []chartTick
	PlotBottomY    float64
	ColorCharge    string
	ColorDischarge string
}

// buildWeekBalance ports the weekly charge/discharge grouped bars (domain
// 0..8, ticks every 2, one narrow weekday letter per day).
func buildWeekBalance(lang string, rows []db.DailyRow, pal Palette) weekBalanceModel {
	m := weekBalanceModel{
		Lang:           lang,
		ColorCharge:    pal.Bateria,
		ColorDischarge: pal.Solar,
		PlotBottomY:    weekH - weekBottom,
	}
	for _, v := range []int{0, 2, 4, 6, 8} {
		m.TicksY = append(m.TicksY, chartTick{X: weekSideL - 6, Y: weekY(float64(v)) + 4, Label: fmt.Sprintf("%d", v)})
	}
	days := lastN(rows, 7)
	if len(days) == 0 {
		return m
	}
	plotW := weekW - weekSideL - weekSideR
	slot := plotW / float64(len(days))
	barW := slot * 0.3
	for i, r := range days {
		bar := weekDayBar{X: weekSideL + float64(i)*slot + (slot-barW*2)/2, BarW: barW}
		if tt, ok := parseDayDate(r.Date); ok {
			bar.Label = weekdayNarrow(lang, tt.Weekday())
		}
		if c := derefF(r.BatteryChargedKwh); c > 0 {
			bar.CargaH = c / weekYMax * (weekH - weekTop - weekBottom)
			bar.CargaY = m.PlotBottomY - bar.CargaH
		}
		if d := derefF(r.BatteryDischargedKwh); d > 0 {
			bar.DescargaH = d / weekYMax * (weekH - weekTop - weekBottom)
			bar.DescargaY = m.PlotBottomY - bar.DescargaH
		}
		m.Days = append(m.Days, bar)
	}
	return m
}

func weekY(v float64) float64 {
	plotH := weekH - weekTop - weekBottom
	return weekTop + (1-v/weekYMax)*plotH
}

// ── Page view + handler ─────────────────────────────────────────────────

// batStat is one compact stat row of the hero card.
type batStat struct {
	IconKey string // arrowDown | arrowUp | refresh | pulse | thermo | clock
	Label   string
	Value   string
}

// bateriaChip is the state pill under the gauge.
type bateriaChip struct {
	Text string
	Cls  string
}

// bateriaView carries everything BateriaPage renders.
type bateriaView struct {
	pageProps
	Signals string
	Palette Palette
	Gauge   socGaugeModel
	Chip    bateriaChip
	Stats   []batStat
	Power   batPowerChartModel
	Soc     socDayChartModel
	Week    weekBalanceModel
	WeekAvg int
	Donut   donutModel
}

func (u *UI) handleBateria(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	lang := u.userLang(userID)
	p := u.baseProps(r, userID, "/bateria", "")
	p.Title = T(lang, "nav.bateria")

	pal := newPalette(p.Theme == "dark")
	m := u.buildDashboard(pal)
	if k, ok := u.exactDayKpis(m); ok {
		m.Kpis = k
	}
	hist := u.dailyHistory()
	week := lastN(hist, 7)

	signals := map[string]any{
		"live": map[string]any{
			"soc":           liveSoc(m.Live),
			"socText":       fmtInt(lang, liveSoc(m.Live)),
			"batteryKwText": fmtKwVal(lang, 0),
			"batteryStatus": T(lang, "common.idle"),
		},
	}
	if m.Live != nil {
		signals["live"] = map[string]any{
			"soc":           m.Live.Soc,
			"socText":       fmtInt(lang, m.Live.Soc),
			"batteryKwText": fmtKwVal(lang, abs(m.Live.BatteryPower)),
			"batteryStatus": T(lang, "common."+batteryState(m.Live.BatteryPower)),
		}
	}
	sigJSON, err := json.Marshal(signals)
	if err != nil {
		sigJSON = []byte("{}")
	}

	v := bateriaView{
		pageProps: p,
		Signals:   string(sigJSON),
		Palette:   pal,
		Gauge:     buildSocGauge(lang, liveSoc(m.Live), pal),
		Power:     buildBatPowerChart(lang, m.Series.Points, pal),
		Soc:       buildSocDayChart(lang, m.Series.Points, untilMin(m), pal),
		Week:      buildWeekBalance(lang, week, pal),
		WeekAvg:   avgDailyAutoconsumo(week),
		Stats:     buildBatStats(lang, m),
		Chip:      buildBatChip(lang, m.Live),
	}
	// Donut "de dónde vino la energía" (fórmula del periodo día de Historico).
	if m.Kpis.ConsumptionKwh > 0 {
		split := sourceSplitFromKpis(m.Kpis)
		v.Donut = buildDonut(lang, []donutSliceIn{
			{Label: T(lang, "historico.solarDirect"), Value: split.Solar, Color: pal.Solar},
			{Label: T(lang, "common.battery"), Value: split.Bateria, Color: pal.Bateria},
			{Label: T(lang, "common.grid"), Value: split.Red, Color: pal.RedCompra},
		}, 2)
		v.Donut.CenterSub = T(lang, "historico.consumed")
	}

	_ = BateriaPage(v).Render(r.Context(), w)
}

func buildBatStats(lang string, m dashboardModel) []batStat {
	charged := fmtEnergy(lang, m.Kpis.BatteryChargedKwh)
	discharged := fmtEnergy(lang, m.Kpis.BatteryDischargedKwh)
	return []batStat{
		{"arrowDown", T(lang, "bateria.chargedToday"), charged + " kWh"},
		{"arrowUp", T(lang, "bateria.dischargedToday"), discharged + " kWh"},
		{"refresh", T(lang, "bateria.cycles"), "312"},
		{"pulse", T(lang, "bateria.health"), "98 %"},
		{"thermo", T(lang, "bateria.temperature"), "28 °C"},
		{"clock", T(lang, "bateria.autonomy"), "~3 h"},
	}
}

// buildBatChip mirrors the stateChip map of Bateria.tsx.
func buildBatChip(lang string, live *solar.LiveData) bateriaChip {
	if live == nil {
		return bateriaChip{Text: T(lang, "common.idle"), Cls: "bg-surface-2 text-muted"}
	}
	switch {
	case live.BatteryPower > 0.05:
		return bateriaChip{
			Text: T(lang, "common.charging") + " · " + fmtKwVal(lang, live.BatteryPower) + " kW",
			Cls:  "bg-emerald-500/12 text-emerald-600 dark:text-emerald-400",
		}
	case live.BatteryPower < -0.05:
		return bateriaChip{
			Text: T(lang, "common.discharging") + " · " + fmtKwVal(lang, math.Abs(live.BatteryPower)) + " kW",
			Cls:  "bg-amber-500/12 text-amber-600 dark:text-amber-400",
		}
	}
	return bateriaChip{Text: T(lang, "common.idle"), Cls: "bg-surface-2 text-muted"}
}

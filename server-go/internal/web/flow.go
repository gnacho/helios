package web

import (
	"fmt"
	"math"
	"time"

	"helios-server/internal/solar"
)

// timeParseISO parses HA ISO timestamps (with or without fractional seconds).
func timeParseISO(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04:05.000000", "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable iso: %s", s)
}

// Flow diagram + sparkline geometry ported from EnergyFlowDiagram.tsx and
// Sparkline.tsx.

// flowNode coordinates (viewBox 340x345).
const (
	flowFvX, flowFvY     = 170.0, 78.0
	flowHomeX, flowHomeY = 170.0, 200.0
	flowBatX, flowBatY   = 62.0, 276.0
	flowGridX, flowGridY = 278.0, 276.0
)

const (
	flowPathFvHome      = "M 170 108 L 170 166"
	flowPathFvBattery   = "M 152 94 C 96 104, 68 180, 66 242"
	flowPathFvGrid      = "M 188 94 C 244 104, 272 180, 274 242"
	flowPathBatteryHome = "M 90 254 C 118 236, 138 226, 150 220"
	flowPathGridHome    = "M 250 254 C 222 236, 202 226, 190 220"
	flowPathGridBattery = "M 250 298 C 218 326, 122 326, 90 298"
)

// flowEdge is one directed energy flow.
type flowEdge struct {
	ID    string
	Path  string
	Value float64
	Var   string // CSS color var
}

// flowModel carries the computed flows and display texts.
type flowModel struct {
	Live *solar.LiveData
	Lang string

	FvToHome      float64
	FvToBattery   float64
	FvGridExport  float64
	BatDischarge  float64
	GridImport    float64
	GridToBattery float64

	BatteryText string
	GridText    string
	HomeColor   string
}

func buildFlow(lang string, live *solar.LiveData) flowModel {
	m := flowModel{Live: live, Lang: lang, BatteryText: T(lang, "common.idle"), GridText: "—", HomeColor: "var(--text-faint)"}
	if live == nil {
		return m
	}
	cons := 0.0
	if live.Consumption != nil {
		cons = *live.Consumption
	}
	m.FvToHome = math.Min(live.Production, cons)
	charging := math.Max(0, live.BatteryPower)
	m.BatDischarge = math.Max(0, -live.BatteryPower)
	var exporting, importing float64
	if live.Grid != nil {
		if *live.Grid < 0 {
			exporting = -*live.Grid
		}
		if *live.Grid > 0 {
			importing = *live.Grid
		}
	}
	m.FvGridExport = exporting
	solarSurplus := math.Max(0, live.Production-cons)
	m.GridToBattery = math.Max(0, math.Min(charging-solarSurplus, importing))
	m.FvToBattery = math.Max(0, charging-m.GridToBattery)

	homeSources := []struct {
		kw   float64
		vvar string
	}{{m.FvToHome, "var(--c-solar)"}, {m.BatDischarge, "var(--c-bateria)"}, {importing, "var(--c-red-compra)"}}
	dominant := homeSources[0]
	for _, s := range homeSources[1:] {
		if s.kw > dominant.kw {
			dominant = s
		}
	}
	if dominant.kw > 0.05 {
		m.HomeColor = dominant.vvar
	}

	switch {
	case live.BatteryPower > 0.05:
		m.BatteryText = fmtKwVal(lang, live.BatteryPower) + " kW"
	case live.BatteryPower < -0.05:
		m.BatteryText = fmtKwVal(lang, -live.BatteryPower) + " kW"
	}
	if live.Grid == nil {
		m.GridText = "—"
	} else if *live.Grid < -0.05 {
		m.GridText = fmtKwVal(lang, -*live.Grid) + " kW ↑"
	} else if *live.Grid > 0.05 {
		m.GridText = fmtKwVal(lang, *live.Grid) + " kW ↓"
	} else {
		m.GridText = "0 W"
	}
	return m
}

func (m flowModel) edges() []flowEdge {
	return []flowEdge{
		{"fv-home", flowPathFvHome, m.FvToHome, "var(--c-solar)"},
		{"fv-battery", flowPathFvBattery, m.FvToBattery, "var(--c-bateria)"},
		{"fv-grid", flowPathFvGrid, m.FvGridExport, "var(--c-red-vertido)"},
		{"battery-home", flowPathBatteryHome, m.BatDischarge, "var(--c-bateria)"},
		{"grid-home", flowPathGridHome, m.GridImport, "var(--c-red-compra)"},
		{"grid-battery", flowPathGridBattery, m.GridToBattery, "var(--c-red-compra)"},
	}
}

// sunMinutes extracts minutes-of-day from the HA sun timestamps, falling back
// to the same defaults as the front (SUNRISE_MIN/SUNSET_MIN).
func sunMinutes(live *solar.LiveData) (sunrise, sunset int) {
	sunrise, sunset = 6*60+50, 21*60+40
	if live == nil {
		return sunrise, sunset
	}
	if v, ok := live.Sun.NextRising.(string); ok {
		if mins, err := isoToMinutes(v); err == nil {
			sunrise = mins
		}
	}
	if v, ok := live.Sun.NextSetting.(string); ok {
		if mins, err := isoToMinutes(v); err == nil {
			sunset = mins
		}
	}
	if sunset <= sunrise {
		sunrise, sunset = 6*60+50, 21*60+40
	}
	return sunrise, sunset
}

// isoToMinutes parses an HA ISO timestamp into minutes-of-day (local time).
func isoToMinutes(s string) (int, error) {
	t, err := timeParseISO(s)
	if err != nil {
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}

// sparklinePath builds a filled area path (0..max Y) for the given values,
// mirroring the Recharts Sparkline (monotone, no axes).
func sparklinePath(values []float64, w, h float64) string {
	if len(values) < 2 {
		return ""
	}
	max := 0.0
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	if max <= 0 {
		max = 1
	}
	pts := make([]cpoint, len(values))
	for i, v := range values {
		x := 2 + float64(i)/(float64(len(values))-1)*(w-4)
		y := 2 + (1-v/max)*(h-4)
		pts[i] = cpoint{x, y}
	}
	baseline := h - 2
	line := monotonePath(pts)
	return fmt.Sprintf("%s L %.2f %.2f L %.2f %.2f Z", line, pts[len(pts)-1].x, baseline, pts[0].x, baseline)
}

// arcModel is the solar arc view (SolarArc.tsx).
type arcModel struct {
	SunFrac       float64
	SunX, SunY   float64
	Sunrise      int
	Sunset       int
	PeakAt       int
	PeakText     string
	Daytime      bool
	WeatherLabel string
	TempText     string
	HasTemp      bool
}

func buildArc(lang string, live *solar.LiveData, k DayKpis) arcModel {
	sunrise, sunset := sunMinutes(live)
	atMin := 0
	if live != nil {
		atMin = live.At
	}
	frac := float64(atMin-sunrise) / float64(sunset-sunrise)
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	angle := math.Pi * (1 - frac)
	a := arcModel{
		SunFrac:  frac,
		SunX:     150 + 108*math.Cos(angle),
		SunY:     128 - 108*math.Sin(angle),
		Sunrise:  sunrise,
		Sunset:   sunset,
		PeakAt:   k.PeakAt,
		PeakText: fmtEnergy(lang, k.PeakProductionKw),
		Daytime:  atMin >= sunrise && atMin <= sunset,
	}
	if live != nil && live.Weather != "" {
		if v, ok := lookup(catalogs[normalizeLang(lang)], "weather."+live.Weather); ok {
			a.WeatherLabel = v
		} else {
			a.WeatherLabel = live.Weather
		}
		if live.WeatherTemp > -40 {
			a.HasTemp = true
			a.TempText = fmtInt(lang, live.WeatherTemp)
		}
	} else {
		a.WeatherLabel = "—"
	}
	return a
}

// invCardModel is one inverter card view (InverterCard.tsx).
type invCardModel struct {
	Key, Name, Model, Color     string
	NowText, KwhText, ShareText string
	SharePct                    float64
	SparkPath                   string
}

func buildInvCards(lang string, m dashboardModel, series []solar.DayPoint, pal Palette) []invCardModel {
	kwhs := map[string]float64{}
	total := 0.0
	keys, names := chartInvKeys(m)
	for _, k := range keys {
		kwh := perInverterKwh(series, k, untilMin(m))
		kwhs[k] = kwh
		total += kwh
	}
	cards := make([]invCardModel, 0, len(keys))
	for i, k := range keys {
		if i >= 2 {
			break
		}
		nowKw := inverterLiveKw(m, k, i)
		share := 0.0
		if total > 0 {
			share = kwhs[k] / total * 100
		} else if len(keys) == 1 {
			share = 100
		}
		c := InverterColor(k, pal, i)
		vals := make([]float64, len(series))
		for j, p := range series {
			vals[j] = seriesInvValue(p, k)
		}
		name := names[k]
		var kwp float64
		if i < len(m.Invs) {
			if m.Invs[i].Key == k {
				kwp = m.Invs[i].Kwp
			}
		}
		if kwp > 0 {
			name = name + " · " + formatNumber(lang, kwp, 1) + " kWp"
		}
		modelTxt := ""
		if i < len(m.Invs) && m.Invs[i].Key == k {
			modelTxt = m.Invs[i].Panels
			if modelTxt == "" {
				modelTxt = m.Invs[i].Model
			}
		}
		cards = append(cards, invCardModel{
			Key: k, Name: name, Model: modelTxt, Color: c,
			NowText:   fmtKwVal(lang, nowKw),
			KwhText:   fmtEnergy(lang, kwhs[k]),
			ShareText: fmtInt(lang, share),
			SharePct:  share,
			SparkPath: sparklinePath(vals, 140, 48),
		})
	}
	return cards
}

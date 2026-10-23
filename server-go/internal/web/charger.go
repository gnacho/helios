package web

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"helios-server/internal/auth"
	"helios-server/internal/db"
	"helios-server/internal/extensions"
	"helios-server/internal/solar"
)

// /cargador page (port of app/src/pages/Cargador.tsx). Tabs are links
// (?tab=key); power and amperage actions call the EXISTING JSON API
// (POST /api/charger/action) through Datastar @post: the action logic is not
// duplicated server-side. The image upload button (client-side canvas
// resize) is not ported; the configured image is rendered when present.

// AMP_STEPS are the preset currents (IEC 61851 min 6 A).
var AMP_STEPS = []int{6, 8, 10, 13, 16, 20, 25, 32}

const chgCurveW = 1160.0 // charger day curve card is full width
const chgHistW = 757.0   // charger history card is lg:col-span-8
const chgCurveH = 220.0

// chargerHistoryFromDays is the history window the React page loads (covers
// the previous-year view of the year period).
const chargerHistoryFromDays = 400

type chgTabView struct {
	Key, Name string
	Active    bool
	Charging  bool
}

type ampChipView struct {
	AmpsText string
	Active   bool
	// ClickExpr is the Datastar expression of the chip action.
	ClickExpr string
}

type chgCurveModel struct {
	Lang        string
	Color       string
	YMax        int
	AreaPath    string
	TicksX      []chartTick
	TicksY      []chartTick
	PlotRightX  float64
	PlotBottomY float64
}

type chgHistRow struct {
	Label        string
	X            float64
	BarW         float64
	PvY, PvH     float64
	RestY, RestH float64
}

type chgHistModel struct {
	W           float64
	Lang        string
	YMax        int
	TicksY      []chartTick
	Rows        []chgHistRow
	PlotRightX  float64
	PlotBottomY float64
	ColorSolar  string
	ColorGrid   string
}

type chgChip struct {
	State string // charging | connected | idle
	Text  string
	Cls   string
}

// chgLiveStat is one stat of the day-curve card header.
type chgLiveStat struct {
	IconKey string // zap | clock | gauge
	Label   string
	Value   string
	Signal  string // data-text binding, empty when static
}

type chgSingleView struct {
	Key  string
	Name string
	Lang string
	Pal  Palette

	HasImage   bool
	ImageURL   string
	Chip       chgChip
	PowerKnown bool
	PowerOn    bool
	PowerLabel string
	PowerTitle string
	PowerClick string // Datastar expression
	// aria-pressed expressions
	PressedExpr string

	HasAmps   bool
	AmpsLabel string
	AmpsAria  string
	AmpChips  []ampChipView
	AmpsText  string

	TempKnown bool
	TempText  string

	Stats []chgLiveStat

	Curve chgCurveModel

	Hist        chgHistModel
	HistHasData bool
	HistSub     string
	NavLabel    string
	Sel         periodSelectorView

	// Totals of the period (side card).
	TotalKwh      float64
	TotalKwhText  string
	PctSolar      float64
	PctSolarText  string
	PvKwhText     string
	RestKwhText   string
	ProdKwhText   string
	PctOfProdText string
	AvgDayText    string
}

type chargerView struct {
	pageProps
	Signals string
	Palette Palette
	Tabs    []chgTabView
	Active  *chgSingleView
}

// chargerSignals builds the $chg subtree for every charger: server-formatted
// texts + raw state for the action buttons. Patched by /ui/stream. With no
// HA connection the subtree carries the idle defaults so the reactive
// bindings on the page always resolve.
func (u *UI) chargerSignals(lang string) map[string]any {
	if u.ExtStore == nil {
		return nil
	}
	cfg := u.ExtStore.Get()
	if !extensions.ChargerActive(cfg) {
		return nil
	}
	chg := map[string]any{}
	for i := range cfg.CarCharger.Chargers {
		c := cfg.CarCharger.Chargers[i]
		if u.ExtHA == nil {
			chg[c.Key] = chgSignalOf(lang, extensions.LiveCharger{Key: c.Key, Name: c.Name})
			continue
		}
		chg[c.Key] = chgSignalOf(lang, extensions.ComputeOneChargerLive(u.ExtHA, c))
	}
	return chg
}

func chgSignalOf(lang string, l extensions.LiveCharger) map[string]any {
	state := "idle"
	if l.Charging {
		state = "charging"
	} else if l.Connected {
		state = "connected"
	}
	var switchOn any
	if l.SwitchOn != nil {
		switchOn = *l.SwitchOn
	}
	var amps any
	if l.Amps != nil {
		amps = *l.Amps
	}
	sessionText, totalText, tempText := "—", "—", "—"
	if l.SessionKwh != nil {
		sessionText = fmtEnergy(lang, *l.SessionKwh) + " kWh"
	}
	if l.TotalKwh != nil {
		totalText = fmtEnergy(lang, *l.TotalKwh) + " kWh"
	}
	if l.TempC != nil {
		tempText = fmtInt(lang, math.Round(*l.TempC)) + " °C"
	}
	return map[string]any{
		"state":       state,
		"stateText":   chgStateText(lang, state, l),
		"powerText":   fmtKwVal(lang, l.PowerKw) + " kW",
		"sessionText": sessionText,
		"totalText":   totalText,
		"tempText":    tempText,
		"switchOn":    switchOn,
		"amps":        amps,
	}
}

func chgStateText(lang, state string, l extensions.LiveCharger) string {
	switch state {
	case "charging":
		return T(lang, "common.charging") + " · " + fmtKwVal(lang, l.PowerKw) + " kW"
	case "connected":
		return T(lang, "cargador.connected")
	}
	return T(lang, "common.idle")
}

func chgChipFor(lang, state string) chgChip {
	switch state {
	case "charging":
		return chgChip{State: state, Cls: "bg-emerald-500/12 text-emerald-600 dark:text-emerald-400"}
	case "connected":
		return chgChip{State: state, Cls: "bg-amber-500/12 text-amber-600 dark:text-amber-400"}
	}
	return chgChip{State: "idle", Cls: "bg-surface-2 text-muted"}
}

// chgDayCurveChart ports the React AreaChart of the charger day curve.
func chgDayCurveChart(lang string, points []extensions.CurvePoint, color string) chgCurveModel {
	m := chgCurveModel{
		Lang:        lang,
		Color:       color,
		TicksX:      chgTicksX(),
		PlotRightX:  chgCurveW - 12,
		PlotBottomY: chgCurveH - 14,
	}
	yMaxF := 1.0
	for _, p := range points {
		if p.Kw > yMaxF {
			yMaxF = p.Kw
		}
	}
	m.YMax = int(math.Ceil(yMaxF))
	// ~5 ticks like recharts instead of 1-by-1.
	tickStep := math.Max(1, math.Ceil(float64(m.YMax)/5))
	for v := 0.0; v <= float64(m.YMax); v += tickStep {
		m.TicksY = append(m.TicksY, chartTick{
			X:     34 - 6,
			Y:     chgCurveY(v, float64(m.YMax)) + 4,
			Label: strconv.Itoa(int(v)),
		})
	}
	var pts []cpoint
	for _, p := range points {
		pts = append(pts, cpoint{chartXin(float64(p.T), chgCurveW), chgCurveY(p.Kw, float64(m.YMax))})
	}
	m.AreaPath = areaPath(pts, chgCurveY(0, float64(m.YMax)))
	return m
}

// chgTicksX is the 3-hour X axis for the charger curve box.
func chgTicksX() []chartTick {
	var ticks []chartTick
	for _, t := range axisTicksX {
		ticks = append(ticks, chartTick{X: chartXin(float64(t), chgCurveW), Y: chgCurveH - 6, Label: fmtTime(t)})
	}
	return ticks
}

// chartXin maps minutes to x inside a box whose left pad is 34 and right 12.
func chartXin(t, w float64) float64 { return (chartGeom{w, 0}).x(t) }

func chgCurveY(v, yMax float64) float64 {
	top, bottom := 10.0, chgCurveH-14.0
	return top + (1-v/yMax)*(bottom-top)
}

// chgHistoryWindow resolves the visible period for the charger history
// (semana anclada a HOY, mes/año como el Histórico) and the bars.
func chgHistoryChart(lang string, p Period, anchor, today time.Time, hist []extensions.ChargerHistoryRow, solarRows []db.DailyRow, pal Palette) (chgHistModel, bool) {
	m := chgHistModel{
		W:           chgHistW,
		Lang:        lang,
		PlotRightX:  chgHistW - 12,
		PlotBottomY: chgCurveH - 14,
		ColorSolar:  pal.Bateria,
		ColorGrid:   pal.Consumo,
	}

	type periodRow struct {
		label         string
		from, to      time.Time
		kwh, pv, prod float64
	}
	var rows []periodRow
	switch p {
	case PeriodAno:
		for mm := time.January; mm <= time.December; mm++ {
			rows = append(rows, periodRow{
				label: monthName(lang, mm, true),
				from:  time.Date(anchor.Year(), mm, 1, 0, 0, 0, 0, time.Local),
				to:    time.Date(anchor.Year(), mm+1, 0, 0, 0, 0, 0, time.Local),
			})
		}
	case PeriodMes:
		last := time.Date(anchor.Year(), anchor.Month()+1, 0, 0, 0, 0, 0, time.Local)
		for d := time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, time.Local); !d.After(last); d = d.AddDate(0, 0, 1) {
			rows = append(rows, periodRow{label: strconv.Itoa(d.Day()), from: d, to: d})
		}
	default: // semana
		start, _ := periodWindow(PeriodSemana, anchor)
		for d := start; !d.After(anchor); d = d.AddDate(0, 0, 1) {
			rows = append(rows, periodRow{label: weekdayNarrow(lang, d.Weekday()) + " " + strconv.Itoa(d.Day()), from: d, to: d})
		}
	}

	byDay := map[string]extensions.ChargerHistoryRow{}
	for _, h := range hist {
		byDay[h.Date] = h
	}
	prodByDay := map[string]float64{}
	for _, r := range solarRows {
		prodByDay[r.Date] = derefF(r.ProductionKwh)
	}

	yMaxF := 1.0
	out := make([]chgHistRow, 0, len(rows))
	for _, rw := range rows {
		var kwh, pv, prod float64
		for d := rw.from; !d.After(rw.to) && !d.After(today); d = d.AddDate(0, 0, 1) {
			key := d.Format("2006-01-02")
			if c, ok := byDay[key]; ok {
				if c.Kwh != nil {
					kwh += *c.Kwh
				}
				if c.PvKwh != nil {
					pv += *c.PvKwh
				}
			}
			prod += prodByDay[key]
		}
		row := chgHistRow{Label: rw.label, PvH: pv, RestH: math.Max(0, kwh-pv)}
		if kwh > yMaxF {
			yMaxF = kwh
		}
		out = append(out, row)
	}
	m.YMax = int(math.Ceil(yMaxF))
	m.TicksY = nil
	// ~5 ticks like recharts instead of 1-by-1.
	tickStep := math.Max(1, math.Ceil(float64(m.YMax)/5))
	for v := 0.0; v <= float64(m.YMax); v += tickStep {
		m.TicksY = append(m.TicksY, chartTick{
			X:     34 - 6,
			Y:     chgCurveY(v, float64(m.YMax)) + 4,
			Label: strconv.Itoa(int(v)),
		})
	}

	plotW := chgHistW - 34 - 12
	slot := plotW / float64(len(out))
	// React uses a fixed barSize: 22 (semana/ano) or 10 (mes), stacked.
	barW := 22.0
	if p == PeriodMes {
		barW = 10.0
	}
	if barW > slot {
		barW = slot
	}
	for i := range out {
		out[i].X = 34 + float64(i)*slot + (slot-barW)/2
		out[i].BarW = barW
		if h := out[i].PvH / float64(m.YMax) * (m.PlotBottomY - 10); h > 0.4 {
			out[i].PvH = h
			out[i].PvY = m.PlotBottomY - h
		} else {
			out[i].PvH = 0
		}
		if h := out[i].RestH / float64(m.YMax) * (m.PlotBottomY - 10); h > 0.4 {
			out[i].RestH = h
			out[i].RestY = out[i].PvY - h
			if out[i].PvH == 0 {
				out[i].RestY = m.PlotBottomY - h
			}
		} else {
			out[i].RestH = 0
		}
	}
	m.Rows = out
	return m, true
}

// chgPeriodStats aggregates the period totals shown in the side card.
type chgPeriodStats struct {
	Kwh, Pv, Prod float64
	ActiveDays    int
}

func chgStats(lang string, p Period, anchor, today time.Time, hist []extensions.ChargerHistoryRow, solarRows []db.DailyRow) chgPeriodStats {
	byDay := map[string]extensions.ChargerHistoryRow{}
	for _, h := range hist {
		byDay[h.Date] = h
	}
	prodByDay := map[string]float64{}
	for _, r := range solarRows {
		prodByDay[r.Date] = derefF(r.ProductionKwh)
	}
	var st chgPeriodStats
	collect := func(from, to time.Time) {
		for d := from; !d.After(to) && !d.After(today); d = d.AddDate(0, 0, 1) {
			key := d.Format("2006-01-02")
			var kwh float64
			if c, ok := byDay[key]; ok && c.Kwh != nil {
				kwh = *c.Kwh
			}
			st.Kwh += kwh
			if c, ok := byDay[key]; ok && c.PvKwh != nil {
				st.Pv += *c.PvKwh
			}
			st.Prod += prodByDay[key]
			if kwh > 0 {
				st.ActiveDays++
			}
		}
	}
	switch p {
	case PeriodAno:
		for mm := time.January; mm <= time.December; mm++ {
			first := time.Date(anchor.Year(), mm, 1, 0, 0, 0, 0, time.Local)
			last := time.Date(anchor.Year(), mm+1, 0, 0, 0, 0, 0, time.Local)
			collect(first, last)
		}
	case PeriodMes:
		last := time.Date(anchor.Year(), anchor.Month()+1, 0, 0, 0, 0, 0, time.Local)
		collect(time.Date(anchor.Year(), anchor.Month(), 1, 0, 0, 0, 0, time.Local), last)
	default:
		start, end := periodWindow(PeriodSemana, anchor)
		collect(start, end)
	}
	return st
}

func (u *UI) handleCargador(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	lang := u.userLang(userID)
	p := u.baseProps(r, userID, "/cargador", "")
	p.Title = T(lang, "nav.cargador")

	if u.ExtStore == nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	cfg := u.ExtStore.Get()
	if !extensions.ChargerActive(cfg) || len(cfg.CarCharger.Chargers) == 0 {
		// Extensión desactivada: vuelta al inicio (como el Navigate de React).
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	pal := newPalette(p.Theme == "dark")
	today := startOfDay(time.Now())

	// Tab activa: ?tab=key (el primero por defecto).
	activeKey := r.URL.Query().Get("tab")
	charger := extensions.GetChargerByKey(cfg, activeKey)
	if charger == nil {
		charger = &cfg.CarCharger.Chargers[0]
	}

	v := chargerView{pageProps: p, Palette: pal}
	for i := range cfg.CarCharger.Chargers {
		c := &cfg.CarCharger.Chargers[i]
		charging := false
		if u.ExtHA != nil {
			live := extensions.ComputeOneChargerLive(u.ExtHA, *c)
			charging = live.Charging
		}
		v.Tabs = append(v.Tabs, chgTabView{Key: c.Key, Name: c.Name, Active: c.Key == charger.Key, Charging: charging})
	}

	// Señales: estado por cargador + ui de acción.
	signals := map[string]any{}
	if chg := u.chargerSignals(lang); chg != nil {
		signals["chg"] = chg
	}
	signals["ui"] = map[string]any{"busy": false, "error": "", "charger": charger.Key, "action": "", "amps": nil}
	sigJSON, err := json.Marshal(signals)
	if err != nil {
		sigJSON = []byte("{}")
	}
	v.Signals = string(sigJSON)

	// Vista del cargador activo.
	sv := &chgSingleView{
		Key:  charger.Key,
		Name: charger.Name,
		Lang: lang,
		Pal:  pal,
	}
	var live extensions.LiveCharger
	hasLive := false
	if u.ExtHA != nil {
		live = extensions.ComputeOneChargerLive(u.ExtHA, *charger)
		hasLive = true
	}

	state := "idle"
	if hasLive {
		if live.Charging {
			state = "charging"
		} else if live.Connected {
			state = "connected"
		}
	}
	sv.Chip = chgChipFor(lang, state)
	sv.Chip.Text = chgStateText(lang, state, live)
	if charger.ImageURL != "" {
		sv.HasImage = true
		sv.ImageURL = charger.ImageURL
	}

	sv.PressedExpr = "$chg." + charger.Key + ".switchOn ? 'true' : 'false'"
	sv.PowerClick = "$charger = '" + charger.Key + "'; $action = $chg." + charger.Key + ".switchOn ? 'off' : 'on'; $ui.error = ''; @post('/api/charger/action')"
	sv.PowerLabel = T(lang, "cargador.powerOn")
	sv.PowerTitle = T(lang, "cargador.powerOn")
	if hasLive && live.SwitchOn != nil {
		sv.PowerKnown = true
		sv.PowerOn = *live.SwitchOn
		if sv.PowerOn {
			sv.PowerLabel = T(lang, "cargador.powerOff")
			sv.PowerTitle = T(lang, "cargador.powerOff")
		}
	}

	// Amperaje (solo con entidad de corriente configurada).
	if charger.AmpID != "" {
		sv.HasAmps = true
		sv.AmpsLabel = T(lang, "cargador.amps")
		sv.AmpsAria = T(lang, "cargador.ampsAria")
		var opts []float64
		if hasLive && len(live.AmpOptions) > 0 {
			opts = live.AmpOptions
		} else {
			for _, a := range AMP_STEPS {
				opts = append(opts, float64(a))
			}
		}
		sort.Float64s(opts)
		for _, a := range opts {
			active := hasLive && live.Amps != nil && *live.Amps == a
			ampsText := strconv.FormatFloat(a, 'f', -1, 64)
			sv.AmpChips = append(sv.AmpChips, ampChipView{
				AmpsText:  ampsText,
				Active:    active,
				ClickExpr: "$charger = '" + charger.Key + "'; $action = 'amp'; $amps = " + ampsText + "; $ui.error = ''; @post('/api/charger/action')",
			})
		}
		if hasLive && live.Amps != nil {
			sv.AmpsText = strconv.FormatFloat(*live.Amps, 'f', -1, 64)
		}
	}

	if hasLive && live.TempC != nil {
		sv.TempKnown = true
		sv.TempText = fmtInt(lang, math.Round(*live.TempC)) + " °C"
	} else {
		sv.TempText = "—"
	}

	powerText, sessionText, totalText := "…", "—", "—"
	if hasLive {
		powerText = fmtKwVal(lang, live.PowerKw) + " kW"
		if live.SessionKwh != nil {
			sessionText = fmtEnergy(lang, *live.SessionKwh) + " kWh"
		}
		if live.TotalKwh != nil {
			totalText = fmtEnergy(lang, *live.TotalKwh) + " kWh"
		}
	}
	k := charger.Key
	sv.Stats = []chgLiveStat{
		{IconKey: "zap", Label: T(lang, "cargador.currentPower"), Value: powerText, Signal: "$chg." + k + ".powerText"},
		{IconKey: "clock", Label: T(lang, "cargador.session"), Value: sessionText, Signal: "$chg." + k + ".sessionText"},
		{IconKey: "gauge", Label: T(lang, "cargador.lifetime"), Value: totalText, Signal: "$chg." + k + ".totalText"},
	}

	// Curva del día (servicio en proceso, con caché propia).
	if u.Ext != nil && u.ExtHA != nil {
		curve := u.Ext.ChargerDayCurve(u.ExtHA, solar.TodayStr(), charger)
		sv.Curve = chgDayCurveChart(lang, curve.Points, pal.Consumo)
	} else {
		sv.Curve = chgDayCurveChart(lang, nil, pal.Consumo)
	}

	// Histórico del cargador (energía diaria, issue 146): ventana de 400 días
	// como la carga de React + periodo visible semana/mes/año.
	period := PeriodSemana
	if ps, ok := periodOf(r.URL.Query().Get("period")); ok && ps != PeriodDia {
		period = ps
	}
	anchor := today // la semana del cargador ancla a HOY (incluye el día en curso)
	if period != PeriodSemana {
		anchor = initialAnchor(period, today)
		if a, err := time.ParseInLocation("2006-01-02", r.URL.Query().Get("anchor"), time.Local); err == nil {
			anchor = startOfDay(a)
		}
	}

	var hist []extensions.ChargerHistoryRow
	if u.Ext != nil {
		from := today.AddDate(0, 0, -chargerHistoryFromDays)
		if rows, err := u.Ext.ChargerHistory(from.Format("2006-01-02"), today.Format("2006-01-02"), charger.Key); err == nil {
			hist = rows
		}
	}
	var solarRows []db.DailyRow
	if u.DB != nil {
		solarRows = u.dailyHistory()
	}

	sv.Sel = buildPeriodSelector(lang, period, anchor, today)
	// La semana del cargador arranca HOY (no ayer como el Histórico).
	for i := range sv.Sel.Tabs {
		if sv.Sel.Tabs[i].Key == string(PeriodSemana) {
			sv.Sel.Tabs[i].URL = histURLChg(period, today)
		}
	}
	sv.Sel.PrevURL = histURLChg(period, shiftAnchor(period, anchor, -1))
	sv.Sel.NextURL = histURLChg(period, shiftAnchor(period, anchor, 1))
	sv.Sel.TodayURL = histURLChg(period, today)
	sv.Sel.CanNext = canGoNext(period, anchor, today)
	sv.Sel.IsCurrent = isCurrentPeriod(period, anchor, today)
	sv.NavLabel = periodNavLabel(period, anchor, lang)
	sv.HistSub = periodSubtitle(period, anchor, lang)

	st := chgStats(lang, period, anchor, today, hist, solarRows)
	sv.Hist, _ = chgHistoryChart(lang, period, anchor, today, hist, solarRows, pal)
	sv.HistHasData = st.Kwh > 0 || st.Prod > 0

	sv.TotalKwh = st.Kwh
	sv.TotalKwhText = fmtEnergy(lang, st.Kwh)
	sv.PctSolar = 0
	if st.Kwh > 0 {
		sv.PctSolar = st.Pv / st.Kwh * 100
	}
	sv.PctSolarText = fmtInt(lang, sv.PctSolar)
	sv.PvKwhText = fmtEnergy(lang, st.Pv)
	sv.RestKwhText = fmtEnergy(lang, math.Max(0, st.Kwh-st.Pv))
	sv.ProdKwhText = fmtEnergy(lang, st.Prod)
	pctOfProd := 0.0
	if st.Prod > 0 {
		pctOfProd = st.Kwh / st.Prod * 100
	}
	sv.PctOfProdText = fmtInt(lang, pctOfProd)
	avgDay := 0.0
	if st.ActiveDays > 0 {
		avgDay = st.Kwh / float64(st.ActiveDays)
	}
	sv.AvgDayText = fmtEnergy(lang, avgDay)

	v.Active = sv
	_ = ChargerPage(v).Render(r.Context(), w)
}

// histURLChg is the cargador history navigation URL.
func histURLChg(p Period, anchor time.Time) string {
	return "/cargador?period=" + string(p) + "&anchor=" + anchor.Format("2006-01-02")
}

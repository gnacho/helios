package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"strings"

	"github.com/starfederation/datastar-go/datastar"

	"helios-server/internal/auth"
	"helios-server/internal/db"
	"helios-server/internal/extensions"
	"helios-server/internal/solar"
)

// /cargador page (port of app/src/pages/Cargador.tsx). Tabs are links
// (?tab=key); power and amperage actions go through Datastar to
// POST /ui/charger/action, which shares extensions.ApplyAction with the JSON
// API and answers with the fresh signals (the state does not wait for the hub
// broadcast, which lags up to a second). The image upload button (client-side
// canvas resize) is not ported; the configured image is rendered when present.

// AMP_STEPS are the preset currents (IEC 61851 min 6 A).
var AMP_STEPS = []int{6, 8, 10, 13, 16, 20, 25, 32}

// ampMaxChips is the widest range rendered step by step. Beyond it the card
// falls back to the presets, so a wall box accepting 8-32 A keeps a short
// picker instead of 25 buttons.
const ampMaxChips = 12

// ampRange lists every current a number entity accepts: min..max by step (1 A
// when the entity does not declare one), capped so a bogus range cannot flood
// the card with chips.
func ampRange(min, max float64, step *float64) []float64 {
	s := 1.0
	if step != nil && *step > 0 {
		s = *step
	}
	out := make([]float64, 0, 16)
	for a := min; a <= max+1e-9 && len(out) < 32; a += s {
		out = append(out, math.Round(a*1000)/1000)
	}
	if len(out) == 0 {
		out = append(out, min)
	}
	return out
}

// Clases del boton de encendido en cada estado: van dentro de una expresion
// Datastar (data-attr-class), asi que el color cambia sin esperar al servidor.
// Los mismos literales aparecen en charger.templ (Tailwind los ve alli).
const (
	chgPowerOnClass  = "inline-flex h-12 w-full shrink-0 items-center justify-center sm:h-[60px] sm:w-[157px] gap-2 rounded-2xl px-2 text-sm font-semibold transition-colors disabled:opacity-50 bg-emerald-500/12 text-emerald-600 hover:bg-emerald-500/20 dark:text-emerald-400"
	chgPowerOffClass = "inline-flex h-12 w-full shrink-0 items-center justify-center sm:h-[60px] sm:w-[157px] gap-2 rounded-2xl px-2 text-sm font-semibold transition-colors disabled:opacity-50 bg-brand text-white hover:opacity-90"
)

// jsQuote escapes a label so it can live inside a single-quoted JS string in a
// Datastar attribute.
func jsQuote(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "'", `\'`)
}

const chgCurveW = 1160.0 // charger day curve card is full width
const chgHistW = 757.0   // charger history card is lg:col-span-8
const chgCurveH = 220.0

// chargerHistoryFromDays is the history window the React page loads (covers
// the previous-year view of the year period).
const chargerHistoryFromDays = 400

// chargerStaleAfter es la antigüedad máxima del dato de potencia antes de
// avisar de que la lectura está congelada. El aparato reporta cada ~10 min
// aunque esté parado, así que por debajo de esto un 0 kW es creíble.
const chargerStaleAfter = 10 * time.Minute

type chgTabView struct {
	Key, Name string
	Active    bool
	Charging  bool
}

type ampChipView struct {
	AmpsText string
	Active   bool
	// Aria es el valor inicial de aria-pressed del chip (true si activo).
	Aria string
	// ClickExpr is the Datastar expression of the chip action.
	ClickExpr string
	// AriaExpr and ClassExpr keep aria-pressed and styling in sync with the
	// live amps signal so the pressed chip updates without a page reload.
	AriaExpr  string
	ClassExpr string
}

type chgCurveModel struct {
	Lang        string
	Color       string
	YMax        int
	AreaPath    string
	LinePath    string
	TicksX      []chartTick
	TicksY      []chartTick
	PlotRightX  float64
	PlotBottomY float64
	Hover       []hoverCol
}

type chgHistRow struct {
	Label        string
	X            float64
	BarW         float64
	PvY, PvH     float64
	RestY, RestH float64
	Dur, Span    string // tiempo en carga (Xh Ym) y franja inicio-fin del día
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
	Hover       []hoverCol
}

type chgChip struct {
	State string // charging | connected | idle
	Text  string
	Cls   string
}

// sameKwh reports whether two counters hold the same reading: a "lifetime"
// total that mirrors the session (the Svitrix and feyree integrations both do
// this) is not an accumulated total and must not be shown as one.
func sameKwh(a, b *float64) bool {
	if a == nil || b == nil {
		return false
	}
	return math.Abs(*a-*b) < 0.005
}

// chgLiveStat is one stat of the day-curve card header.
type chgLiveStat struct {
	AnimID  string  // count-up tween id ("" = plain data-text)
	Num     float64 // initial numeric value
	Dec     int     // decimals kept while counting (matches the formatter)
	Suffix  string  // constant unit text after the number
	IconKey string  // zap | clock | gauge
	Label   string
	Value   string
	Signal  string // data-text binding, empty when static
}

type chgSingleView struct {
	Key    string
	Name   string
	Lang   string
	Pal    Palette
	Period Period // active history period (recreates the bars on change)

	HasImage   bool
	ImageURL   string
	Chip       chgChip
	PowerKnown bool
	PowerOn    bool
	PowerLabel string
	PowerTitle string
	PowerClick string // Datastar expression
	// Live expressions: la etiqueta, el title y el color siguen a la señal
	// (asi el boton responde al instante y se corrige solo cuando HA confirma).
	LabelExpr   string
	TitleExpr   string
	ClassExpr   string
	PendingExpr string
	PendingText string
	// PendingClearExpr quita el estado "Encendiendo/Apagando" cuando HA ya
	// refleja lo pedido, o tras 12 s si nunca llega.
	PendingClearExpr string
	// aria-pressed expressions
	PressedExpr string

	HasAmps   bool
	AmpsLabel string
	AmpsAria  string
	AmpChips  []ampChipView
	AmpsText  string

	// Fila de programación, idéntica en ambos cargadores: hora de inicio,
	// minutos, duración y botón Programar. El portátil la ejecuta en su
	// hardware (dps 118/119); el wall la arma por software en HA.
	HasSchedule    bool
	ScheduleLabel  string
	ScheduleHour   chgSelectView
	ScheduleMinute chgSelectView
	ScheduleDur    chgSelectView
	ScheduleClick  string
	ScheduleCancel string
	ScheduleArmed  bool
	ScheduleText   string

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
	Signals     string
	Palette     Palette
	Tabs        []chgTabView
	Panels      []*chgSingleView // one per charger, for the mobile swipe track
	Active      *chgSingleView
	ActiveIndex int
	IsAdmin     bool
	ImgErr      bool
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
	powerKw, sessionKwh, totalKwh := l.PowerKw, 0.0, 0.0
	if l.SessionKwh != nil {
		sessionKwh = *l.SessionKwh
		sessionText = fmtEnergy(lang, *l.SessionKwh) + " kWh"
	}
	if l.TotalKwh != nil {
		totalKwh = *l.TotalKwh
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
		// Numeric twins for the count-up tween.
		"powerKw":    powerKw,
		"sessionKwh": sessionKwh,
		"totalKwh":   totalKwh,
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
		PlotBottomY: chgCurveH - 24,
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
	m.LinePath = monotoneLine(pts)
	// Hover: una columna por punto de la curva (potencia de ese instante).
	stepC := (m.PlotRightX - 34) / 96
	var samplesC []hoverSample
	for _, p := range points {
		samplesC = append(samplesC, hoverSample{
			X:     chartXin(float64(p.T), chgCurveW),
			W:     stepC,
			Title: fmtTime(p.T),
			Rows: []hoverRow{{
				Color: color,
				Label: T(lang, "cargador.currentPower"),
				Value: fmtKwVal(lang, p.Kw) + " kW",
			}},
		})
	}
	m.Hover = buildHoverCols(samplesC, hoverBox{34, m.PlotRightX, 10, m.PlotBottomY})
	return m
}

// chgTicksX is the 3-hour X axis for the charger curve box.
func chgTicksX() []chartTick {
	var ticks []chartTick
	for _, t := range axisTicksX {
		ticks = append(ticks, chartTick{X: chartXin(float64(t), chgCurveW), Y: chgCurveH - 8, Label: fmtTime(t)})
	}
	return ticks
}

// chartXin maps minutes to x inside a box whose left pad is 34 and right 12.
func chartXin(t, w float64) float64 { return (chartGeom{w, 0}).x(t) }

func chgCurveY(v, yMax float64) float64 {
	top, bottom := 10.0, chgCurveH-24.0
	return top + (1-v/yMax)*(bottom-top)
}

// chgHistoryWindow resolves the visible period for the charger history
// (semana anclada a HOY, mes/año como el Histórico) and the bars.
func chgHistoryChart(lang string, p Period, anchor, today time.Time, hist []extensions.ChargerHistoryRow, solarRows []db.DailyRow, times map[string]extensions.ChargerDayTimes, pal Palette) (chgHistModel, bool) {
	m := chgHistModel{
		W:           chgHistW,
		Lang:        lang,
		PlotRightX:  chgHistW - 12,
		PlotBottomY: chgCurveH - 24,
		ColorSolar:  pal.Bateria,
		ColorGrid:   pal.Consumo,
	}

	type periodRow struct {
		label    string
		from, to time.Time
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
	var vals [][2]float64 // kWh totales y kWh solares por barra
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
		var mins int
		var firstStart, lastEnd string
		for d := rw.from; !d.After(rw.to); d = d.AddDate(0, 0, 1) {
			if td, ok := times[d.Format("2006-01-02")]; ok {
				mins += td.Minutes
				if firstStart == "" && td.Start != "" {
					firstStart = td.Start
				}
				if td.End != "" {
					lastEnd = td.End
				}
			}
		}
		if mins > 0 {
			row.Dur = fmtChgDur(mins)
			// La franja inicio-fin solo tiene sentido en barras de un día.
			if rw.from.Equal(rw.to) && firstStart != "" && lastEnd != "" {
				row.Span = firstStart + " - " + lastEnd
			}
		}
		if kwh > yMaxF {
			yMaxF = kwh
		}
		out = append(out, row)
		vals = append(vals, [2]float64{kwh, pv})
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
	// Hover: una columna por barra con lo solar y el resto (red y batería).
	var samplesH []hoverSample
	for i := range out {
		if i >= len(vals) {
			break
		}
		var rowsH []hoverRow
		if vals[i][1] > 0 {
			rowsH = append(rowsH, hoverRow{Color: m.ColorSolar, Label: T(lang, "cargador.fromSolar"), Value: fmtEnergy(lang, vals[i][1]) + " kWh"})
		}
		if rest := vals[i][0] - vals[i][1]; rest > 0 {
			rowsH = append(rowsH, hoverRow{Color: m.ColorGrid, Label: T(lang, "cargador.fromGrid"), Value: fmtEnergy(lang, rest) + " kWh"})
		}
		if out[i].Dur != "" {
			rowsH = append(rowsH, hoverRow{Label: T(lang, "cargador.chargeHours"), Value: out[i].Dur})
		}
		if out[i].Span != "" {
			rowsH = append(rowsH, hoverRow{Label: T(lang, "cargador.chargeTimes"), Value: out[i].Span})
		}
		samplesH = append(samplesH, hoverSample{X: out[i].X + out[i].BarW/2, W: slot, Title: out[i].Label, Rows: rowsH})
	}
	m.Hover = buildHoverCols(samplesH, hoverBox{34, chgHistW - 12, 10, m.PlotBottomY})
	return m, true
}

// fmtChgDur renders charging minutes as "Xh Ym" (or "Ym" under an hour).
func fmtChgDur(min int) string {
	h := min / 60
	m := min % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
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

// handleChargerAction runs a power/amperage action from the templ UI and
// answers with the FRESH charger signals. The JSON API already answers in
// ~10 ms, but the new state only reached the page on the next hub broadcast
// (throttled to 1 s and dependent on HA emitting), so the button looked slow.
// Here the handler waits for HA to report the requested state and patches the
// signals in the same response: the button confirms at once.
func (u *UI) handleChargerAction(w http.ResponseWriter, r *http.Request) {
	username, userID, isAdmin := u.currentUserActor(r)
	lang := u.userLang(userID)

	// El cuerpo se lee ANTES de crear el generador SSE: datastar.NewSSE
	// consume y cierra el Body, y despues ya no se puede decodificar.
	var body struct {
		Charger string   `json:"charger"`
		Action  string   `json:"action"`
		Amps    *float64 `json:"amps"`
		Time    string   `json:"schedTime"`
		Hours   *float64 `json:"schedHours"`
	}
	decodeErr := json.NewDecoder(r.Body).Decode(&body)

	gen := datastar.NewSSE(w, r)
	fail := func(msg string) {
		_ = gen.MarshalAndPatchSignals(map[string]any{"ui": map[string]any{"pending": "", "error": msg}})
	}
	if !isAdmin || decodeErr != nil {
		fail(T(lang, "common.error"))
		return
	}
	if body.Charger == "" {
		body.Charger = "main"
	}
	charger := extensions.GetChargerByKey(u.ExtStore.Get(), body.Charger)
	if charger == nil {
		fail(T(lang, "common.error"))
		return
	}
	if u.DB != nil {
		detail := map[string]any{"charger": charger.Key, "action": body.Action}
		if body.Amps != nil {
			detail["amps"] = *body.Amps
		}
		u.DB.Audit(username, userID, "charger.action", detail)
	}

	start := time.Now()
	var err error
	confirmed := false
	if body.Action == "schedule" || body.Action == "unschedule" || body.Action == "timed" {
		if body.Action == "schedule" {
			hours := 0.0
			if body.Hours != nil {
				hours = *body.Hours
			}
			err = extensions.ApplySchedule(u.ExtHA, *charger, body.Time, hours)
		} else if body.Action == "timed" {
			hours := 0.0
			if body.Hours != nil {
				hours = *body.Hours
			}
			err = extensions.ApplyTimed(u.ExtHA, *charger, body.Time, hours)
		} else {
			err = extensions.ClearSchedule(u.ExtHA, *charger)
		}
		confirmed = err == nil
		if err != nil {
			log.Printf("[chg-action] %s %s error=%v", charger.Key, body.Action, err)
		}
	} else {
		_, err = extensions.ApplyAction(u.ExtHA, *charger, body.Action, body.Amps)
		if err == nil {
			confirmed = u.waitChargerState(*charger, body.Action, body.Amps)
			log.Printf("[chg-action] %s %s confirmed=%v in %s", charger.Key, body.Action, confirmed, time.Since(start).Round(time.Millisecond))
		} else {
			log.Printf("[chg-action] %s %s error=%v", charger.Key, body.Action, err)
		}
	}

	patch := map[string]any{"ui": map[string]any{"pending": "", "error": ""}}
	if chg := u.chargerSignals(lang); chg != nil {
		patch["chg"] = chg
	}
	if err != nil {
		patch["ui"] = map[string]any{"pending": "", "error": chargerActionError(lang, err)}
	} else if !confirmed {
		// The command went through but the charger never reported the new
		// state: tell the user instead of failing silently.
		patch["ui"] = map[string]any{"pending": "", "error": T(lang, "cargador.errorNotConfirmed")}
	}
	_ = gen.MarshalAndPatchSignals(patch)
}

// waitChargerState blocks (bounded) until Home Assistant reports the requested
// state, so the answer carries the new state instead of the old one.
func (u *UI) waitChargerState(charger extensions.Charger, action string, amps *float64) bool {
	if u.ExtHA == nil {
		return false
	}
	entity := charger.SwitchID
	if action == "amp" {
		entity = charger.AmpID
	}
	if action == "delay" {
		entity = charger.DelayID
	}
	if action == "duration" {
		entity = charger.DurationID
	}
	deadline := time.Now().Add(1500 * time.Millisecond)
	for {
		if st, ok := u.ExtHA.GetState(entity); ok && stateMatches(st.State, action, amps) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// stateMatches tells whether a HA state already reflects the requested action.
func stateMatches(state, action string, amps *float64) bool {
	switch action {
	case "on":
		return state == "on"
	case "off":
		return state == "off"
	case "amp":
		if amps == nil {
			return true
		}
		v, err := strconv.ParseFloat(state, 64)
		return err == nil && math.Abs(v-*amps) < 0.05
	case "delay", "duration":
		if amps == nil {
			return true
		}
		v, err := strconv.ParseFloat(state, 64)
		return err == nil && math.Abs(v-*amps) < 0.3
	}
	return true
}

// chargerActionError turns a validation/HA error into a short message.
func chargerActionError(lang string, err error) string {
	switch {
	case errors.Is(err, extensions.ErrChargerNoSwitch):
		return T(lang, "cargador.errorNoSwitch")
	case errors.Is(err, extensions.ErrChargerNoAmp):
		return T(lang, "cargador.errorNoAmp")
	case errors.Is(err, extensions.ErrChargerAmps):
		return T(lang, "cargador.errorAmps")
	case errors.Is(err, extensions.ErrChargerDelay):
		return T(lang, "cargador.errorDelay")
	case errors.Is(err, extensions.ErrChargerDuration):
		return T(lang, "cargador.errorDuration")
	}
	return T(lang, "common.error")
}

// chgSelectView is a native select bound to one charger action.
type chgSelectView struct {
	ID      string
	Options []chgSelectOption
	// SelectedLabel/SelectedValue describe the selected option: the closed
	// dropdown shows the label and the hidden input carries the value.
	SelectedLabel string
	SelectedValue string
}

// newChgSelect fills SelectedLabel from the option list so the markup can
// render the closed control server side.
func newChgSelect(id string, opts []chgSelectOption) chgSelectView {
	v := chgSelectView{ID: id, Options: opts}
	for _, o := range opts {
		if o.Selected {
			v.SelectedLabel = o.Label
			v.SelectedValue = o.Value
			break
		}
	}
	if v.SelectedValue == "" && len(opts) > 0 {
		v.SelectedLabel = opts[0].Label
		v.SelectedValue = opts[0].Value
	}
	return v
}

type chgSelectOption struct {
	Value    string
	Label    string
	Selected bool
}

// chgSelectClass is the shared look of the scheduling selects: full width and
// tall enough to balance the power button, so the card has no hollow gaps.
const chgSelectClass = "h-11 rounded-xl border border-app bg-surface px-3 text-sm font-semibold tabular-nums text-app transition-colors hover:bg-surface-2 disabled:opacity-50"

// buildStartSelects renders the start time as two selects (24h hour plus
// half-hour minutes). The device dp only understands hours from now: the
// Programar button converts the chosen clock time into that delay.
func buildStartSelects(key string, delay *float64, now time.Time) (chgSelectView, chgSelectView) {
	// Por defecto: hora actual y minutos 00 (petición del usuario).
	hour, minute := now.Hour(), 0
	if delay != nil {
		t := now.Add(time.Duration(*delay * float64(time.Hour))).Round(time.Minute)
		mins := int(math.Round(float64(t.Hour()*60+t.Minute())/30)) * 30
		hour = (mins / 60) % 24
		minute = mins % 60
	}
	hourOpts := make([]chgSelectOption, 0, 24)
	for h := 0; h < 24; h++ {
		hourOpts = append(hourOpts, chgSelectOption{
			Value:    strconv.Itoa(h),
			Label:    fmt.Sprintf("%02d", h),
			Selected: h == hour,
		})
	}
	minOpts := []chgSelectOption{
		{Value: "0", Label: "00", Selected: minute == 0},
		{Value: "30", Label: "30", Selected: minute == 30},
	}
	return newChgSelect("chg-h-"+key, hourOpts),
		newChgSelect("chg-m-"+key, minOpts)
}

// buildDurationSelect renders the 1-12 h duration selector.
func buildDurationSelect(key string, duration *float64) chgSelectView {
	cur := 6
	if duration != nil {
		cur = int(math.Round(*duration))
	}
	opts := make([]chgSelectOption, 0, 12)
	for h := 1; h <= 12; h++ {
		opts = append(opts, chgSelectOption{
			Value:    strconv.Itoa(h),
			Label:    strconv.Itoa(h) + " h",
			Selected: h == cur,
		})
	}
	return newChgSelect("chg-d-"+key, opts)
}

// chgTimedClickExpr posts both hardware scheduling values at once: the clock
// time of the start pickers and the duration picker.
func chgTimedClickExpr(key string) string {
	hid, mid, did := "chg-h-"+key, "chg-m-"+key, "chg-d-"+key
	return "$charger = '" + key + "'; $action = 'timed'; " +
		"$schedTime = (function(){var h = parseInt(document.getElementById('" + hid + "').value, 10); " +
		"var m = parseInt(document.getElementById('" + mid + "').value, 10); " +
		"return (h < 10 ? '0' : '') + h + ':' + (m < 10 ? '0' : '') + m;})(); " +
		"$schedHours = parseInt(document.getElementById('" + did + "').value, 10); " +
		"$ui.error = ''; @post('/ui/charger/action')"
}

// chgSchedClickExpr arms a software schedule: reads the two time selects plus
// the duration select and posts them with the schedule action.
func chgSchedClickExpr(key string) string {
	hid, mid, did := "chg-sh-"+key, "chg-sm-"+key, "chg-sd-"+key
	return "$charger = '" + key + "'; $action = 'schedule'; " +
		"$schedTime = (function(){var h = parseInt(document.getElementById('" + hid + "').value, 10); " +
		"var m = parseInt(document.getElementById('" + mid + "').value, 10); " +
		"return (h < 10 ? '0' : '') + h + ':' + (m < 10 ? '0' : '') + m;})(); " +
		"$schedHours = parseInt(document.getElementById('" + did + "').value, 10); " +
		"$ui.error = ''; @post('/ui/charger/action')"
}

// buildSchedStartSelects renders the start time pickers from the HA
// input_datetime state (HH:MM:SS), defaulting to the next hour when unset.
func buildSchedStartSelects(key, start string, now time.Time) (chgSelectView, chgSelectView) {
	t := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
	if p, err := time.ParseInLocation("15:04:05", start, time.Local); err == nil {
		t = p
	}
	hour, minute := t.Hour(), t.Minute()
	hourOpts := make([]chgSelectOption, 0, 24)
	for h := 0; h < 24; h++ {
		hourOpts = append(hourOpts, chgSelectOption{
			Value:    strconv.Itoa(h),
			Label:    fmt.Sprintf("%02d", h),
			Selected: h == hour,
		})
	}
	minOpts := []chgSelectOption{
		{Value: "0", Label: "00", Selected: minute < 30},
		{Value: "30", Label: "30", Selected: minute >= 30},
	}
	return newChgSelect("chg-sh-"+key, hourOpts),
		newChgSelect("chg-sm-"+key, minOpts)
}

// buildSchedDurSelect renders the 1-12 h duration picker of a software
// schedule (independent of the portable charger hardware dps).
func buildSchedDurSelect(key string, hours *float64) chgSelectView {
	cur := 6
	if hours != nil {
		cur = int(math.Round(*hours))
	}
	opts := make([]chgSelectOption, 0, 12)
	for h := 1; h <= 12; h++ {
		opts = append(opts, chgSelectOption{
			Value:    strconv.Itoa(h),
			Label:    strconv.Itoa(h) + " h",
			Selected: h == cur,
		})
	}
	return newChgSelect("chg-sd-"+key, opts)
}

// chgClock trims an HA time state (HH:MM:SS) to HH:MM for display.
func chgClock(s string) string {
	if len(s) >= 5 {
		return s[:5]
	}
	return "--:--"
}

// derefFloat returns the value pointed to by v, or def when nil.
func derefFloat(v *float64, def float64) float64 {
	if v == nil {
		return def
	}
	return *v
}

// currentOr returns the live value when available, nil otherwise.
func currentOr(hasLive bool, v *float64) *float64 {
	if !hasLive {
		return nil
	}
	return v
}

// buildChgSingle builds one charger panel: hero, live stats, today curve and
// history. The page paints every charger so the mobile track can show the
// neighbour while the finger drags.
func (u *UI) buildChgSingle(r *http.Request, lang string, pal Palette, today time.Time, charger *extensions.Charger) *chgSingleView {
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

	key := charger.Key
	sw := "$chg." + key + ".switchOn"
	sv.PressedExpr = sw + " ? 'true' : 'false'"
	// Feedback inmediato: al pulsar se marca el estado pedido ($ui.pending) y el
	// boton se pinta como "Encendiendo/Apagando" hasta que HA confirma. Si el
	// estado no llega (o falla), el intervalo del boton limpia pending.
	sv.PowerClick = "$charger = '" + key + "'; $action = " + sw + " ? 'off' : 'on'; $ui.error = ''; " +
		"$ui.pending = $action; $ui.pendingAt = Date.now(); @post('/ui/charger/action')"
	sv.LabelExpr = sw + " ? '" + jsQuote(T(lang, "cargador.powerOff")) + "' : '" + jsQuote(T(lang, "cargador.powerOn")) + "'"
	sv.TitleExpr = sv.LabelExpr
	sv.ClassExpr = sw + " ? '" + chgPowerOnClass + "' : '" + chgPowerOffClass + "'"
	sv.PendingExpr = "$ui.pending == 'on' ? '" + jsQuote(T(lang, "cargador.turningOn")) + "' : '" + jsQuote(T(lang, "cargador.turningOff")) + "'"
	sv.PendingText = T(lang, "cargador.turningOn")
	sv.PendingClearExpr = "if ($ui.pending != '' && ((($ui.pending == 'on') == " + sw +
		") || Date.now() - $ui.pendingAt > 12000)) { $ui.pending = '' }"
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
		switch {
		case hasLive && len(live.AmpOptions) > 0:
			opts = live.AmpOptions
		case hasLive && live.AmpMin != nil && live.AmpMax != nil:
			// A number entity declares the range it accepts, so mirror it with
			// its own step: a fixed preset list leaves valid currents (12 A)
			// without a chip and nothing marked as active. Long ranges fall
			// back to the presets: a wall box accepting 8-32 A would render
			// 25 chips, which is a picker nobody can use.
			opts = ampRange(*live.AmpMin, *live.AmpMax, live.AmpStep)
			if len(opts) > ampMaxChips {
				opts = nil
				for _, a := range AMP_STEPS {
					opts = append(opts, float64(a))
				}
			}
		default:
			for _, a := range AMP_STEPS {
				opts = append(opts, float64(a))
			}
		}
		// Clamp the chips to the real range of the amp entity (a number
		// entity exposes its min/max as attributes): out-of-range chips
		// would always fail validation on set.
		if hasLive && (live.AmpMin != nil || live.AmpMax != nil) {
			filtered := make([]float64, 0, len(opts))
			for _, a := range opts {
				if live.AmpMin != nil && a < *live.AmpMin {
					continue
				}
				if live.AmpMax != nil && a > *live.AmpMax {
					continue
				}
				filtered = append(filtered, a)
			}
			if len(filtered) > 0 {
				opts = filtered
			}
		}
		// The applied current must always have a chip: with the preset
		// fallback, a wall box set to 9 A would otherwise mark nothing.
		if hasLive && live.Amps != nil {
			found := false
			for _, a := range opts {
				if a == *live.Amps {
					found = true
					break
				}
			}
			inRange := (live.AmpMin == nil || *live.Amps >= *live.AmpMin) &&
				(live.AmpMax == nil || *live.Amps <= *live.AmpMax)
			if !found && inRange {
				opts = append(opts, *live.Amps)
			}
		}
		sort.Float64s(opts)
		ampClassOn := "inline-flex h-9 min-w-10 items-center justify-center rounded-xl border border-brand bg-brand/10 px-3 text-sm font-semibold tabular-nums text-brand transition-colors disabled:opacity-50"
		ampClassOff := "inline-flex h-9 min-w-10 items-center justify-center rounded-xl border border-app bg-surface px-3 text-sm font-semibold tabular-nums text-muted transition-colors hover:bg-surface-2 hover:text-app disabled:opacity-50"
		ampsSignal := "$chg." + charger.Key + ".amps"
		for _, a := range opts {
			active := hasLive && live.Amps != nil && *live.Amps == a
			ampsText := strconv.FormatFloat(a, 'f', -1, 64)
			sv.AmpChips = append(sv.AmpChips, ampChipView{
				AmpsText:  ampsText,
				Active:    active,
				Aria:      map[bool]string{true: "true", false: "false"}[active],
				ClickExpr: "$charger = '" + charger.Key + "'; $action = 'amp'; $amps = " + ampsText + "; $ui.error = ''; @post('/ui/charger/action')",
				AriaExpr:  "(" + ampsSignal + " == " + ampsText + ") ? 'true' : 'false'",
				ClassExpr: "(" + ampsSignal + " == " + ampsText + ") ? '" + ampClassOn + "' : '" + ampClassOff + "'",
			})
		}
		if hasLive && live.Amps != nil {
			sv.AmpsText = strconv.FormatFloat(*live.Amps, 'f', -1, 64)
		}
	}

	// Fila de programación (misma en ambos cargadores): hora de inicio +
	// duración + Programar. El portátil la aplica en su hardware con los dps
	// 118/119 vía helpers; el wall la arma con el input_datetime y el timer.
	if charger.DelayID != "" && charger.DurationID != "" {
		sv.HasSchedule = true
		sv.ScheduleLabel = T(lang, "cargador.schedule")
		sv.ScheduleHour, sv.ScheduleMinute = buildStartSelects(charger.Key, nil, time.Now())
		sv.ScheduleDur = buildDurationSelect(charger.Key, nil)
		sv.ScheduleClick = chgTimedClickExpr(charger.Key)
	} else if charger.ScheduleStartID != "" && charger.ScheduleDurationID != "" && charger.ScheduleArmID != "" {
		sv.HasSchedule = true
		sv.ScheduleLabel = T(lang, "cargador.schedule")
		var start string
		var hours *float64
		if hasLive {
			sv.ScheduleArmed = live.Scheduled
			// Sin una programación armada, los pickers arrancan en la hora
			// actual, minutos 00 y 6 h en lugar de mostrar lo último guardado.
			if live.Scheduled {
				start = live.ScheduleStart
				hours = live.ScheduleHours
			}
		}
		sv.ScheduleHour, sv.ScheduleMinute = buildSchedStartSelects(charger.Key, start, time.Now())
		sv.ScheduleDur = buildSchedDurSelect(charger.Key, hours)
		sv.ScheduleClick = chgSchedClickExpr(charger.Key)
		sv.ScheduleCancel = "$charger = '" + charger.Key + "'; $action = 'unschedule'; $ui.error = ''; @post('/ui/charger/action')"
		if sv.ScheduleArmed {
			sv.ScheduleText = T(lang, "cargador.scheduled") + " " + chgClock(start) + " · " + fmtInt(lang, math.Round(derefFloat(hours, 0))) + " h"
		}
	}

	if hasLive && live.TempC != nil {
		sv.TempKnown = true
		sv.TempText = fmtInt(lang, math.Round(*live.TempC)) + " °C"
	} else {
		sv.TempText = "—"
	}

	powerText := "…"
	animPow := 0.0
	if hasLive {
		animPow = live.PowerKw
		powerText = fmtKwVal(lang, live.PowerKw) + " kW"
	}
	k := charger.Key
	powStat := chgLiveStat{IconKey: "zap", Label: T(lang, "cargador.currentPower"), Value: powerText, Signal: "$chg." + k + ".powerKw",
		AnimID: "chg" + k + "Pow", Num: animPow, Dec: 2, Suffix: " kW"}
	// La energía sale de la contabilidad propia (daily_charger), no del
	// contador del aparato: hay firmware que no lo publica y otros que lo
	// repiten por sesión, así que se usa una sola fuente para todos.
	var stats []chgLiveStat
	switch {
	case !hasLive:
		stats = append(stats, powStat)
	case live.DeviceOffline:
		// El aparato no está en la red (apagado o desenchufado): es su estado
		// normal cuando no se usa, no una avería ni un dato que falte.
		stats = append(stats, chgLiveStat{
			IconKey: "zap",
			Label:   T(lang, "cargador.currentPower"),
			Value:   T(lang, "cargador.disconnected"),
		})
	case live.PowerAgeS > int(chargerStaleAfter/time.Second):
		// Socket abierto sin frames: mejor decir desde cuándo no hay dato
		// que pintar un 0 kW que parece una lectura real.
		stats = append(stats, chgLiveStat{
			IconKey: "clock",
			Label:   T(lang, "cargador.currentPower"),
			Value:   T(lang, "cargador.staleData", map[string]string{"n": strconv.Itoa(live.PowerAgeS / 60)}),
		})
	case live.PowerKnown:
		stats = append(stats, powStat)
	}
	if u.DB != nil {
		if todayKwh, totalKwh, err := u.DB.ChargerEnergyTotals(k, solar.TodayStr()); err == nil {
			stats = append(stats,
				chgLiveStat{IconKey: "sun", Label: T(lang, "inversores.energyToday"),
					Value: fmtEnergy(lang, todayKwh) + " kWh"},
				chgLiveStat{IconKey: "gauge", Label: T(lang, "cargador.lifetime"),
					Value: fmtEnergy(lang, totalKwh) + " kWh"},
			)
		}
	}
	sv.Stats = stats

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

	// Horas de carga y franja inicio-fin por día (issue 173): del historial de
	// estados del cargador, para el periodo visible.
	var times map[string]extensions.ChargerDayTimes
	if u.Ext != nil {
		fromD, toD := periodWindow(period, anchor)
		times = u.Ext.ChargerDayTimes(u.ExtHA, fromD.Format("2006-01-02"), toD.Format("2006-01-02"), charger)
	}

	// Selector del cargador: mismas pestañas que el React (semana/mes/año, sin
	// día) y TODAS las URLs apuntan a /cargador conservando el cargador activo
	// (antes solo la de semana lo hacía y el resto saltaba al Histórico).
	sv.Period = period
	sv.Sel = buildPeriodSelector(lang, period, anchor, today, 0)
	tabKey := charger.Key
	chgTabAnchor := func(p Period) time.Time {
		if p == PeriodSemana {
			return today // la semana del cargador incluye el día en curso
		}
		return initialAnchor(p, today)
	}
	tabs := sv.Sel.Tabs[:0]
	for _, tab := range sv.Sel.Tabs {
		p := Period(tab.Key)
		if p == PeriodDia {
			continue
		}
		tab.URL = histURLChg(p, chgTabAnchor(p), tabKey)
		tabs = append(tabs, tab)
	}
	sv.Sel.Tabs = tabs
	sv.Sel.PrevURL = histURLChg(period, shiftAnchor(period, anchor, -1), tabKey)
	sv.Sel.NextURL = histURLChg(period, shiftAnchor(period, anchor, 1), tabKey)
	sv.Sel.TodayURL = histURLChg(period, chgTabAnchor(period), tabKey)
	sv.Sel.CanNext = canGoNext(period, anchor, today)
	sv.Sel.IsCurrent = isCurrentPeriod(period, anchor, today)
	sv.NavLabel = periodNavLabel(period, anchor, lang)
	sv.HistSub = periodSubtitle(period, anchor, lang)

	st := chgStats(lang, period, anchor, today, hist, solarRows)
	sv.Hist, _ = chgHistoryChart(lang, period, anchor, today, hist, solarRows, times, pal)
	// Only the energy actually charged counts: st.Prod is the whole-house solar
	// production, which is always > 0 and would hide the empty state forever.
	sv.HistHasData = st.Kwh > 0

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
	return sv
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

	_, _, isAdmin := u.currentUserActor(r)
	v := chargerView{pageProps: p, Palette: pal, IsAdmin: isAdmin, ImgErr: r.URL.Query().Get("imgerr") == "1"}
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
	signals["ui"] = map[string]any{"busy": false, "error": "", "charger": charger.Key, "action": "", "amps": nil,
		"pending": "", "pendingAt": 0}
	signals["schedTime"] = ""
	signals["schedHours"] = 4
	sigJSON, err := json.Marshal(signals)
	if err != nil {
		sigJSON = []byte("{}")
	}
	v.Signals = string(sigJSON)

	// Paneles de TODOS los cargadores: en movil el track se desliza con el dedo
	// y necesita la vecina ya pintada (React la montaba en memoria).
	activeIndex := 0
	for i := range cfg.CarCharger.Chargers {
		c := &cfg.CarCharger.Chargers[i]
		if c.Key == charger.Key {
			activeIndex = i
		}
		v.Panels = append(v.Panels, u.buildChgSingle(r, lang, pal, today, c))
	}
	v.ActiveIndex = activeIndex
	v.Active = v.Panels[activeIndex]
	_ = ChargerPage(v).Render(r.Context(), w)
}

// histURLChg is the cargador history navigation URL (keeps the active
// charger tab so period changes stay in the same card).
func histURLChg(p Period, anchor time.Time, tabKey string) string {
	u := "/cargador?period=" + string(p) + "&anchor=" + anchor.Format("2006-01-02")
	if tabKey != "" {
		u += "&tab=" + url.QueryEscape(tabKey)
	}
	// #historial-carga: mantener la vista en la tarjeta del histórico.
	return u + "#historial-carga"
}

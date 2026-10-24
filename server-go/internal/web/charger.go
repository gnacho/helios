package web

import (
	"encoding/json"
	"errors"
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

// Clases del boton de encendido en cada estado: van dentro de una expresion
// Datastar (data-attr-class), asi que el color cambia sin esperar al servidor.
// Los mismos literales aparecen en charger.templ (Tailwind los ve alli).
const (
	chgPowerOnClass  = "inline-flex h-14 w-full items-center justify-center gap-2.5 rounded-2xl text-base font-semibold transition-colors disabled:opacity-50 bg-emerald-500/12 text-emerald-600 hover:bg-emerald-500/20 dark:text-emerald-400"
	chgPowerOffClass = "inline-flex h-14 w-full items-center justify-center gap-2.5 rounded-2xl text-base font-semibold transition-colors disabled:opacity-50 bg-brand text-white hover:opacity-90"
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
	LinePath    string
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
	IsAdmin bool
	ImgErr  bool
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
func chgHistoryChart(lang string, p Period, anchor, today time.Time, hist []extensions.ChargerHistoryRow, solarRows []db.DailyRow, pal Palette) (chgHistModel, bool) {
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
	_, err := extensions.ApplyAction(u.ExtHA, *charger, body.Action, body.Amps)
	if err == nil {
		confirmed := u.waitChargerState(*charger, body.Action, body.Amps)
		log.Printf("[chg-action] %s %s confirmed=%v in %s", charger.Key, body.Action, confirmed, time.Since(start).Round(time.Millisecond))
	} else {
		log.Printf("[chg-action] %s %s error=%v", charger.Key, body.Action, err)
	}

	patch := map[string]any{"ui": map[string]any{"pending": "", "error": ""}}
	if chg := u.chargerSignals(lang); chg != nil {
		patch["chg"] = chg
	}
	if err != nil {
		patch["ui"] = map[string]any{"pending": "", "error": chargerActionError(lang, err)}
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
	}
	return T(lang, "common.error")
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
				ClickExpr: "$charger = '" + charger.Key + "'; $action = 'amp'; $amps = " + ampsText + "; $ui.error = ''; @post('/ui/charger/action')",
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
	animPow, animSes, animTot := 0.0, 0.0, 0.0
	if hasLive {
		animPow = live.PowerKw
		powerText = fmtKwVal(lang, live.PowerKw) + " kW"
		if live.SessionKwh != nil {
			animSes = *live.SessionKwh
			sessionText = fmtEnergy(lang, *live.SessionKwh) + " kWh"
		}
		if live.TotalKwh != nil {
			animTot = *live.TotalKwh
			totalText = fmtEnergy(lang, *live.TotalKwh) + " kWh"
		}
	}
	k := charger.Key
	powStat := chgLiveStat{IconKey: "zap", Label: T(lang, "cargador.currentPower"), Value: powerText, Signal: "$chg." + k + ".powerKw",
		AnimID: "chg" + k + "Pow", Num: animPow, Dec: 2, Suffix: " kW"}
	sesStat := chgLiveStat{IconKey: "clock", Label: T(lang, "cargador.session"), Value: sessionText, Signal: "$chg." + k + ".sessionKwh",
		AnimID: "chg" + k + "Ses", Num: animSes, Dec: 1, Suffix: " kWh"}
	totStat := chgLiveStat{IconKey: "gauge", Label: T(lang, "cargador.lifetime"), Value: totalText, Signal: "$chg." + k + ".totalKwh",
		AnimID: "chg" + k + "Tot", Num: animTot, Dec: 1, Suffix: " kWh"}
	// Solo se muestra lo que el cargador publica de verdad: un 0,00 kW de una
	// entidad unavailable y un "total" que repite la sesión son ruido, no datos.
	if !hasLive {
		sv.Stats = []chgLiveStat{powStat, sesStat, totStat}
	} else {
		var stats []chgLiveStat
		if live.PowerKnown {
			stats = append(stats, powStat)
		}
		stats = append(stats, sesStat)
		if live.TotalKwh != nil && !sameKwh(live.SessionKwh, live.TotalKwh) {
			stats = append(stats, totStat)
		}
		sv.Stats = stats
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

	// Selector del cargador: mismas pestañas que el React (semana/mes/año, sin
	// día) y TODAS las URLs apuntan a /cargador conservando el cargador activo
	// (antes solo la de semana lo hacía y el resto saltaba al Histórico).
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
	sv.Hist, _ = chgHistoryChart(lang, period, anchor, today, hist, solarRows, pal)
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

	v.Active = sv
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

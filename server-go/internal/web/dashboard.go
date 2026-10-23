package web

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/starfederation/datastar-go/datastar"

	"helios-server/internal/install"
	"helios-server/internal/solar"
)

// dashboardModel is everything the dashboard page and its signals need.
type dashboardModel struct {
	Live    *solar.LiveData // nil when HAOS is not configured
	Kpis    DayKpis
	Series  solar.DaySeries
	Invs    []install.Inverter
	Online  bool
	Palette Palette
}

// buildDashboard gathers live data, day series (cached internally by the
// solar package), integrated KPIs and the inverter topology.
func (u *UI) buildDashboard(p Palette) dashboardModel {
	var m dashboardModel
	m.Palette = p
	if u.Install != nil {
		m.Invs = u.Install.Get().Inverters
	}
	if u.Solar == nil || u.HA == nil {
		return m
	}
	live := u.Solar.ComputeLive(u.HA)
	m.Live = &live
	m.Online = u.HA.IsConnected()
	if res, err := u.Solar.GetDaySeries(u.HA, solar.TodayStr()); err == nil {
		m.Series = res
	}
	m.Kpis = IntegrateSeries(m.Series.Points, untilMin(m), liveSoc(m.Live))
	return m
}

func liveSoc(l *solar.LiveData) float64 {
	if l == nil {
		return 0
	}
	return l.Soc
}

func untilMin(m dashboardModel) int {
	if m.Live != nil {
		return m.Live.At
	}
	n := time.Now()
	return n.Hour()*60 + n.Minute()
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func absPtr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	a := abs(*v)
	return &a
}

// dashboardSignals builds the Datastar signals tree. Display values are
// server-formatted strings (locale aware) so the browser has zero formatting
// logic; data-text/data-style/data-show bindings consume them directly.
func dashboardSignals(lang string, m dashboardModel) map[string]any {
	signals := map[string]any{}

	defaults := newPalette(false)
	live := map[string]any{
		"productionText":  "—",
		"productionHot":   false,
		"productionColor": "",
		"consumptionText": "—",
		"batteryKwText":   fmtKwVal(lang, 0),
		"socText":         fmtInt(lang, 0),
		"batteryStatus":   "",
		"gridKwText":      "—",
		"gridStatus":      "—",
		"gridExporting":   false,
		"gridColor":       defaults.RedCompra,
		"batteryText":     T(lang, "common.idle"),
		"gridText":        "—",
		"atText":          "--:--",
		"online":          m.Online,
	}
	if m.Live != nil {
		p := &m.Palette
		l := m.Live
		exporting := l.Grid != nil && *l.Grid < -0.05
		gridColor := p.RedCompra
		gridStatus := T(lang, "live.balanced")
		if l.Grid == nil {
			gridStatus = "—"
		} else if *l.Grid > 0.05 {
			gridStatus = T(lang, "live.buying")
		} else if *l.Grid < -0.05 {
			gridStatus = T(lang, "live.exporting")
			gridColor = p.RedVertido
		}
		prodColor := p.Solar
		if l.Production <= 0.1 {
			prodColor = ""
		}
		live = map[string]any{
			"productionText":  fmtKwVal(lang, l.Production),
			"productionHot":   l.Production > 0.1,
			"productionColor": prodColor,
			"consumptionText": fmtKw(lang, l.Consumption),
			"batteryKwText":   fmtKwVal(lang, abs(l.BatteryPower)),
			"socText":         fmtInt(lang, l.Soc),
			"batteryStatus":   T(lang, "common."+batteryState(l.BatteryPower)),
			"gridKwText":      fmtKw(lang, absPtr(l.Grid)),
			"gridStatus":      gridStatus,
			"gridExporting":   exporting,
			"gridColor":       gridColor,
			"batteryText":     batteryFlowText(lang, m.Live),
			"gridText":        gridFlowText(lang, m.Live),
			"atText":          fmtTime(l.At),
			"online":          m.Online,
		}
	}
	signals["live"] = live

	kpis := map[string]any{
		"productionText":  fmtEnergy(lang, m.Kpis.ProductionKwh),
		"consumptionText": fmtEnergy(lang, m.Kpis.ConsumptionKwh),
		"batteryKwhText":  fmtEnergy(lang, m.Kpis.Soc/100*5),
		"socSubText":      "",
		"autoconsumoText": fmtInt(lang, m.Kpis.AutoconsumoPct),
		"autoconsumoPct":  m.Kpis.AutoconsumoPct,
		"gridBalanceText": fmtEnergy(lang, m.Kpis.GridExportKwh),
		"gridExportText":  fmtEnergy(lang, m.Kpis.GridExportKwh),
		"gridImportText":  fmtEnergy(lang, m.Kpis.GridImportKwh),
	}
	if m.Live != nil {
		kpis["socSubText"] = fmtInt(lang, m.Live.Soc) + "% · " + T(lang, "common."+batteryState(m.Live.BatteryPower))
	}
	signals["kpis"] = kpis

	kwhs := map[string]float64{}
	total := 0.0
	for _, in := range m.Invs {
		kwh := perInverterKwh(m.Series.Points, in.Key, untilMin(m))
		kwhs[in.Key] = kwh
		total += kwh
	}
	inv := map[string]any{}
	for i, in := range m.Invs {
		nowKw := inverterLiveKw(m, in.Key, i)
		share := 0.0
		if total > 0 {
			share = kwhs[in.Key] / total * 100
		} else if len(m.Invs) == 1 {
			share = 100
		}
		inv[in.Key] = map[string]any{
			"nowText":   fmtKwVal(lang, nowKw),
			"kwhText":   fmtEnergy(lang, kwhs[in.Key]),
			"shareText": fmtInt(lang, share),
			"sharePct":  share,
			"nowPct":    nowPct(in.Kwp, nowKw),
		}
	}
	signals["inv"] = inv

	// Señales de visibilidad del chart (port de los chips de DayChart).
	chart := map[string]any{"total": true, "consumo": true}
	for i, in := range m.Invs {
		chart[in.Key] = i < 2 // inversores ocultos por defecto, como en React
	}
	signals["chart"] = chart
	return signals
}

// chartInvKeys replica el fallback de Dashboard.tsx: topología o solis/fox.
func chartInvKeys(m dashboardModel) ([]string, map[string]string) {
	if len(m.Invs) > 0 {
		keys := make([]string, 0, len(m.Invs))
		names := map[string]string{}
		for _, in := range m.Invs {
			keys = append(keys, in.Key)
			names[in.Key] = in.Name
		}
		return keys, names
	}
	return []string{"solis", "fox"}, map[string]string{"solis": "Solis", "fox": "Fox"}
}

// inverterLiveKw mirrors liveKw() in Dashboard.tsx.
func inverterLiveKw(m dashboardModel, key string, index int) float64 {
	if m.Live == nil {
		return 0
	}
	if len(m.Live.Inverters) > 0 {
		for _, inv := range m.Live.Inverters {
			if inv.Key == key {
				return inv.Kw
			}
		}
		return 0
	}
	switch key {
	case "solis":
		return m.Live.Solis
	case "fox":
		return m.Live.Fox
	}
	return 0
}

// nowPct is the load bar of InverterCard: now / kwp, clamped 0..100.
func nowPct(kwp, nowKw float64) float64 {
	if kwp <= 0 {
		return 0
	}
	pct := nowKw / kwp * 100
	if pct > 100 {
		pct = 100
	}
	return pct
}

// handleStream serves GET /ui/stream: a Datastar SSE stream that patches the
// dashboard signals on every hub broadcast (throttled to 1s in the hub).
func (u *UI) handleStream(w http.ResponseWriter, r *http.Request) {
	if u.Hub == nil || u.Solar == nil || u.HA == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	ch := u.Hub.SubscribeDatastar()
	if ch == nil {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	defer u.Hub.UnsubscribeDatastar(ch)

	lang := u.signalLang(r)
	gen := datastar.NewSSE(w, r)

	// Estado inicial: señales completas antes del primer tick del hub.
	if err := gen.MarshalAndPatchSignals(dashboardSignals(lang, u.buildDashboard(newPalette(themeFrom(r) == "dark")))); err != nil {
		return
	}

	flusher, _ := w.(http.Flusher)
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-ch:
			live, err := decodeLivePayload(payload)
			if err != nil {
				continue
			}
			m := u.buildDashboard(newPalette(themeFrom(r) == "dark"))
			m.Live = live
			m.Online = u.HA.IsConnected()
			m.Kpis = IntegrateSeries(m.Series.Points, untilMin(m), liveSoc(live))
			if uerr := gen.MarshalAndPatchSignals(dashboardSignals(lang, m)); uerr != nil {
				return
			}
		case <-heartbeat.C:
			// Comentario SSE: mantiene viva la conexión tras proxies sin tocar señales.
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

// decodeLivePayload extracts the LiveData from the hub's JSON payload.
func decodeLivePayload(payload []byte) (*solar.LiveData, error) {
	var wrapper struct {
		Type string         `json:"type"`
		Data solar.LiveData `json:"data"`
	}
	if err := json.Unmarshal(payload, &wrapper); err != nil {
		return nil, err
	}
	return &wrapper.Data, nil
}

// batteryFlowText / gridFlowText are the flow-node value texts.
func batteryFlowText(lang string, l *solar.LiveData) string {
	switch {
	case l.BatteryPower > 0.05:
		return fmtKwVal(lang, l.BatteryPower) + " kW"
	case l.BatteryPower < -0.05:
		return fmtKwVal(lang, -l.BatteryPower) + " kW"
	}
	return T(lang, "common.idle")
}

func gridFlowText(lang string, l *solar.LiveData) string {
	if l.Grid == nil {
		return "—"
	}
	if *l.Grid < -0.05 {
		return fmtKwVal(lang, -*l.Grid) + " kW ↑"
	}
	if *l.Grid > 0.05 {
		return fmtKwVal(lang, *l.Grid) + " kW ↓"
	}
	return "0 W"
}

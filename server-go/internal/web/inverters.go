package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"helios-server/internal/auth"
	"helios-server/internal/db"
	"helios-server/internal/install"
	"helios-server/internal/solar"
)

// /inversores page (port of app/src/pages/Inversores.tsx with the
// InverterHeroCard, InverterDayChart, CompareChart, Last14Chart and the share
// donut/bar). Tabs are plain links (?inv=key|compare); the mobile swipe
// carousel and the interactive toggles are not ported.

// dailyHistory loads the whole daily table (same window as the API default),
// paginating past the 1000-row LIMIT so recent days are never truncated.
func (u *UI) dailyHistory() []db.DailyRow {
	if u.DB == nil {
		return nil
	}
	from, err := u.DB.MinDailyDate()
	if err != nil || from == "" {
		return nil
	}
	to := solar.TodayStr()
	var rows []db.DailyRow
	for offset := 0; ; offset += 1000 {
		page, err := u.DB.DailyRange(from, to, 1000, offset)
		if err != nil {
			return rows
		}
		rows = append(rows, page...)
		if len(page) < 1000 {
			break
		}
	}
	return rows
}

// invMetasOf builds InvMeta from the topology (or the classic Solis/Fox
// fallback), with month/total energy from the daily history. tempC keeps the
// same placeholders as the React page (the topology has no live temp).
func invMetasOf(inst []install.Inverter, hist []db.DailyRow, now time.Time) []invMetaView {
	if len(inst) == 0 {
		inst = []install.Inverter{
			{Key: "solis", Name: "Solis", Model: "Solis S5-EH1P5K-L", Kwp: 4.4, Panels: "10 × 440 W", HasBattery: true, BatteryKwh: 5, TempC: 46},
			{Key: "fox", Name: "Fox", Model: "Fox H1-3.0-E", Kwp: 2.7, Panels: "6 × 450 W", TempC: 41},
		}
	}
	month := monthRows(hist, now)
	metas := make([]invMetaView, 0, len(inst))
	for _, inv := range inst {
		tempC := 41
		if inv.TempC > 0 {
			tempC = int(inv.TempC)
		}
		metas = append(metas, invMetaView{
			Key:        inv.Key,
			Name:       inv.Name,
			Model:      inv.Model,
			Panels:     inv.Panels,
			Kwp:        inv.Kwp,
			HasBattery: inv.HasBattery,
			BatteryKwh: inv.BatteryKwh,
			TempC:      tempC,
			MonthKwh:   kwhSum(month, inv.Key),
			TotalMwh:   kwhSum(hist, inv.Key) / 1000,
		})
	}
	return metas
}

// invSignalsOf replicates the "inv" subtree of dashboardSignals for arbitrary
// keys (the page shows the fallback keys when the topology is empty).
func invSignalsOf(lang string, m dashboardModel, keys []string) map[string]any {
	kwhs := map[string]float64{}
	total := 0.0
	for _, k := range keys {
		kwh := perInverterKwh(m.Series.Points, k, untilMin(m))
		kwhs[k] = kwh
		total += kwh
	}
	inv := map[string]any{}
	for i, k := range keys {
		nowKw := inverterLiveKw(m, k, i)
		share := 0.0
		if total > 0 {
			share = kwhs[k] / total * 100
		} else if len(keys) == 1 {
			share = 100
		}
		inv[k] = map[string]any{
			"nowText":   fmtKwVal(lang, nowKw),
			"kwhText":   fmtEnergy(lang, kwhs[k]),
			"shareText": fmtInt(lang, share),
			"sharePct":  share,
			"nowPct":    nowPct(kwpOf(m, k), nowKw),
		}
	}
	return map[string]any{"inv": inv}
}

// kwpOf finds the installed kWp of a key (0 when unknown).
func kwpOf(m dashboardModel, key string) float64 {
	for _, in := range m.Invs {
		if in.Key == key {
			return in.Kwp
		}
	}
	return 0
}

// invDayKwhAndPeaks integrates per-inverter kWh and the daily peak from the
// day series (same loop as CompareView/Inversores.tsx).
func invDayKwhAndPeaks(points []solar.DayPoint, keys []string) ([]float64, []peakAt) {
	kwhs := make([]float64, len(keys))
	peaks := make([]peakAt, len(keys))
	for i, k := range keys {
		peaks[i] = peakAt{V: 0, T: 840}
		for _, p := range points {
			v := seriesInvValue(p, k)
			kwhs[i] += v * (float64(stepMin) / 60)
			if v > peaks[i].V {
				peaks[i] = peakAt{V: v, T: p.T}
			}
		}
	}
	return kwhs, peaks
}

type peakAt struct {
	V float64
	T int
}

// inversoresView carries everything InversoresPage renders.
type inversoresView struct {
	pageProps
	Signals     string
	Palette     Palette
	Tabs        []invTabView
	Panels      []invPanelView // one per tab, for the mobile swipe track
	ActiveIndex int
	Single      *invSingleView
	Compare     *invCompareView
	Note        string
}

// invPanelView is one tab of the inverters page as painted inside the track.
type invPanelView struct {
	Key     string
	Active  bool
	Compare *invCompareView
	Single  *invSingleView
}

// IsCompare reports whether the panel is the side-by-side comparison.
func (p invPanelView) IsCompare() bool { return p.Compare != nil }

type invSingleView struct {
	Hero   invHeroModel
	Kpis   []invKpiView
	Chart  invDayChartModel
	Last14 last14Model
}

func (u *UI) handleInversores(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	lang := u.userLang(userID)
	p := u.baseProps(r, userID, "/inversores", "")
	p.Title = T(lang, "nav.inversores")

	pal := newPalette(p.Theme == "dark")
	m := u.buildDashboard(pal)
	hist := u.dailyHistory()

	keys, names := chartInvKeys(m)
	metas := invMetasOf(m.Invs, hist, time.Now())
	if len(metas) > 0 {
		// Nombres reales de la topología para el chart y las señales.
		for i := range keys {
			if names[keys[i]] == "" {
				names[keys[i]] = keys[i]
			}
		}
		for i, mt := range metas {
			if i < len(keys) {
				keys[i] = mt.Key
				names[mt.Key] = mt.Name
			}
		}
	}

	// Tab activa: ?inv= (canónico) o ?tab= (alias histórico del dashboard).
	tab := r.URL.Query().Get("inv")
	if tab == "" {
		tab = r.URL.Query().Get("tab")
	}
	tabKeys := make([]string, 0, len(metas)+1)
	if len(metas) >= 2 {
		tabKeys = append(tabKeys, "compare")
	}
	for _, mt := range metas {
		tabKeys = append(tabKeys, mt.Key)
	}
	active := tabKeys[0]
	if len(tabKeys) == 0 {
		active = "compare"
	} else {
		for _, k := range tabKeys {
			if tab == k {
				active = k
				break
			}
		}
	}

	sig := invSignalsOf(lang, m, keys)
	if len(metas) >= 2 {
		sig["share"] = map[string]any{"mode": "donut"}
	}
	sigJSON, err := json.Marshal(sig)
	if err != nil {
		sigJSON = []byte("{}")
	}

	v := inversoresView{
		pageProps: p,
		Signals:   string(sigJSON),
		Palette:   pal,
		Note:      T(lang, "inversores.localData"),
	}
	for _, k := range tabKeys {
		isCompare := k == "compare"
		label := T(lang, "inversores.compare")
		color := ""
		if !isCompare {
			for i := range metas {
				if metas[i].Key == k {
					label = metas[i].Name
					color = InverterColor(k, pal, i)
				}
			}
		}
		v.Tabs = append(v.Tabs, invTabView{Key: k, Label: label, Color: color, Active: k == active, IsCompare: isCompare})
	}

	// Todas las pestanas se pintan: en movil el track desliza y ensena la
	// vecina sin pedir nada al servidor (React las tenia en memoria).
	norm := r.URL.Query().Get("norm") == "1"
	for _, k := range tabKeys {
		if k == "compare" {
			if len(metas) >= 2 {
				v.Panels = append(v.Panels, invPanelView{Key: k, Compare: buildCompareView(lang, pal, m, metas, keys, names, norm)})
			}
			continue
		}
		v.Panels = append(v.Panels, invPanelView{Key: k, Single: buildSingleView(lang, pal, m, metas, keys, k, hist)})
	}
	for i := range v.Panels {
		if v.Panels[i].Key != active {
			continue
		}
		v.Panels[i].Active = true
		v.Single = v.Panels[i].Single
		v.Compare = v.Panels[i].Compare
		v.ActiveIndex = i
	}

	_ = InversoresPage(v).Render(r.Context(), w)
}

// buildSingleView builds the per-inverter tab (hero + KPIs + curve + bars).
func buildSingleView(lang string, pal Palette, m dashboardModel, metas []invMetaView, keys []string, key string, hist []db.DailyRow) *invSingleView {
	idx := -1
	for i := range metas {
		if metas[i].Key == key {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	meta := metas[idx]
	color := InverterColor(key, pal, idx)

	kwhs, _ := invDayKwhAndPeaks(m.Series.Points, keys)
	dayKwh := 0.0
	if idx < len(kwhs) {
		dayKwh = kwhs[idx]
	}
	nowKw := inverterLiveKw(m, key, idx)
	share := totalShareOf(kwhs, idx)

	nowSignal := "$inv." + key + ".nowText"

	v := &invSingleView{
		Hero: invHeroModel{
			Aria:           T(lang, "inverter.aria", map[string]string{"name": meta.Name}),
			Name:           meta.Name,
			ModelText:      meta.Model,
			Color:          color,
			NowText:        fmtKwVal(lang, nowKw),
			NowSignal:      nowSignal,
			Panels:         meta.Panels,
			HasBattery:     meta.HasBattery,
			BatteryKwhText: formatNumber(lang, meta.BatteryKwh, 0),
			TempC:          meta.TempC,
			TempText:       fmtInt(lang, float64(meta.TempC)),
			TempHot:        meta.TempC > 60,
			OnlineText:     T(lang, "common.online"),
			ProducingText:  T(lang, "inverter.producingNow"),
			SolarFieldText: T(lang, "inverter.solarField"),
			CoupledText:    T(lang, "inverter.batteryCoupled"),
		},
		Chart:  BuildInverterDayChart(lang, m.Series.Points, key, meta.Name, color, untilMin(m)),
		Last14: buildLast14(lang, hist, share, color),
	}

	perf := 0.0
	if meta.Kwp > 0 {
		perf = dayKwh / meta.Kwp
	}
	v.Kpis = []invKpiView{
		{Label: T(lang, "inversores.today"), ValueText: fmtEnergy(lang, dayKwh), Num: dayKwh, Dec: 1, Unit: "kWh", Accent: color,
			HasDelta: true, DeltaText: T(lang, "inversores.vsYesterday")},
		{Label: T(lang, "inversores.thisMonth"), ValueText: formatNumber(lang, meta.MonthKwh, 0), Num: meta.MonthKwh, Dec: 0, Unit: "kWh", Accent: color},
		{Label: T(lang, "inversores.total"), ValueText: formatNumber(lang, meta.TotalMwh, 2), Num: meta.TotalMwh, Dec: 2, Unit: "MWh", Accent: color},
		{Label: T(lang, "inversores.performance"), ValueText: formatNumber(lang, perf, 2), Num: perf, Dec: 2, Unit: "kWh/kWp", Accent: color},
	}
	return v
}

func totalShareOf(kwhs []float64, idx int) float64 {
	var total float64
	for _, v := range kwhs {
		total += v
	}
	if total <= 0 {
		if idx == 0 {
			return 100
		}
		return 0
	}
	return kwhs[idx] / total * 100
}

// buildCompareView builds the compare tab: hero, superposed chart, metrics
// table and share (donut + bar list).
func buildCompareView(lang string, pal Palette, m dashboardModel, metas []invMetaView, keys []string, names map[string]string, norm bool) *invCompareView {
	colors := make([]string, len(metas))
	for i := range metas {
		colors[i] = InverterColor(metas[i].Key, pal, i)
	}
	nowKws := make([]float64, len(metas))
	for i := range metas {
		nowKws[i] = inverterLiveKw(m, metas[i].Key, i)
	}
	kwhs, peaks := invDayKwhAndPeaks(m.Series.Points, keys)
	shares := make([]float64, len(metas))
	{
		var total float64
		for _, v := range kwhs {
			total += v
		}
		for i := range kwhs {
			if total > 0 {
				shares[i] = kwhs[i] / total * 100
			} else {
				shares[i] = 100 / float64(len(kwhs))
			}
		}
	}

	v := &invCompareView{
		Norm:         norm,
		BestAria:     T(lang, "inversores.bestMetric"),
		ShareInsight: T(lang, "inversores.shareInsight"),
	}
	for i := range metas {
		kwpText := ""
		if metas[i].Kwp > 0 {
			kwpText = formatNumber(lang, metas[i].Kwp, 2) + " kWp"
		}
		v.Heroes = append(v.Heroes, invCompareHero{
			Key:        metas[i].Key,
			Name:       metas[i].Name,
			Color:      colors[i],
			KwpText:    kwpText,
			NowText:    fmtKwVal(lang, nowKws[i]),
			NowSignal:  "$inv." + metas[i].Key + ".nowText",
			KwhText:    fmtEnergy(lang, kwhs[i]),
			TodayLabel: T(lang, "inversores.today"),
		})
		v.HeadNames = append(v.HeadNames, metas[i].Name)
		v.HeadColors = append(v.HeadColors, colors[i])
	}

	kwps := make([]float64, len(metas))
	for i := range metas {
		kwps[i] = metas[i].Kwp
	}
	v.Chart = BuildCompareChart(lang, m.Series.Points, untilMin(m), keys, names, colors, kwps, norm)

	// Tabla de métricas (7 filas, con chip de "mejor" cuando aplica).
	perf := make([]float64, len(metas))
	perfTxt := make([]string, len(metas))
	for i := range metas {
		if metas[i].Kwp > 0 {
			perf[i] = kwhs[i] / metas[i].Kwp
			perfTxt[i] = formatNumber(lang, perf[i], 2)
		} else {
			perfTxt[i] = "—"
		}
	}
	tempBest := -1
	if len(metas) == 2 {
		if metas[0].TempC <= metas[1].TempC {
			tempBest = 0
		} else {
			tempBest = 1
		}
	}
	cell := func(txt string, i, best int) invTableCell {
		return invTableCell{Text: txt, Color: colors[i], Best: best == i}
	}
	row := func(label string, cells []invTableCell) invTableRow {
		return invTableRow{Label: label, Cells: cells}
	}
	powCells := make([]invTableCell, len(metas))
	energyCells := make([]invTableCell, len(metas))
	peakCells := make([]invTableCell, len(metas))
	perfCells := make([]invTableCell, len(metas))
	tempCells := make([]invTableCell, len(metas))
	shareCells := make([]invTableCell, len(metas))
	statusCells := make([]invTableCell, len(metas))
	powBest, energyBest, peakBest := bestIdx(nowKws), bestIdx(kwhs), bestIdx(peakVals(peaks))
	perfBest := bestIdx(perf)
	shareBest := bestIdx(shares)
	sharePct := integerShares(shares)
	for i := range metas {
		powCells[i] = cell(fmtKwVal(lang, nowKws[i])+" kW", i, powBest)
		energyCells[i] = cell(fmtEnergy(lang, kwhs[i])+" kWh", i, energyBest)
		peakCells[i] = cell(fmtKwVal(lang, peaks[i].V)+" kW · "+fmtTime(peaks[i].T), i, peakBest)
		perfCells[i] = cell(perfTxt[i], i, perfBest)
		tempCells[i] = cell(fmtInt(lang, float64(metas[i].TempC))+" °C", i, tempBest)
		shareCells[i] = cell(strconv.Itoa(sharePct[i])+" %", i, shareBest)
		statusCells[i] = invTableCell{Color: colors[i], Chip: true}
	}
	v.Rows = []invTableRow{
		row(T(lang, "inversores.powerNow"), powCells),
		row(T(lang, "inversores.energyToday"), energyCells),
		row(T(lang, "inversores.peakDay"), peakCells),
		row("kWh/kWp", perfCells),
		row(T(lang, "inverter.temperature"), tempCells),
		row(T(lang, "inversores.contribution"), shareCells),
		row(T(lang, "inversores.status"), statusCells),
	}

	// Reparto: donut (por defecto) + barra horizontal apilada.
	slices := make([]donutSliceIn, len(metas))
	for i := range metas {
		slices[i] = donutSliceIn{Label: metas[i].Name, Value: kwhs[i], Color: colors[i]}
	}
	v.Share = buildDonut(lang, slices, 3)
	v.Share.CenterSub = T(lang, "inversores.kwhToday")
	for i := range metas {
		v.ShareBars = append(v.ShareBars, invShareBar{
			Name:      metas[i].Name,
			Color:     colors[i],
			WidthText: strconv.Itoa(sharePct[i]) + "%",
			LabelText: metas[i].Name + " " + strconv.Itoa(sharePct[i]) + " %",
		})
	}
	return v
}

func peakVals(peaks []peakAt) []float64 {
	out := make([]float64, len(peaks))
	for i, p := range peaks {
		out[i] = p.V
	}
	return out
}

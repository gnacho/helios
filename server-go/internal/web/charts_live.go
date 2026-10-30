package web

import (
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"

	"helios-server/internal/auth"
	"helios-server/internal/extensions"
	"helios-server/internal/solar"
)

// Refresco de las graficas que crecen durante el dia. El trazo se pinta en el
// servidor al cargar y solo los numeros viajan por SSE, asi que la curva se
// quedaba congelada hasta recargar. Cada contenedor pide su fragmento con
// data-on-interval y Datastar reemplaza su contenido interior: mismo patron
// que el polling de Ajustes.

type chartPatch struct {
	sel   string
	comp  templ.Component
	outer bool
}

func patchCharts(w http.ResponseWriter, r *http.Request, patches []chartPatch) {
	gen := datastar.NewSSE(w, r)
	for _, p := range patches {
		mode := datastar.WithModeInner()
		if p.outer {
			mode = datastar.WithModeOuter()
		}
		if err := gen.PatchElementTempl(p.comp, datastar.WithSelectorID(p.sel), mode); err != nil {
			return
		}
	}
}

// handleChgCurvePatch serves GET /ui/chg-curve?key=: curva del dia de un cargador.
func (u *UI) handleChgCurvePatch(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	lang := u.userLang(userID)
	pal := newPalette(themeFrom(r) == "dark")
	key := r.URL.Query().Get("key")
	curve := chgDayCurveChart(lang, nil, pal.Consumo)
	if u.ExtStore != nil && u.Ext != nil && u.ExtHA != nil {
		if ch := extensions.GetChargerByKey(u.ExtStore.Get(), key); ch != nil {
			c := u.Ext.ChargerDayCurve(u.ExtHA, solar.TodayStr(), ch)
			curve = chgDayCurveChart(lang, c.Points, pal.Consumo)
		}
	}
	patchCharts(w, r, []chartPatch{{sel: "chg-curve-" + key, comp: chgCurveSVG(curve)}})
}

// handleDayChartPatch serves GET /ui/day-chart: curva del dia del panel.
func (u *UI) handleDayChartPatch(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	lang := u.userLang(userID)
	pal := newPalette(themeFrom(r) == "dark")
	m := u.buildDashboard(pal)
	ik, in := chartInvKeys(m)
	chart := BuildDayChart(lang, m.Series.Points, untilMin(m), ik, in)
	patchCharts(w, r, []chartPatch{{sel: "day-chart", comp: dayChartSVGInner(chart)}})
}

// handleBatChartsPatch serves GET /ui/bat-charts: aguja de SOC y graficas del dia.
func (u *UI) handleBatChartsPatch(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	lang := u.userLang(userID)
	pal := newPalette(themeFrom(r) == "dark")
	m := u.buildDashboard(pal)
	view := bateriaView{pageProps: pageProps{Lang: lang}, Palette: pal}
	patchCharts(w, r, []chartPatch{
		{sel: "soc-gauge", comp: socGaugeSVG(view, buildSocGauge(lang, liveSoc(m.Live), pal)), outer: true},
		{sel: "bat-power", comp: batPowerSVG(buildBatPowerChart(lang, m.Series.Points, pal)), outer: true},
		{sel: "soc-day", comp: socDaySVG(buildSocDayChart(lang, m.Series.Points, untilMin(m), pal)), outer: true},
	})
}

// handleInvChartsPatch serves GET /ui/inv-charts?key=: curva del dia de un
// inversor o la comparativa (?key=compare).
func (u *UI) handleInvChartsPatch(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	lang := u.userLang(userID)
	pal := newPalette(themeFrom(r) == "dark")
	m := u.buildDashboard(pal)
	metas := invMetasOf(m.Invs, u.dailyHistory(), time.Now())
	key := r.URL.Query().Get("key")

	if key == "compare" {
		keys, names := invKeysAndNames(m, metas)
		norm := r.URL.Query().Get("norm") == "1"
		cv := buildCompareView(lang, pal, m, metas, keys, names, norm)
		if cv == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		view := inversoresView{pageProps: pageProps{Lang: lang}, Palette: pal}
		patchCharts(w, r, []chartPatch{{sel: "inv-compare", comp: invCompareSVG(view, cv.Chart, norm), outer: true}})
		return
	}

	idx := -1
	for i := range metas {
		if metas[i].Key == key {
			idx = i
			break
		}
	}
	if idx < 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	chart := BuildInverterDayChart(lang, m.Series.Points, key, metas[idx].Name, InverterColor(key, pal, idx), untilMin(m))
	patchCharts(w, r, []chartPatch{{sel: "inv-day-" + key, comp: invDaySVG(chart), outer: true}})
}

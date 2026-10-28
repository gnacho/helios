package web

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/starfederation/datastar-go/datastar"

	"helios-server/internal/auth"
	"helios-server/internal/db"
	"helios-server/internal/extensions"
	"helios-server/internal/install"
	"helios-server/internal/push"
	"helios-server/internal/solar"
	"helios-server/internal/sse"
	"helios-server/internal/version"
)

//go:embed assets
var assetsEmbed embed.FS

// assetVersion is a short hash of the CSS and JS we ship. It travels in the
// query string (?v=...) because the files have a fixed name and are cached:
// without it, a change in the stylesheet could take minutes to show up.
var assetVersion = func() string {
	h := sha256.New()
	for _, name := range []string{"assets/css/app.build.css", "assets/js/gestures.js", "assets/js/datastar.js"} {
		if b, err := assetsEmbed.ReadFile(name); err == nil {
			h.Write(b)
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}()

const (
	themeCookie      = "helios_theme"
	heatMetricCookie = "helios_heat_metric"
	accentCookie     = "helios_accent"
	densityCookie    = "helios_density"
	motionCookie     = "helios_motion"
)

type navItem struct {
	Path string
	Key  string
}

// navItems is the shared page registry; each entry becomes a route in Mount.
var navItems = []navItem{
	{"/", "nav.dashboard"},
	{"/inversores", "nav.inversores"},
	{"/bateria", "nav.bateria"},
	{"/historico", "nav.historico"},
	{"/cargador", "nav.cargador"},
	{"/ajustes", "nav.ajustes"},
}

// pageProps carries everything a page render needs.
type pageProps struct {
	Lang    string
	Theme   string
	Version string
	Path    string
	Title   string
	Error   string
	Next    string

	Username   string
	Collapsed  bool
	ChargerOn  bool
	DateText   string
	InvCount   []string
	BatKwh     float64 // installed battery capacity (0 = no battery)
	Town       string  // install location without the province
	NavDir     string  // "up" | "down": where the selected item comes from
	Alerts     []solar.LiveAlert
	AlertLevel string // "critical" | "warning" | "info" | ""
	AlertCount int
	AssetVer   string // content hash for /assets URLs
	NavShift   int    // px between the previous and current menu item (accent slide)

	// Per-device appearance prefs (cookies, like the React localStorage).
	AccentName   string // naranja|verde|azul|violeta
	AccentRGB    string // "" = brand orange
	Density      string // "comfortable" | "compact"
	ReduceMotion bool
}

// UI serves the templ pages. Root files (favicon, PWA manifest, service
// worker, icons) travel embedded in the binary, so a deployment needs no
// static directory. Ext/ExtHA give the pages access to the charger services
// in process (day curve + history); Prices are the configured energy prices.
// DataDir feeds the update-progress state of the Ajustes page; Push the VAPID
// config.
type UI struct {
	Auth     *auth.Auth
	DB       *db.DB
	Solar    *solar.Solar
	HA       solar.HAReader
	Install  *install.Store
	ExtStore *extensions.Store
	Ext      *extensions.Ext
	ExtHA    extensions.HA
	Prices   solar.PriceConfig
	Hub      *sse.Hub
	DataDir  string
	Push     *push.Service
	assets   http.Handler
}

func New(a *auth.Auth, database *db.DB, sol *solar.Solar, ha solar.HAReader, installStore *install.Store, extStore *extensions.Store, ext *extensions.Ext, extHA extensions.HA, prices solar.PriceConfig, hub *sse.Hub) *UI {
	sub, err := fs.Sub(assetsEmbed, "assets")
	if err != nil {
		panic("web: assets subtree: " + err.Error())
	}
	return &UI{
		Auth:     a,
		DB:       database,
		Solar:    sol,
		HA:       ha,
		Install:  installStore,
		ExtStore: extStore,
		Ext:      ext,
		ExtHA:    extHA,
		Prices:   prices,
		Hub:      hub,
		assets:   http.StripPrefix("/assets/", http.FileServer(http.FS(sub))),
	}
}

// Mount registers the page routes on the API router. JSON routes already
// registered keep working: this only adds HTML routes.
// navLog registra cada peticion de la UI (ruta, estado, duracion, agente y si
// el navegador revalida) cuando HELIOS_NAV_LOG esta activo. Sirve para
// diagnosticar problemas de pintado en un navegador concreto desde el servidor.
func navLog(next http.Handler) http.Handler {
	if os.Getenv("HELIOS_NAV_LOG") == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if r.Method == http.MethodGet {
			ua := r.UserAgent()
			if i := strings.Index(ua, ")"); i > 0 && i < 60 {
				ua = ua[:i+1]
			}
			log.Printf("[nav] %s %d %s mode=%s inm=%t ua=%q",
				r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond),
				r.Header.Get("Sec-Fetch-Mode"), r.Header.Get("If-None-Match") != "", ua)
		}
	})
}

// statusRecorder guarda el codigo de estado para el log de navegacion.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// navPatchExpr is the Datastar expression that navigates without reloading the
// document: it updates the URL, scrolls to the top and asks for the fragments
// (content, page head and menu state). Over the network a full navigation waits
// a round trip with a blank viewport in Firefox (measured: 114 ms TTFB to the
// public domain); patching removes that wait and keeps the shell untouched.
func navPatchExpr(path string) string {
	// El pushState solo si la URL cambia: al volver con atras/adelante la ruta
	// ya es la destino y un pushState ahi romperia la historia del navegador.
	return "if (location.pathname !== '" + path + "') { history.pushState({}, '', '" + path +
		"'); } window.scrollTo(0, 0); @get('/ui/nav?path=" + url.QueryEscape(path) + "')"
}

// pageHandlers maps the navigable paths to the handler that renders the page.
func (u *UI) pageHandler(path string) http.HandlerFunc {
	switch path {
	case "/":
		return u.handleDashboard
	case "/inversores":
		return u.handleInversores
	case "/bateria":
		return u.handleBateria
	case "/historico":
		return u.handleHistorico
	case "/cargador":
		return u.handleCargador
	case "/ajustes":
		return u.handleAjustes
	}
	return nil
}

// fragment returns the HTML between two markers of a rendered page.
func fragment(html, name string) string {
	open := "<!--#" + name + "-->"
	close := "<!--/#" + name + "-->"
	i := strings.Index(html, open)
	if i < 0 {
		return ""
	}
	j := strings.Index(html[i:], close)
	if j < 0 {
		return ""
	}
	return html[i+len(open) : i+j]
}

// handleNavPatch renders the target page and patches only the regions that
// change: the content, the page head and the menu state. The shell (logo,
// sidebar frame, top bar) is never re-created.
func (u *UI) handleNavPatch(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("path")
	handler := u.pageHandler(target)
	if handler == nil {
		http.Error(w, "ruta desconocida", http.StatusBadRequest)
		return
	}
	// La pagina se renderiza con su propia ruta: de ahi salen el titulo y el
	// elemento activo del menu.
	r2 := r.Clone(r.Context())
	r2.URL.Path = target
	r2.URL.RawQuery = ""
	rec := httptest.NewRecorder()
	handler(rec, r2)
	if rec.Code != http.StatusOK {
		http.Error(w, "error al renderizar", http.StatusInternalServerError)
		return
	}
	html := rec.Body.String()

	gen := datastar.NewSSE(w, r)
	for _, region := range []string{"page", "page-head", "nav-primary", "nav-rail", "nav-bottom"} {
		body := fragment(html, region)
		if body == "" {
			continue
		}
		if err := gen.PatchElementTempl(templ.Raw(body), datastar.WithSelectorID(region), datastar.WithModeInner()); err != nil {
			return
		}
	}
}

// patchPanel re-renders a page with its own query and patches one marked
// region of it: the shell and everything that does not change stay untouched.
func (u *UI) patchPanel(w http.ResponseWriter, r *http.Request, path, marker string, handler http.HandlerFunc) {
	r2 := r.Clone(r.Context())
	r2.URL.Path = path
	rec := httptest.NewRecorder()
	handler(rec, r2)
	if rec.Code != http.StatusOK {
		http.Error(w, "error al renderizar", http.StatusInternalServerError)
		return
	}
	body := fragment(rec.Body.String(), marker)
	if body == "" {
		http.Error(w, "panel desconocido", http.StatusInternalServerError)
		return
	}
	gen := datastar.NewSSE(w, r)
	_ = gen.PatchElementTempl(templ.Raw(body), datastar.WithSelectorID(marker), datastar.WithModeInner())
}

// handleChgPatch swaps the active charger without reloading the page.
func (u *UI) handleChgPatch(w http.ResponseWriter, r *http.Request) {
	u.patchPanel(w, r, "/cargador", "chg-body", u.handleCargador)
}

// handleInvPatch swaps the active inverter (or the comparison) in place.
func (u *UI) handleInvPatch(w http.ResponseWriter, r *http.Request) {
	u.patchPanel(w, r, "/inversores", "inv-body", u.handleInversores)
}

// pagerStyle places the mobile track on the i-th panel without JS: the panels
// are w-full, so translating -i*100% of the track width lands on panel i.
func pagerStyle(i int) string {
	if i <= 0 {
		return "transform: translate3d(0, 0, 0)"
	}
	return "transform: translate3d(-" + strconv.Itoa(i*100) + "%, 0, 0)"
}

// clickTab is the click expression of the "clone" tabs: it updates the URL and
// patches the panel instead of navigating. Enter on a link fires a click with
// detail 0, so keyboard activation keeps the plain navigation.
func clickTab(url, patch string) string {
	return "if (evt.detail === 0) { location.href = '" + jsQuote(url) + "'; return; } " +
		"history.pushState({}, '', '" + jsQuote(url) + "'); " + patch
}

// panelSwapClick swaps a page panel in place: the browser URL is pushed and the
// panel endpoint (which reads the same query) is patched. Links keep their href.
func panelSwapClick(pageURL, endpoint string) string {
	q := pageURL
	if i := strings.IndexByte(q, '#'); i >= 0 {
		q = q[:i]
	}
	if i := strings.IndexByte(q, '?'); i >= 0 {
		q = q[i:]
	} else {
		q = ""
	}
	return clickTab(pageURL, "@get('"+jsQuote(endpoint)+q+"')")
}

// chgPeriodSwapClick changes the charger history period without reloading.
// On a phone the swipe changes the charger without a render, so the tab is read
// from the address bar at click time: the href still carries the one that was
// active when the page was painted.
func chgPeriodSwapClick(u string) string {
	pu, err := url.Parse(u)
	if err != nil {
		return panelSwapClick(u, "/ui/chg")
	}
	q := pu.Query()
	q.Del("tab")
	prefix := ""
	if enc := q.Encode(); enc != "" {
		prefix = enc + "&"
	}
	prefix += "tab="
	return "var tab = new URLSearchParams(location.search).get('tab') || '';" +
		" history.pushState({}, '', '" + jsQuote(pu.Path+"?"+prefix) + "' + tab);" +
		" @get('/ui/chg?" + jsQuote(prefix) + "' + tab)"
}

func chgTabSwapClick(key string) string {
	return clickTab("/cargador?tab="+url.QueryEscape(key), "@get('/ui/chg?tab="+jsQuote(key)+"')")
}

func invTabSwapClick(key string) string {
	return clickTab("/inversores?inv="+url.QueryEscape(key), "@get('/ui/inv?inv="+jsQuote(key)+"')")
}

func noStorePrivate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (u *UI) Mount(parent chi.Router) {
	parent.Group(func(r chi.Router) {
		r.Use(noStorePrivate)
		u.mount(r)
	})
}

func (u *UI) mount(r chi.Router) {
	r.Get("/assets/*", navLog(http.HandlerFunc(u.handleAssets)).ServeHTTP)

	// Root files come from the embed too: the binary is self-contained.
	for _, name := range []string{
		"/favicon.svg", "/favicon-32.png", "/apple-touch-icon.png",
		"/pwa-icon-192.png", "/pwa-icon-512.png", "/manifest.webmanifest",
		"/sw.js", "/empty-solar.svg", "/offline.html",
	} {
		r.Get(name, u.handleRootFile)
	}

	r.Get("/login", u.handleLoginPage)
	r.Post("/login", u.handleLoginSubmit)
	r.Post("/logout", u.handleLogout)
	r.Post("/theme", u.handleTheme)
	r.Post("/appearance", u.handleAppearance)
	r.Post("/sidebar", u.handleSidebarToggle)

	r.Group(func(r chi.Router) {
		r.Use(u.Auth.RequireAuthRedirect)
		r.Get("/", u.handleDashboard)
		r.Get("/ui/stream", u.handleStream)
		r.Get("/ui/historico", u.handleHistoricoPatch)
		r.Get("/ui/chg", u.handleChgPatch)
		r.Get("/ui/inv", u.handleInvPatch)
		r.Get("/ui/heat", u.handleHeatMetric)
		r.Get("/ui/heat-day", u.handleHeatDay)
		r.Post("/ui/alerts/dismiss", u.handleAlertDismiss)
		r.Get("/ui/nav", navLog(http.HandlerFunc(u.handleNavPatch)).ServeHTTP)
		r.Get("/ui/municipios", u.handleMunicipios)

		r.Get("/inversores", u.handleInversores)
		r.Get("/bateria", u.handleBateria)
		r.Get("/historico", u.handleHistorico)
		r.Get("/cargador", u.handleCargador)
		r.Post("/cargador/charger-image", u.handleChargerImageUpload)
		r.Post("/ui/charger/action", u.handleChargerAction)
		r.Get("/ajustes", u.handleAjustes)

		// Ajustes form wrappers (PRG): llaman a los mismos servicios en
		// proceso que los endpoints JSON (ver ajustes.go).
		r.Post("/ajustes/topology", u.handleAjustesTopology)
		r.Post("/ajustes/prices", u.handleAjustesPrices)
		r.Post("/ajustes/install", u.handleAjustesInstall)
		r.Post("/ajustes/extensions", u.handleAjustesExtensions)
		r.Post("/ajustes/push-pref", u.handleAjustesPushPref)
		r.Post("/ajustes/profile", u.handleAjustesProfile)
		r.Post("/ajustes/password", u.handleAjustesPassword)
		r.Post("/ajustes/users", u.handleAjustesUserCreate)
		r.Post("/ajustes/users/{id}", u.handleAjustesUserSave)
		r.Post("/ajustes/users/{id}/delete", u.handleAjustesUserDelete)

		for _, item := range navItems[1:] {
			if item.Path == "/inversores" || item.Path == "/bateria" ||
				item.Path == "/historico" || item.Path == "/cargador" ||
				item.Path == "/ajustes" {
				continue // real handlers above
			}
			r.Get(item.Path, u.stubHandler(item))
		}
	})
}

// handleDashboard renders the dashboard (chunk 1: live strip + KPI row).
func (u *UI) handleDashboard(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	lang := u.userLang(userID)
	p := u.baseProps(r, userID, "/", "")
	p.Title = T(lang, "nav.dashboard")

	pal := newPalette(p.Theme == "dark")
	m := u.buildDashboard(pal)
	if k, ok := u.exactDayKpis(m); ok {
		m.Kpis = k
	}

	signals, err := json.Marshal(dashboardSignals(lang, m))
	if err != nil {
		signals = []byte("{}")
	}

	invKeys, invNames := chartInvKeys(m)
	flow := buildFlow(lang, m.Live)
	sunrise, sunset := u.configuredSunTimes(m.Live)
	_ = sunrise
	_ = sunset
	d := dashboardView{
		pageProps:       p,
		Signals:         string(signals),
		Palette:         pal,
		Chart:           BuildDayChart(lang, m.Series.Points, untilMin(m), invKeys, invNames),
		AtText:          "--:--",
		GridKwText:      "—",
		GridStatus:      "—",
		GridColor:       pal.RedCompra,
		KProduction:     fmtEnergy(lang, m.Kpis.ProductionKwh),
		KConsumption:    fmtEnergy(lang, m.Kpis.ConsumptionKwh),
		KBatteryKwh:     fmtEnergy(lang, m.Kpis.Soc/100*5),
		KSocSub:         socSub(lang, m),
		KAutoconsumo:    fmtInt(lang, m.Kpis.AutoconsumoPct),
		KGridBalance:    fmtEnergy(lang, m.Kpis.GridExportKwh),
		KGridExport:     fmtEnergy(lang, m.Kpis.GridExportKwh),
		KGridImport:     fmtEnergy(lang, m.Kpis.GridImportKwh),
		AutoconsumoPct:  m.Kpis.AutoconsumoPct,
		KProdKwh:        m.Kpis.ProductionKwh,
		KConsKwh:        m.Kpis.ConsumptionKwh,
		KBatKwh:         m.Kpis.Soc / 100 * 5,
		KGridBalanceKwh: m.Kpis.GridExportKwh,
		KGridExportKwh:  m.Kpis.GridExportKwh,
		KGridImportKwh:  m.Kpis.GridImportKwh,
		Flow:            flow,
		Arc:             buildArc(lang, m.Live, m.Kpis, sunrise, sunset),
		Invs:            buildInvCards(lang, m, m.Series.Points, pal),
	}
	if m.Live != nil {
		d.LiveProdKw = m.Live.Production
		d.LiveConsKw = derefKw(m.Live.Consumption)
		d.LiveBatKw = abs(m.Live.BatteryPower)
		if m.Live.Grid != nil {
			d.GridKw = abs(*m.Live.Grid)
		}
		exporting := m.Live.Grid != nil && *m.Live.Grid < -0.05
		gridStatus := T(lang, "live.balanced")
		if m.Live.Grid == nil {
			gridStatus = "—"
		} else if *m.Live.Grid > 0.05 {
			gridStatus = T(lang, "live.buying")
		} else if *m.Live.Grid < -0.05 {
			gridStatus = T(lang, "live.exporting")
		}
		gridColor := pal.RedCompra
		if exporting {
			gridColor = pal.RedVertido
		}
		prodIconColor := pal.Solar
		if m.Live.Production <= 0.1 {
			prodIconColor = ""
		}
		d.AtText = fmtTime(m.Live.At)
		d.Soc = m.Live.Soc
		d.Charging = m.Live.BatteryPower > 0.05
		d.ProdIconColor = prodIconColor
		d.GridKwText = fmtKw(lang, absPtr(m.Live.Grid))
		d.GridStatus = gridStatus
		d.GridColor = gridColor
		d.GridExporting = exporting
		d.ProductionText = fmtKwVal(lang, m.Live.Production)
		d.ConsumptionText = fmtKw(lang, m.Live.Consumption)
		d.BatteryKwText = fmtKwVal(lang, abs(m.Live.BatteryPower))
		d.BatteryStatus = T(lang, "common."+batteryState(m.Live.BatteryPower))
	}

	_ = DashboardPage(d).Render(r.Context(), w)
}

func socSub(lang string, m dashboardModel) string {
	if m.Live == nil {
		return ""
	}
	return fmtInt(lang, m.Live.Soc) + "% · " + T(lang, "common."+batteryState(m.Live.BatteryPower))
}

// userLang resolves the page language from the user profile.
func (u *UI) userLang(userID string) string {
	if userID != "" {
		if usr, _ := u.DB.GetUserByID(userID); usr != nil && usr.Language != "" {
			return usr.Language
		}
	}
	return "es"
}

// signalLang is userLang for the stream handler (auth guaranteed).
func (u *UI) signalLang(r *http.Request) string {
	id, _ := auth.UserIDFrom(r.Context())
	return u.userLang(id)
}

// NotFound is the router fallback: an unknown path is a page inside the app
// (redirect to login) or a 404 with the shell, never a SPA fallback.
func (u *UI) NotFound(w http.ResponseWriter, r *http.Request) {
	sess := u.Auth.SessionIDFromCookie(r)
	if sess == nil {
		http.Redirect(w, r, loginURL(r.URL.RequestURI()), http.StatusFound)
		return
	}
	p := u.baseProps(r, sess.UserID, r.URL.Path, "")
	p.Title = T(p.Lang, "stub.title")
	w.WriteHeader(http.StatusNotFound)
	_ = NotFoundPage(p).Render(r.Context(), w)
}

// handleRootFile serves an embedded root file. The service worker must
// revalidate (otherwise a stale one keeps controlling the tab); the icons and
// the favicon can be cached for a day.
func (u *UI) handleRootFile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "." || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(assetsEmbed, "assets/root/"+name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if name == "sw.js" {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

func (u *UI) handleAssets(w http.ResponseWriter, r *http.Request) {
	// A versioned URL points at immutable content; the bare one revalidates.
	if r.URL.Query().Get("v") != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	u.assets.ServeHTTP(w, r)
}

func (u *UI) stubHandler(item navItem) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := u.Auth.SessionIDFromCookie(r)
		if sess == nil {
			http.Redirect(w, r, loginURL(r.URL.RequestURI()), http.StatusFound)
			return
		}
		p := u.baseProps(r, sess.UserID, item.Path, "")
		p.Title = T(p.Lang, item.Key)
		_ = StubPage(p).Render(r.Context(), w)
	}
}

func (u *UI) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if u.Auth.SessionIDFromCookie(r) != nil {
		http.Redirect(w, r, safeNext(r.URL.Query().Get("next")), http.StatusFound)
		return
	}
	u.renderLogin(w, r, loginRender{status: http.StatusOK})
}

type loginRender struct {
	status   int
	errorKey string
}

func (u *UI) renderLogin(w http.ResponseWriter, r *http.Request, lr loginRender) {
	p := u.baseProps(r, "", "/login", "es")
	p.Theme = themeFrom(r)
	p.Title = T(p.Lang, "login.title")
	p.Next = safeNext(r.URL.Query().Get("next"))
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		p.Next = safeNext(r.PostFormValue("next"))
	}
	if lr.errorKey != "" {
		p.Error = T(p.Lang, lr.errorKey)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(lr.status)
	_ = LoginPage(p).Render(r.Context(), w)
}

func (u *UI) handleLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if u.Auth.LoginRateLimited(r) {
		u.renderLogin(w, r, loginRender{status: http.StatusTooManyRequests, errorKey: "login.rateLimited"})
		return
	}
	if err := r.ParseForm(); err != nil {
		u.renderLogin(w, r, loginRender{status: http.StatusBadRequest, errorKey: "login.error"})
		return
	}
	username := r.PostFormValue("username")
	password := r.PostFormValue("password")
	next := safeNext(r.PostFormValue("next"))

	user, err := u.Auth.HandleLogin(w, r, username, password)
	if err != nil || user == nil {
		_ = u.Auth.RegisterLoginFail(r)
		http.Redirect(w, r, "/login?err=1&next="+url.QueryEscape(next), http.StatusSeeOther)
		return
	}
	_ = u.Auth.LoginOk(r)
	u.DB.Audit(user.Username, user.ID, "auth.login", nil)
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (u *UI) handleLogout(w http.ResponseWriter, r *http.Request) {
	if session := u.Auth.SessionIDFromCookie(r); session != nil {
		if usr, _ := u.DB.GetUserByID(session.UserID); usr != nil {
			u.DB.Audit(usr.Username, usr.ID, "auth.logout", nil)
		}
	}
	_ = u.Auth.HandleLogout(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (u *UI) handleTheme(w http.ResponseWriter, r *http.Request) {
	theme := r.PostFormValue("theme")
	if theme != "light" && theme != "dark" && theme != "auto" {
		theme = "auto"
	}
	next := safeNext(r.PostFormValue("next"))
	if theme == "auto" {
		// Sin override: borrar la cookie para volver a la preferencia del SO.
		http.SetCookie(w, &http.Cookie{Name: themeCookie, Value: "", Path: "/", MaxAge: -1, SameSite: http.SameSiteLaxMode})
	} else {
		http.SetCookie(w, &http.Cookie{Name: themeCookie, Value: theme, Path: "/", MaxAge: 31536000, SameSite: http.SameSiteLaxMode})
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// accentRGB maps the accent cookie to the --accent-rgb triplet ("" keeps the
// default brand orange, exactly like the React ACCENTS list).
func accentRGB(name string) string {
	switch name {
	case "verde":
		return "34 197 94"
	case "azul":
		return "59 130 246"
	case "violeta":
		return "139 92 246"
	}
	return ""
}

// appearanceFrom reads the per-device appearance cookies (React stored the
// same knobs in localStorage).
func appearanceFrom(r *http.Request) (name, rgb, density string, reduceMotion bool) {
	name, density = "naranja", "comfortable"
	if c, err := r.Cookie(accentCookie); err == nil {
		if v := accentRGB(c.Value); v != "" || c.Value == "naranja" {
			name, rgb = c.Value, v
		}
	}
	if c, err := r.Cookie(densityCookie); err == nil && c.Value == "compact" {
		density = "compact"
	}
	if c, err := r.Cookie(motionCookie); err == nil && c.Value == "off" {
		reduceMotion = true
	}
	return name, rgb, density, reduceMotion
}

// handleAppearance persists the appearance knobs as cookies (per device).
func (u *UI) handleAppearance(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r.PostFormValue("next"))
	set := func(name, value string) {
		http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: 31536000, SameSite: http.SameSiteLaxMode})
	}
	switch r.PostFormValue("accent") {
	case "naranja":
		set(accentCookie, "naranja")
	case "verde", "azul", "violeta":
		set(accentCookie, r.PostFormValue("accent"))
	}
	switch r.PostFormValue("density") {
	case "comfortable", "compact":
		set(densityCookie, r.PostFormValue("density"))
	}
	switch r.PostFormValue("motion") {
	case "on":
		set(motionCookie, "on")
	case "off":
		set(motionCookie, "off")
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// rootStyle renders the html style attr for density + accent overrides.
// El fondo del primer pintado va en un <style> del head (ver layout.templ):
// aqui no se puede expresar prefers-color-scheme.
func rootStyle(p pageProps) string {
	var b strings.Builder
	if p.Density == "compact" {
		b.WriteString("font-size:13.5px;")
	}
	if p.AccentRGB != "" {
		b.WriteString("--accent-rgb:" + p.AccentRGB + ";")
	}
	return b.String()
}

// dayKpisCache keeps the exact day counters (same source as the JSON API) for
// a minute so page renders do not hammer the HA statistics API.
var dayKpisCache struct {
	mu   sync.Mutex
	at   time.Time
	date string
	k    DayKpis
	ok   bool
}

// exactDayKpis prefers the day KPI counters (daily row / HA statistics), which
// is exactly what the React app gets from /api/solar/kpis, so both UIs show
// the same numbers. Falls back to the integrated series without HA.
func (u *UI) exactDayKpis(m dashboardModel) (DayKpis, bool) {
	if u.Solar == nil || u.HA == nil {
		return DayKpis{}, false
	}
	date := solar.TodayStr()
	dayKpisCache.mu.Lock()
	if dayKpisCache.at.After(time.Now().Add(-time.Minute)) && dayKpisCache.date == date && dayKpisCache.ok {
		k := dayKpisCache.k
		dayKpisCache.mu.Unlock()
		k.Soc = m.Kpis.Soc
		return k, true
	}
	dayKpisCache.mu.Unlock()

	sk, err := u.Solar.GetKpis(u.HA, date, u.Prices)
	if err != nil {
		return DayKpis{}, false
	}
	k := DayKpis{
		ProductionKwh:        sk.ProductionKwh,
		ConsumptionKwh:       sk.ConsumptionKwh,
		GridImportKwh:        sk.GridImportKwh,
		GridExportKwh:        sk.GridExportKwh,
		BatteryChargedKwh:    sk.BatteryChargedKwh,
		BatteryDischargedKwh: sk.BatteryDischargedKwh,
		Soc:                  m.Kpis.Soc,
		AutoconsumoPct:       sk.AutoconsumoPct,
		AutosuficienciaPct:   sk.AutosuficienciaPct,
		PeakProductionKw:     sk.PeakProductionKw,
		PeakAt:               sk.PeakAt,
	}
	dayKpisCache.mu.Lock()
	dayKpisCache.at, dayKpisCache.date, dayKpisCache.k, dayKpisCache.ok = time.Now(), date, k, true
	dayKpisCache.mu.Unlock()
	return k, true
}

// baseProps builds the common props: lang from the user profile, theme from
// the cookie, sidebar state from its cookie, user chip data and the date.
func (u *UI) baseProps(r *http.Request, userID, path, lang string) pageProps {
	p := pageProps{
		Lang:    normalizeLang(lang),
		Theme:   themeFrom(r),
		Version: version.Version,
		Path:    path,
	}
	p.AccentName, p.AccentRGB, p.Density, p.ReduceMotion = appearanceFrom(r)
	if c, err := r.Cookie("helios_sidebar"); err == nil && c.Value == "1" {
		p.Collapsed = true
	}
	if userID != "" {
		if usr, _ := u.DB.GetUserByID(userID); usr != nil {
			if usr.Language != "" {
				p.Lang = normalizeLang(usr.Language)
			}
			p.Username = usr.Username
		}
	}
	if u.Install != nil {
		inst := u.Install.Get()
		for _, inv := range inst.Inverters {
			p.InvCount = append(p.InvCount, inv.Key)
		}
		if inst.Battery.Enabled {
			p.BatKwh = inst.Battery.CapacityKwh
		}
	}
	if u.ExtStore != nil {
		cfg := u.ExtStore.Get()
		p.ChargerOn = cfg.Enabled && cfg.CarCharger.Enabled
	}
	p.Alerts, p.AlertLevel = u.liveAlerts(userID)
	p.AlertCount = len(p.Alerts)
	if loc, _ := u.townFromKV(); loc != "" {
		p.Town = loc
	}
	p.AssetVer = assetVersion
	p.DateText = weekdayDate(p.Lang, time.Now())
	p.NavDir, p.NavShift = navDirFrom(p, r)
	return p
}

// townFromKV is the configured location without the "(Provincia de ...)" part.
func (u *UI) townFromKV() (string, string) {
	if u.DB == nil {
		return "", ""
	}
	loc, _ := u.DB.KVGet("installLocation")
	if loc == "" {
		return "", ""
	}
	if i := strings.Index(loc, " ("); i > 0 {
		return loc[:i], loc
	}
	return loc, loc
}

// configuredSunTimes prefers the configured location (Ajustes -> Ubicación,
// calculated with SunCalc like the React app) and falls back to Home
// Assistant's sun entity.
func (u *UI) configuredSunTimes(live *solar.LiveData) (int, int) {
	if u.DB != nil {
		latS, _ := u.DB.KVGet("installLat")
		lonS, _ := u.DB.KVGet("installLon")
		if latS != "" && lonS != "" {
			lat, e1 := strconv.ParseFloat(latS, 64)
			lon, e2 := strconv.ParseFloat(lonS, 64)
			if e1 == nil && e2 == nil {
				if rise, set, ok := SunTimes(time.Now(), lat, lon); ok {
					return rise, set
				}
			}
		}
	}
	return sunMinutes(live)
}

// installSunTimes returns the sunrise/sunset for Ajustes: configured location
// when set, otherwise Home Assistant's cached sun times.
func (u *UI) installSunTimes() (int, int) {
	if u.DB != nil {
		latS, _ := u.DB.KVGet("installLat")
		lonS, _ := u.DB.KVGet("installLon")
		if latS != "" && lonS != "" {
			lat, e1 := strconv.ParseFloat(latS, 64)
			lon, e2 := strconv.ParseFloat(lonS, 64)
			if e1 == nil && e2 == nil {
				if rise, set, ok := SunTimes(time.Now(), lat, lon); ok {
					return rise, set
				}
			}
		}
	}
	if u.Solar == nil || u.HA == nil {
		return 6*60 + 50, 21*60 + 40
	}
	return u.sunTimesCached()
}

// installSunNote is the "Sol hoy en X" line under the location field.
func (u *UI) installSunNote(lang, location string) string {
	if u.Solar == nil || u.HA == nil {
		return ""
	}
	sr, ss := u.installSunTimes()
	if location != "" {
		return T(lang, "ajustes.install.sunTodayAt", map[string]string{
			"location": location,
			"sunrise":  fmtTime(sr),
			"sunset":   fmtTime(ss),
		})
	}
	return T(lang, "ajustes.install.sunTodayHa", map[string]string{
		"sunrise": fmtTime(sr),
		"sunset":  fmtTime(ss),
	})
}

// alertsSnapshot returns the live alerts (same list the React bell shows) with
// a short cache so every page render does not hit HA again.
var alertsCache struct {
	mu              sync.Mutex
	at              time.Time
	alerts          []solar.LiveAlert
	level           string
	sunrise, sunset int
}

// sunTimesCached returns the sunrise/sunset minutes from the same cached live
// snapshot the alerts use (15 s).
func (u *UI) sunTimesCached() (int, int) {
	u.alertsSnapshot()
	alertsCache.mu.Lock()
	defer alertsCache.mu.Unlock()
	return alertsCache.sunrise, alertsCache.sunset
}

func (u *UI) alertsSnapshot() ([]solar.LiveAlert, string) {
	if u.Solar == nil || u.HA == nil {
		return nil, ""
	}
	alertsCache.mu.Lock()
	if alertsCache.at.After(time.Now().Add(-15 * time.Second)) {
		a, l := alertsCache.alerts, alertsCache.level
		alertsCache.mu.Unlock()
		return a, l
	}
	alertsCache.mu.Unlock()

	live := u.Solar.ComputeLive(u.HA)
	sunrise, sunset := sunMinutes(&live)
	level := ""
	for _, a := range live.Alerts {
		switch a.Severity {
		case "critical":
			level = "critical"
		case "warning":
			if level != "critical" {
				level = "warning"
			}
		default:
			if level == "" {
				level = "info"
			}
		}
	}
	alertsCache.mu.Lock()
	alertsCache.at, alertsCache.alerts, alertsCache.level = time.Now(), live.Alerts, level
	alertsCache.sunrise, alertsCache.sunset = sunrise, sunset
	alertsCache.mu.Unlock()
	return live.Alerts, level
}

// navDirFrom compares the previous page (Referer) with the current one in the
// nav order, so the active pill can slide in from the right side.
func navDirFrom(p pageProps, r *http.Request) (dir string, shift int) {
	ref := r.Referer()
	if ref == "" {
		return "", 0
	}
	u, err := url.Parse(ref)
	if err != nil || u.Host != r.Host {
		return "", 0
	}
	idx := func(path string) int {
		for i, item := range navEntriesFor(p.ChargerOn) {
			if navIsActive(path, item.Path) {
				return i
			}
		}
		return -1
	}
	prev, cur := idx(u.Path), idx(p.Path)
	if prev < 0 || cur < 0 || prev == cur {
		return "", 0
	}
	// 44 px = alto del item (h-10) + separación del menú.
	shift = (prev - cur) * 44
	if prev > cur {
		return "up", shift
	}
	return "down", shift
}

// navShiftStyle feeds the accent bar animation: it slides in from where the
// previously selected item was.
func navShiftStyle(p pageProps) string {
	if p.NavShift == 0 {
		return ""
	}
	return "--nav-shift: " + strconv.Itoa(p.NavShift) + "px"
}

// weekdayDate mirrors fmtWeekdayDate in the front (capitalized, localized).
func weekdayDate(lang string, t time.Time) string {
	esDays := []string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}
	esMonths := []string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
	enDays := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	enMonths := []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	zhDays := []string{"日", "一", "二", "三", "四", "五", "六"}
	d, m := t.Weekday(), int(t.Month())
	switch normalizeLang(lang) {
	case "es":
		s := esDays[d] + ", " + strconv.Itoa(t.Day()) + " de " + esMonths[m-1]
		return strings.ToUpper(s[:1]) + s[1:]
	case "zh-CN":
		return strconv.Itoa(t.Day()) + "月" + strconv.Itoa(m) + "日星期" + zhDays[d]
	default:
		return enDays[d] + ", " + enMonths[m-1] + " " + strconv.Itoa(t.Day())
	}
}

// handleSidebarToggle flips the helios_sidebar cookie and returns to the page.
func (u *UI) handleSidebarToggle(w http.ResponseWriter, r *http.Request) {
	collapsed := false
	if c, err := r.Cookie("helios_sidebar"); err == nil && c.Value == "1" {
		collapsed = true
	}
	next := safeNext(r.PostFormValue("next"))
	value := "1"
	maxAge := 31536000
	if collapsed {
		value = "0"
	}
	http.SetCookie(w, &http.Cookie{
		Name: "helios_sidebar", Value: value, Path: "/",
		MaxAge: maxAge, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func loginURL(next string) string {
	n := safeNext(next)
	if n == "/" {
		return "/login"
	}
	return "/login?next=" + url.QueryEscape(n)
}

// safeNext only allows local paths (no open redirects). The browser decodes
// percent escapes and treats backslashes and control characters as separators,
// so "/%09/evil.com" or "/\\evil.com" can become "//evil.com". Validate both
// the original and decoded forms before returning the original path.
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return "/"
	}
	decoded, err := url.PathUnescape(next)
	if err != nil || strings.HasPrefix(decoded, "//") || strings.Contains(decoded, `\`) {
		return "/"
	}
	for _, r := range decoded {
		if r < 0x20 || r == 0x7f {
			return "/"
		}
	}
	u, err := url.Parse(next)
	if err != nil || u.IsAbs() || u.Host != "" || u.User != nil {
		return "/"
	}
	return next
}

// themeFrom returns "dark"/"light" when the cookie pins an explicit choice,
// or "" for auto (no cookie: the CSS media variant follows the OS).
func themeFrom(r *http.Request) string {
	if c, err := r.Cookie(themeCookie); err == nil && (c.Value == "dark" || c.Value == "light") {
		return c.Value
	}
	return ""
}

// themeNext is the toggle cycle: auto -> light -> dark -> auto.
func themeNext(theme string) string {
	switch theme {
	case "light":
		return "dark"
	case "dark":
		return "auto"
	}
	return "light"
}

func themeStateName(theme string) string {
	switch theme {
	case "light":
		return "light"
	case "dark":
		return "dark"
	}
	return "auto"
}

// handleMunicipios patches the location datalist with the matches for ?q.
func (u *UI) handleMunicipios(w http.ResponseWriter, r *http.Request) {
	items := searchMunicipios(r.URL.Query().Get("q"), 8)
	gen := datastar.NewSSE(w, r)
	_ = gen.PatchElementTempl(municipiosPanel(items), datastar.WithSelectorID("municipios-panel"), datastar.WithModeInner())
}

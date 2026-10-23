package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

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

const themeCookie = "helios_theme"

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

	Username  string
	Collapsed bool
	ChargerOn bool
	DateText  string
	InvCount  []string
}

// UI serves the templ pages. RootFiles lets the host serve favicon-like
// files from the SPA static dir (no index.html fallback) before the 404.
// Ext/ExtHA give the pages access to the charger services in process (day
// curve + history); Prices are the configured energy prices. DataDir feeds
// the update-progress state of the Ajustes page; Push the VAPID config.
type UI struct {
	Auth      *auth.Auth
	DB        *db.DB
	Solar     *solar.Solar
	HA        solar.HAReader
	Install   *install.Store
	ExtStore  *extensions.Store
	Ext       *extensions.Ext
	ExtHA     extensions.HA
	Prices    solar.PriceConfig
	Hub       *sse.Hub
	RootFiles func(w http.ResponseWriter, r *http.Request) bool
	DataDir   string
	Push      *push.Service
	assets    http.Handler
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
func (u *UI) Mount(r chi.Router) {
	r.Get("/assets/*", u.handleAssets)

	r.Get("/login", u.handleLoginPage)
	r.Post("/login", u.handleLoginSubmit)
	r.Post("/logout", u.handleLogout)
	r.Post("/theme", u.handleTheme)
	r.Post("/sidebar", u.handleSidebarToggle)

	r.Group(func(r chi.Router) {
		r.Use(u.Auth.RequireAuthRedirect)
		r.Get("/", u.handleDashboard)
		r.Get("/ui/stream", u.handleStream)
		r.Get("/inversores", u.handleInversores)
		r.Get("/bateria", u.handleBateria)
		r.Get("/historico", u.handleHistorico)
		r.Get("/cargador", u.handleCargador)
		r.Get("/ajustes", u.handleAjustes)

		// Ajustes form wrappers (PRG): llaman a los mismos servicios en
		// proceso que los endpoints JSON (ver ajustes.go).
		r.Post("/ajustes/topology", u.handleAjustesTopology)
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

	signals, err := json.Marshal(dashboardSignals(lang, m))
	if err != nil {
		signals = []byte("{}")
	}

	invKeys, invNames := chartInvKeys(m)
	flow := buildFlow(lang, m.Live)
	sunrise, sunset := sunMinutes(m.Live)
	_ = sunrise
	_ = sunset
	d := dashboardView{
		pageProps:      p,
		Signals:        string(signals),
		Palette:        pal,
		Chart:          BuildDayChart(lang, m.Series.Points, untilMin(m), invKeys, invNames),
		AtText:         "--:--",
		GridKwText:     "—",
		GridStatus:     "—",
		GridColor:      pal.RedCompra,
		KProduction:    fmtEnergy(lang, m.Kpis.ProductionKwh),
		KConsumption:   fmtEnergy(lang, m.Kpis.ConsumptionKwh),
		KBatteryKwh:    fmtEnergy(lang, m.Kpis.Soc/100*5),
		KSocSub:        socSub(lang, m),
		KAutoconsumo:   fmtInt(lang, m.Kpis.AutoconsumoPct),
		KGridBalance:   fmtEnergy(lang, m.Kpis.GridExportKwh),
		KGridExport:    fmtEnergy(lang, m.Kpis.GridExportKwh),
		KGridImport:    fmtEnergy(lang, m.Kpis.GridImportKwh),
		AutoconsumoPct: m.Kpis.AutoconsumoPct,
		Flow:           flow,
		Arc:            buildArc(lang, m.Live, m.Kpis),
		Invs:           buildInvCards(lang, m, m.Series.Points, pal),
	}
	if m.Live != nil {
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

// NotFound is the router fallback in templ UI mode: root static files
// (favicon, manifest) first, then redirect-or-404 for pages.
func (u *UI) NotFound(w http.ResponseWriter, r *http.Request) {
	if u.RootFiles != nil && u.RootFiles(w, r) {
		return
	}
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

func (u *UI) handleAssets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=300")
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

// baseProps builds the common props: lang from the user profile, theme from
// the cookie, sidebar state from its cookie, user chip data and the date.
func (u *UI) baseProps(r *http.Request, userID, path, lang string) pageProps {
	p := pageProps{
		Lang:    normalizeLang(lang),
		Theme:   themeFrom(r),
		Version: version.Version,
		Path:    path,
	}
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
		for _, inv := range u.Install.Get().Inverters {
			p.InvCount = append(p.InvCount, inv.Key)
		}
	}
	if u.ExtStore != nil {
		cfg := u.ExtStore.Get()
		p.ChargerOn = cfg.Enabled && cfg.CarCharger.Enabled
	}
	p.DateText = weekdayDate(p.Lang, time.Now())
	return p
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

// safeNext only allows local paths (no open redirects).
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
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

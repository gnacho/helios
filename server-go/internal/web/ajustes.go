package web

import (
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/go-chi/chi/v5"

	"github.com/starfederation/datastar-go/datastar"

	"helios-server/internal/auth"
	"helios-server/internal/db"
	"helios-server/internal/extensions"
	"helios-server/internal/install"
	"helios-server/internal/push"
	"helios-server/internal/update"
	"helios-server/internal/version"
)

// processStart feeds the "Acerca de" uptime (same as the API's system info).
var processStart = time.Now()

// /ajustes page (port of Ajustes.tsx + TopologyEditor.tsx, phases 5).
//
// Data flows (same services in process as the JSON API - the API itself is
// untouched):
//   - Topology editor: standard HTML form POST to /ajustes/topology. The
//     wrapper validates with install.Normalize (the exact validation of PUT
//     /api/config), merges the "topology" key into the existing install_config
//     KV and redirects (PRG). Inline errors come back via ?err=topology.
//   - Extensions/BYD config: form POST to /ajustes/extensions, validated with
//     extensions.NormalizeConfig against the same GenericExtensions profile as
//     PUT /api/extensions.
//   - BYD actions: Datastar @post to the EXISTING /api/byd/action (same
//     whitelist) like the charger page actions.
//   - Update: check button + Datastar @get('/api/update/status'); the "update
//     now" flow posts to the EXISTING /api/update/apply and polls
//     /api/update/progress with data-on-interval (the raw SSE stream has a
//     foreign event format). No custom JS.
//   - Profile/password/users/audit: form POSTs to small wrappers calling the
//     same services in process (Auth.ChangeOwnPassword, Auth.RegisterUser,
//     DB.UpdateUser and the same user-table statements as the API handlers).
//   - Push: preferences toggle via wrapper (DB.SetPreference). The web-push
//     subscription itself needs a service worker + pushManager (browser API):
//     the current subscription state is rendered, the subscribe button is
//     deferred to the phase 6 PWA work.
//
// Deferred (needs client-side JS or is phase 6 scope, see report):
//   - Charger image upload (FileReader + canvas resize).
//   - Municipality autocomplete for the location field (fetch + dropdown) and
//     the localStorage-only settings (install name, location, prices,
//     accent/density/reduce-motion theme knobs): they have no server endpoint
//     in the React app either (localStorage only).
//   - PWA install prompt (beforeinstallprompt browser event).

// ── View types ──────────────────────────────────────────────────────────────

type invCardView struct {
	Name        string
	KwpText     string
	HasPanels   bool
	PanelsText  string
	HasBattery  bool
	BatteryText string
	Color       string
	IsTotal     bool
}

type entityRowView struct {
	Entity string
	Desc   string
}

type chargerFormView struct {
	Idx                  int
	Key                  string
	Name                 string
	PowerID              string
	PowerUnit            string
	EnergyTotalID        string
	EnergyDivisor        string
	EnergySessionID      string
	StateID              string
	TempID               string
	SwitchID             string
	AmpID                string
	ImageURL             string
	ChargingStates       string
	ConnectedStates      string
	ChargerInHouseMeters bool
	CanRemove            bool
	NamePlaceholder      string
}

type bydActionView struct {
	Action string
	Label  string
	// ClickExpr is the Datastar @post expression.
	ClickExpr string
	IsToggle  bool
}

type pushPrefView struct {
	Tipo    string
	Label   string
	Enabled bool
}

type langOption struct {
	Code string
	Name string
	Flag string
}

// profileLanguages mirrors the React selector (no "auto": the server resolves
// the language from the profile).
func profileLanguages() []langOption {
	return []langOption{
		{"es", "Español", "🇪🇸"},
		{"en", "English", "🇬🇧"},
		{"zh-CN", "简体中文", "🇨🇳"},
	}
}

type userRowView struct {
	ID       string
	Username string
	Role     string
	RoleText string
	Language string
	IsMe     bool
	IsAdmin  bool
}

type auditRowView struct {
	When   string
	Actor  string
	Action string
}

type updateStepView struct {
	Key   string
	Label string
}

type ajustesView struct {
	pageProps
	IsAdmin bool

	Saved string // PRG flag: topology|extensions|profile|password|users|push
	Err   string // PRG flag: section of the failed save

	// Installación (read-only cards from the resolved topology).
	InvCards []invCardView

	// Conexión.
	HAOk      bool
	Entities  []entityRowView
	Topo      install.Topology
	TopoCount int

	// Extensiones + BYD.
	ExtEnabled     bool
	ChargerEnabled bool
	BydEnabled     bool
	Chargers       []chargerFormView
	ChargerSel     int // pestaña activa del panel del cargador
	BydActions     []bydActionView
	BydValues      map[string]string

	// Push.
	PushConfigured bool
	PushSubscribed bool
	PushPrefs      []pushPrefView

	// Perfil.
	DisplayName string
	Email       string
	ProfileLang string
	Languages   []langOption

	// Admin.
	Users []userRowView
	MeID  string
	Audit []auditRowView

	// Update (admin).
	UpdateRunning bool
	UpdateStep    string
	UpdatePct     int
	UpdateSteps   []updateStepView

	// Instalación (nombre y ubicación del dispositivo; viven en KV).
	InstallName     string
	InstallLocation string
	SunNote         string

	// Precios (energía).
	PriceImport string
	PriceExport string
	Co2Factor   string

	// Acerca de.
	GoVersion   string
	UptimeText  string
	RepoVersion string
}

var ajustesSteps = []string{"fetch", "download", "verify", "install", "restart"}

// updateStepIndex maps a progress step to its position (-1 unknown, and
// "done" maps after the last one).
func updateStepIndex(step string) int {
	for i, s := range ajustesSteps {
		if s == step {
			return i
		}
	}
	return -1
}

// parsePanelsStr ports the React parsePanels("10 × 440 W"). Uses Fields so
// the multi-byte "×" separator is never split mid-rune.
func parsePanelsStr(s string) (panels, watts int) {
	f := strings.Fields(s)
	if len(f) < 3 {
		return 0, 0
	}
	panels, _ = strconv.Atoi(f[0])
	for i, tok := range f[1:] {
		if strings.HasPrefix(tok, "×") || tok == "x" || tok == "X" {
			if i+2 < len(f) {
				watts, _ = strconv.Atoi(strings.TrimSuffix(f[i+2], "W"))
			}
			break
		}
	}
	return panels, watts
}

// f2s formats a float for a form value without trailing zeros.
func f2s(v float64) string {
	if v == 0 {
		return ""
	}
	s := strconv.FormatFloat(v, 'f', -1, 64)
	return s
}

// csvJoin/parseCsvForm port the React arrToCsv/csvToArr round-trip.
func csvJoin(a []string) string {
	return strings.Join(a, ", ")
}

func parseCsvForm(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// inverterColor mirrors Ajustes.tsx card colors.
func inverterColor(key string) string {
	switch key {
	case "solis":
		return "var(--c-solis)"
	case "fox":
		return "var(--c-fox)"
	}
	return "var(--brand)"
}

// descKeyFor ports the ConnectionSection role -> i18n key mapping.
func descKeyFor(role, name string) string {
	switch role {
	case "inverter":
		return "desc_inverter"
	case "inverter_energy":
		return "desc_inverter_energy"
	case "battery_power":
		return "desc_battery_power"
	case "battery_soc":
		return "desc_solis_bateria_soc"
	case "battery_state":
		return "desc_battery_state"
	case "grid_sensor":
		return "desc_grid_sensor"
	case "grid_attrs":
		return "desc_grid_attrs"
	case "grid_import":
		return "desc_grid_import"
	case "grid_export":
		return "desc_grid_export"
	case "inverter_status":
		return "desc_inverter_status"
	case "sun":
		return "desc_sun"
	case "weather":
		return "desc_weather"
	case "weather_temp":
		return "desc_weather_temp"
	}
	return "desc_consumption"
}

// entityRows replicates the rows of GET /api/install (handleGetInstall).
func entityRows(t install.Topology) []entityRowView {
	var rows []entityRowView
	add := func(role, entity, name string) {
		rows = append(rows, entityRowView{Entity: entity, Desc: descKeyFor(role, name)})
	}
	for _, inv := range t.Inverters {
		add("inverter", inv.PowerID, inv.Name)
		if inv.EnergyID != "" {
			add("inverter_energy", inv.EnergyID, inv.Name)
		}
	}
	for _, id := range t.Consumption.PowerIDs {
		add("consumption", id, "")
	}
	if t.Battery.Enabled {
		if t.Battery.PowerID != "" {
			add("battery_power", t.Battery.PowerID, "")
		}
		if t.Battery.SocID != "" {
			add("battery_soc", t.Battery.SocID, "")
		}
		if t.Battery.StateID != "" {
			add("battery_state", t.Battery.StateID, "")
		}
	}
	if t.Grid.Mode == "attrs" {
		if t.Grid.AttrsID != nil {
			add("grid_attrs", *t.Grid.AttrsID, "")
		}
	} else {
		if t.Grid.SensorID != nil {
			add("grid_sensor", *t.Grid.SensorID, "")
		}
		if t.Grid.ImportID != nil {
			add("grid_import", *t.Grid.ImportID, "")
		}
		if t.Grid.ExportID != nil {
			add("grid_export", *t.Grid.ExportID, "")
		}
	}
	if t.StatusAttrsID != nil {
		add("inverter_status", *t.StatusAttrsID, "")
	}
	add("sun", t.Sun, "")
	if t.Weather != "" {
		add("weather", t.Weather, "")
	}
	if t.WeatherTemp != "" {
		add("weather_temp", t.WeatherTemp, "")
	}
	return rows
}

// buildInvCards ports InstallationSection's inverter cards.
func buildInstallCards(lang string, t install.Topology) []invCardView {
	cards := make([]invCardView, 0, len(t.Inverters)+1)
	var totalKwp float64
	totalPanels := 0
	for _, inv := range t.Inverters {
		panels, watts := parsePanelsStr(inv.Panels)
		totalKwp += inv.Kwp
		totalPanels += panels
		c := invCardView{
			// Paridad con el React: name = model || name.
			Name:    inv.Model,
			KwpText: formatNumber(lang, inv.Kwp, 1),
			Color:   inverterColor(inv.Key),
		}
		if c.Name == "" {
			c.Name = inv.Name
		}
		if panels > 0 {
			c.HasPanels = true
			c.PanelsText = T(lang, "ajustes.install.panels", map[string]string{"count": strconv.Itoa(panels), "watts": strconv.Itoa(watts)})
		}
		if inv.HasBattery {
			c.HasBattery = true
			// El perfil de instalación no trae marca de batería: "Batería · 5,0 kWh"
			// (antes salía "Batería Batería").
			c.BatteryText = T(lang, "ajustes.install.batterySize", map[string]string{
				"label": T(lang, "common.battery"),
				"kwh":   formatNumber(lang, inv.BatteryKwh, 1),
			})
		}
		cards = append(cards, c)
	}
	cards = append(cards, invCardView{
		Name:       T(lang, "ajustes.install.totalLabel"),
		KwpText:    formatNumber(lang, totalKwp, 1),
		Color:      "var(--brand)",
		IsTotal:    true,
		HasPanels:  totalPanels > 0,
		PanelsText: strconv.Itoa(totalPanels) + " " + T(lang, "ajustes.install.panelsShort"),
	})
	return cards
}

// buildChargerForms maps the extension config to the per-charger form fields.
func buildChargerForms(cfg extensions.Config, lang string) []chargerFormView {
	out := make([]chargerFormView, 0, len(cfg.CarCharger.Chargers))
	for i := range cfg.CarCharger.Chargers {
		c := &cfg.CarCharger.Chargers[i]
		name := c.Name
		if name == "" {
			name = T(lang, "ajustes.extensions.chargerN", map[string]string{"n": strconv.Itoa(i + 1)})
		}
		out = append(out, chargerFormView{
			Idx:                  i,
			Key:                  c.Key,
			Name:                 name,
			PowerID:              c.PowerID,
			PowerUnit:            string(c.PowerUnit),
			EnergyTotalID:        c.EnergyTotalID,
			EnergyDivisor:        strconv.Itoa(c.EnergyDivisor),
			EnergySessionID:      c.EnergySessionID,
			StateID:              c.StateID,
			TempID:               c.TempID,
			SwitchID:             c.SwitchID,
			AmpID:                c.AmpID,
			ImageURL:             c.ImageURL,
			ChargingStates:       csvJoin(c.ChargingStates),
			ConnectedStates:      csvJoin(c.ConnectedStates),
			ChargerInHouseMeters: c.ChargerInHouseMeters,
			CanRemove:            len(cfg.CarCharger.Chargers) > 1,
			NamePlaceholder:      name,
		})
	}
	return out
}

// bydPressActions are the whitelisted press actions shown as buttons (only
// rendered when the corresponding entity is configured). BYD is the personal
// overlay module: its labels are Spanish literals (no i18n keys exist in the
// shared catalogs, which are synced from the React app).
func bydPressActions(lang string) []bydActionView {
	_ = lang
	return []bydActionView{
		{Action: "start_charging", Label: "Iniciar carga"},
		{Action: "stop_charging", Label: "Detener carga"},
		{Action: "force_poll", Label: "Forzar sondeo"},
		{Action: "flash_lights", Label: "Luces"},
		{Action: "find_car", Label: "Buscar coche"},
	}
}

// bydFieldLabels are the form labels of the BYD fields (Spanish literals,
// same reason as bydPressActions).
var bydFieldLabels = map[string]string{
	"name":           "Nombre",
	"socId":          "Entidad SOC",
	"rangeId":        "Entidad autonomía",
	"odometerId":     "Entidad cuentakilómetros",
	"batteryPowerId": "Entidad potencia batería",
	"chargingId":     "Entidad cargando",
	"pluggedId":      "Entidad enchufado",
	"onlineId":       "Entidad online",
	"lockedId":       "Entidad cerrado",
	"startChargeId":  "Entidad iniciar carga",
	"stopChargeId":   "Entidad detener carga",
	"forcePollId":    "Entidad forzar sondeo",
	"chargeToFullId": "Entidad cargar al máximo",
}

// auditWhenText formats the audit timestamp like the React fmtTs (day short
// month hour:minute).
func auditWhenText(lang string, ts int64) string {
	t := time.UnixMilli(ts)
	months := map[string][]string{
		"es":    {"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"},
		"en":    {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
		"zh-CN": {"1月", "2月", "3月", "4月", "5月", "6月", "7月", "8月", "9月", "10月", "11月", "12月"},
	}
	m := months["es"]
	if v, ok := months[normalizeLang(lang)]; ok {
		m = v
	}
	day := strconv.Itoa(t.Day())
	if lang != "zh-CN" && t.Day() < 10 {
		day = "0" + day
	}
	hh, mm := t.Hour(), t.Minute()
	pad := func(v int) string {
		if v < 10 {
			return "0" + strconv.Itoa(v)
		}
		return strconv.Itoa(v)
	}
	return day + " " + m[int(t.Month())-1] + " " + pad(hh) + ":" + pad(mm)
}

// buildAjustes assembles the page view.
func (u *UI) buildAjustes(r *http.Request, userID string) ajustesView {
	lang := u.userLang(userID)
	p := u.baseProps(r, userID, "/ajustes", "")
	p.Title = T(lang, "nav.ajustes")

	me, _ := u.DB.GetUserByID(userID)
	isAdmin := me != nil && me.Role == "admin"

	v := ajustesView{
		pageProps: p,
		IsAdmin:   isAdmin,
		Languages: profileLanguages(),
	}

	if me != nil {
		v.MeID = me.ID
		v.DisplayName = me.Username
		if me.DisplayName != nil && *me.DisplayName != "" {
			v.DisplayName = *me.DisplayName
		}
		v.Email = ""
		if me.Email != nil {
			v.Email = *me.Email
		}
		v.ProfileLang = me.Language
		if v.ProfileLang == "" {
			v.ProfileLang = "es"
		}
	}

	// Resultado de un guardado (PRG).
	q := r.URL.Query()
	v.Saved = q.Get("saved")
	v.Err = q.Get("err")

	// Instalación.
	if u.Install != nil {
		topo := u.Install.Get()
		v.Topo = topo
		v.TopoCount = len(topo.Inverters)
		v.InvCards = buildInstallCards(lang, topo)
		v.Entities = entityRows(topo)
	}

	// Salud de la conexión HA: el estado del websocket (como el badge React,
	// que usa connectionStatus del proveedor de datos).
	if u.HA != nil {
		v.HAOk = u.HA.IsConnected()
	}

	// Extensiones.
	if u.ExtStore != nil {
		cfg := u.ExtStore.Get()
		v.ExtEnabled = cfg.Enabled
		v.ChargerEnabled = cfg.CarCharger.Enabled
		v.Chargers = buildChargerForms(cfg, lang)
		v.BydEnabled = cfg.Byd.Enabled
		v.BydValues = map[string]string{
			"name":           cfg.Byd.Name,
			"socId":          cfg.Byd.SocID,
			"rangeId":        cfg.Byd.RangeID,
			"odometerId":     cfg.Byd.OdometerID,
			"batteryPowerId": cfg.Byd.BatteryPowerID,
			"chargingId":     cfg.Byd.ChargingID,
			"pluggedId":      cfg.Byd.PluggedID,
			"onlineId":       cfg.Byd.OnlineID,
			"lockedId":       cfg.Byd.LockedID,
			"startChargeId":  cfg.Byd.StartChargeID,
			"stopChargeId":   cfg.Byd.StopChargeID,
			"forcePollId":    cfg.Byd.ForcePollID,
			"chargeToFullId": cfg.Byd.ChargeToFullID,
		}
		if isAdmin && extensions.BydActive(cfg) {
			for _, a := range bydPressActions(lang) {
				a.ClickExpr = "$action = '" + a.Action + "'; @post('/api/byd/action')"
				v.BydActions = append(v.BydActions, a)
			}
		}
	}

	// Push.
	prefs, _ := u.DB.PreferencesForUser(userID)
	if subs, err := u.DB.SubscriptionsByUser(userID); err == nil {
		v.PushSubscribed = len(subs) > 0
	}
	if u.Push != nil && u.Push.IsConfigured() {
		v.PushConfigured = true
	}
	for _, tipo := range push.TiposAlerta {
		enabled := true
		if e, ok := prefs[tipo]; ok {
			enabled = e
		}
		v.PushPrefs = append(v.PushPrefs, pushPrefView{
			Tipo:    tipo,
			Label:   T(lang, "ajustes.notif.tipos."+tipo),
			Enabled: enabled,
		})
	}

	// Update (solo admin): adoptar un update en curso del fichero local de
	// progreso (barato); el check de versión va on-demand con Datastar.
	if isAdmin {
		for _, s := range ajustesSteps {
			v.UpdateSteps = append(v.UpdateSteps, updateStepView{Key: s, Label: T(lang, "update.step."+s)})
		}
		if prog := update.ReadProgress(u.DataDir); prog != nil && prog.Step != "" && prog.Step != "done" && prog.Step != "idle" {
			v.UpdateRunning = true
			v.UpdateStep = prog.Step
			v.UpdatePct = prog.Pct
		}
	}

	// Admin: usuarios + auditoría.
	if isAdmin {
		if users, err := u.DB.ListUsers(); err == nil {
			for _, usr := range users {
				roleText := T(lang, "admin.users.roleUser")
				isAdminRow := usr.Role == "admin"
				if isAdminRow {
					roleText = T(lang, "admin.users.roleAdmin")
				}
				v.Users = append(v.Users, userRowView{
					ID:       usr.ID,
					Username: usr.Username,
					Role:     usr.Role,
					RoleText: roleText,
					Language: usr.Language,
					IsMe:     usr.ID == userID,
					IsAdmin:  isAdminRow,
				})
			}
		}
		if entries, err := u.DB.AuditRange(50, 0); err == nil {
			for _, e := range entries {
				v.Audit = append(v.Audit, auditRowView{
					When:   auditWhenText(lang, e.Ts),
					Actor:  e.Actor,
					Action: e.Action,
				})
			}
		}
	}

	// Acerca de: versión + runtime (mismo contenido que GET /api/system/info,
	// computado en proceso; el endpoint JSON no cambia).
	// Precios en formato decimal castellano (el parse acepta coma o punto).
	if u.DB != nil {
		v.InstallName, _ = u.DB.KVGet("installName")
		v.InstallLocation, _ = u.DB.KVGet("installLocation")
	}
	if u.Solar != nil && u.HA != nil {
		sr, ss := u.installSunTimes()
		v.SunNote = T(v.Lang, "ajustes.install.sunTodayHa", map[string]string{
			"sunrise": fmtTime(sr),
			"sunset":  fmtTime(ss),
		})
		if v.InstallLocation != "" {
			v.SunNote = T(v.Lang, "ajustes.install.sunTodayAt", map[string]string{
				"location": v.InstallLocation,
				"sunrise":  fmtTime(sr),
				"sunset":   fmtTime(ss),
			})
		}
	}
	v.PriceImport = strings.Replace(strconv.FormatFloat(u.Prices.PriceImport, 'f', -1, 64), ".", ",", 1)
	v.PriceExport = strings.Replace(strconv.FormatFloat(u.Prices.PriceExport, 'f', -1, 64), ".", ",", 1)
	v.Co2Factor = strings.Replace(strconv.FormatFloat(u.Prices.Co2PerKwh, 'f', -1, 64), ".", ",", 1)
	v.RepoVersion = version.Version
	v.GoVersion = strings.Split(strings.TrimPrefix(runtime.Version(), "go"), "-")[0]
	up := int(time.Since(processStart).Seconds())
	v.UptimeText = strconv.Itoa(up/3600) + "h " + strconv.Itoa((up%3600)/60) + "m"

	return v
}

// ── Handlers ────────────────────────────────────────────────────────────────

func (u *UI) handleAjustes(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	v := u.buildAjustes(r, userID)
	_ = AjustesPage(v).Render(r.Context(), w)
}

// ajustesRedirectFlag is the PRG helper: back to /ajustes with saved/err
// flag (the page shows a banner and jumps to the section anchor).
func ajustesRedirectFlag(w http.ResponseWriter, r *http.Request, key, value string) {
	http.Redirect(w, r, "/ajustes?"+key+"="+value, http.StatusSeeOther)
}

func (u *UI) currentUserActor(r *http.Request) (username, userID string, isAdmin bool) {
	userID, _ = auth.UserIDFrom(r.Context())
	if usr, _ := u.DB.GetUserByID(userID); usr != nil {
		return usr.Username, userID, usr.Role == "admin"
	}
	return "", userID, false
}

// handleAjustesTopology is the TopologyEditor save (form POST wrapper of
// PUT /api/config with the same install.Normalize validation).
func (u *UI) handleAjustesTopology(w http.ResponseWriter, r *http.Request) {
	username, userID, isAdmin := u.currentUserActor(r)
	if !isAdmin {
		http.Redirect(w, r, "/ajustes?err=forbidden", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		ajustesRedirectFlag(w, r, "err", "topology")
		return
	}
	f := r.PostForm

	count, _ := strconv.Atoi(f.Get("inv.count"))
	if count < 1 || count > 10 {
		count = 1
	}
	input := install.TopologyInput{}
	for i := 0; i < count; i++ {
		p := "inv." + strconv.Itoa(i) + "."
		kwp, _ := strconv.ParseFloat(f.Get(p+"kwp"), 64)
		capPtr := new(float64)
		*capPtr, _ = strconv.ParseFloat(f.Get(p+"energyCap"), 64)
		input.Inverters = append(input.Inverters, install.InverterInput{
			Key:             f.Get(p + "key"),
			Name:            f.Get(p + "name"),
			Model:           f.Get(p + "model"),
			Kwp:             kwp,
			Panels:          f.Get(p + "panels"),
			HasBattery:      f.Get(p+"hasBattery") == "1",
			BatteryKwh:      atof(f.Get(p + "batteryKwh")),
			PowerID:         f.Get(p + "powerId"),
			PowerUnit:       install.PowerUnit(f.Get(p + "powerUnit")),
			BackupPowerID:   f.Get(p + "backupPowerId"),
			BackupPowerUnit: install.PowerUnit(f.Get(p + "backupPowerUnit")),
			EnergyID:        f.Get(p + "energyId"),
			EnergyAcc:       install.EnergyAcc(f.Get(p + "energyAcc")),
			EnergyCap:       capPtr,
			DeepIDs:         parseCsvForm(f.Get(p + "deepIds")),
		})
	}

	input.Battery = &install.BatteryConfig{
		Enabled:           f.Get("battery.enabled") == "1",
		PowerID:           f.Get("battery.powerId"),
		StateID:           f.Get("battery.stateId"),
		SocID:             f.Get("battery.socId"),
		CapacityKwh:       atof(f.Get("battery.capacityKwh")),
		ChargingStates:    parseCsvForm(f.Get("battery.chargingStates")),
		DischargingStates: parseCsvForm(f.Get("battery.dischargingStates")),
	}
	spOrNil := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	input.Grid = &install.GridConfig{
		Mode:     f.Get("grid.mode"),
		AttrsID:  spOrNil(f.Get("grid.attrsId")),
		SensorID: spOrNil(f.Get("grid.sensorId")),
		ImportID: spOrNil(f.Get("grid.importId")),
		ExportID: spOrNil(f.Get("grid.exportId")),
	}
	statusAttrs := spOrNil(f.Get("statusAttrsId"))
	input.StatusAttrsID = statusAttrs
	input.Consumption = &install.ConsumptionConfig{
		PowerIDs:       parseCsvForm(f.Get("cons.powerIds")),
		EnergyIDs:      parseCsvForm(f.Get("cons.energyIds")),
		RespaldoID:     spOrNil(f.Get("cons.respaldoId")),
		NoRespaldadaID: spOrNil(f.Get("cons.noRespaldadaId")),
		// El form no edita PowerUnit: preserva el del perfil actual (el
		// editor React lo envía dentro de su draft completo).
		PowerUnit: u.Install.Get().Consumption.PowerUnit,
	}
	input.Energy = &install.EnergyConfig{
		GridImportID:   f.Get("energy.gridImportId"),
		GridExportID:   f.Get("energy.gridExportId"),
		ConsumptionID:  f.Get("energy.consumptionId"),
		BatChargeID:    f.Get("energy.batChargeId"),
		BatDischargeID: f.Get("energy.batDischargeId"),
	}
	input.Sun = f.Get("env.sun")
	input.Weather = f.Get("env.weather")
	input.WeatherTemp = f.Get("env.weatherTemp")

	normalized := install.Normalize(input, u.Install.Get())

	// Merge sobre el KV existente (la API machaca el body completo; el form
	// solo gestiona "topology", así que preserva cualquier otra clave).
	var cfg map[string]any
	if raw, err := u.DB.KVGet("install_config"); err == nil && raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	cfg["topology"] = normalized
	out, err := json.Marshal(cfg)
	if err != nil || u.DB.KVSet("install_config", string(out)) != nil {
		ajustesRedirectFlag(w, r, "err", "topology")
		return
	}
	u.Install.Set(install.ResolveInstall(u.DB))
	u.DB.Audit(username, userID, "config.change", map[string]any{"keys": []string{"topology"}})
	ajustesRedirectFlag(w, r, "saved", "topology")
}

func atof(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

// handleAjustesExtensions is the extensions + BYD config form (wrapper of
// PUT /api/extensions with the same NormalizeConfig profile).
func (u *UI) handleAjustesExtensions(w http.ResponseWriter, r *http.Request) {
	username, userID, isAdmin := u.currentUserActor(r)
	if !isAdmin {
		ajustesRedirectFlag(w, r, "err", "forbidden")
		return
	}
	if err := r.ParseForm(); err != nil {
		ajustesRedirectFlag(w, r, "err", "extensions")
		return
	}
	f := r.PostForm

	// Add/remove chargers: botones dedicados que re-renderizan el form con el
	// estado nuevo (sin JS). El resto de submits guardan.
	if f.Get("action") == "add-charger" {
		u.extAddCharger(username, userID)
		ajustesRedirectFlag(w, r, "saved", "extensions")
		return
	}
	if f.Get("action") == "remove-charger" {
		idx, _ := strconv.Atoi(f.Get("remove"))
		u.extRemoveCharger(idx, username, userID)
		ajustesRedirectFlag(w, r, "saved", "extensions")
		return
	}

	count, _ := strconv.Atoi(f.Get("chg.count"))
	if count < 0 || count > 10 {
		count = 0
	}
	chargers := make([]map[string]any, 0, count)
	existing := u.ExtStore.Get().CarCharger.Chargers
	for i := 0; i < count; i++ {
		p := "chg." + strconv.Itoa(i) + "."
		div, _ := strconv.Atoi(f.Get(p + "energyDivisor"))
		if div < 1 {
			div = 1
		}
		// delayId/durationId no se editan en este formulario: se conservan del
		// cargador guardado (por key, o por posición si la key cambió). Sin
		// esto, guardar ajustes los borraba del kv (issue 173).
		delayID, durationID := "", ""
		if old := extensions.GetChargerByKey(u.ExtStore.Get(), f.Get(p+"key")); old != nil {
			delayID, durationID = old.DelayID, old.DurationID
		} else if i < len(existing) {
			delayID, durationID = existing[i].DelayID, existing[i].DurationID
		}
		chargers = append(chargers, map[string]any{
			"key":                  f.Get(p + "key"),
			"name":                 f.Get(p + "name"),
			"powerId":              f.Get(p + "powerId"),
			"powerUnit":            f.Get(p + "powerUnit"),
			"energyTotalId":        f.Get(p + "energyTotalId"),
			"energyDivisor":        div,
			"energySessionId":      f.Get(p + "energySessionId"),
			"stateId":              f.Get(p + "stateId"),
			"tempId":               f.Get(p + "tempId"),
			"switchId":             f.Get(p + "switchId"),
			"ampId":                f.Get(p + "ampId"),
			"delayId":              delayID,
			"durationId":           durationID,
			"imageUrl":             f.Get(p + "imageUrl"),
			"chargerInHouseMeters": f.Get(p+"inHouse") == "1",
			"chargingStates":       parseCsvForm(f.Get(p + "chargingStates")),
			"connectedStates":      parseCsvForm(f.Get(p + "connectedStates")),
		})
	}

	byd := map[string]any{"enabled": f.Get("byd.enabled") == "1"}
	for _, field := range bydConfigFields {
		byd[field] = f.Get("byd." + field)
	}

	raw := map[string]any{
		"enabled":    f.Get("ext.enabled") == "1",
		"carCharger": map[string]any{"enabled": f.Get("chg.enabled") == "1", "chargers": chargers},
		"byd":        byd,
	}
	cfg := extensions.NormalizeConfig(raw, extensions.GenericExtensions)
	out, err := json.Marshal(cfg)
	if err != nil || u.DB.KVSet("extensions_config", string(out)) != nil {
		ajustesRedirectFlag(w, r, "err", "extensions")
		return
	}
	u.ExtStore.Set(cfg)
	u.DB.Audit(username, userID, "extensions.change", map[string]any{"enabled": cfg.Enabled})
	ajustesRedirectFlag(w, r, "saved", "extensions")
}

// extAddCharger appends a default charger (React NEW_CHARGER parity).
func (u *UI) extAddCharger(username, userID string) {
	cfg := u.ExtStore.Get()
	key := "c" + strconv.Itoa(len(cfg.CarCharger.Chargers)+1)
	if len(cfg.CarCharger.Chargers) == 0 {
		key = "main"
	}
	cfg.CarCharger.Chargers = append(cfg.CarCharger.Chargers, extensions.Charger{
		Key:                  key,
		Name:                 "Cargador " + strconv.Itoa(len(cfg.CarCharger.Chargers)+1),
		PowerUnit:            extensions.UnitKW,
		EnergyDivisor:        1,
		ChargerInHouseMeters: true,
		ChargingStates:       []string{"charging"},
		ConnectedStates:      []string{"charger_insert", "charging"},
	})
	u.extSave(username, userID, cfg)
}

// extRemoveCharger deletes the charger at idx (keeps at least one).
func (u *UI) extRemoveCharger(idx int, username, userID string) {
	cfg := u.ExtStore.Get()
	if len(cfg.CarCharger.Chargers) <= 1 || idx < 0 || idx >= len(cfg.CarCharger.Chargers) {
		return
	}
	cfg.CarCharger.Chargers = append(cfg.CarCharger.Chargers[:idx], cfg.CarCharger.Chargers[idx+1:]...)
	u.extSave(username, userID, cfg)
}

func (u *UI) extSave(username, userID string, cfg extensions.Config) {
	if out, err := json.Marshal(cfg); err == nil {
		if u.DB.KVSet("extensions_config", string(out)) == nil {
			u.ExtStore.Set(extensions.ResolveConfig(u.DB))
			u.DB.Audit(username, userID, "extensions.change", map[string]any{"enabled": cfg.Enabled})
		}
	}
}

// bydConfigFields are the BYD fields editable in the form (subset shown by
// the overlay: identity + charge + poll; the rest keep their stored value).
var bydConfigFields = []string{
	"name", "socId", "rangeId", "odometerId", "batteryPowerId", "chargingId",
	"pluggedId", "onlineId", "lockedId", "startChargeId", "stopChargeId",
	"forcePollId", "chargeToFullId",
}

// handleAjustesPushPref toggles one push preference (wrapper of
// PUT /api/push/preferences).
func (u *UI) handleAjustesPushPref(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	if err := r.ParseForm(); err != nil || !push.IsValidTipo(r.PostFormValue("tipo")) {
		ajustesRedirectFlag(w, r, "err", "pushpref")
		return
	}
	enabled := r.PostFormValue("enabled") == "1"
	if err := u.DB.SetPreference(userID, r.PostFormValue("tipo"), enabled, time.Now().UnixMilli()); err != nil {
		ajustesRedirectFlag(w, r, "err", "pushpref")
		return
	}
	ajustesRedirectFlag(w, r, "saved", "pushpref")
}

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// handleAjustesProfile saves display name / email / phone / language (wrapper
// of PUT /api/auth/profile).
func (u *UI) handleAjustesProfile(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	if err := r.ParseForm(); err != nil {
		ajustesRedirectFlag(w, r, "err", "profile")
		return
	}
	name := strings.TrimSpace(r.PostFormValue("display_name"))
	email := strings.TrimSpace(r.PostFormValue("email"))
	phone := strings.TrimSpace(r.PostFormValue("phone"))
	language := r.PostFormValue("language")

	validLang := false
	for _, l := range Languages {
		if l == language {
			validLang = true
			break
		}
	}
	if (email != "" && !emailRe.MatchString(email)) || (language != "" && !validLang) {
		ajustesRedirectFlag(w, r, "err", "profile")
		return
	}
	updates := db.UserUpdates{}
	if name != "" {
		n := name
		updates.DisplayName = &n
	} else {
		empty := ""
		updates.DisplayName = &empty
	}
	if email != "" {
		e := email
		updates.Email = &e
	} else {
		empty := ""
		updates.Email = &empty
	}
	if phone != "" {
		p := phone
		updates.Phone = &p
	}
	if language != "" {
		l := language
		updates.Language = &l
	}
	if _, err := u.DB.UpdateUser(userID, updates); err != nil {
		ajustesRedirectFlag(w, r, "err", "profile")
		return
	}
	ajustesRedirectFlag(w, r, "saved", "profile")
}

// handleAjustesPassword changes the own password (wrapper of
// PUT /api/auth/password).
func (u *UI) handleAjustesPassword(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	if err := r.ParseForm(); err != nil {
		ajustesRedirectFlag(w, r, "err", "password")
		return
	}
	current := r.PostFormValue("current")
	password := r.PostFormValue("password")
	confirm := r.PostFormValue("confirm")
	if password != confirm || len(password) < 6 {
		ajustesRedirectFlag(w, r, "err", "password")
		return
	}
	keep := ""
	if sess := u.Auth.SessionIDFromCookie(r); sess != nil {
		keep = sess.ID
	}
	ok, _, err := u.Auth.ChangeOwnPassword(userID, current, password, keep)
	if err != nil || !ok {
		ajustesRedirectFlag(w, r, "err", "password")
		return
	}
	ajustesRedirectFlag(w, r, "saved", "password")
}

// handleAjustesUserCreate registers a user (wrapper of POST /api/auth/register).
func (u *UI) handleAjustesUserCreate(w http.ResponseWriter, r *http.Request) {
	username, userID, isAdmin := u.currentUserActor(r)
	if !isAdmin {
		ajustesRedirectFlag(w, r, "err", "forbidden")
		return
	}
	if err := r.ParseForm(); err != nil {
		ajustesRedirectFlag(w, r, "err", "users")
		return
	}
	newUser := strings.TrimSpace(r.PostFormValue("username"))
	password := r.PostFormValue("password")
	language := r.PostFormValue("language")
	role := r.PostFormValue("role")
	if len(newUser) < 3 || len(newUser) > 50 || len(password) < 6 {
		ajustesRedirectFlag(w, r, "err", "users")
		return
	}
	if role != "user" && role != "admin" {
		role = "user"
	}
	if language == "" {
		language = "es"
	}
	user, err := u.Auth.RegisterUser(newUser, password, language, role)
	if err != nil || user == nil {
		ajustesRedirectFlag(w, r, "err", "users")
		return
	}
	u.DB.Audit(username, userID, "user.create", map[string]string{"username": user.Username, "role": user.Role})
	ajustesRedirectFlag(w, r, "saved", "users")
}

// targetUserOf extracts {id} and enforces the admin guard like the API
// handlers (same error shape via PRG). Devuelve usuario E id del actor: el
// guard de "no sobre uno mismo" comparaba el username con el id del objetivo y
// nunca casaba, asi que un admin podia cambiarse el rol o borrarse a si mismo.
func (u *UI) targetUserOf(r *http.Request) (adminUser, adminID, targetID string, ok bool) {
	adminUser, adminID, isAdmin := u.currentUserActor(r)
	targetID = chi.URLParam(r, "id")
	return adminUser, adminID, targetID, isAdmin && targetID != ""
}

// handleAjustesUserSave applies language + role + optional password of one
// user (wrappers of PUT /api/auth/users/{id}/language|role|password).
func (u *UI) handleAjustesUserSave(w http.ResponseWriter, r *http.Request) {
	adminUser, adminID, target, ok := u.targetUserOf(r)
	if !ok {
		ajustesRedirectFlag(w, r, "err", "forbidden")
		return
	}
	if err := r.ParseForm(); err != nil {
		ajustesRedirectFlag(w, r, "err", "users")
		return
	}
	if adminID == target {
		// Sobre uno mismo: la API no permite role; password/language sí.
		if pwd := r.PostFormValue("password"); pwd != "" {
			if len(pwd) < 6 {
				ajustesRedirectFlag(w, r, "err", "users")
				return
			}
			u.adminSetUserPassword(adminUser, adminID, target, pwd)
		}
		if lang := r.PostFormValue("language"); lang != "" && validLangCode(lang) {
			u.adminSetUserLanguage(adminUser, adminID, target, lang)
		}
		ajustesRedirectFlag(w, r, "saved", "users")
		return
	}
	if lang := r.PostFormValue("language"); lang != "" && validLangCode(lang) {
		u.adminSetUserLanguage(adminUser, adminID, target, lang)
	}
	if role := r.PostFormValue("role"); role == "user" || role == "admin" {
		u.adminSetUserRole(adminUser, adminID, target, role)
	}
	if pwd := r.PostFormValue("password"); pwd != "" {
		if len(pwd) < 6 {
			ajustesRedirectFlag(w, r, "err", "users")
			return
		}
		u.adminSetUserPassword(adminUser, adminID, target, pwd)
	}
	ajustesRedirectFlag(w, r, "saved", "users")
}

// Same statements and audit actions as the API admin handlers.
func (u *UI) adminSetUserPassword(adminUser, adminID, target, password string) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return
	}
	if _, err := u.DB.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), target); err == nil {
		u.DB.Audit(adminUser, adminID, "user.password.admin", map[string]string{"target": target})
	}
}

func (u *UI) adminSetUserLanguage(adminUser, adminID, target, language string) {
	if _, err := u.DB.Exec(`UPDATE users SET language = ? WHERE id = ?`, language, target); err == nil {
		u.DB.Audit(adminUser, adminID, "user.language.admin", map[string]string{"target": target})
	}
}

func (u *UI) adminSetUserRole(adminUser, adminID, target, role string) {
	if _, err := u.DB.Exec(`UPDATE users SET role = ? WHERE id = ?`, role, target); err == nil {
		u.DB.Audit(adminUser, adminID, "user.role.admin", map[string]string{"target": target, "role": role})
	}
}

// handleAjustesUserDelete deletes a user (wrapper of DELETE /api/auth/users/{id}).
func (u *UI) handleAjustesUserDelete(w http.ResponseWriter, r *http.Request) {
	adminUser, adminID, target, ok := u.targetUserOf(r)
	if !ok {
		ajustesRedirectFlag(w, r, "err", "forbidden")
		return
	}
	if adminID == target {
		ajustesRedirectFlag(w, r, "err", "users")
		return
	}
	if _, err := u.DB.Exec(`DELETE FROM users WHERE id = ?`, target); err == nil {
		u.DB.Audit(adminUser, adminID, "user.delete", map[string]string{"target": target})
	}
	ajustesRedirectFlag(w, r, "saved", "users")
}

func validLangCode(l string) bool {
	for _, v := range Languages {
		if v == l {
			return true
		}
	}
	return false
}

// firstNonEmpty returns a if set, else b (profile header falls back to the
// login name when no display name was saved).
func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// accentOption is one entry of the appearance accent picker. Hex is only the
// swatch colour; RGB feeds --accent-rgb through the accent cookie.
type accentOption struct {
	ID    string
	Hex   string
	Label string
}

// accentOptions mirrors ACCENTS in app/src/pages/Ajustes.tsx.
var accentOptions = []accentOption{
	{"naranja", "#F59E0B", "ajustes.accent.naranja"},
	{"verde", "#22C55E", "ajustes.accent.verde"},
	{"azul", "#3B82F6", "ajustes.accent.azul"},
	{"violeta", "#8B5CF6", "ajustes.accent.violeta"},
}

// handleAjustesPrices stores the energy prices (KV) and applies them to the
// server-side savings/CO2 estimates. The React app kept them in localStorage;
// server-rendered there is no per-device settings store, so they live with the
// installation (read back on startup).
func (u *UI) handleAjustesPrices(w http.ResponseWriter, r *http.Request) {
	username, userID, isAdmin := u.currentUserActor(r)
	if !isAdmin {
		ajustesRedirectFlag(w, r, "err", "forbidden")
		return
	}
	parse := func(name string) (float64, bool) {
		raw := strings.TrimSpace(strings.ReplaceAll(r.PostFormValue(name), ",", "."))
		v, err := strconv.ParseFloat(raw, 64)
		// ParseFloat acepta "NaN" e "Inf": una comparación de rango normal es
		// falsa para NaN y dejaba pasar el valor.
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 10 {
			return 0, false
		}
		return v, true
	}
	imp, ok1 := parse("price_import")
	exp, ok2 := parse("price_export")
	co2, ok3 := parse("co2_factor")
	if !ok1 || !ok2 || !ok3 {
		ajustesRedirectFlag(w, r, "err", "prices")
		return
	}
	kv := map[string]float64{
		"priceImportEur": imp,
		"priceExportEur": exp,
		"co2Factor":      co2,
	}
	if u.DB != nil {
		for k, val := range kv {
			u.DB.KVSet(k, strconv.FormatFloat(val, 'f', -1, 64))
		}
		u.DB.Audit(username, userID, "prices.update", map[string]string{
			"import": strconv.FormatFloat(imp, 'f', -1, 64),
			"export": strconv.FormatFloat(exp, 'f', -1, 64),
			"co2":    strconv.FormatFloat(co2, 'f', -1, 64),
		})
	}
	u.Prices.PriceImport = imp
	u.Prices.PriceExport = exp
	u.Prices.Co2PerKwh = co2
	ajustesRedirectFlag(w, r, "saved", "prices")
}

// handleAjustesInstall stores the installation name and location (KV). They
// were per-device settings in the React app; server-rendered they describe the
// installation.
func (u *UI) handleAjustesInstall(w http.ResponseWriter, r *http.Request) {
	username, userID, isAdmin := u.currentUserActor(r)
	if !isAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	name, location := "", ""
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		// Datastar @post sends the bound signals as JSON: save on the fly.
		var body struct {
			InstallName     string `json:"installName"`
			InstallLocation string `json:"installLocation"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			name, location = strings.TrimSpace(body.InstallName), strings.TrimSpace(body.InstallLocation)
		}
	} else {
		name = strings.TrimSpace(r.PostFormValue("install_name"))
		location = strings.TrimSpace(r.PostFormValue("install_location"))
	}
	var lat, lon float64
	hasCoords := false
	if location != "" {
		if m, ok := findMunicipio(location); ok {
			lat, lon, hasCoords = m.Lat, m.Lon, true
		}
	}
	if u.DB != nil {
		u.DB.KVSet("installName", name)
		u.DB.KVSet("installLocation", location)
		if hasCoords {
			u.DB.KVSet("installLat", strconv.FormatFloat(lat, 'f', 6, 64))
			u.DB.KVSet("installLon", strconv.FormatFloat(lon, 'f', 6, 64))
		} else {
			u.DB.KVSet("installLat", "")
			u.DB.KVSet("installLon", "")
		}
		u.DB.Audit(username, userID, "install.update", map[string]string{"name": name, "location": location})
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		// Autosave: refresh the sun line in place (the page does not reload).
		lang := u.userLang(userID)
		gen := datastar.NewSSE(w, r)
		_ = gen.PatchElementTempl(ajustesSunNote(u.installSunNote(lang, location)), datastar.WithSelectorID("sun-note"), datastar.WithModeOuter())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

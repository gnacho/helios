package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"helios-server/internal/auth"
	"helios-server/internal/db"
	"helios-server/internal/extensions"
	"helios-server/internal/install"
	"helios-server/internal/solar"
)

// ajustesTestUI builds a UI with install + extensions stores (no HA).
func ajustesTestUI(t *testing.T) (*UI, *chi.Mux) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "aj.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	a := &auth.Auth{DB: d, SessionTTL: time.Hour, SessionSecret: "test-secret"}
	if _, err := a.RegisterUser("nacho", "password1", "es", "admin"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := a.RegisterUser("pepe", "password2", "es", "user"); err != nil {
		t.Fatalf("register: %v", err)
	}

	store := install.NewStore()
	store.Set(install.ResolveInstall(d))
	extStore := extensions.NewStore()
	extStore.Set(extensions.ResolveConfig(d))

	u := New(a, d, nil, nil, store, extStore, nil, nil, solar.PriceConfig{}, nil)

	r := chi.NewRouter()
	u.Mount(r)
	r.NotFound(u.NotFound)
	return u, r
}

func doLoginAs(t *testing.T, r *chi.Mux, username, password string) *http.Cookie {
	t.Helper()
	rec := doLogin(t, r, username, password)
	return sessionCookie(t, rec)
}

func ajustesGet(t *testing.T, r *chi.Mux, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return ajustesGetPath(t, r, cookie, "/ajustes")
}

func ajustesGetPath(t *testing.T, r *chi.Mux, cookie *http.Cookie, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func ajustesPostForm(t *testing.T, r *chi.Mux, cookie *http.Cookie, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestParsePanelsStr(t *testing.T) {
	p, w := parsePanelsStr("10 × 440 W")
	if p != 10 || w != 440 {
		t.Fatalf("parsePanelsStr = %d, %d (esperado 10, 440)", p, w)
	}
	p, w = parsePanelsStr("6 × 450W")
	if p != 6 || w != 450 {
		t.Fatalf("parsePanelsStr sin espacio = %d, %d", p, w)
	}
	p, w = parsePanelsStr("")
	if p != 0 || w != 0 {
		t.Fatalf("vacío = %d, %d", p, w)
	}
}

func TestUpdateStepIndex(t *testing.T) {
	if updateStepIndex("fetch") != 0 || updateStepIndex("download") != 1 ||
		updateStepIndex("verify") != 2 || updateStepIndex("install") != 3 ||
		updateStepIndex("restart") != 4 {
		t.Fatal("índices de pasos incorrectos")
	}
	if updateStepIndex("done") != -1 || updateStepIndex("unknown") != -1 {
		t.Fatal("pasos fuera de la lista deben dar -1")
	}
}

func TestEntityRows(t *testing.T) {
	topo := install.Normalize(install.TopologyInput{
		Inverters: []install.InverterInput{{Key: "solis", Name: "Solis", PowerID: "sensor.pv", EnergyID: "sensor.pv_e"}},
		Battery:   &install.BatteryConfig{Enabled: true, PowerID: "sensor.bat_p", SocID: "sensor.bat_soc", StateID: "sensor.bat_st"},
		Grid:      &install.GridConfig{Mode: "attrs", AttrsID: strPtr("sensor.scraper")},
		Consumption: &install.ConsumptionConfig{
			PowerIDs:  []string{"sensor.house_p"},
			EnergyIDs: []string{"sensor.house_e"},
		},
		Energy: &install.EnergyConfig{},
		Sun:    "sun.sun",
	}, install.GenericTopology)
	rows := entityRows(topo)
	var got []string
	for _, r := range rows {
		got = append(got, r.Desc)
	}
	// El perfil genérico añade weather/weather_temp.
	want := []string{"desc_inverter", "desc_inverter_energy", "desc_consumption", "desc_battery_power", "desc_solis_bateria_soc", "desc_battery_state", "desc_grid_attrs", "desc_sun", "desc_weather", "desc_weather_temp"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("entityRows = %v (esperado %v)", got, want)
	}
}

func strPtr(s string) *string { return &s }

func TestAuditWhenText(t *testing.T) {
	// 2026-10-23 18:04 local.
	ts := time.Date(2026, time.October, 23, 18, 4, 0, 0, time.Local).UnixMilli()
	if got := auditWhenText("es", ts); got != "23 oct 18:04" {
		t.Fatalf("es = %q", got)
	}
	if got := auditWhenText("en", ts); got != "23 Oct 18:04" {
		t.Fatalf("en = %q", got)
	}
}

func TestAjustesPageAdminSeesAdminSections(t *testing.T) {
	_, r := ajustesTestUI(t)
	cookie := doLoginAs(t, r, "nacho", "password1")
	rec := ajustesGet(t, r, cookie)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"/ajustes/topology",                 // editor de topología
		"/ajustes/extensions",               // extensiones
		`action="/ajustes/users"`,           // admin usuarios (form de creación)
		"/api/update/status",                // check de updates
		"/api/update/progress",              // polling de progreso
		"inv.0.name",                        // campo del inverter 0
		"battery.powerId",                   // batería
		T("es", "admin.users.create"),       // crear usuario
		T("es", "update.dialog.downNotice"), // checkbox del diálogo de update
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("falta %q en la página admin", want)
		}
	}
}

func TestAjustesPageUserHidesAdminSections(t *testing.T) {
	_, r := ajustesTestUI(t)
	cookie := doLoginAs(t, r, "pepe", "password2")
	rec := ajustesGet(t, r, cookie)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"/ajustes/topology", "/ajustes/extensions", "/ajustes/users", "/api/update/status"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("un no-admin no debe ver %q", forbidden)
		}
	}
	if !strings.Contains(body, "/ajustes/profile") {
		t.Fatal("el perfil debe estar para todos")
	}
}

func TestAjustesTopologySaveRoundTrip(t *testing.T) {
	u, r := ajustesTestUI(t)
	cookie := doLoginAs(t, r, "nacho", "password1")

	form := url.Values{
		"inv.count":           {"1"},
		"inv.0.key":           {"solis"},
		"inv.0.name":          {"Solis Editado"},
		"inv.0.model":         {"S5-EH1P5K-L"},
		"inv.0.panels":        {"10 × 440 W"},
		"inv.0.kwp":           {"4.4"},
		"inv.0.powerId":       {"sensor.solis_potencia_actual"},
		"inv.0.powerUnit":     {"kW"},
		"inv.0.energyCap":     {"35"},
		"inv.0.energyId":      {"sensor.solis_energia_hoy"},
		"inv.0.energyAcc":     {"sum"},
		"inv.0.hasBattery":    {"1"},
		"inv.0.batteryKwh":    {"5"},
		"battery.enabled":     {"1"},
		"battery.powerId":     {"sensor.solis_bateria_potencia"},
		"battery.capacityKwh": {"5.12"},
		"grid.mode":           {"attrs"},
		"grid.attrsId":        {"sensor.solis_scraper"},
		"cons.powerIds":       {"sensor.medidor_respaldo_power, sensor.vivienda_medidor_power"},
		"energy.gridImportId": {"sensor.energia_red_importada_solis"},
		"env.sun":             {"sun.sun"},
		"env.weather":         {"weather.forecast_casa"},
	}
	rec := ajustesPostForm(t, r, cookie, "/ajustes/topology", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d (esperado 303 PRG)", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "saved=topology") {
		t.Fatalf("Location = %q (esperado saved=topology)", loc)
	}

	topo := u.Install.Get()
	if topo.Inverters[0].Name != "Solis Editado" {
		t.Fatalf("nombre = %q", topo.Inverters[0].Name)
	}
	if !topo.Inverters[0].HasBattery || topo.Inverters[0].BatteryKwh != 5 {
		t.Fatalf("batería del inverter = %+v", topo.Inverters[0])
	}
	if !topo.Battery.Enabled || topo.Battery.CapacityKwh != 5.12 {
		t.Fatalf("batería = %+v", topo.Battery)
	}
	if len(topo.Consumption.PowerIDs) != 2 {
		t.Fatalf("powerIds = %v", topo.Consumption.PowerIDs)
	}
	// El fox del perfil base (posición 2) no se pierde: el form solo envía 1.
	if len(topo.Inverters) != 1 {
		t.Fatalf("inverters = %d (el form manda el listado completo)", len(topo.Inverters))
	}
	// La página renderiza el flag de guardado tras el PRG (el navegador
	// seguiría el redirect con la query).
	rec = ajustesGetPath(t, r, cookie, "/ajustes?saved=topology")
	if !strings.Contains(rec.Body.String(), T("es", "ajustes.topology.savedRestart")) {
		t.Fatal("falta el banner de guardado")
	}
}

func TestAjustesTopologyForbiddenForUser(t *testing.T) {
	_, r := ajustesTestUI(t)
	cookie := doLoginAs(t, r, "pepe", "password2")
	form := url.Values{"inv.count": {"1"}, "inv.0.name": {"hack"}}
	rec := ajustesPostForm(t, r, cookie, "/ajustes/topology", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "err=forbidden") {
		t.Fatalf("Location = %q", loc)
	}
}

func TestAjustesProfileSave(t *testing.T) {
	u, r := ajustesTestUI(t)
	cookie := doLoginAs(t, r, "nacho", "password1")
	form := url.Values{
		"display_name": {"Nacho B."},
		"email":        {"nacho@example.com"},
		"language":     {"en"},
	}
	rec := ajustesPostForm(t, r, cookie, "/ajustes/profile", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	// El idioma del perfil cambia el render de la página.
	rec = ajustesGet(t, r, cookie)
	if !strings.Contains(rec.Body.String(), `lang="en"`) {
		t.Fatal("la página debería renderizar en en tras guardar el perfil")
	}
	users, _ := u.DB.ListUsers()
	var nacho *db.UserView
	for i := range users {
		if users[i].Username == "nacho" {
			nacho = &users[i]
		}
	}
	if nacho == nil || nacho.DisplayName == nil || *nacho.DisplayName != "Nacho B." {
		t.Fatalf("display_name no guardado: %+v", nacho)
	}
}

func TestAjustesPasswordMismatchRejected(t *testing.T) {
	_, r := ajustesTestUI(t)
	cookie := doLoginAs(t, r, "nacho", "password1")
	form := url.Values{"current": {"password1"}, "password": {"newpass1"}, "confirm": {"newpass2"}}
	rec := ajustesPostForm(t, r, cookie, "/ajustes/password", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "err=password") {
		t.Fatalf("Location = %q", loc)
	}
}

func TestAjustesUserCreateAndDelete(t *testing.T) {
	_, r := ajustesTestUI(t)
	cookie := doLoginAs(t, r, "nacho", "password1")

	form := url.Values{"username": {"marta"}, "password": {"secret1"}, "language": {"es"}, "role": {"user"}}
	rec := ajustesPostForm(t, r, cookie, "/ajustes/users", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create status = %d", rec.Code)
	}

	// La nueva usuaria aparece en la página y NO puede borrar a nadie.
	adminCookie := cookie
	listRec := ajustesGet(t, r, adminCookie)
	if !strings.Contains(listRec.Body.String(), "marta") {
		t.Fatal("marta no aparece en la lista")
	}

	martaCookie := doLoginAs(t, r, "marta", "secret1")
	delForm := url.Values{"confirm": {"1"}}
	rec = ajustesPostForm(t, r, martaCookie, "/ajustes/users/whatever/delete", delForm)
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "err=forbidden") {
		t.Fatalf("delete sin admin debe dar forbidden, Location = %q", loc)
	}
}

// TestAdminCannotChangeOwnRoleOrDeleteSelf: el guard comparaba username con id
// del objetivo y nunca casaba, así que el POST manual (role=user) o un DELETE
// sobre tu propio id se aplicaban.
func TestAdminCannotChangeOwnRoleOrDeleteSelf(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	_, router := buildChargerUI(t, d, extensions.NewStore(), extensions.New(extensions.NewStore(), d, nil))

	me, err := d.GetUserByUsername("nacho")
	if err != nil || me == nil {
		t.Fatalf("usuario: %v", err)
	}
	cookie := sessionCookie(t, doLogin(t, router, "nacho", "password1"))

	// Cambio de rol sobre uno mismo: se ignora.
	req := httptest.NewRequest("POST", "/ajustes/users/"+me.ID, strings.NewReader("role=user"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if after, _ := d.GetUserByID(me.ID); after == nil || after.Role != "admin" {
		t.Fatalf("se cambió el rol a uno mismo: %+v", after)
	}

	// Borrado sobre uno mismo: rechazado.
	req = httptest.NewRequest("POST", "/ajustes/users/"+me.ID+"/delete", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if after, _ := d.GetUserByID(me.ID); after == nil {
		t.Fatal("se borró a sí mismo")
	}
}

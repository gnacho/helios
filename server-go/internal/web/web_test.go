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
	"helios-server/internal/solar"
)

func testUI(t *testing.T) (*UI, *chi.Mux, *auth.Auth) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "web.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	a := &auth.Auth{DB: d, SessionTTL: time.Hour, SessionSecret: "test-secret"}
	if _, err := a.RegisterUser("nacho", "password1", "es", "admin"); err != nil {
		t.Fatalf("register: %v", err)
	}
	u := New(a, d, nil, nil, nil, nil, nil, nil, solar.PriceConfig{}, nil)

	r := chi.NewRouter()
	u.Mount(r)
	r.NotFound(u.NotFound)
	return u, r, a
}

func doLogin(t *testing.T, r *chi.Mux, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{"username": {username}, "password": {password}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName && c.Value != "" {
			return c
		}
	}
	t.Fatalf("no session cookie in login response (status=%d)", rec.Code)
	return nil
}

func TestI18n(t *testing.T) {
	if got := T("es", "login.title"); got != "Helios" {
		t.Fatalf("es login.title = %q", got)
	}
	if got := T("en", "nav.dashboard"); got != "Today" {
		t.Fatalf("en nav.dashboard = %q", got)
	}
	if got := T("zh-CN", "login.title"); got == "login.title" {
		t.Fatalf("zh-CN login.title missing")
	}
	if got := T("fr", "login.title"); got != "Helios" {
		t.Fatalf("fallback es for unknown lang = %q", got)
	}
	if got := T("es", "login.missing_key"); got != "login.missing_key" {
		t.Fatalf("missing key should return the key, got %q", got)
	}
	if got := T("es", "footer.line1", map[string]string{"version": "9.9.9"}); !strings.Contains(got, "9.9.9") {
		t.Fatalf("version replacement failed: %q", got)
	}
}

func TestLoginPageAnonymous(t *testing.T) {
	_, r, _ := testUI(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/login", nil))
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`action="/login"`, `name="username"`, `name="password"`, "Helios", `lang="es"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("login page missing %q", want)
		}
	}
}

func TestLoginSubmitOK(t *testing.T) {
	_, r, _ := testUI(t)
	rec := doLogin(t, r, "nacho", "password1")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("location = %q", loc)
	}
	if sessionCookie(t, rec) == nil {
		t.Fatal("no session cookie")
	}
}

func TestLoginSubmitBad(t *testing.T) {
	_, r, _ := testUI(t)
	rec := doLogin(t, r, "nacho", "wrong-password")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/login?err=1") {
		t.Fatalf("location = %q", loc)
	}
}

func TestLoginRateLimited(t *testing.T) {
	_, r, _ := testUI(t)
	for i := 0; i < 5; i++ {
		doLogin(t, r, "nacho", "wrong-password")
	}
	rec := doLogin(t, r, "nacho", "password1")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d (esperado 429)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Demasiados intentos") {
		t.Fatalf("body sin mensaje de rate limit: %s", rec.Body.String()[:min(300, len(rec.Body.String()))])
	}
}

func TestLoginPageWithSessionRedirects(t *testing.T) {
	_, r, _ := testUI(t)
	rec := doLogin(t, r, "nacho", "password1")
	cookie := sessionCookie(t, rec)

	req := httptest.NewRequest("GET", "/login", nil)
	req.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusFound || rec2.Header().Get("Location") != "/" {
		t.Fatalf("status=%d location=%q", rec2.Code, rec2.Header().Get("Location"))
	}
}

func TestRequirePageRedirect(t *testing.T) {
	_, r, _ := testUI(t)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/cargador", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.HasPrefix(loc, "/login?next=") || !strings.Contains(loc, "%2Fcargador") {
		t.Fatalf("location = %q", loc)
	}
}

func TestStubPages(t *testing.T) {
	_, r, _ := testUI(t)
	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))

	// /ajustes is a real page since phase 5 (covered by ajustes_test): the
	// nav has no stub left; it must render the real page, not the placeholder.
	req := httptest.NewRequest("GET", "/ajustes", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Ajustes") {
		t.Fatalf("falta el título %q", "Ajustes")
	}
	if strings.Contains(body, "Esta vista está en construcción") {
		t.Fatal("/ajustes ya no es un stub")
	}
}

// TestInversoresPage: the real page renders without HA/install (empty
// fallback topology) with tabs, hero and charts.
func TestInversoresPage(t *testing.T) {
	_, r, _ := testUI(t)
	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))

	cases := []struct {
		name string
		url  string
		want []string
	}{
		{"compare por defecto", "/inversores", []string{"Comparativa", "Solis", "Fox", "Métricas de hoy", "Reparto de hoy", "grad-cmp-solis"}},
		{"tab por inversor", "/inversores?inv=solis", []string{"Solis S5-EH1P5K-L", "Produciendo ahora", "Últimos 14 días", "grad-inv-solis"}},
		{"alias tab=", "/inversores?tab=fox", []string{"Fox H1-3.0-E", "grad-inv-fox"}},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", c.url, nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s: status = %d", c.name, rec.Code)
		}
		body := rec.Body.String()
		for _, want := range c.want {
			if !strings.Contains(body, want) {
				t.Fatalf("%s: falta %q", c.name, want)
			}
		}
	}
}

// TestBateriaPage: gauge and charts render without HA data; the donut only
// appears with consumption (covered by buildDonut unit tests).
func TestBateriaPage(t *testing.T) {
	_, r, _ := testUI(t)
	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))

	req := httptest.NewRequest("GET", "/bateria", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"En reposo",    // chip inicial sin live
		"Cargada hoy",  // stats
		"grad-soc-day", // curva SOC
		"Reserva",      // gauge tick
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("falta %q en /bateria", want)
		}
	}
	if strings.Contains(body, "¿De dónde vino tu energía?") {
		t.Fatal("el donut no debería renderizar sin datos de consumo")
	}
}

func TestLanguageFromProfile(t *testing.T) {
	u, r, a := testUI(t)
	if _, err := a.RegisterUser("carlos", "password1", "en", "user"); err != nil {
		t.Fatalf("register: %v", err)
	}
	cookie := sessionCookie(t, doLogin(t, r, "carlos", "password1"))

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "Today") {
		t.Fatalf("la página no está en inglés según el perfil: %s", body[:min(400, len(body))])
	}
	if strings.Contains(body, "Hoy") {
		t.Fatalf("aparece el título en español con perfil en")
	}
	_ = u
}

func TestNotFoundPage(t *testing.T) {
	_, r, _ := testUI(t)
	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))

	req := httptest.NewRequest("GET", "/no-existe", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "La vista que buscas no existe.") {
		t.Fatal("falta el texto 404")
	}

	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest("GET", "/no-existe", nil))
	if rec2.Code != http.StatusFound {
		t.Fatalf("anónimo: status = %d (esperado redirect a login)", rec2.Code)
	}
}

func TestThemeToggle(t *testing.T) {
	_, r, _ := testUI(t)
	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))

	form := url.Values{"theme": {"dark"}, "next": {"/"}}
	req := httptest.NewRequest("POST", "/theme", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	var themeSet *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == themeCookie {
			themeSet = c
		}
	}
	if themeSet == nil || themeSet.Value != "dark" {
		t.Fatal("no se fijó la cookie de tema")
	}

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.AddCookie(cookie)
	req2.AddCookie(themeSet)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if !strings.Contains(rec2.Body.String(), `class="dark"`) {
		t.Fatal("el html no lleva la clase dark")
	}
}

func TestLogout(t *testing.T) {
	_, r, _ := testUI(t)
	cookie := sessionCookie(t, doLogin(t, r, "nacho", "password1"))

	req := httptest.NewRequest("POST", "/logout", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	var cleared bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("la cookie de sesión no se limpió")
	}

	req2 := httptest.NewRequest("GET", "/", nil)
	req2.AddCookie(cookie)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusFound {
		t.Fatalf("la sesión sigue viva tras logout (status %d)", rec2.Code)
	}
}

func TestAssetsServed(t *testing.T) {
	_, r, _ := testUI(t)
	cases := []struct {
		path    string
		ctPart  string
		bodyHas string
	}{
		{"/assets/css/app.build.css", "text/css", "helios-card"},
		{"/assets/js/datastar.js", "javascript", "Datastar"},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", c.path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s: status = %d", c.path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, c.ctPart) {
			t.Fatalf("%s: content-type = %q", c.path, ct)
		}
		if !strings.Contains(rec.Body.String(), c.bodyHas) {
			t.Fatalf("%s: body sin %q", c.path, c.bodyHas)
		}
		if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age") {
			t.Fatalf("%s: sin cache-control", c.path)
		}
	}
}

func TestSafeNext(t *testing.T) {
	cases := map[string]string{
		"":           "/",
		"/cargador":  "/cargador",
		"//evil.com": "/",
		"http://x":   "/",
		"/a?b=c":     "/a?b=c",
	}
	for in, want := range cases {
		if got := safeNext(in); got != want {
			t.Fatalf("safeNext(%q) = %q (esperado %q)", in, got, want)
		}
	}
}

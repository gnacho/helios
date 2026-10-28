package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"helios-server/internal/auth"
	"helios-server/internal/extensions"
	"helios-server/internal/install"
	"helios-server/internal/solar"
	"helios-server/internal/web"
)

const (
	securityAdminUser = "security-admin"
	securityAdminPass = "admin-password-1"
	securityUser      = "security-user"
	securityUserPass  = "user-password-1"
)

func newSecurityRouter(t *testing.T) (*Server, *auth.Auth, chi.Router) {
	t.Helper()
	d := testDB(t)
	a := &auth.Auth{DB: d, SessionTTL: time.Hour, SessionSecret: "security-test-secret"}
	if _, err := a.RegisterUser(securityAdminUser, securityAdminPass, "es", "admin"); err != nil {
		t.Fatalf("register admin: %v", err)
	}
	if _, err := a.RegisterUser(securityUser, securityUserPass, "es", "user"); err != nil {
		t.Fatalf("register user: %v", err)
	}
	installStore := install.NewStore()
	installStore.Set(install.ResolveInstall(d))
	extStore := extensions.NewStore()
	extStore.Set(extensions.ResolveConfig(d))
	sol := solar.New(nil, nil)
	ext := extensions.New(extStore, d, nil)
	srv := NewServer(a, d, t.TempDir(), sol, nil, ext, extStore, nil, solar.PriceConfig{}, installStore)
	srv.SSE = httpNoop()
	srv.UI = web.New(a, d, sol, nil, installStore, extStore, ext, nil, solar.PriceConfig{}, nil)
	return srv, a, srv.Routes()
}

func securityLogin(t *testing.T, router chi.Router, username, password string) *http.Cookie {
	t.Helper()
	body, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		t.Fatalf("marshal login: %v", err)
	}
	rec := securityRequest(t, router, http.MethodPost, "/api/auth/login", nil, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == auth.CookieName {
			return cookie
		}
	}
	t.Fatal("login sin cookie de sesión")
	return nil
}

func securityRequest(t *testing.T, router chi.Router, method, path string, cookie *http.Cookie, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func securitySessionCount(t *testing.T, a *auth.Auth, userID string) int {
	t.Helper()
	var count int
	if err := a.DB.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, userID).Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	return count
}

func securityOrphanCookie(t *testing.T, router chi.Router, a *auth.Auth) *http.Cookie {
	t.Helper()
	cookie := securityLogin(t, router, securityUser, securityUserPass)
	user, err := a.DB.GetUserByUsername(securityUser)
	if err != nil || user == nil {
		t.Fatalf("load user: %v", err)
	}
	if _, err := a.DB.Exec(`DELETE FROM users WHERE id = ?`, user.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	return cookie
}

func TestRoutesRejectOversizedLoginBody(t *testing.T) {
	_, _, router := newSecurityRouter(t)
	body := bytes.Repeat([]byte("a"), (1<<20)+1)
	rec := securityRequest(t, router, http.MethodPost, "/api/auth/login", nil, body)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestRoutesRejectChunkedOversizedLoginBody(t *testing.T) {
	_, _, router := newSecurityRouter(t)
	var body bytes.Buffer
	body.WriteString(`{"username":"`)
	body.Write(bytes.Repeat([]byte("a"), (1<<20)+1))
	body.WriteString(`","password":"x"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body.Bytes()))
	req.ContentLength = -1
	req.TransferEncoding = []string{"chunked"}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestPutConfigRejectsUnsafeInverterKey(t *testing.T) {
	_, _, router := newSecurityRouter(t)
	adminCookie := securityLogin(t, router, securityAdminUser, securityAdminPass)
	body := []byte(`{"topology":{"inverters":[{"key":"x[(console.info(1),'x')]","name":"unsafe"}]}}`)
	rec := securityRequest(t, router, http.MethodPut, "/api/config", adminCookie, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestPutConfigRequiresAdmin(t *testing.T) {
	_, _, router := newSecurityRouter(t)
	adminCookie := securityLogin(t, router, securityAdminUser, securityAdminPass)
	userCookie := securityLogin(t, router, securityUser, securityUserPass)
	body := []byte(`{"securityAuditMarker":"nonadmin-write"}`)
	rec := securityRequest(t, router, http.MethodPut, "/api/config", userCookie, body)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	rec = securityRequest(t, router, http.MethodGet, "/api/config", adminCookie, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get config status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var cfg map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if _, ok := cfg["securityAuditMarker"]; ok {
		t.Fatal("non-admin config write was persisted")
	}
}

func TestListUsersRequiresAdmin(t *testing.T) {
	_, _, router := newSecurityRouter(t)
	userCookie := securityLogin(t, router, securityUser, securityUserPass)
	rec := securityRequest(t, router, http.MethodGet, "/api/auth/users", userCookie, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestAuditRequiresAdmin(t *testing.T) {
	_, _, router := newSecurityRouter(t)
	userCookie := securityLogin(t, router, securityUser, securityUserPass)
	rec := securityRequest(t, router, http.MethodGet, "/api/auth/audit", userCookie, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestAdminCannotChangeOwnRole(t *testing.T) {
	_, a, router := newSecurityRouter(t)
	adminCookie := securityLogin(t, router, securityAdminUser, securityAdminPass)
	admin, err := a.DB.GetUserByUsername(securityAdminUser)
	if err != nil || admin == nil {
		t.Fatalf("load admin: %v", err)
	}
	rec := securityRequest(t, router, http.MethodPut, "/api/auth/users/"+admin.ID+"/role", adminCookie, []byte(`{"role":"user"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	stored, err := a.DB.GetUserByID(admin.ID)
	if err != nil || stored == nil {
		t.Fatalf("reload admin: %v", err)
	}
	if stored.Role != "admin" {
		t.Fatalf("role = %q, want admin", stored.Role)
	}
}

func TestDeleteUserRevokesSessions(t *testing.T) {
	_, a, router := newSecurityRouter(t)
	adminCookie := securityLogin(t, router, securityAdminUser, securityAdminPass)
	userCookie := securityLogin(t, router, securityUser, securityUserPass)
	user, err := a.DB.GetUserByUsername(securityUser)
	if err != nil || user == nil {
		t.Fatalf("load user: %v", err)
	}
	if got := securitySessionCount(t, a, user.ID); got != 1 {
		t.Fatalf("sessions before delete = %d, want 1", got)
	}
	rec := securityRequest(t, router, http.MethodDelete, "/api/auth/users/"+user.ID, adminCookie, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = securityRequest(t, router, http.MethodGet, "/api/config", userCookie, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old session status = %d, want %d, body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	if got := securitySessionCount(t, a, user.ID); got != 0 {
		t.Fatalf("sessions after delete = %d, want 0", got)
	}
}

func TestAdminPasswordResetRevokesSessions(t *testing.T) {
	_, a, router := newSecurityRouter(t)
	adminCookie := securityLogin(t, router, securityAdminUser, securityAdminPass)
	userCookie := securityLogin(t, router, securityUser, securityUserPass)
	user, err := a.DB.GetUserByUsername(securityUser)
	if err != nil || user == nil {
		t.Fatalf("load user: %v", err)
	}
	if got := securitySessionCount(t, a, user.ID); got != 1 {
		t.Fatalf("sessions before reset = %d, want 1", got)
	}
	body := []byte(`{"password":"new-password-1"}`)
	rec := securityRequest(t, router, http.MethodPut, "/api/auth/users/"+user.ID+"/password", adminCookie, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = securityRequest(t, router, http.MethodGet, "/api/config", userCookie, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old session status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := securitySessionCount(t, a, user.ID); got != 0 {
		t.Fatalf("sessions after reset = %d, want 0", got)
	}
}

func TestRequireAuthRejectsOrphanSession(t *testing.T) {
	_, a, router := newSecurityRouter(t)
	cookie := securityOrphanCookie(t, router, a)
	rec := securityRequest(t, router, http.MethodGet, "/api/config", cookie, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestRequireAuthRedirectRejectsOrphanSession(t *testing.T) {
	_, a, router := newSecurityRouter(t)
	cookie := securityOrphanCookie(t, router, a)
	rec := securityRequest(t, router, http.MethodGet, "/", cookie, nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if location := rec.Header().Get("Location"); !strings.HasPrefix(location, "/login?next=") {
		t.Fatalf("location = %q, want login redirect", location)
	}
}

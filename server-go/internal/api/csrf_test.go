package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func securityRequestOrigin(t *testing.T, router chi.Router, method, path string, cookie *http.Cookie, origin, referer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Host = "helios.example"
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestMutatingRequestRejectsCrossOrigin(t *testing.T) {
	cases := []struct {
		name    string
		origin  string
		referer string
	}{
		{name: "origin distinto", origin: "https://evil.example"},
		{name: "origin nulo", origin: "null"},
		{name: "referer distinto sin origin", referer: "https://evil.example/page"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, router := newSecurityRouter(t)
			cookie := securityLogin(t, router, securityUser, securityUserPass)
			rec := securityRequestOrigin(t, router, http.MethodPost, "/api/auth/logout", cookie, tc.origin, tc.referer)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusForbidden)
			}
			check := securityRequestOrigin(t, router, http.MethodGet, "/api/config", cookie, "", "")
			if check.Code != http.StatusOK {
				t.Fatalf("la sesión se perdió tras rechazar el cruce: status = %d", check.Code)
			}
		})
	}
}

func TestMutatingRequestAllowsSameOriginAndClientsWithoutOrigin(t *testing.T) {
	cases := []struct {
		name    string
		origin  string
		referer string
	}{
		{name: "mismo origin", origin: "https://helios.example"},
		{name: "mismo referer sin origin", referer: "https://helios.example/page"},
		{name: "cliente sin cabeceras"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, router := newSecurityRouter(t)
			cookie := securityLogin(t, router, securityUser, securityUserPass)
			rec := securityRequestOrigin(t, router, http.MethodPost, "/api/auth/logout", cookie, tc.origin, tc.referer)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
			}
		})
	}
}

func TestGetIgnoresCrossOriginHeaders(t *testing.T) {
	_, _, router := newSecurityRouter(t)
	cookie := securityLogin(t, router, securityUser, securityUserPass)
	rec := securityRequestOrigin(t, router, http.MethodGet, "/api/config", cookie, "https://evil.example", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}
}

// TestMutatingRequestAcceptsProxyHosts cubre el caso real: el navegador firma el
// host público y el proxy entrega la petición con su propio Host (o con el
// público en X-Forwarded-Host).
func TestMutatingRequestAcceptsProxyHosts(t *testing.T) {
	cases := []struct {
		name    string
		host    string
		xfh     string
		origin  string
		allowed bool
	}{
		{name: "host reescrito por el proxy", host: "10.0.2.6:80", xfh: "panelsolar.duckdns.org",
			origin: "https://panelsolar.duckdns.org", allowed: true},
		{name: "puerto en el host de la peticion", host: "helios.example:8080",
			origin: "https://helios.example", allowed: true},
		{name: "puerto en el origin", host: "helios.example",
			origin: "https://helios.example:443", allowed: true},
		{name: "sitio distinto sigue rechazado", host: "10.0.2.6:80", xfh: "panelsolar.duckdns.org",
			origin: "https://evil.example", allowed: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, router := newSecurityRouter(t)
			cookie := securityLogin(t, router, securityUser, securityUserPass)

			req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
			req.Host = tc.host
			if tc.xfh != "" {
				req.Header.Set("X-Forwarded-Host", tc.xfh)
			}
			req.Header.Set("Origin", tc.origin)
			req.AddCookie(cookie)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			want := http.StatusForbidden
			if tc.allowed {
				want = http.StatusOK
			}
			if rec.Code != want {
				t.Fatalf("status = %d, esperado %d (body=%s)", rec.Code, want, rec.Body.String())
			}
		})
	}
}

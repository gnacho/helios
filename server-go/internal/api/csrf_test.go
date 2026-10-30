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

func TestMutatingRequestAllowsSameSiteBehindReverseProxy(t *testing.T) {
	cases := []struct {
		name          string
		host          string
		forwardedHost string
		origin        string
		referer       string
	}{
		{name: "origin con puerto distinto", origin: "https://helios.example:8443"},
		{name: "referer con puerto distinto", referer: "https://helios.example:8443/page"},
		{name: "host interno y x-forwarded-host", host: "10.0.0.5:8080", forwardedHost: "helios.example", origin: "https://helios.example"},
		{name: "x-forwarded-host en cadena", host: "10.0.0.5:8080", forwardedHost: "helios.example, edge.local", origin: "https://helios.example"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, router := newSecurityRouter(t)
			cookie := securityLogin(t, router, securityUser, securityUserPass)

			req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
			req.Host = "helios.example"
			if tc.host != "" {
				req.Host = tc.host
			}
			req.AddCookie(cookie)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.referer != "" {
				req.Header.Set("Referer", tc.referer)
			}
			if tc.forwardedHost != "" {
				req.Header.Set("X-Forwarded-Host", tc.forwardedHost)
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
			}
		})
	}
}

func TestMutatingRequestRejectsCrossOriginBehindReverseProxy(t *testing.T) {
	_, _, router := newSecurityRouter(t)
	cookie := securityLogin(t, router, securityUser, securityUserPass)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.Host = "10.0.0.5:8080"
	req.AddCookie(cookie)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("X-Forwarded-Host", "helios.example")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusForbidden)
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

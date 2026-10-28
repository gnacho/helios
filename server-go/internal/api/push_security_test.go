package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"helios-server/internal/push"
)

func pushSecurityRouter(t *testing.T) chi.Router {
	t.Helper()
	srv, _, _ := newSecurityRouter(t)
	srv.Push = push.New(srv.DB, "pub", "priv", "mailto:test@example.com")
	return srv.Routes()
}

func pushSubscribeRequest(t *testing.T, router chi.Router, cookie *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()
	return pushRequest(t, router, http.MethodPost, "/api/push/subscribe", cookie, body)
}

func pushUnsubscribeRequest(t *testing.T, router chi.Router, cookie *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()
	return pushRequest(t, router, http.MethodDelete, "/api/push/unsubscribe", cookie, body)
}

func pushRequest(t *testing.T, router chi.Router, method, path string, cookie *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestPushSubscribeRejectsUnsafeEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
	}{
		{name: "http plano", endpoint: "http://push.example/sub"},
		{name: "loopback", endpoint: "https://127.0.0.1/sub"},
		{name: "red privada", endpoint: "https://192.168.1.10/sub"},
		{name: "metadata", endpoint: "https://169.254.169.254/latest/meta-data"},
		{name: "userinfo", endpoint: "https://user:pass@push.example/sub"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := pushSecurityRouter(t)
			cookie := securityLogin(t, router, securityUser, securityUserPass)
			rec := pushSubscribeRequest(t, router, cookie, map[string]any{
				"endpoint": tc.endpoint,
				"keys":     map[string]string{"p256dh": "p256dh-value", "auth": "auth-value"},
			})
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestPushSubscribeAcceptsPublicEndpoint(t *testing.T) {
	router := pushSecurityRouter(t)
	cookie := securityLogin(t, router, securityUser, securityUserPass)
	rec := pushSubscribeRequest(t, router, cookie, map[string]any{
		"endpoint": "https://push.example/sub",
		"keys":     map[string]string{"p256dh": "p256dh-value", "auth": "auth-value"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusCreated)
	}
}

func TestPushUnsubscribeStillAcceptsStoredLegacyEndpoint(t *testing.T) {
	router := pushSecurityRouter(t)
	cookie := securityLogin(t, router, securityUser, securityUserPass)
	rec := pushUnsubscribeRequest(t, router, cookie, map[string]any{
		"endpoint": "http://legacy.example/sub",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}
}

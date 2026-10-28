package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponsesCarryDefensiveHeaders(t *testing.T) {
	_, _, router := newSecurityRouter(t)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/version", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}
	headers := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for name, want := range headers {
		if got := rec.Header().Get(name); got != want {
			t.Fatalf("%s = %q, esperado %q", name, got, want)
		}
	}
	if got := rec.Header().Get("Permissions-Policy"); got == "" {
		t.Fatal("falta Permissions-Policy")
	}
}

func pngDataURL(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestAvatarIsNotPubliclyCacheable(t *testing.T) {
	a, _, router := newSecurityRouter(t)
	cookie := securityLogin(t, router, securityUser, securityUserPass)
	user, err := a.DB.GetUserByUsername(securityUser)
	if err != nil {
		t.Fatalf("usuario de prueba: %v", err)
	}

	payload, err := json.Marshal(map[string]any{"avatar": pngDataURL(t)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	upload := securityRequest(t, router, http.MethodPut, "/api/auth/profile/avatar", cookie, payload)
	if upload.Code != http.StatusOK {
		t.Fatalf("upload status = %d, esperado %d, body: %s", upload.Code, http.StatusOK, upload.Body.String())
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/avatar/"+user.ID, nil)
	req.AddCookie(cookie)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusOK)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, max-age=3600" {
		t.Fatalf("Cache-Control = %q, esperado %q", cc, "private, max-age=3600")
	}
}

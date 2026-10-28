package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := 0; x < 4; x++ {
		for y := 0; y < 4; y++ {
			img.Set(x, y, color.RGBA{R: 10, G: 120, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func avatarBody(t *testing.T, dataURL string) *bytes.Buffer {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"avatar": dataURL})
	if err != nil {
		t.Fatalf("marshal avatar: %v", err)
	}
	return bytes.NewBuffer(payload)
}

func TestAvatarRejectsNonImagePayload(t *testing.T) {
	a, _, router := newSecurityRouter(t)
	cookie := securityLogin(t, router, securityUser, securityUserPass)

	me, err := a.DB.GetUserByUsername(securityUser)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}

	forged := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("<svg onload=alert(1)>"))
	req := httptest.NewRequest(http.MethodPut, "/api/auth/profile/avatar", avatarBody(t, forged))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusBadRequest)
	}

	get := httptest.NewRequest(http.MethodGet, "/api/auth/avatar/"+me.ID, nil)
	get.AddCookie(cookie)
	got := httptest.NewRecorder()
	router.ServeHTTP(got, get)
	if got.Code == http.StatusOK && strings.Contains(got.Body.String(), "onload") {
		t.Fatal("el servidor guardó y sirvió el payload no imagen")
	}
}

func TestAvatarRejectsMismatchedDeclaredType(t *testing.T) {
	_, _, router := newSecurityRouter(t)
	cookie := securityLogin(t, router, securityUser, securityUserPass)

	mislabeled := "data:image/webp;base64," + base64.StdEncoding.EncodeToString(tinyPNG(t))
	req := httptest.NewRequest(http.MethodPut, "/api/auth/profile/avatar", avatarBody(t, mislabeled))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAvatarStoresValidImage(t *testing.T) {
	a, _, router := newSecurityRouter(t)
	cookie := securityLogin(t, router, securityUser, securityUserPass)

	me, err := a.DB.GetUserByUsername(securityUser)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}

	valid := "data:image/png;base64," + base64.StdEncoding.EncodeToString(tinyPNG(t))
	req := httptest.NewRequest(http.MethodPut, "/api/auth/profile/avatar", avatarBody(t, valid))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, esperado %d (%s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/api/auth/avatar/"+me.ID, nil)
	get.AddCookie(cookie)
	got := httptest.NewRecorder()
	router.ServeHTTP(got, get)
	if got.Code != http.StatusOK {
		t.Fatalf("download status = %d", got.Code)
	}
	if ct := got.Header().Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
		t.Fatalf("content-type = %q", ct)
	}
	if _, _, err := image.DecodeConfig(bytes.NewReader(got.Body.Bytes())); err != nil {
		t.Fatalf("la respuesta no es una imagen decodificable: %v", err)
	}
	if !strings.Contains(got.Header().Get("Cache-Control"), "private") {
		fmt.Println("cache:", got.Header().Get("Cache-Control"))
	}
}

package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"helios-server/internal/db"
)

func newTokenTestAuth(t *testing.T) (*Auth, string) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "tokens.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	u, err := d.CreateUser("nacho", "hash", "es", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	a := &Auth{DB: d, SessionTTL: time.Hour, SessionSecret: "test-secret"}
	return a, u.ID
}

func issueToken(t *testing.T, a *Auth, userID, scopes string, expiresAt int64) (plain string, id string) {
	t.Helper()
	plain, prefix, hash, err := NewAPIToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	row, err := a.DB.CreateAPIToken(userID, "widget", prefix, hash, scopes, expiresAt)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return plain, row.ID
}

func TestNewAPITokenFormat(t *testing.T) {
	plain, prefix, hash, err := NewAPIToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	p, secret, ok := ParseAPIToken(plain)
	if !ok {
		t.Fatalf("token %q does not parse", plain)
	}
	if p != prefix {
		t.Fatalf("prefix = %q, want %q", p, prefix)
	}
	if hash != HashTokenSecret(secret) {
		t.Fatal("stored hash does not match the secret")
	}
	if strings.Contains(hash, secret) {
		t.Fatal("hash contains the secret")
	}

	second, _, _, err := NewAPIToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if second == plain {
		t.Fatal("two generated tokens are identical")
	}
}

func TestParseAPITokenRejectsGarbage(t *testing.T) {
	for _, in := range []string{"", "abc", "hx_only", "hx__secret", "hx_prefix_", "xx_prefix_secret", "hx_a_b_c"} {
		if _, _, ok := ParseAPIToken(in); ok {
			t.Fatalf("%q parsed as a valid token", in)
		}
	}
}

func TestNormalizeScopes(t *testing.T) {
	got, err := NormalizeScopes([]string{"read"})
	if err != nil || got != "read" {
		t.Fatalf("read: %q %v", got, err)
	}
	got, err = NormalizeScopes([]string{"write"})
	if err != nil || got != "read,write" {
		t.Fatalf("write should imply read: %q %v", got, err)
	}
	if _, err := NormalizeScopes([]string{"admin"}); err == nil {
		t.Fatal("unknown scope accepted")
	}
	if _, err := NormalizeScopes(nil); err == nil {
		t.Fatal("empty scope accepted")
	}
}

func TestAuthenticateToken(t *testing.T) {
	a, userID := newTokenTestAuth(t)
	plain, id := issueToken(t, a, userID, "read", 0)

	row, err := a.AuthenticateToken(plain)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if row.ID != id {
		t.Fatalf("id = %q, want %q", row.ID, id)
	}

	if _, err := a.AuthenticateToken("hx_000000000000_" + strings.Repeat("A", 43)); err != ErrTokenInvalid {
		t.Fatalf("unknown prefix error = %v", err)
	}
	parts := strings.Split(plain, "_")
	if _, err := a.AuthenticateToken("hx_" + parts[1] + "_" + strings.Repeat("B", 43)); err != ErrTokenInvalid {
		t.Fatalf("wrong secret error = %v", err)
	}
	if _, err := a.AuthenticateToken("basura"); err != ErrTokenInvalid {
		t.Fatalf("garbage error = %v", err)
	}
}

func TestAuthenticateTokenRevokedAndExpired(t *testing.T) {
	a, userID := newTokenTestAuth(t)

	revoked, revokedID := issueToken(t, a, userID, "read", 0)
	if _, err := a.DB.RevokeAPIToken(revokedID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := a.AuthenticateToken(revoked); err != ErrTokenRevoked {
		t.Fatalf("revoked error = %v", err)
	}

	expired, _ := issueToken(t, a, userID, "read", time.Now().Add(-time.Minute).UnixMilli())
	if _, err := a.AuthenticateToken(expired); err != ErrTokenExpired {
		t.Fatalf("expired error = %v", err)
	}
}

func TestRequireAuthWithBearer(t *testing.T) {
	a, userID := newTokenTestAuth(t)
	readToken, readID := issueToken(t, a, userID, "read", 0)
	writeToken, _ := issueToken(t, a, userID, "read,write", 0)
	revokedToken, revokedID := issueToken(t, a, userID, "read,write", 0)
	if _, err := a.DB.RevokeAPIToken(revokedID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	expiredToken, _ := issueToken(t, a, userID, "read,write", time.Now().Add(-time.Hour).UnixMilli())

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := UserIDFrom(r.Context()); !ok || id != userID {
			t.Errorf("context user = %q ok=%v", id, ok)
		}
		w.WriteHeader(http.StatusOK)
	})
	guarded := a.RequireAuth(okHandler)

	cases := []struct {
		name   string
		method string
		path   string
		token  string
		want   int
	}{
		{"read token on GET", "GET", "/api/solar/live", readToken, http.StatusOK},
		{"read token on POST", "POST", "/api/charger/action", readToken, http.StatusForbidden},
		{"write token on POST", "POST", "/api/charger/action", writeToken, http.StatusOK},
		{"write token on PUT config", "PUT", "/api/config", writeToken, http.StatusOK},
		{"read token on GET config", "GET", "/api/config", readToken, http.StatusOK},
		{"token on token management", "GET", "/api/auth/tokens", writeToken, http.StatusForbidden},
		{"token on user management", "GET", "/api/auth/users", writeToken, http.StatusForbidden},
		{"token on updater", "POST", "/api/update/apply", writeToken, http.StatusForbidden},
		{"revoked token", "GET", "/api/solar/live", revokedToken, http.StatusUnauthorized},
		{"expired token", "GET", "/api/solar/live", expiredToken, http.StatusUnauthorized},
		{"garbage token", "GET", "/api/solar/live", "hx_nope_nope", http.StatusUnauthorized},
		{"no credential", "GET", "/api/solar/live", "", http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			guarded.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tc.want, w.Body.String())
			}
		})
	}

	// Touching the row must not break the next request.
	if got := recordOf(t, a, readID); got == "" {
		t.Fatal("last_used_at not persisted")
	}
}

func TestRequireAuthCookieStillWorks(t *testing.T) {
	a, userID := newTokenTestAuth(t)
	sessionID, err := a.DB.CreateSession(userID, time.Hour.Milliseconds(), "test")
	if err != nil {
		t.Fatalf("session: %v", err)
	}

	guarded := a.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if TokenFrom(r.Context()) != nil {
			t.Error("cookie request carried token info")
		}
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest("POST", "/api/charger/action", nil)
	r.AddCookie(&http.Cookie{Name: CookieName, Value: sessionID + "." + mustSign(a, sessionID)})
	w := httptest.NewRecorder()
	guarded.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
}

func recordOf(t *testing.T, a *Auth, id string) string {
	t.Helper()
	row, err := a.DB.GetAPIToken(id)
	if err != nil || row == nil {
		t.Fatalf("get token: %v", err)
	}
	if row.LastUsedAt == 0 {
		return ""
	}
	return "used"
}

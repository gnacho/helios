package auth

import (
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"helios-server/internal/db"
)

func TestLoginRateLimitCannotBeBypassedWithXFF(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	a := &Auth{DB: d, SessionTTL: time.Hour, SessionSecret: "test-secret"}

	for i := 0; i < 5; i++ {
		r := httptest.NewRequest("POST", "/api/auth/login", nil)
		r.RemoteAddr = "198.51.100.7:4321"
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", 10+i))
		if err := a.RegisterLoginFail(r); err != nil {
			t.Fatalf("register failure %d: %v", i, err)
		}
	}

	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.RemoteAddr = "198.51.100.7:4321"
	r.Header.Set("X-Forwarded-For", "203.0.113.99")
	if !a.LoginRateLimited(r) {
		t.Fatal("login rate limit bypassed by changing X-Forwarded-For")
	}
}

func TestRegisterLoginFailPrunesStaleAttempts(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	staleIP := "198.51.100.10"
	stale := time.Now().Add(-25 * time.Hour).UnixMilli()
	if _, err := d.Exec(`INSERT INTO login_attempts (ip, attempts, locked_until) VALUES (?, 2, ?)`, staleIP, stale); err != nil {
		t.Fatalf("insert stale attempt: %v", err)
	}

	a := &Auth{DB: d, SessionTTL: time.Hour, SessionSecret: "test-secret"}
	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.RemoteAddr = "198.51.100.11:4321"
	if err := a.RegisterLoginFail(r); err != nil {
		t.Fatalf("register failure: %v", err)
	}

	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM login_attempts WHERE ip = ?`, staleIP).Scan(&count); err != nil {
		t.Fatalf("count stale attempts: %v", err)
	}
	if count != 0 {
		t.Fatalf("stale login attempts = %d, want 0", count)
	}
}

func TestLoginRateLimitResetsAfterWindow(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	ip := "198.51.100.12"
	expired := time.Now().Add(-time.Minute).UnixMilli()
	if _, err := d.Exec(`INSERT INTO login_attempts (ip, attempts, locked_until) VALUES (?, 5, ?)`, ip, expired); err != nil {
		t.Fatalf("insert expired attempt: %v", err)
	}

	a := &Auth{DB: d, SessionTTL: time.Hour, SessionSecret: "test-secret"}
	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.RemoteAddr = ip + ":4321"
	if a.LoginRateLimited(r) {
		t.Fatal("expired window still locked")
	}
	if err := a.RegisterLoginFail(r); err != nil {
		t.Fatalf("register failure: %v", err)
	}
	if a.LoginRateLimited(r) {
		t.Fatal("one failure after expiry immediately re-locked the IP")
	}
}

func TestClientIPUsesForwardedForBehindTrustedProxy(t *testing.T) {
	networks, err := ParseTrustedProxies([]string{"127.0.0.1"})
	if err != nil {
		t.Fatalf("parse trusted proxies: %v", err)
	}
	a := &Auth{TrustedProxies: networks}
	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.RemoteAddr = "127.0.0.1:4321"
	r.Header.Set("X-Forwarded-For", "198.51.100.7")

	if got := a.clientIP(r); got != "198.51.100.7" {
		t.Fatalf("clientIP = %q, want %q", got, "198.51.100.7")
	}
}

func TestClientIPSkipsTrustedProxyHops(t *testing.T) {
	networks, err := ParseTrustedProxies([]string{"127.0.0.1", "10.0.0.0/8"})
	if err != nil {
		t.Fatalf("parse trusted proxies: %v", err)
	}
	a := &Auth{TrustedProxies: networks}
	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.RemoteAddr = "127.0.0.1:4321"
	r.Header.Set("X-Forwarded-For", "198.51.100.7, 10.1.2.3")

	if got := a.clientIP(r); got != "198.51.100.7" {
		t.Fatalf("clientIP = %q, want %q", got, "198.51.100.7")
	}
}

func TestClientIPFallsBackOnMalformedChain(t *testing.T) {
	networks, err := ParseTrustedProxies([]string{"127.0.0.1"})
	if err != nil {
		t.Fatalf("parse trusted proxies: %v", err)
	}
	a := &Auth{TrustedProxies: networks}
	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.RemoteAddr = "127.0.0.1:4321"
	r.Header.Set("X-Forwarded-For", "198.51.100.7, not-an-ip")

	if got := a.clientIP(r); got != "127.0.0.1" {
		t.Fatalf("clientIP = %q, want %q", got, "127.0.0.1")
	}
}

func TestParseTrustedProxiesRejectsInvalidEntry(t *testing.T) {
	if _, err := ParseTrustedProxies([]string{"not-an-ip"}); err == nil {
		t.Fatal("ParseTrustedProxies accepted an invalid entry")
	}
}

package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"helios-server/internal/db"
)

const CookieName = "helios_session"

type Auth struct {
	DB             *db.DB
	SessionTTL     time.Duration
	SessionSecret  string // if empty, read from kv
	TrustedProxies []*net.IPNet
}

type PublicUser struct {
	ID       string  `json:"id"`
	Username string  `json:"username"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
	Language string  `json:"language"`
	Role     string  `json:"role"`
}

func (a *Auth) secret() (string, error) {
	if a.SessionSecret != "" {
		return a.SessionSecret, nil
	}
	s, err := a.DB.KVGet("session_secret")
	if err != nil {
		return "", err
	}
	if s != "" {
		return s, nil
	}
	// generate
	b := make([]byte, 32)
	// crypto/rand
	_, _ = readRandom(b)
	s = hex.EncodeToString(b)
	if err := a.DB.KVSet("session_secret", s); err != nil {
		return "", err
	}
	return s, nil
}

func readRandom(b []byte) (int, error) {
	// wrapper to allow stubbing in tests if needed; delegates to crypto/rand
	return randRead(b)
}

func (a *Auth) sign(id string) (string, error) {
	secret, err := a.secret()
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(id))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func safeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return hmac.Equal([]byte(a), []byte(b))
}

// ParseTrustedProxies accepts IP addresses and CIDR ranges. A bare address is
// treated as a single-host network.
func ParseTrustedProxies(entries []string) ([]*net.IPNet, error) {
	var networks []*net.IPNet
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			_, network, err := net.ParseCIDR(entry)
			if err != nil {
				return nil, fmt.Errorf("invalid trusted proxy %q: %w", entry, err)
			}
			networks = append(networks, network)
			continue
		}
		ip := net.ParseIP(entry)
		if ip == nil {
			return nil, fmt.Errorf("invalid trusted proxy %q", entry)
		}
		bits := 128
		if v4 := ip.To4(); v4 != nil {
			ip = v4
			bits = 32
		}
		networks = append(networks, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return networks, nil
}

func (a *Auth) trustedProxy(ip net.IP) bool {
	for _, network := range a.TrustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host
}

// clientIP returns the peer address unless the immediate peer is a configured
// trusted proxy. In that case it walks X-Forwarded-For from right to left and
// returns the first address that is not another trusted proxy.
func (a *Auth) clientIP(r *http.Request) string {
	peer := remoteIP(r)
	peerIP := net.ParseIP(peer)
	if peerIP == nil || !a.trustedProxy(peerIP) {
		return peer
	}

	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(hops[i]))
		if ip == nil {
			return peer
		}
		if !a.trustedProxy(ip) {
			return ip.String()
		}
	}
	return peer
}

func (a *Auth) LoginRateLimited(r *http.Request) bool {
	ip := a.clientIP(r)
	var attempts int
	var lockedUntil int64
	err := a.DB.QueryRow(`SELECT attempts, locked_until FROM login_attempts WHERE ip = ?`, ip).Scan(&attempts, &lockedUntil)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		return false
	}
	return attempts >= 5 && lockedUntil > time.Now().UnixMilli()
}

func (a *Auth) RegisterLoginFail(r *http.Request) error {
	ip := a.clientIP(r)
	now := time.Now().UnixMilli()
	windowEnd := now + 5*60*1000

	// Lazy retention: stale rows from other addresses are removed when a new
	// failure is recorded.
	if _, err := a.DB.Exec(`DELETE FROM login_attempts WHERE locked_until < ?`, now-24*60*60*1000); err != nil {
		return err
	}
	_, err := a.DB.Exec(`
		INSERT INTO login_attempts (ip, attempts, locked_until) VALUES (?, 1, ?)
		ON CONFLICT(ip) DO UPDATE SET
		  attempts = CASE WHEN login_attempts.locked_until < ? THEN 1 ELSE login_attempts.attempts + 1 END,
		  locked_until = CASE
		    WHEN login_attempts.locked_until < ? THEN ?
		    WHEN login_attempts.attempts + 1 >= 5 THEN ?
		    ELSE ?
		  END
	`, ip, windowEnd, now, now, windowEnd, windowEnd, windowEnd)
	return err
}

func (a *Auth) LoginOk(r *http.Request) error {
	ip := a.clientIP(r)
	_, err := a.DB.Exec(`DELETE FROM login_attempts WHERE ip = ?`, ip)
	return err
}

const dummyHash = "$2b$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012"

func (a *Auth) HandleLogin(w http.ResponseWriter, r *http.Request, username, password string) (*PublicUser, error) {
	user, err := a.DB.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}
	if user == nil {
		// dummy compare to mitigate timing
		bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		return nil, nil
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, nil
	}

	// rotate session
	if prev := a.SessionIDFromCookie(r); prev != nil {
		a.DB.DeleteSession(prev.ID)
	}

	sessionID, err := a.DB.CreateSession(user.ID, a.SessionTTL.Milliseconds(), r.UserAgent())
	if err != nil {
		return nil, err
	}

	value := sessionID + "." + mustSign(a, sessionID)
	setSessionCookie(w, r, value, a.SessionTTL)

	return &PublicUser{
		ID: user.ID, Username: user.Username, Email: user.Email,
		Phone: user.Phone, Language: user.Language, Role: user.Role,
	}, nil
}

func (a *Auth) HandleLogout(w http.ResponseWriter, r *http.Request) error {
	if s := a.SessionIDFromCookie(r); s != nil {
		a.DB.DeleteSession(s.ID)
	}
	clearSessionCookie(w)
	return nil
}

type SessionInfo struct {
	ID     string
	UserID string
}

func (a *Auth) SessionIDFromCookie(r *http.Request) *SessionInfo {
	cookie, err := r.Cookie(CookieName)
	if err != nil {
		return nil
	}
	raw := cookie.Value
	dot := strings.LastIndex(raw, ".")
	if dot < 0 {
		return nil
	}
	id := raw[:dot]
	sig := raw[dot+1:]

	expected, err := a.sign(id)
	if err != nil {
		return nil
	}
	if !safeEqual(sig, expected) {
		return nil
	}

	session, err := a.DB.GetSession(id)
	if err != nil || session == nil {
		return nil
	}
	return &SessionInfo{ID: session.ID, UserID: session.UserID}
}

func (a *Auth) ChangeOwnPassword(userID, currentPassword, newPassword, keepSessionID string) (ok bool, reason string, err error) {
	u, err := a.DB.GetUserByUsernameByID(userID)
	if err != nil {
		return false, "not_found", err
	}
	if u == nil {
		return false, "not_found", nil
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(currentPassword)); err != nil {
		return false, "wrong_current", nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 10)
	if err != nil {
		return false, "", err
	}
	if _, err := a.DB.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), userID); err != nil {
		return false, "", err
	}
	if keepSessionID != "" {
		a.DB.DeleteUserSessionsExcept(userID, keepSessionID)
	}
	return true, "", nil
}

func (a *Auth) RegisterUser(username, password, language, role string) (*db.UserView, error) {
	if username == "" || password == "" {
		return nil, errors.New("empty")
	}
	existing, err := a.DB.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return nil, err
	}
	if language == "" {
		language = "es"
	}
	if role == "" {
		role = "user"
	}
	return a.DB.CreateUser(username, string(hash), language, role)
}

func mustSign(a *Auth, id string) string {
	s, err := a.sign(id)
	if err != nil {
		return ""
	}
	return s
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, value string, ttl time.Duration) {
	isHTTPS := r.Header.Get("X-Forwarded-Proto") == "https" || r.TLS != nil
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isHTTPS,
		MaxAge:   int(ttl.Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// RequireAuth middleware: valida sesión, renueva si aplica, inyecta userID en contexto.
func (a *Auth) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := a.SessionIDFromCookie(r)
		if s == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "no autorizado"})
			return
		}
		user, err := a.DB.GetUserByID(s.UserID)
		if err != nil || user == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "no autorizado"})
			return
		}
		if renewed, _ := a.DB.RenewSessionIfDue(s.ID, a.SessionTTL.Milliseconds()); renewed {
			value := s.ID + "." + mustSign(a, s.ID)
			setSessionCookie(w, r, value, a.SessionTTL)
		}
		ctx := withUserID(r.Context(), s.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuthRedirect is the HTML variant of RequireAuth: instead of a 401
// JSON response it redirects to /login?next=… (page navigation), keeping the
// same session validation and sliding renewal.
func (a *Auth) RequireAuthRedirect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := a.SessionIDFromCookie(r)
		if s == nil {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
			return
		}
		user, err := a.DB.GetUserByID(s.UserID)
		if err != nil || user == nil {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
			return
		}
		if renewed, _ := a.DB.RenewSessionIfDue(s.ID, a.SessionTTL.Milliseconds()); renewed {
			setSessionCookie(w, r, s.ID+"."+mustSign(a, s.ID), a.SessionTTL)
		}
		ctx := withUserID(r.Context(), s.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	jsonNewEncoder(w).Encode(v)
}

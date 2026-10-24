package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"helios-server/internal/db"
)

const CookieName = "helios_session"

type Auth struct {
	DB            *db.DB
	SessionTTL    time.Duration
	SessionSecret string // if empty, read from kv
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

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	// chi/httplog puede sobreescribir; fallback a RemoteAddr
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		return host[:i]
	}
	return host
}

func (a *Auth) LoginRateLimited(r *http.Request) bool {
	ip := clientIP(r)
	var lockedUntil int64
	err := a.DB.QueryRow(`SELECT locked_until FROM login_attempts WHERE ip = ?`, ip).Scan(&lockedUntil)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		return false
	}
	return lockedUntil > time.Now().UnixMilli()
}

func (a *Auth) RegisterLoginFail(r *http.Request) error {
	ip := clientIP(r)
	_, err := a.DB.Exec(`
		INSERT INTO login_attempts (ip, attempts, locked_until) VALUES (?, 1, 0)
		ON CONFLICT(ip) DO UPDATE SET
		  attempts = attempts + 1,
		  locked_until = CASE WHEN attempts >= 4 THEN ? ELSE locked_until END
	`, ip, time.Now().UnixMilli()+5*60*1000)
	return err
}

func (a *Auth) LoginOk(r *http.Request) error {
	ip := clientIP(r)
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

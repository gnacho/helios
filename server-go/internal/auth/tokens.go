package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"helios-server/internal/db"
)

// TokenPrefix is the visible, non-secret part of an API token. The wire format
// is "<TokenPrefix>_<prefix>_<secret>", where prefix is used to find the row and
// the secret only ever exists in plain text in the response that creates it.
const TokenPrefix = "hx"

const (
	ScopeRead  = "read"
	ScopeWrite = "write"
)

// tokenHexRe validates both halves of a token. The prefix is 12 hex chars, the
// secret 64, and the same pattern keeps anything odd out of the query.
var tokenHexRe = regexp.MustCompile(`^[a-f0-9]{12,64}$`)

// tokenIDRe validates the row id used by the revoke routes.
var tokenIDRe = regexp.MustCompile(`^[a-f0-9]{16,64}$`)

// lastUsedResolution throttles the last_used_at write. A widget polling every
// few minutes must not turn into a write per request.
const lastUsedResolution = time.Minute

var (
	ErrTokenInvalid = errors.New("token invalid")
	ErrTokenRevoked = errors.New("token revoked")
	ErrTokenExpired = errors.New("token expired")
	ErrTokenScope   = errors.New("token lacks scope")
	ErrTokenName    = errors.New("token name invalid")
	ErrTokenScopes  = errors.New("token scopes invalid")
	ErrTokenDays    = errors.New("token expiry invalid")
	ErrTokenLimit   = errors.New("too many active tokens")
	ErrTokenMissing = errors.New("token not found")
	ErrTokenDenied  = errors.New("token belongs to another user")
)

const (
	// MaxTokensPerUser caps active tokens per account.
	MaxTokensPerUser = 20
	// MaxTokenNameLen is the visible label limit.
	MaxTokenNameLen = 60
	// MaxTokenDays is the upper bound of the expiry select.
	MaxTokenDays = 3650
)

type tokenInfoKey struct{}

// TokenInfo describes how the current request authenticated, when it did so
// with an API token instead of the session cookie.
type TokenInfo struct {
	TokenID string
	Prefix  string
	Scopes  []string
	// LastUsedAt mirrors the stored value at authentication time, so the
	// middleware knows whether the throttled touch is due.
	LastUsedAt int64
}

func (t *TokenInfo) HasScope(scope string) bool {
	if t == nil {
		return false
	}
	for _, s := range t.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// TokenFrom returns the token info stored in the context, or nil when the
// request authenticated with the cookie.
func TokenFrom(ctx context.Context) *TokenInfo {
	t, _ := ctx.Value(tokenInfoKey{}).(*TokenInfo)
	return t
}

func withTokenInfo(ctx context.Context, t *TokenInfo) context.Context {
	return context.WithValue(ctx, tokenInfoKey{}, t)
}

// NormalizeScopes validates a requested scope list and returns it as the stored
// comma separated form. Unknown scopes are rejected. "write" implies "read".
func NormalizeScopes(in []string) (string, error) {
	var read, write bool
	for _, s := range in {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case ScopeRead:
			read = true
		case ScopeWrite:
			write = true
		case "":
		default:
			return "", errors.New("unknown scope: " + s)
		}
	}
	scopes := []string{}
	if read || write {
		scopes = append(scopes, ScopeRead)
	}
	if write {
		scopes = append(scopes, ScopeWrite)
	}
	if len(scopes) == 0 {
		return "", errors.New("no scope requested")
	}
	return strings.Join(scopes, ","), nil
}

// SplitScopes parses the stored form.
func SplitScopes(stored string) []string {
	out := []string{}
	for _, s := range strings.Split(stored, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// NewAPIToken generates a token, returning the plain text to hand to the user
// once, and the parts that get stored. Both halves are hex: the secret must not
// contain the separator, and base64url does.
func NewAPIToken() (plain, prefix, hash string, err error) {
	var p [6]byte
	if _, err = rand.Read(p[:]); err != nil {
		return "", "", "", err
	}
	var secret [32]byte
	if _, err = rand.Read(secret[:]); err != nil {
		return "", "", "", err
	}
	prefix = hex.EncodeToString(p[:])
	secretStr := hex.EncodeToString(secret[:])
	plain = TokenPrefix + "_" + prefix + "_" + secretStr
	return plain, prefix, HashTokenSecret(secretStr), nil
}

// HashTokenSecret is the stored form of the secret half.
func HashTokenSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// ParseAPIToken splits the wire format without touching the database.
func ParseAPIToken(raw string) (prefix, secret string, ok bool) {
	head, rest, found := strings.Cut(strings.TrimSpace(raw), "_")
	if !found || head != TokenPrefix {
		return "", "", false
	}
	prefix, secret, found = strings.Cut(rest, "_")
	if !found || !tokenHexRe.MatchString(prefix) || !tokenHexRe.MatchString(secret) {
		return "", "", false
	}
	return prefix, secret, true
}

// bearerToken extracts the credential from an Authorization header.
func bearerToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "" {
		return ""
	}
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// AuthenticateToken resolves a plain token to its row, checking revocation and
// expiry. The error tells the caller which check failed.
func (a *Auth) AuthenticateToken(raw string) (*db.APITokenRow, error) {
	prefix, secret, ok := ParseAPIToken(raw)
	if !ok {
		return nil, ErrTokenInvalid
	}
	row, err := a.DB.GetAPITokenByPrefix(prefix)
	if err != nil {
		return nil, err
	}
	// Always run the comparison, so a missing row and a wrong secret cost the
	// same and the failure does not leak which prefixes exist.
	expected := HashTokenSecret(secret)
	if row == nil {
		subtle.ConstantTimeCompare([]byte(expected), []byte(strings.Repeat("0", len(expected))))
		return nil, ErrTokenInvalid
	}
	if subtle.ConstantTimeCompare([]byte(expected), []byte(row.Hash)) != 1 {
		return nil, ErrTokenInvalid
	}
	if row.RevokedAt != 0 {
		return nil, ErrTokenRevoked
	}
	if row.ExpiresAt != 0 && row.ExpiresAt < time.Now().UnixMilli() {
		return nil, ErrTokenExpired
	}
	return row, nil
}

// cookieOnlyPath lists the endpoints an API token may never reach, whatever its
// scopes. Token management and the updater stay behind a browser session.
func cookieOnlyPath(path string) bool {
	switch {
	case strings.HasPrefix(path, "/api/auth/"):
		return true
	case strings.HasPrefix(path, "/api/update/"):
		return true
	}
	return false
}

// authenticateWithToken resolves an Authorization: Bearer request. Reading is
// allowed with the read scope, anything else needs write. It never issues a
// cookie and never renews a session: a token has its own lifetime.
func (a *Auth) authenticateWithToken(w http.ResponseWriter, r *http.Request, raw string, next http.Handler) {
	row, err := a.AuthenticateToken(raw)
	if err != nil {
		switch {
		case errors.Is(err, ErrTokenRevoked):
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "token revocado"})
		case errors.Is(err, ErrTokenExpired):
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "token caducado"})
		case errors.Is(err, ErrTokenInvalid):
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "token no válido"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "error interno"})
		}
		return
	}

	if cookieOnlyPath(r.URL.Path) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "esta ruta exige sesión de navegador"})
		return
	}

	info := &TokenInfo{
		TokenID:    row.ID,
		Prefix:     row.Prefix,
		Scopes:     SplitScopes(row.Scopes),
		LastUsedAt: row.LastUsedAt,
	}

	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
	default:
		if !info.HasScope(ScopeWrite) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "el token no tiene permiso de escritura"})
			return
		}
	}

	user, err := a.DB.GetUserByID(row.UserID)
	if err != nil || user == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "token no válido"})
		return
	}

	now := time.Now().UnixMilli()
	if now-row.LastUsedAt > lastUsedResolution.Milliseconds() {
		a.DB.TouchAPIToken(row.ID, now)
		info.LastUsedAt = now
	}

	ctx := withTokenInfo(withUserID(r.Context(), row.UserID), info)
	next.ServeHTTP(w, r.WithContext(ctx))
}

// CreateToken validates the request, enforces the per account limit and stores
// a new token. The plain text is returned once and never recoverable.
func (a *Auth) CreateToken(userID, name string, scopes []string, expiresDays int) (string, *db.APITokenRow, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > MaxTokenNameLen {
		return "", nil, ErrTokenName
	}
	normalized, err := NormalizeScopes(scopes)
	if err != nil {
		return "", nil, ErrTokenScopes
	}
	if expiresDays < 0 || expiresDays > MaxTokenDays {
		return "", nil, ErrTokenDays
	}

	rows, err := a.DB.ListAPITokens()
	if err != nil {
		return "", nil, err
	}
	active := 0
	for _, t := range rows {
		if t.UserID == userID && t.RevokedAt == 0 {
			active++
		}
	}
	if active >= MaxTokensPerUser {
		return "", nil, ErrTokenLimit
	}

	var expiresAt int64
	if expiresDays > 0 {
		expiresAt = time.Now().Add(time.Duration(expiresDays) * 24 * time.Hour).UnixMilli()
	}

	plain, prefix, hash, err := NewAPIToken()
	if err != nil {
		return "", nil, err
	}
	row, err := a.DB.CreateAPIToken(userID, name, prefix, hash, normalized, expiresAt)
	if err != nil {
		return "", nil, err
	}
	return plain, row, nil
}

// ListTokens returns the tokens the actor may see: every token for an admin,
// only its own for everyone else.
func (a *Auth) ListTokens(actorID string, isAdmin bool) ([]db.APITokenRow, error) {
	rows, err := a.DB.ListAPITokens()
	if err != nil {
		return nil, err
	}
	if isAdmin {
		return rows, nil
	}
	out := []db.APITokenRow{}
	for _, t := range rows {
		if t.UserID == actorID {
			out = append(out, t)
		}
	}
	return out, nil
}

// RevokeToken revokes one token after checking the actor owns it (or is admin).
func (a *Auth) RevokeToken(actorID string, isAdmin bool, id string) (*db.APITokenRow, error) {
	if !tokenIDRe.MatchString(id) {
		return nil, ErrTokenMissing
	}
	row, err := a.DB.GetAPIToken(id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrTokenMissing
	}
	if !isAdmin && row.UserID != actorID {
		return nil, ErrTokenDenied
	}
	ok, err := a.DB.RevokeAPIToken(id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrTokenMissing
	}
	return row, nil
}

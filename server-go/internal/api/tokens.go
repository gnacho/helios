package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"helios-server/internal/auth"
	"helios-server/internal/db"
)

// tokenView is the JSON shape of a stored token. The secret is never included:
// it exists in plain text only in the response that creates it.
type tokenView struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	Scopes     string  `json:"scopes"`
	CreatedAt  int64   `json:"created_at"`
	LastUsedAt int64   `json:"last_used_at"`
	ExpiresAt  int64   `json:"expires_at"`
	RevokedAt  int64   `json:"revoked_at"`
	Username   *string `json:"username,omitempty"`
	Owner      bool    `json:"owner"`
}

func toTokenView(t db.APITokenRow, userID string) tokenView {
	return tokenView{
		ID: t.ID, Name: t.Name, Prefix: t.Prefix, Scopes: t.Scopes,
		CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt,
		ExpiresAt: t.ExpiresAt, RevokedAt: t.RevokedAt,
		Username: t.Username, Owner: t.UserID == userID,
	}
}

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	_, userID, isAdmin := s.currentUser(r)
	rows, err := s.Auth.ListTokens(userID, isAdmin)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	out := make([]tokenView, 0, len(rows))
	for _, t := range rows {
		out = append(out, toTokenView(t, userID))
	}
	writeJSON(w, 200, map[string]any{"tokens": out})
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	username, userID, _ := s.currentUser(r)

	var in struct {
		Name        string   `json:"name"`
		Scopes      []string `json:"scopes"`
		ExpiresDays int      `json:"expires_days"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]any{"error": "cuerpo no válido"})
		return
	}

	plain, row, err := s.Auth.CreateToken(userID, in.Name, in.Scopes, in.ExpiresDays)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrTokenName):
			writeJSON(w, 400, map[string]any{"error": "nombre no válido"})
		case errors.Is(err, auth.ErrTokenScopes):
			writeJSON(w, 400, map[string]any{"error": "scopes no válidos"})
		case errors.Is(err, auth.ErrTokenDays):
			writeJSON(w, 400, map[string]any{"error": "caducidad no válida"})
		case errors.Is(err, auth.ErrTokenLimit):
			writeJSON(w, 409, map[string]any{"error": "demasiados tokens activos"})
		default:
			writeJSON(w, 500, map[string]any{"error": "error interno"})
		}
		return
	}

	_ = s.DB.Audit(username, userID, "token.create", map[string]any{
		"id": row.ID, "name": row.Name, "prefix": row.Prefix, "scopes": row.Scopes,
	})

	// The only response that ever carries the secret.
	writeJSON(w, 201, map[string]any{
		"token": plain,
		"item":  toTokenView(*row, userID),
	})
}

func (s *Server) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	username, userID, isAdmin := s.currentUser(r)
	row, err := s.Auth.RevokeToken(userID, isAdmin, chi.URLParam(r, "id"))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrTokenMissing):
			writeJSON(w, 404, map[string]any{"error": "token no encontrado"})
		case errors.Is(err, auth.ErrTokenDenied):
			writeJSON(w, 403, map[string]any{"error": "no autorizado"})
		default:
			writeJSON(w, 500, map[string]any{"error": "error interno"})
		}
		return
	}
	_ = s.DB.Audit(username, userID, "token.revoke", map[string]any{
		"id": row.ID, "name": row.Name, "prefix": row.Prefix,
	})
	writeJSON(w, 200, map[string]any{"ok": true, "id": row.ID})
}

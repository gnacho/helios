package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"helios-server/internal/auth"
	"helios-server/internal/push"
)

func (s *Server) handleVapidPublicKey(w http.ResponseWriter, r *http.Request) {
	if s.Push == nil || !s.Push.IsConfigured() {
		writeJSON(w, 503, map[string]any{"error": "push no configurado en el servidor"})
		return
	}
	writeJSON(w, 200, map[string]any{"publicKey": s.Push.PublicKeyOrEmpty()})
}

func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	if s.Push == nil || !s.Push.IsConfigured() {
		writeJSON(w, 503, map[string]any{"error": "push no configurado en el servidor"})
		return
	}
	var body struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "suscripción inválida"})
		return
	}
	if !isValidURL(body.Endpoint, 1000) || body.Keys.P256dh == "" || body.Keys.Auth == "" ||
		len(body.Keys.P256dh) > 200 || len(body.Keys.Auth) > 200 {
		writeJSON(w, 400, map[string]any{"error": "suscripción inválida"})
		return
	}
	if err := push.ValidateEndpoint(body.Endpoint); err != nil {
		writeJSON(w, 400, map[string]any{"error": "suscripción inválida"})
		return
	}

	userID, _ := auth.UserIDFrom(r.Context())
	now := time.Now().UnixMilli()
	ua := r.UserAgent()

	if err := s.DB.UpsertPushSubscription(newID(), userID, body.Endpoint, body.Keys.P256dh, body.Keys.Auth, ua, now); err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	writeJSON(w, 201, map[string]any{"ok": true})
}

func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !isValidURL(body.Endpoint, 1000) {
		writeJSON(w, 400, map[string]any{"error": "petición inválida"})
		return
	}
	userID, _ := auth.UserIDFrom(r.Context())
	s.DB.DeletePushByEndpointAndUser(body.Endpoint, userID)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleGetPushPreferences(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	rows, err := s.DB.PreferencesForUser(userID)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	prefs := map[string]bool{}
	for _, tipo := range push.TiposAlerta {
		enabled, ok := rows[tipo]
		prefs[tipo] = !ok || enabled
	}
	writeJSON(w, 200, map[string]any{"prefs": prefs})
}

func (s *Server) handlePutPushPreferences(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Tipo    string `json:"tipo"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !push.IsValidTipo(body.Tipo) {
		writeJSON(w, 400, map[string]any{"error": "preferencia inválida"})
		return
	}
	userID, _ := auth.UserIDFrom(r.Context())
	if err := s.DB.SetPreference(userID, body.Tipo, body.Enabled, time.Now().UnixMilli()); err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func isValidURL(raw string, max int) bool {
	if raw == "" || len(raw) > max {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "https" || u.Scheme == "http"
}

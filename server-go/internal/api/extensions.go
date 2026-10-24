package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"helios-server/internal/auth"
	"helios-server/internal/extensions"
	"helios-server/internal/solar"
)

func (s *Server) currentUser(r *http.Request) (username, userID string, isAdmin bool) {
	userID, _ = auth.UserIDFrom(r.Context())
	u, err := s.DB.GetUserByID(userID)
	if err != nil || u == nil {
		return "", userID, false
	}
	return u.Username, userID, u.Role == "admin"
}

func (s *Server) handleGetExtensions(w http.ResponseWriter, r *http.Request) {
	cfg := s.ExtStore.Get()
	writeJSON(w, 200, map[string]any{
		"enabled":       cfg.Enabled,
		"carCharger":    cfg.CarCharger,
		"chargerActive": extensions.ChargerActive(cfg),
		"byd":           cfg.Byd,
		"bydActive":     extensions.BydActive(cfg),
	})
}

func (s *Server) handlePutExtensions(w http.ResponseWriter, r *http.Request) {
	username, userID, isAdmin := s.currentUser(r)
	if !isAdmin {
		writeJSON(w, 403, map[string]any{"error": "solo administradores"})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "body inválido"})
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil || raw == nil {
		writeJSON(w, 400, map[string]any{"error": "body inválido"})
		return
	}

	cfg := extensions.NormalizeConfig(raw, extensions.GenericExtensions)

	// Persistimos el config normalizado y re-resolvemos en memoria.
	if out, err := json.Marshal(cfg); err == nil {
		if err := s.DB.KVSet("extensions_config", string(out)); err != nil {
			writeJSON(w, 500, map[string]any{"error": "error interno"})
			return
		}
	}
	s.ExtStore.Set(cfg)
	s.DB.Audit(username, userID, "extensions.change", map[string]any{"enabled": cfg.Enabled})

	writeJSON(w, 200, map[string]any{"ok": true, "restartNeeded": true})
}

func (s *Server) handleChargerDay(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if !validDate(date) {
		writeJSON(w, 400, map[string]any{"error": "formato inválido, usa YYYY-MM-DD"})
		return
	}
	if s.ExtHA == nil {
		writeJSON(w, 503, map[string]any{"error": "HAOS no configurado"})
		return
	}
	key := r.URL.Query().Get("charger")
	if key == "" {
		key = "main"
	}
	charger := extensions.GetChargerByKey(s.ExtStore.Get(), key)
	if charger == nil {
		writeJSON(w, 404, map[string]any{"error": "cargador no encontrado"})
		return
	}
	keyDate := date
	if keyDate == "" {
		keyDate = solar.TodayStr()
	}

	curve := s.Ext.ChargerDayCurve(s.ExtHA, keyDate, charger)
	writeJSON(w, 200, map[string]any{"date": keyDate, "points": curve.Points})
}

func (s *Server) handleChargerHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if !validDate(from) || !validDate(to) {
		writeJSON(w, 400, map[string]any{"error": "formato inválido, usa YYYY-MM-DD"})
		return
	}
	key := q.Get("charger")
	if key == "" {
		key = "main"
	}
	charger := extensions.GetChargerByKey(s.ExtStore.Get(), key)
	if charger == nil {
		writeJSON(w, 404, map[string]any{"error": "cargador no encontrado"})
		return
	}

	toFinal := to
	if toFinal == "" {
		toFinal = solar.TodayStr()
	}
	fromFinal := from
	if fromFinal == "" {
		base, err := parseDay(toFinal)
		if err != nil {
			writeJSON(w, 400, map[string]any{"error": "fecha inválida"})
			return
		}
		fromFinal = base.AddDate(0, 0, -364).Format("2006-01-02")
	}

	days, err := s.Ext.ChargerHistory(fromFinal, toFinal, charger.Key)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	writeJSON(w, 200, map[string]any{"from": fromFinal, "to": toFinal, "days": days})
}

func (s *Server) handleChargerAction(w http.ResponseWriter, r *http.Request) {
	username, userID, isAdmin := s.currentUser(r)
	if !isAdmin {
		writeJSON(w, 403, map[string]any{"error": "solo admin"})
		return
	}

	var body struct {
		Charger string   `json:"charger"`
		Action  string   `json:"action"`
		Amps    *float64 `json:"amps"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "body inválido"})
		return
	}
	if body.Charger == "" {
		body.Charger = "main"
	}

	charger := extensions.GetChargerByKey(s.ExtStore.Get(), body.Charger)
	if charger == nil {
		writeJSON(w, 404, map[string]any{"error": "cargador no encontrado"})
		return
	}
	if s.ExtHA == nil {
		writeJSON(w, 503, map[string]any{"error": "HAOS no conectado"})
		return
	}

	detail := map[string]any{"charger": charger.Key, "action": body.Action}
	if body.Amps != nil {
		detail["amps"] = *body.Amps
	}
	s.DB.Audit(username, userID, "charger.action", detail)

	amps, err := extensions.ApplyAction(s.ExtHA, *charger, body.Action, body.Amps)
	if err != nil {
		switch {
		case errors.Is(err, extensions.ErrChargerAction),
			errors.Is(err, extensions.ErrChargerNoSwitch),
			errors.Is(err, extensions.ErrChargerNoAmp),
			errors.Is(err, extensions.ErrChargerAmps):
			writeJSON(w, 400, map[string]any{"error": err.Error()})
		default:
			writeJSON(w, 502, map[string]any{"error": err.Error()})
		}
		return
	}
	resp := map[string]any{"ok": true}
	if body.Action == "amp" {
		resp["amps"] = amps
	}
	writeJSON(w, 200, resp)
}

func (s *Server) handleBydAction(w http.ResponseWriter, r *http.Request) {
	username, userID, isAdmin := s.currentUser(r)
	if !isAdmin {
		writeJSON(w, 403, map[string]any{"error": "solo admin"})
		return
	}
	if s.ExtHA == nil {
		writeJSON(w, 503, map[string]any{"error": "HAOS no conectado"})
		return
	}
	var body struct {
		Action string `json:"action"`
		Value  any    `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "body inválido"})
		return
	}
	s.DB.Audit(username, userID, "byd.action", map[string]any{"action": body.Action, "value": body.Value})
	if err := extensions.BydAction(s.ExtHA, s.ExtStore.Get(), body.Action, body.Value); err != nil {
		msg := err.Error()
		if msg == "unknown_action" {
			writeJSON(w, 400, map[string]any{"error": "acción no soportada"})
			return
		}
		if msg == "byd_disabled" {
			writeJSON(w, 400, map[string]any{"error": "BYD desactivado"})
			return
		}
		writeJSON(w, 502, map[string]any{"error": msg})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleGetChargerImage(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if !extensions.SanitizeImageKey(key) {
		writeJSON(w, 400, map[string]any{"error": "clave invalida"})
		return
	}
	path, mime := extensions.FindChargerImage(s.dataDir, key)
	if path == "" {
		w.WriteHeader(404)
		return
	}
	// El ETag evita revalidar con la foto entera en cada navegación (la UI
	// templ es MPA) y cambia solo cuando se sube otra imagen.
	st, statErr := os.Stat(path)
	if statErr == nil {
		etag := fmt.Sprintf(`"%x-%x"`, st.Size(), st.ModTime().UnixNano())
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "no-cache")
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(304)
			return
		}
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		w.WriteHeader(404)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(buf)
}

func (s *Server) handlePutChargerImage(w http.ResponseWriter, r *http.Request) {
	_, _, isAdmin := s.currentUser(r)
	if !isAdmin {
		writeJSON(w, 403, map[string]any{"error": "solo admin"})
		return
	}
	key := chi.URLParam(r, "key")
	if !extensions.SanitizeImageKey(key) {
		writeJSON(w, 400, map[string]any{"error": "clave invalida"})
		return
	}
	var body struct {
		Image string `json:"image"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "body inválido"})
		return
	}
	ok, msg := s.Ext.SaveChargerImage(s.dataDir, key, body.Image)
	if !ok {
		writeJSON(w, 400, map[string]any{"error": msg})
		return
	}
	username, userID, _ := s.currentUser(r)
	s.DB.Audit(username, userID, "charger.image", map[string]any{"charger": key})
	writeJSON(w, 200, map[string]any{"ok": true, "url": "/api/charger/image/" + key})
}

func (s *Server) handleDeleteChargerImage(w http.ResponseWriter, r *http.Request) {
	_, _, isAdmin := s.currentUser(r)
	if !isAdmin {
		writeJSON(w, 403, map[string]any{"error": "solo admin"})
		return
	}
	key := chi.URLParam(r, "key")
	if !extensions.SanitizeImageKey(key) {
		writeJSON(w, 400, map[string]any{"error": "clave invalida"})
		return
	}
	s.Ext.ClearChargerImage(s.dataDir, key)
	username, userID, _ := s.currentUser(r)
	s.DB.Audit(username, userID, "charger.image.clear", map[string]any{"charger": key})
	writeJSON(w, 200, map[string]any{"ok": true})
}

package api

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"

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

var switchIDRe = regexp.MustCompile(`^switch\.[a-z0-9_]+$`)
var ampIDRe = regexp.MustCompile(`^(number|input_select|select)\.[a-z0-9_]+$`)

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

	switch body.Action {
	case "on", "off":
		if !switchIDRe.MatchString(charger.SwitchID) {
			writeJSON(w, 400, map[string]any{"error": "el cargador no tiene switch configurado"})
			return
		}
		service := "turn_off"
		if body.Action == "on" {
			service = "turn_on"
		}
		if err := s.ExtHA.CallService("switch", service, map[string]any{"entity_id": charger.SwitchID}); err != nil {
			writeJSON(w, 502, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})

	case "amp":
		if body.Amps == nil {
			writeJSON(w, 400, map[string]any{"error": "amperaje requerido"})
			return
		}
		amps := math.Round(*body.Amps*10) / 10
		if amps < 6 || amps > 32 {
			writeJSON(w, 400, map[string]any{"error": "amperaje fuera de rango (6-32 A)"})
			return
		}
		if !ampIDRe.MatchString(charger.AmpID) {
			writeJSON(w, 400, map[string]any{"error": "el cargador no tiene entidad de corriente configurada"})
			return
		}
		domain := charger.AmpID[:indexOf(charger.AmpID, '.')]
		var err error
		if domain == "number" {
			err = s.ExtHA.CallService("number", "set_value", map[string]any{
				"entity_id": charger.AmpID, "value": amps,
			})
		} else {
			err = s.ExtHA.CallService(domain, "select_option", map[string]any{
				"entity_id": charger.AmpID, "option": strconv.FormatFloat(amps, 'f', -1, 64),
			})
		}
		if err != nil {
			writeJSON(w, 502, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "amps": amps})

	default:
		writeJSON(w, 400, map[string]any{"error": "acción no soportada"})
	}
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

func indexOf(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

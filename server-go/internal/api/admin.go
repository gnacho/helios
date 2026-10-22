package api

import (
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"helios-server/internal/auth"
	"helios-server/internal/db"
)

var avatarNameRe = regexp.MustCompile(`^[\w-]+$`)

func (s *Server) handleGetAvatar(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !avatarNameRe.MatchString(id) {
		w.WriteHeader(400)
		return
	}
	file := filepath.Join(s.avatarsDir(), id+".webp")
	if _, err := os.Stat(file); err != nil {
		w.WriteHeader(404)
		return
	}
	buf, err := os.ReadFile(file)
	if err != nil {
		w.WriteHeader(404)
		return
	}
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(buf)
}

type avatarUploadRequest struct {
	Avatar string `json:"avatar"`
}

var avatarDataRe = regexp.MustCompile(`^data:image/(webp|png|jpeg);base64,(.+)$`)

const avatarMax = 200 * 1024

func (s *Server) handleUploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	var body avatarUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Avatar == "" {
		writeJSON(w, 400, map[string]any{"error": "formato inválido"})
		return
	}
	m := avatarDataRe.FindStringSubmatch(body.Avatar)
	if m == nil {
		writeJSON(w, 400, map[string]any{"error": "formato de imagen inválido"})
		return
	}
	buf, err := base64.StdEncoding.DecodeString(m[2])
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "formato de imagen inválido"})
		return
	}
	if len(buf) > avatarMax {
		writeJSON(w, 413, map[string]any{"error": "imagen demasiado grande (máx 200 KB)"})
		return
	}
	if err := os.MkdirAll(s.avatarsDir(), 0o755); err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	if err := os.WriteFile(filepath.Join(s.avatarsDir(), userID+".webp"), buf, 0o644); err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	me, _ := s.DB.GetUserByID(userID)
	actor := userID
	if me != nil {
		actor = me.Username
	}
	s.DB.Audit(actor, userID, "user.avatar", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleDeleteAvatar(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	file := filepath.Join(s.avatarsDir(), userID+".webp")
	os.Remove(file)
	me, _ := s.DB.GetUserByID(userID)
	actor := userID
	if me != nil {
		actor = me.Username
	}
	s.DB.Audit(actor, userID, "user.avatar.delete", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) avatarsDir() string {
	return filepath.Join(s.dataDir, "avatars")
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.DB.ListUsers()
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	writeJSON(w, 200, map[string]any{"users": users})
}

func (s *Server) handleAdminSetPassword(w http.ResponseWriter, r *http.Request) {
	adminID, _ := auth.UserIDFrom(r.Context())
	targetID := chi.URLParam(r, "id")
	admin, _ := s.DB.GetUserByID(adminID)
	if admin == nil || admin.Role != "admin" {
		writeJSON(w, 403, map[string]any{"error": "solo administradores"})
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Password) < 6 {
		writeJSON(w, 400, map[string]any{"error": "formato inválido"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 10)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	if _, err := s.DB.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), targetID); err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	s.DB.Audit(admin.Username, admin.ID, "user.password.admin", map[string]string{"target": targetID})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleAdminSetLanguage(w http.ResponseWriter, r *http.Request) {
	adminID, _ := auth.UserIDFrom(r.Context())
	targetID := chi.URLParam(r, "id")
	admin, _ := s.DB.GetUserByID(adminID)
	if admin == nil || admin.Role != "admin" {
		writeJSON(w, 403, map[string]any{"error": "solo administradores"})
		return
	}
	var body struct {
		Language string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !langRe.MatchString(body.Language) {
		writeJSON(w, 400, map[string]any{"error": "formato inválido"})
		return
	}
	if _, err := s.DB.Exec(`UPDATE users SET language = ? WHERE id = ?`, body.Language, targetID); err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	s.DB.Audit(admin.Username, admin.ID, "user.language.admin", map[string]string{"target": targetID})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleAdminSetRole(w http.ResponseWriter, r *http.Request) {
	adminID, _ := auth.UserIDFrom(r.Context())
	targetID := chi.URLParam(r, "id")
	admin, _ := s.DB.GetUserByID(adminID)
	if admin == nil || admin.Role != "admin" {
		writeJSON(w, 403, map[string]any{"error": "solo administradores"})
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || (body.Role != "user" && body.Role != "admin") {
		writeJSON(w, 400, map[string]any{"error": "formato inválido"})
		return
	}
	if _, err := s.DB.Exec(`UPDATE users SET role = ? WHERE id = ?`, body.Role, targetID); err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	s.DB.Audit(admin.Username, admin.ID, "user.role.admin", map[string]string{"target": targetID, "role": body.Role})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	adminID, _ := auth.UserIDFrom(r.Context())
	targetID := chi.URLParam(r, "id")
	admin, _ := s.DB.GetUserByID(adminID)
	if admin == nil || admin.Role != "admin" {
		writeJSON(w, 403, map[string]any{"error": "solo administradores"})
		return
	}
	if targetID == adminID {
		writeJSON(w, 400, map[string]any{"error": "no puedes borrarte a ti mismo"})
		return
	}
	if _, err := s.DB.Exec(`DELETE FROM users WHERE id = ?`, targetID); err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	s.DB.Audit(admin.Username, admin.ID, "user.delete", map[string]string{"target": targetID})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	entries, err := s.DB.AuditRange(limit, offset)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	writeJSON(w, 200, map[string]any{"entries": entries})
}

// handleRecover: emergency kit de contraseñas. SOLO conexión directa desde
// localhost (rechaza si hay cabeceras de proxy): resetea la contraseña de
// todos los admins a una temporal y la devuelve. Documentado en el README.
func (s *Server) handleRecover(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("X-Real-IP") != "" {
		writeJSON(w, 403, map[string]any{"error": "solo desde localhost"})
		return
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if host != "127.0.0.1" && host != "::1" {
		writeJSON(w, 403, map[string]any{"error": "solo desde localhost"})
		return
	}

	raw := make([]byte, 9)
	if _, err := cryptorand.Read(raw); err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	temp := base64.RawURLEncoding.EncodeToString(raw)
	hash, err := bcrypt.GenerateFromPassword([]byte(temp), 10)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	if _, err := s.DB.Exec(`UPDATE users SET password_hash = ? WHERE role = ?`, string(hash), "admin"); err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	log.Printf("[helios] recovery: contraseña de admins reseteada (temporal)")
	writeJSON(w, 200, map[string]any{"ok": true, "tempPassword": temp})
}

var _ = db.UserView{}

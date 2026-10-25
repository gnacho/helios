package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"helios-server/internal/auth"
	"helios-server/internal/db"
	"helios-server/internal/extensions"
	"helios-server/internal/install"
	"helios-server/internal/push"
	"helios-server/internal/solar"
	"helios-server/internal/web"
)

type Server struct {
	Auth     *auth.Auth
	DB       *db.DB
	Solar    *solar.Solar
	HA       solar.HAReader
	Ext      *extensions.Ext
	ExtStore *extensions.Store
	ExtHA    extensions.HA
	Push     *push.Service
	SSE      http.Handler
	Prices   solar.PriceConfig
	Install  *install.Store
	UI       *web.UI
	dataDir  string
}

func NewServer(a *auth.Auth, database *db.DB, dataDir string, sol *solar.Solar, haReader solar.HAReader, ext *extensions.Ext, extStore *extensions.Store, extHA extensions.HA, prices solar.PriceConfig, installStore *install.Store) *Server {
	return &Server{
		Auth: a, DB: database, dataDir: dataDir,
		Solar: sol, HA: haReader,
		Ext: ext, ExtStore: extStore, ExtHA: extHA,
		Prices:  prices,
		Install: installStore,
	}
}

func parseDay(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", s, time.Local)
}

func (s *Server) Routes() chi.Router {
	r := chi.NewRouter()

	r.Route("/api/auth", func(r chi.Router) {
		r.Post("/login", s.handleLogin)
		r.Post("/logout", s.handleLogout)
		r.Post("/register", s.handleRegister)
		r.Post("/recover", s.handleRecover)

		r.Group(func(r chi.Router) {
			r.Use(s.Auth.RequireAuth)
			r.Get("/me", s.handleMe)
			r.Get("/profile", s.handleProfile)
			r.Put("/profile", s.handleUpdateProfile)
			r.Put("/password", s.handleChangePassword)
			r.Put("/profile/avatar", s.handleUploadAvatar)
			r.Delete("/profile/avatar", s.handleDeleteAvatar)
			r.Get("/avatar/{id}", s.handleGetAvatar)
			r.Get("/users", s.handleListUsers)
			r.Put("/users/{id}/password", s.handleAdminSetPassword)
			r.Put("/users/{id}/language", s.handleAdminSetLanguage)
			r.Put("/users/{id}/role", s.handleAdminSetRole)
			r.Delete("/users/{id}", s.handleAdminDeleteUser)
			r.Get("/audit", s.handleAudit)
		})
	})

	r.Get("/health", s.handleHealth)
	r.Get("/api/version", s.handleVersion)

	// Público en el original (registrado antes de guarded).
	r.Get("/api/system/info", s.handleSystemInfo)

	r.Group(func(r chi.Router) {
		r.Use(s.Auth.RequireAuth)
		r.Get("/api/update/status", s.handleUpdateStatus)
		r.Get("/api/update/progress", s.handleUpdateProgress)
		r.Get("/api/update/stream", s.handleUpdateStream)
		r.Post("/api/update/apply", s.handleUpdateApply)
	})

	r.Group(func(r chi.Router) {
		r.Use(s.Auth.RequireAuth)
		r.Get("/api/solar/live", s.handleSolarLive)
		r.Get("/api/solar/day", s.handleSolarDay)
		r.Get("/api/solar/kpis", s.handleSolarKpis)
		r.Get("/api/solar/history", s.handleSolarHistory)

		r.Get("/api/install", s.handleGetInstall)
		r.Get("/api/config", s.handleGetConfig)
		r.Put("/api/config", s.handlePutConfig)

		r.Get("/api/extensions", s.handleGetExtensions)
		r.Put("/api/extensions", s.handlePutExtensions)
		r.Get("/api/charger/day", s.handleChargerDay)
		r.Get("/api/charger/history", s.handleChargerHistory)
		r.Post("/api/charger/action", s.handleChargerAction)
		r.Post("/api/byd/action", s.handleBydAction)
		r.Get("/api/solar/stream", s.SSE.ServeHTTP)
		r.Get("/api/charger/stream", s.SSE.ServeHTTP)
		r.Get("/api/charger/image/{key}", s.handleGetChargerImage)
		r.Put("/api/charger/image/{key}", s.handlePutChargerImage)
		r.Delete("/api/charger/image/{key}", s.handleDeleteChargerImage)

		r.Get("/api/push/vapid-public-key", s.handleVapidPublicKey)
		r.Post("/api/push/subscribe", s.handlePushSubscribe)
		r.Delete("/api/push/unsubscribe", s.handlePushUnsubscribe)
		r.Get("/api/push/preferences", s.handleGetPushPreferences)
		r.Put("/api/push/preferences", s.handlePutPushPreferences)
	})

	// UI: páginas HTML, assets propios y auth de página.
	s.UI.Mount(r)

	// Fallback: API 404 en JSON; una ruta desconocida de la app va al login o
	// al 404 con el shell (no hay SPA que sirva index.html).
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/health" {
			writeJSON(w, 404, map[string]any{"error": "ruta no encontrada"})
			return
		}
		s.UI.NotFound(w, r)
	})

	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"status": "ok"})
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.Auth.LoginRateLimited(r) {
		writeJSON(w, 429, map[string]any{"error": "demasiados intentos, espera 5 minutos"})
		return
	}
	var body loginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Password == "" {
		writeJSON(w, 400, map[string]any{"error": "formato inválido"})
		return
	}
	user, err := s.Auth.HandleLogin(w, r, body.Username, body.Password)
	if err != nil || user == nil {
		s.Auth.RegisterLoginFail(r)
		writeJSON(w, 401, map[string]any{"error": "usuario o contraseña incorrectos"})
		return
	}
	s.Auth.LoginOk(r)
	s.DB.Audit(user.Username, user.ID, "auth.login", nil)
	writeJSON(w, 200, map[string]any{"ok": true, "user": user})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if session := s.Auth.SessionIDFromCookie(r); session != nil {
		if u, _ := s.DB.GetUserByID(session.UserID); u != nil {
			s.DB.Audit(u.Username, u.ID, "auth.logout", nil)
		}
	}
	s.Auth.HandleLogout(w, r)
	writeJSON(w, 200, map[string]any{"ok": true})
}

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Language string `json:"language"`
	Role     string `json:"role"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	session := s.Auth.SessionIDFromCookie(r)
	if session == nil {
		writeJSON(w, 401, map[string]any{"authenticated": false})
		return
	}
	currentUser, err := s.DB.GetUserByID(session.UserID)
	if err != nil || currentUser == nil || currentUser.Role != "admin" {
		writeJSON(w, 403, map[string]any{"error": "solo administradores pueden crear usuarios"})
		return
	}

	var body registerRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "formato inválido"})
		return
	}
	if len(body.Username) < 3 || len(body.Username) > 50 || len(body.Password) < 6 {
		writeJSON(w, 400, map[string]any{"error": "formato inválido"})
		return
	}
	if body.Role != "user" && body.Role != "admin" {
		body.Role = "user"
	}
	if body.Language == "" {
		body.Language = "es"
	}

	user, err := s.Auth.RegisterUser(body.Username, body.Password, body.Language, body.Role)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	if user == nil {
		writeJSON(w, 409, map[string]any{"error": "usuario ya existe"})
		return
	}
	s.DB.Audit(currentUser.Username, currentUser.ID, "user.create", map[string]string{"username": user.Username, "role": user.Role})
	writeJSON(w, 200, map[string]any{"ok": true, "user": user})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	user, err := s.DB.GetUserByID(userID)
	if err != nil || user == nil {
		writeJSON(w, 401, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, 200, map[string]any{"authenticated": true, "user": user})
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	user, err := s.DB.GetUserByID(userID)
	if err != nil || user == nil {
		writeJSON(w, 401, map[string]any{"authenticated": false})
		return
	}
	writeJSON(w, 200, map[string]any{"user": user})
}

type profileRequest struct {
	DisplayName *string `json:"display_name"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
	Language    *string `json:"language"`
}

var emailRe = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
var langRe = regexp.MustCompile(`^(es|en|zh-CN)$`)

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	var body profileRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "formato inválido"})
		return
	}
	if body.Email != nil && *body.Email != "" && !emailRe.MatchString(*body.Email) {
		writeJSON(w, 400, map[string]any{"error": "formato inválido"})
		return
	}
	if body.Language != nil && !langRe.MatchString(*body.Language) {
		writeJSON(w, 400, map[string]any{"error": "formato inválido"})
		return
	}

	updated, err := s.DB.UpdateUser(userID, db.UserUpdates{
		DisplayName: body.DisplayName,
		Email:       body.Email,
		Phone:       body.Phone,
		Language:    body.Language,
	})
	if err != nil || updated == nil {
		writeJSON(w, 500, map[string]any{"error": "no se pudo actualizar"})
		return
	}
	fields := []string{}
	if body.DisplayName != nil {
		fields = append(fields, "display_name")
	}
	if body.Email != nil {
		fields = append(fields, "email")
	}
	if body.Phone != nil {
		fields = append(fields, "phone")
	}
	if body.Language != nil {
		fields = append(fields, "language")
	}
	s.DB.Audit(updated.Username, updated.ID, "user.profile", map[string]any{"fields": fields})
	writeJSON(w, 200, map[string]any{"ok": true, "user": updated})
}

type passwordRequest struct {
	Current  string `json:"current"`
	Password string `json:"password"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserIDFrom(r.Context())
	var body passwordRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Current == "" || len(body.Password) < 6 {
		writeJSON(w, 400, map[string]any{"error": "formato inválido"})
		return
	}
	session := s.Auth.SessionIDFromCookie(r)
	keepID := ""
	if session != nil {
		keepID = session.ID
	}
	ok, reason, err := s.Auth.ChangeOwnPassword(userID, body.Current, body.Password, keepID)
	if err != nil || !ok {
		msg := "no se pudo cambiar"
		if reason == "wrong_current" {
			msg = "la contraseña actual no es correcta"
		}
		writeJSON(w, 400, map[string]any{"error": msg})
		return
	}
	me, _ := s.DB.GetUserByID(userID)
	actor := userID
	if me != nil {
		actor = me.Username
	}
	s.DB.Audit(actor, userID, "user.password", nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

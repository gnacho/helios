package health

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

type Handler struct {
	DB *sql.DB
}

type response struct {
	Status string `json:"status"`
	DB     bool   `json:"db"`
	Time   string `json:"time"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dbOK := false
	if h.DB != nil {
		if err := h.DB.Ping(); err == nil {
			dbOK = true
		}
	}

	status := "ok"
	if !dbOK {
		status = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response{
		Status: status,
		DB:     dbOK,
		Time:   time.Now().UTC().Format(time.RFC3339),
	})
}

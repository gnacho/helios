package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"helios-server/internal/update"
	"helios-server/internal/version"
)

var (
	// appVersion la inyecta el linker en el paquete version (ver CI).
	appVersion   = version.Version
	appName      = "Helios"
	reactVersion = "19.2.0"
	processStart = time.Now()
	// updateHeartbeatInterval is a variable so tests can force the heartbeat
	// to overlap with the subscriber callback.
	updateHeartbeatInterval = 15 * time.Second
)

// updateEventBuffer bounds how many pending progress frames a slow client may
// accumulate before frames start being dropped.
const updateEventBuffer = 32

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	build := update.CurrentID()
	if build == "" {
		build = appVersion
	}
	writeJSON(w, 200, map[string]any{
		"version": appVersion,
		"build":   build,
	})
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	writeJSON(w, 200, map[string]any{
		"app": map[string]any{
			"version": appVersion,
			"name":    appName,
		},
		// El frontend renderiza "Node <version>": mismo contrato que el server
		// original (ahora reporta el runtime Go).
		"node": map[string]any{
			"version":  strings.Split(strings.TrimPrefix(runtime.Version(), "go"), "-")[0],
			"platform": runtime.GOOS,
			"arch":     runtime.GOARCH,
		},
		"react":  reactVersion,
		"uptime": int(time.Since(processStart).Seconds()),
		"memory": map[string]any{
			"rss":      mem.Sys / 1024 / 1024,
			"heapUsed": mem.HeapAlloc / 1024 / 1024,
		},
	})
}

func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	_, _, isAdmin := s.currentUser(r)
	if !isAdmin {
		writeJSON(w, 403, map[string]any{"error": "solo administradores pueden consultar actualizaciones"})
		return
	}
	st, err := update.GetStatus(s.DB, s.dataDir)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "error consultando actualizaciones"})
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) handleUpdateProgress(w http.ResponseWriter, r *http.Request) {
	_, _, isAdmin := s.currentUser(r)
	if !isAdmin {
		writeJSON(w, 403, map[string]any{"error": "solo administradores"})
		return
	}
	if p := update.ReadProgress(s.dataDir); p != nil {
		writeJSON(w, 200, p)
		return
	}
	writeJSON(w, 200, map[string]any{"step": "idle", "pct": 0})
}

func (s *Server) handleUpdateStream(w http.ResponseWriter, r *http.Request) {
	_, _, isAdmin := s.currentUser(r)
	if !isAdmin {
		writeJSON(w, 403, map[string]any{"error": "solo administradores"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	events := make(chan any, updateEventBuffer)
	done := make(chan struct{})
	defer close(done)

	// The subscriber callback runs on the WatchProgress goroutine, so it must
	// not touch the ResponseWriter. It hands events to the handler loop, which
	// is the only writer.
	unsub := update.Subscribe(func(p update.ProgressPayload) {
		payload := map[string]any{"type": p.Type, "step": p.Step, "pct": p.Pct}
		select {
		case events <- payload:
		case <-done:
		default:
			// Slow client: drop the oldest progress frame rather than block
			// the broadcast for every other subscriber.
		}
	})
	defer unsub()

	if st, err := update.GetStatus(s.DB, s.dataDir); err == nil {
		if ok, err := writeJSONEvent(w, flusher, st); err != nil || !ok {
			return
		}
	}

	heartbeat := time.NewTicker(updateHeartbeatInterval)
	defer heartbeat.Stop()

	// Observa también el fichero de progreso (watch del original).
	stopWatch := update.WatchProgress(s.dataDir)
	defer stopWatch()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case payload := <-events:
			if ok, err := writeJSONEvent(w, flusher, payload); err != nil || !ok {
				return
			}
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, "event: heartbeat\ndata: \n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeJSONEvent serializes one SSE frame. It returns ok=false when the payload
// cannot be encoded and err when the connection is gone.
func writeJSONEvent(w io.Writer, flusher http.Flusher, data any) (bool, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return false, nil
	}
	if _, err := fmt.Fprintf(w, "event: update\ndata: %s\n\n", raw); err != nil {
		return false, err
	}
	flusher.Flush()
	return true, nil
}

func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	username, userID, isAdmin := s.currentUser(r)
	if !isAdmin {
		writeJSON(w, 403, map[string]any{"error": "solo administradores pueden actualizar"})
		return
	}
	s.DB.Audit(username, userID, "update.apply", nil)
	if !update.RequestUpdate(s.dataDir) {
		writeJSON(w, 500, map[string]any{"error": "no se pudo solicitar la actualización"})
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true, "restarting": true})
}

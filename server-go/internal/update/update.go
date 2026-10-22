package update

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"helios-server/internal/db"
)

var (
	Repo   = getenv("GITHUB_REPO", "gnacho/helios")
	Marker = getenv("RELEASE_MARKER", "/opt/helios/.release-id")
)

const (
	cacheKey      = "gh_latest_release"
	cacheTTL      = 5 * time.Minute
	progressFile  = "update-progress.json"
	progressStale = 15 * time.Minute
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type ProgressPayload struct {
	Type string `json:"type"`
	Step string `json:"step,omitempty"`
	Pct  int    `json:"pct,omitempty"`
}

type releaseEntry struct {
	At   int64  `json:"at"`
	ID   string `json:"id"`
	Body string `json:"body"`
}

type ProgressState struct {
	Ts   int64  `json:"ts,omitempty"`
	Step string `json:"step,omitempty"`
	Pct  int    `json:"pct,omitempty"`
}

type StatusResult struct {
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	LatestBody string `json:"latestBody"`
	Available  bool   `json:"available"`
	CanApply   bool   `json:"canApply"`
	Updating   any    `json:"updating"`
	Repo       string `json:"repo"`
}

var listeners = struct {
	sync.Mutex
	set map[*func(ProgressPayload)]struct{}
}{set: map[*func(ProgressPayload)]struct{}{}}

func Subscribe(fn func(ProgressPayload)) func() {
	fp := &fn
	listeners.Lock()
	listeners.set[fp] = struct{}{}
	listeners.Unlock()
	return func() {
		listeners.Lock()
		delete(listeners.set, fp)
		listeners.Unlock()
	}
}

func broadcast(payload ProgressPayload) {
	listeners.Lock()
	fns := make([]func(ProgressPayload), 0, len(listeners.set))
	for fp := range listeners.set {
		fns = append(fns, *fp)
	}
	listeners.Unlock()

	for _, fn := range fns {
		func() {
			defer func() { _ = recover() }()
			fn(payload)
		}()
	}
}

// CurrentID lee el release-id del marker (vacío si no existe).
func CurrentID() string {
	b, err := os.ReadFile(Marker)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// LatestRelease consulta releases/latest con caché de 5 min en kv.
func LatestRelease(database *db.DB) (*releaseEntry, error) {
	if cached, err := database.KVGet(cacheKey); err == nil && cached != "" {
		var c releaseEntry
		if err := json.Unmarshal([]byte(cached), &c); err == nil {
			if time.Since(time.UnixMilli(c.At)) < cacheTTL {
				return &c, nil
			}
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", "https://api.github.com/repos/"+Repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "helios-updater")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var data struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&data); err != nil {
		return nil, err
	}

	entry := &releaseEntry{
		At:   time.Now().UnixMilli(),
		ID:   strings.TrimPrefix(data.TagName, "v"),
		Body: strings.TrimSpace(data.Body),
	}
	if raw, err := json.Marshal(entry); err == nil {
		database.KVSet(cacheKey, string(raw))
	}
	return entry, nil
}

func compareSemver(a, b string) int {
	pa := parseVersion(a)
	pb := parseVersion(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] - pb[i]
		}
	}
	return 0
}

func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			n = 0
		}
		out[i] = n
	}
	return out
}

// ReadProgress lee el estado del update; null si el fichero es viejo o falta.
func ReadProgress(dataDir string) *ProgressState {
	b, err := os.ReadFile(filepath.Join(dataDir, progressFile))
	if err != nil {
		return nil
	}
	var p ProgressState
	if err := json.Unmarshal(b, &p); err != nil {
		return nil
	}
	if p.Ts > 0 && time.Since(time.UnixMilli(p.Ts)) > progressStale {
		return nil
	}
	return &p
}

func GetStatus(database *db.DB, dataDir string) (*StatusResult, error) {
	current := CurrentID()

	rel, err := LatestRelease(database)
	var latest, latestBody string
	if err == nil && rel != nil {
		latest = rel.ID
		latestBody = rel.Body
	}

	available := latest != "" && current != "" && compareSemver(latest, current) > 0

	var updating any
	if p := ReadProgress(dataDir); p != nil {
		updating = map[string]any{"step": p.Step, "progress": p.Pct}
	}

	return &StatusResult{
		Current:    current,
		Latest:     latest,
		LatestBody: latestBody,
		Available:  available,
		CanApply:   available && updating == nil,
		Updating:   updating,
		Repo:       Repo,
	}, nil
}

// RequestUpdate escribe el flag que helios-update.sh observa.
func RequestUpdate(dataDir string) bool {
	flag := filepath.Join(dataDir, ".update-requested")
	if err := os.WriteFile(flag, []byte(time.Now().UTC().Format(time.RFC3339)), 0o644); err != nil {
		return false
	}
	return true
}

// WatchProgress hace polling del fichero de progreso (500 ms) y emite eventos.
// Devuelve función para detener.
func WatchProgress(dataDir string) func() {
	stop := make(chan struct{})
	go func() {
		file := filepath.Join(dataDir, progressFile)
		var lastMtime int64
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				info, err := os.Stat(file)
				if err != nil {
					continue
				}
				mtime := info.ModTime().UnixMilli()
				if mtime <= lastMtime {
					continue
				}
				lastMtime = mtime
				if p := ReadProgress(dataDir); p != nil {
					broadcast(ProgressPayload{Type: "progress", Step: p.Step, Pct: p.Pct})
				}
			}
		}
	}()
	return func() { close(stop) }
}

package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// StaticFiles sirve el frontend SPA desde staticDir con fallback a index.html.
// Replica las cabeceras de caché del servidor Node original.
type StaticFiles struct {
	Dir   string
	Index string
}

func NewStaticFiles(dir string) *StaticFiles {
	return &StaticFiles{Dir: dir, Index: filepath.Join(dir, "index.html")}
}

func (s *StaticFiles) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// API y health los maneja el router principal.
	if strings.HasPrefix(path, "/api/") || path == "/health" {
		http.NotFound(w, r)
		return
	}

	// Sanitiza: sin traversal.
	clean := filepath.Clean(strings.TrimPrefix(path, "/"))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	if clean == "." {
		// Raíz: sirve el index con cabecera no-cache.
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, s.Index)
		return
	}

	full := filepath.Join(s.Dir, clean)

	// Cabeceras de caché (misma política que el servidor Node).
	if strings.HasPrefix(path, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else if path == "/" || path == "/index.html" || path == "/sw.js" || path == "/manifest.webmanifest" {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}

	if info, err := os.Stat(full); err == nil && !info.IsDir() {
		http.ServeFile(w, r, full)
		return
	}

	// Fallback SPA: cualquier otra ruta sirve index.html.
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeFile(w, r, s.Index)
}

package api

import (
	"net/http"
	"net/url"
	"strings"
)

func sameOriginOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			next.ServeHTTP(w, r)
			return
		}

		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			origin = strings.TrimSpace(r.Header.Get("Referer"))
		}
		if origin != "" && !matchesRequestHost(origin, r.Host) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "origen no permitido"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func matchesRequestHost(raw, host string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return strings.EqualFold(strings.TrimSuffix(u.Host, "."), strings.TrimSuffix(host, "."))
}

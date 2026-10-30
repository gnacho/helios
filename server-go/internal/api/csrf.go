package api

import (
	"log"
	"net"
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
		forwardedHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
		if origin != "" && !matchesRequestHost(origin, r.Host, forwardedHost) {
			log.Printf("[helios] csrf: origen no permitido origin=%q host=%q x-forwarded-host=%q", origin, r.Host, forwardedHost)
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "origen no permitido"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// matchesRequestHost accepts a request whose Origin/Referer points at the
// host the browser used. Behind a reverse proxy r.Host can be an internal
// address while the browser sees the public one, which the proxy passes in
// X-Forwarded-Host; ports are ignored because they can differ between the
// two hops.
func matchesRequestHost(raw, host, forwardedHost string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	originHost := normalizeHost(u.Host)
	if originHost == "" {
		return false
	}
	if originHost == normalizeHost(host) {
		return true
	}
	// X-Forwarded-Host may carry a comma-separated chain; the first value is
	// the one the client originally requested.
	if first, _, ok := strings.Cut(forwardedHost, ","); ok {
		forwardedHost = first
	}
	return originHost == normalizeHost(forwardedHost)
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

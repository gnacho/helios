package api

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) sameOriginOnly(next http.Handler) http.Handler {
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
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		forwardedHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
		if matchesRequestHost(origin, r.Host, forwardedHost) {
			next.ServeHTTP(w, r)
			return
		}

		// A service worker (pushsubscriptionchange) and a few sandboxed contexts
		// send a null origin, with no way to name the host they belong to. The
		// session cookie is SameSite=Lax, so a cross-site POST never carries it:
		// requiring a valid session keeps the check meaningful.
		if strings.EqualFold(origin, "null") && s.hasValidSession(r) {
			log.Printf("[helios] csrf: origen nulo con sesión válida path=%q", r.URL.Path)
			next.ServeHTTP(w, r)
			return
		}

		log.Printf("[helios] csrf: origen no permitido origin=%q host=%q x-forwarded-host=%q", origin, r.Host, forwardedHost)
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "origen no permitido"})
	})
}

func (s *Server) hasValidSession(r *http.Request) bool {
	return s.Auth != nil && s.Auth.SessionIDFromCookie(r) != nil
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

// normalizeHost drops the port, the brackets around an IPv6 literal and the
// trailing dot of a fully qualified name, then folds case.
func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

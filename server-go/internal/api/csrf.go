package api

import (
	"log"
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
		if origin != "" && !matchesAnyRequestHost(origin, r) {
			// Sin estos valores, un rechazo legítimo detrás de un proxy es
			// indistinguible de un ataque y no hay forma de diagnosticarlo.
			log.Printf("[api] origen rechazado origin=%q host=%q x-forwarded-host=%q",
				origin, r.Host, r.Header.Get("X-Forwarded-Host"))
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "origen no permitido"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// matchesAnyRequestHost acepta el origen si coincide con el Host de la petición
// o con el host que anuncia el proxy. Detrás de un reverse proxy el Host puede
// venir reescrito (el de la aplicación) mientras el navegador firma el público.
func matchesAnyRequestHost(origin string, r *http.Request) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	want := hostOnly(u.Host)
	for _, candidate := range requestHosts(r) {
		if strings.EqualFold(hostOnly(candidate), want) {
			return true
		}
	}
	return false
}

func requestHosts(r *http.Request) []string {
	hosts := []string{r.Host}
	for _, raw := range strings.Split(r.Header.Get("X-Forwarded-Host"), ",") {
		if h := strings.TrimSpace(raw); h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

// hostOnly deja el host sin puerto ni punto final: el puerto no cambia el sitio.
func hostOnly(host string) string {
	h := strings.TrimSuffix(strings.TrimSpace(host), ".")
	if i := strings.LastIndex(h, ":"); i > 0 && !strings.Contains(h[i+1:], "]") {
		return strings.TrimSuffix(h[:i], ".")
	}
	return h
}

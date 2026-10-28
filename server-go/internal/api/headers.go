package api

import "net/http"

// defensiveHeaders sets conservative defaults for every response. The content
// security policy is deliberately absent: the UI relies on inline Datastar
// attributes, so a blocking policy needs its own audit.
func defensiveHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=()")
		next.ServeHTTP(w, r)
	})
}

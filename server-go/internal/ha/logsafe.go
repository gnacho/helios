package ha

import (
	"net/url"
	"strconv"
	"strings"
)

// LogSafeURL renders a Home Assistant URL without credentials, query, fragment
// or control characters, so it can be logged safely.
func LogSafeURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "invalid"
	}
	return sanitize(u.Scheme + "://" + u.Host + u.Path)
}

func sanitize(v string) string {
	if len(v) > 160 {
		v = v[:160] + "..."
	}
	q := strconv.QuoteToASCII(v)
	return q[1 : len(q)-1]
}

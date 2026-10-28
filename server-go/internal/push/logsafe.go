package push

import (
	"net/url"
	"strconv"
	"strings"
)

// maxLogValueLen bounds untrusted values written to the log.
const maxLogValueLen = 160

// logSafeValue renders untrusted text as a single printable line so a remote
// endpoint or upstream error cannot forge extra log entries.
func logSafeValue(v string) string {
	if len(v) > maxLogValueLen {
		v = v[:maxLogValueLen] + "..."
	}
	q := strconv.QuoteToASCII(v)
	return q[1 : len(q)-1]
}

// redactEndpoint logs only the scheme and host of a push endpoint: the path is
// a capability token that must not reach the log.
func redactEndpoint(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "invalid"
	}
	return u.Scheme + "://" + u.Host
}

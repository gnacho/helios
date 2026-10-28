package web

import "strconv"

// maxLogValueLen keeps a single log line readable and bounds attacker input.
const maxLogValueLen = 160

// logSafeValue turns untrusted input (URL paths, upstream errors) into a single
// printable line, so it cannot forge extra log entries or break the format.
// Control characters and non-ASCII bytes are escaped by QuoteToASCII.
func logSafeValue(v string) string {
	if len(v) > maxLogValueLen {
		v = v[:maxLogValueLen] + "..."
	}
	q := strconv.QuoteToASCII(v)
	return q[1 : len(q)-1]
}

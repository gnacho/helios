package extensions

import (
	"strconv"
	"strings"
)

// parseFloatStrict replica parseFloat + Number.isFinite.
func parseFloatStrict(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// parseFloat acepta prefijos numéricos ("3.5 kW"); replicamos ese
	// comportamiento tomando el prefijo válido.
	end := 0
	seenDigit := false
	seenDot := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			seenDigit = true
			end = i + 1
			continue
		}
		if c == '.' && !seenDot {
			seenDot = true
			end = i + 1
			continue
		}
		if (c == '-' || c == '+') && i == 0 {
			end = i + 1
			continue
		}
		if (c == 'e' || c == 'E') && seenDigit {
			// exponente
			j := i + 1
			if j < len(s) && (s[j] == '+' || s[j] == '-') {
				j++
			}
			k := j
			for k < len(s) && s[k] >= '0' && s[k] <= '9' {
				k++
			}
			if k > j {
				end = k
				break
			}
		}
		break
	}
	if !seenDigit {
		return nil
	}
	f, err := strconv.ParseFloat(s[:end], 64)
	if err != nil {
		return nil
	}
	return &f
}

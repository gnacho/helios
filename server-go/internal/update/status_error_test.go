package update

import (
	"errors"
	"testing"
)

// #228: the handler used to build a "no update available" status when the
// lookup failed, so the UI claimed up to date without having compared anything.
func TestStatusError(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		err     error
		want    string
	}{
		{"both resolved", "1.2.5", "1.2.9", nil, ""},
		{"github unreachable", "1.2.5", "", errors.New("dial tcp: timeout"), "no se pudo consultar GitHub"},
		{"github answered without a release", "1.2.5", "", nil, "GitHub no devolvió ninguna versión publicada"},
		{"marker missing", "", "1.2.9", nil, "no se pudo leer la versión instalada"},
		{"everything missing", "", "", nil, "GitHub no devolvió ninguna versión publicada"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusError(tc.current, tc.latest, tc.err); got != tc.want {
				t.Fatalf("statusError(%q, %q, %v) = %q, want %q", tc.current, tc.latest, tc.err, got, tc.want)
			}
		})
	}
}

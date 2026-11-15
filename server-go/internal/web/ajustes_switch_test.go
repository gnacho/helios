package web

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// The styled checkbox is used across Settings. Its knob used to sit still when
// toggled: the class driving it was `peer-checked:translate-x-4`, and that
// variant only reaches a following sibling of the peer input, while the knob is
// a grandchild inside the track (#226). This pins the mechanism that works.
func TestAjCheckboxKnobMovesWithTheInput(t *testing.T) {
	var buf bytes.Buffer
	if err := ajCheckbox("demo", true, "Etiqueta").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()

	if !strings.Contains(html, "group/switch") {
		t.Fatalf("label lost its named group: %s", html)
	}
	if !strings.Contains(html, "group-has-[:checked]/switch:left-[1.125rem]") {
		t.Fatalf("knob does not move with a has() condition: %s", html)
	}
	if strings.Contains(html, "peer-checked:translate-x-4") {
		t.Fatalf("knob is back on a sibling selector that cannot reach it: %s", html)
	}
	// The track itself keeps using the sibling variant, which is correct there.
	if !strings.Contains(html, "peer-checked:bg-brand") {
		t.Fatalf("track lost its checked colour: %s", html)
	}
}

package web

import (
	"testing"

	"helios-server/internal/solar"
)

// TestFlowHomeColor covers the four sources of the home node: the color follows
// the dominant supply (solar, battery, grid import) and falls back to the faint
// grey when nothing is feeding the house.
func TestFlowHomeColor(t *testing.T) {
	cons := func(v float64) *float64 { return &v }
	cases := []struct {
		name string
		live solar.LiveData
		want string
	}{
		{
			name: "fv dominante",
			live: solar.LiveData{Production: 3, Consumption: cons(0.5), BatteryPower: 0, Grid: cons(0)},
			want: "var(--c-solar)",
		},
		{
			name: "batería dominante",
			live: solar.LiveData{Production: 0.1, Consumption: cons(0.5), BatteryPower: -2, Grid: cons(0)},
			want: "var(--c-bateria)",
		},
		{
			name: "red dominante",
			live: solar.LiveData{Production: 0, Consumption: cons(0.5), BatteryPower: 0, Grid: cons(1.2)},
			want: "var(--c-red-compra)",
		},
		{
			name: "sin aportes",
			live: solar.LiveData{Production: 0, Consumption: cons(0), BatteryPower: 0, Grid: cons(0)},
			want: "var(--text-faint)",
		},
	}
	for _, c := range cases {
		live := c.live
		m := buildFlow("es", &live)
		if m.HomeColor != c.want {
			t.Fatalf("%s: HomeColor = %q (esperado %q)", c.name, m.HomeColor, c.want)
		}
	}
}

package web

import "testing"

// Same inputs and expected outputs as the TS sources (thresholds.ts,
// SocHeroGauge.tsx arcColor, lib/battery.ts batteryIcon).
func TestSocGaugeColor(t *testing.T) {
	pal := newPalette(false)
	cases := []struct {
		soc  float64
		want string
	}{
		{0, pal.RedCompra},
		{14.9, pal.RedCompra},
		{15, pal.Solar},
		{20, pal.Solar},
		{24.9, pal.Solar},
		{25, pal.Bateria},
		{84.6, pal.Bateria},
		{100, pal.Bateria},
		// fuera de rango: se recorta como en el TS (Math.min/max)
		{-5, pal.RedCompra},
		{130, pal.Bateria},
	}
	for _, c := range cases {
		if got := socGaugeColor(c.soc, pal); got != c.want {
			t.Fatalf("socGaugeColor(%v) = %v (esperado %v)", c.soc, got, c.want)
		}
	}
}

func TestBatteryIconKey(t *testing.T) {
	cases := []struct {
		soc      float64
		charging bool
		want     string
	}{
		{100, true, "charging"}, // el flujo de carga manda aunque esté llena
		{50, true, "charging"},
		{100, false, "full"},
		{80, false, "full"}, // >= 80
		{79.9, false, "medium"},
		{30.1, false, "medium"},
		{30, false, "low"}, // > 30 estricto
		{20.1, false, "low"},
		{20, false, "warning"}, // > 20 estricto
		{0, false, "warning"},
	}
	for _, c := range cases {
		if got := batteryIconKey(c.soc, c.charging); got != c.want {
			t.Fatalf("batteryIconKey(%v, %v) = %q (esperado %q)", c.soc, c.charging, got, c.want)
		}
	}
}

func TestClampPct(t *testing.T) {
	if clampPct(-1) != 0 || clampPct(50) != 50 || clampPct(101) != 100 {
		t.Fatal("clampPct fuera de rango")
	}
}

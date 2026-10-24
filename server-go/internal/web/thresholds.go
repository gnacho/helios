package web

// Thresholds ported from app/src/lib/thresholds.ts (single source for the
// battery magic numbers), plus the SOC color bands of SocHeroGauge.tsx and
// the batteryIcon() ladder of app/src/lib/battery.ts. The tests replicate the
// TS behavior for the same inputs.

// BatteryReservePct is BATTERY_RESERVE_PCT: below this the battery counts as
// "in reserve" (gauge tick, SOC day chart zone, Ajustes slider default).
const BatteryReservePct = 20

// BatteryFullPct is BATTERY_FULL_PCT: SoC displayed as full in gauges.
const BatteryFullPct = 100

// clampPct clamps a percentage to 0..100.
func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// socGaugeColor mirrors the arcColor pick in SocHeroGauge.tsx:
// below 15 red (under reserve), below 25 amber, else battery green.
func socGaugeColor(soc float64, p Palette) string {
	c := clampPct(soc)
	if c < 15 {
		return p.RedCompra
	}
	if c < 25 {
		return p.Solar
	}
	return p.Bateria
}

// batteryIconKey mirrors batteryIcon() in lib/battery.ts. Evaluation order
// matters: charging wins even at 100% (the inverter rarely reports "charging"
// at full, and the bolt communicates active flow).
func batteryIconKey(soc float64, charging bool) string {
	if charging {
		return "charging"
	}
	switch {
	case soc >= 80:
		return "full"
	case soc > 30:
		return "medium"
	case soc > 20:
		return "low"
	}
	return "warning"
}

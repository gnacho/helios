package web

import "helios-server/internal/solar"

// DayKpis + integration ported from integrateSeries() in
// app/src/data/EnergyDataProvider.tsx (exact same semantics, tests assert the
// same numbers for the same series).

const stepMin = 5 // STEP_MIN

// DayKpis are the integrated day KPIs (up to a minute of the day).
type DayKpis struct {
	ProductionKwh        float64
	ConsumptionKwh       float64
	GridImportKwh        float64
	GridExportKwh        float64
	BatteryChargedKwh    float64
	BatteryDischargedKwh float64
	Soc                  float64
	AutoconsumoPct       float64
	AutosuficienciaPct   float64
	PeakProductionKw     float64
	PeakAt               int
}

// IntegrateSeries replicates integrateSeries(series, untilMin, socFallback).
func IntegrateSeries(points []solar.DayPoint, untilMin int, socFallback float64) DayKpis {
	dtH := float64(stepMin) / 60
	var k DayKpis
	k.Soc = socFallback
	k.PeakAt = 14 * 60

	for _, p := range points {
		if p.T >= untilMin {
			break
		}
		k.ProductionKwh += p.Production * dtH
		k.ConsumptionKwh += p.Consumption * dtH
		if p.Grid != nil {
			if *p.Grid > 0 {
				k.GridImportKwh += *p.Grid * dtH
			} else {
				k.GridExportKwh += -*p.Grid * dtH
			}
		}
		if p.BatteryPower > 0 {
			k.BatteryChargedKwh += p.BatteryPower * dtH
		}
		if p.BatteryPower < 0 {
			k.BatteryDischargedKwh += -p.BatteryPower * dtH
		}
		if p.Production > k.PeakProductionKw {
			k.PeakProductionKw = p.Production
			k.PeakAt = p.T
		}
		if p.Soc > 0 {
			k.Soc = p.Soc
		}
	}

	if k.ProductionKwh > 0 {
		k.AutoconsumoPct = (k.ProductionKwh - k.GridExportKwh) / k.ProductionKwh * 100
		if k.AutoconsumoPct > 100 {
			k.AutoconsumoPct = 100
		}
	}
	if k.ConsumptionKwh > 0 {
		k.AutosuficienciaPct = (k.ConsumptionKwh - k.GridImportKwh) / k.ConsumptionKwh * 100
		if k.AutosuficienciaPct > 100 {
			k.AutosuficienciaPct = 100
		}
	}
	return k
}

// perInverterKwh replicates the Dashboard per-inverter integration:
// sum of seriesInvValue(p, key) * STEP_MIN/60 for p.t < untilMin.
func perInverterKwh(points []solar.DayPoint, key string, untilMin int) float64 {
	dtH := float64(stepMin) / 60
	var sum float64
	for _, p := range points {
		if p.T >= untilMin {
			break
		}
		sum += seriesInvValue(p, key) * dtH
	}
	return sum
}

// seriesInvValue mirrors seriesInvValue() from app/src/data/types.ts.
func seriesInvValue(p solar.DayPoint, key string) float64 {
	if p.Inverters != nil {
		if v, ok := p.Inverters[key]; ok {
			return v
		}
	}
	switch key {
	case "solis":
		return p.Solis
	case "fox":
		return p.Fox
	}
	return 0
}

// batteryState mirrors batteryState() in Dashboard.tsx.
func batteryState(bp float64) string {
	switch {
	case bp > 0.05:
		return "charging"
	case bp < -0.05:
		return "discharging"
	default:
		return "idle"
	}
}

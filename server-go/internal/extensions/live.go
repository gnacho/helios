package extensions

import (
	"math"
	"sort"

	"helios-server/internal/ha"
	"helios-server/internal/install"
)

// HA es lo que extensions necesita de Home Assistant.
type HA interface {
	GetState(id string) (ha.EntityState, bool)
	IsConnected() bool
	StatisticsDuringPeriod(startTime, endTime string, statisticIDs []string, period string, types []string) (ha.Stats, error)
	HistoryDuringPeriod(startTime, endTime, entityID string) ([]ha.HistoryRow, error)
	CallService(domain, service string, data map[string]any) error
}

// Los campos opcionales van con omitempty: el server Node omitía las claves
// undefined y el front distingue "sin dato" (undefined) de null.
type LiveCharger struct {
	Key        string    `json:"key"`
	Name       string    `json:"name"`
	Charging   bool      `json:"charging"`
	Connected  bool      `json:"connected"`
	State      *string   `json:"state,omitempty"`
	PowerKw    float64   `json:"powerKw"`
	SessionKwh *float64  `json:"sessionKwh,omitempty"`
	TotalKwh   *float64  `json:"totalKwh,omitempty"`
	TempC      *float64  `json:"tempC,omitempty"`
	SwitchOn   *bool     `json:"switchOn,omitempty"`
	Amps       *float64  `json:"amps,omitempty"`
	AmpOptions []float64 `json:"ampOptions,omitempty"`
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// entityNumOrUndefined devuelve nil si no hay entidad o está unavailable/unknown.
func entityNumOrUndefined(h HA, id string) *float64 {
	if id == "" {
		return nil
	}
	st, ok := h.GetState(id)
	if !ok || st.State == "unavailable" || st.State == "unknown" {
		return nil
	}
	f := parseFloatStrict(st.State)
	if f == nil {
		return nil
	}
	return f
}

func divOpt(v *float64, divisor int) *float64 {
	if v == nil {
		return nil
	}
	if divisor == 0 {
		divisor = 1
	}
	r := round3(*v / float64(divisor))
	return &r
}

func entityStateOrUndefined(h HA, id string) *string {
	if id == "" {
		return nil
	}
	st, ok := h.GetState(id)
	if !ok || st.State == "unavailable" || st.State == "unknown" {
		return nil
	}
	s := st.State
	return &s
}

// ComputeOneChargerLive replica computeOneChargerLive().
func ComputeOneChargerLive(h HA, c Charger) LiveCharger {
	rawPower := entityNumOrUndefined(h, c.PowerID)
	var powerKw *float64
	if rawPower != nil {
		var v float64
		if c.PowerUnit == UnitW {
			v = round3(*rawPower / 1000)
		} else {
			v = round3(*rawPower)
		}
		powerKw = &v
	}

	state := entityStateOrUndefined(h, c.StateID)

	charging := false
	if state != nil && containsStr(c.ChargingStates, *state) {
		charging = true
	} else if powerKw != nil && *powerKw > 0.05 {
		charging = true
	}

	connected := charging
	if !connected && state != nil && containsStr(c.ConnectedStates, *state) {
		connected = true
	}

	sw := entityStateOrUndefined(h, c.SwitchID)

	var ampOptions []float64
	if c.AmpID != "" {
		if ent, ok := h.GetState(c.AmpID); ok && ent.Attributes != nil {
			if opts, ok := ent.Attributes["options"].([]any); ok {
				for _, o := range opts {
					if f := toFloat(o); f != nil {
						ampOptions = append(ampOptions, *f)
					}
				}
				sort.Float64s(ampOptions)
			}
		}
	}

	out := LiveCharger{
		Key:        c.Key,
		Name:       c.Name,
		Charging:   charging,
		Connected:  connected,
		State:      state,
		SessionKwh: divOpt(entityNumOrUndefined(h, c.EnergySessionID), c.EnergyDivisor),
		TotalKwh:   divOpt(entityNumOrUndefined(h, c.EnergyTotalID), c.EnergyDivisor),
		TempC:      entityNumOrUndefined(h, c.TempID),
		Amps:       entityNumOrUndefined(h, c.AmpID),
		AmpOptions: ampOptions,
	}
	if powerKw != nil {
		out.PowerKw = *powerKw
	}
	if sw != nil {
		on := *sw == "on"
		out.SwitchOn = &on
	}
	return out
}

// ComputeChargerLive devuelve todos los cargadores activos.
func ComputeChargerLive(h HA, c Config) []LiveCharger {
	if !ChargerActive(c) {
		return []LiveCharger{}
	}
	out := make([]LiveCharger, 0, len(c.CarCharger.Chargers))
	for _, ch := range c.CarCharger.Chargers {
		out = append(out, ComputeOneChargerLive(h, ch))
	}
	return out
}

// PVShareNow: fracción [0..1] del delta que pudo venir de excedente FV.
func PVShareNow(h HA, store *install.Store, chargerPowerKw *float64, c Charger) float64 {
	t := store.Get()

	production := 0.0
	for _, inv := range t.Inverters {
		raw := entityNumOrUndefined(h, inv.PowerID)
		if raw == nil {
			continue
		}
		if inv.PowerUnit == install.UnitW {
			production += *raw / 1000
		} else {
			production += *raw
		}
	}

	consDiv := 1.0
	if t.Consumption.PowerUnit == install.UnitW {
		consDiv = 1000
	}
	consumption := 0.0
	for _, id := range t.Consumption.PowerIDs {
		if raw := entityNumOrUndefined(h, id); raw != nil {
			consumption += *raw / consDiv
		}
	}

	if chargerPowerKw != nil && *chargerPowerKw > 0.05 {
		other := consumption
		if c.ChargerInHouseMeters {
			other = consumption - *chargerPowerKw
		}
		if other < 0 {
			other = 0
		}
		solarToCharger := math.Min(*chargerPowerKw, math.Max(0, production-other))
		return math.Max(0, math.Min(1, solarToCharger/(*chargerPowerKw)))
	}

	if consumption > 0.01 {
		return math.Max(0, math.Min(1, production/consumption))
	}
	if production > 0 {
		return 1
	}
	return 0
}

func containsStr(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func toFloat(v any) *float64 {
	switch x := v.(type) {
	case float64:
		return &x
	case int:
		f := float64(x)
		return &f
	case string:
		return parseFloatStrict(x)
	}
	return nil
}

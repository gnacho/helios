package extensions

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"helios-server/internal/ha"
	"helios-server/internal/install"
	"helios-server/internal/solar"
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
	Key        string  `json:"key"`
	Name       string  `json:"name"`
	Charging   bool    `json:"charging"`
	Connected  bool    `json:"connected"`
	State      *string `json:"state,omitempty"`
	PowerKw    float64 `json:"powerKw"`
	PowerKnown bool    `json:"powerKnown"`
	// PowerAgeS es la antigüedad en segundos del último dato de potencia.
	// localtuya puede dejar el socket abierto sin recibir frames: sin esto,
	// un 0 kW viejo parece una lectura real.
	PowerAgeS int `json:"powerAgeS,omitempty"`
	// DeviceOffline indica que el aparato no está disponible en HA (apagado o
	// desenchufado), que es distinto de estar encendido y callado.
	DeviceOffline bool      `json:"deviceOffline,omitempty"`
	SessionKwh    *float64  `json:"sessionKwh,omitempty"`
	TotalKwh      *float64  `json:"totalKwh,omitempty"`
	TempC         *float64  `json:"tempC,omitempty"`
	SwitchOn      *bool     `json:"switchOn,omitempty"`
	Amps          *float64  `json:"amps,omitempty"`
	AmpOptions    []float64 `json:"ampOptions,omitempty"`
	AmpMin        *float64  `json:"ampMin,omitempty"`
	AmpMax        *float64  `json:"ampMax,omitempty"`
	AmpStep       *float64  `json:"ampStep,omitempty"`
	Delay         *float64  `json:"delay,omitempty"`
	Duration      *float64  `json:"duration,omitempty"`
	// Programación por software (wall): hora de inicio, duración y armado.
	ScheduleStart string   `json:"scheduleStart,omitempty"`
	ScheduleHours *float64 `json:"scheduleHours,omitempty"`
	Scheduled     bool     `json:"scheduled,omitempty"`
	// TimerRemainingS son los segundos que le quedan al timer HA de la carga
	// programada (solo mientras está active): da la hora prevista de fin.
	TimerRemainingS *int `json:"timerRemainingS,omitempty"`
}

// timerRemainingSeconds lee cuántos segundos le quedan a un timer helper de
// HA. Prefiere finishes_at (RFC3339, presente mientras está active); si no
// viene, cae al atributo remaining ("HH:MM:SS" o segundos planos).
func timerRemainingSeconds(st ha.EntityState) (int, bool) {
	if st.Attributes != nil {
		if v, ok := st.Attributes["finishes_at"].(string); ok && v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				secs := int(time.Until(t).Seconds())
				if secs < 0 {
					secs = 0
				}
				return secs, true
			}
		}
		if v, ok := st.Attributes["remaining"].(string); ok && v != "" {
			parts := strings.Split(v, ":")
			if len(parts) >= 1 && len(parts) <= 3 {
				secs, bad := 0, false
				for i, p := range parts {
					n, err := strconv.Atoi(strings.TrimSpace(p))
					if err != nil {
						bad = true
						break
					}
					secs += n * []int{3600, 60, 1}[i+3-len(parts)]
				}
				if !bad {
					return secs, true
				}
			}
			if n, err := strconv.Atoi(v); err == nil {
				return n, true
			}
		}
	}
	return 0, false
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

	// Antigüedad del dato de potencia: el aparato puede dejar el socket
	// abierto sin enviar frames y la lectura se quedaría congelada.
	powerAgeS := 0
	if c.PowerID != "" {
		if ent, ok := h.GetState(c.PowerID); ok && ent.LastUpdated != "" {
			if ts, err := time.Parse(time.RFC3339, ent.LastUpdated); err == nil {
				if age := time.Since(ts); age > 0 {
					powerAgeS = int(age / time.Second)
				}
			}
		}
	}

	// Aparato ausente: todas sus entidades configuradas sin datos. Un cargador
	// portátil solo se enciende cuando se va a usar, así que este es su estado
	// normal y no un fallo de comunicaciones.
	offline, seen := true, false
	for _, id := range []string{c.PowerID, c.StateID, c.SwitchID} {
		if id == "" {
			continue
		}
		seen = true
		if ent, ok := h.GetState(id); ok && ent.State != "unavailable" && ent.State != "unknown" {
			offline = false
		}
	}
	deviceOffline := seen && offline

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
	var ampMin, ampMax, ampStep *float64
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
			ampMin = toFloat(ent.Attributes["min"])
			ampMax = toFloat(ent.Attributes["max"])
			ampStep = toFloat(ent.Attributes["step"])
		}
	}

	out := LiveCharger{
		Key:           c.Key,
		Name:          c.Name,
		Charging:      charging,
		Connected:     connected,
		State:         state,
		SessionKwh:    divOpt(entityNumOrUndefined(h, c.EnergySessionID), c.EnergyDivisor),
		TotalKwh:      divOpt(entityNumOrUndefined(h, c.EnergyTotalID), c.EnergyDivisor),
		TempC:         entityNumOrUndefined(h, c.TempID),
		Amps:          entityNumOrUndefined(h, c.AmpID),
		AmpOptions:    ampOptions,
		AmpMin:        ampMin,
		AmpMax:        ampMax,
		AmpStep:       ampStep,
		Delay:         entityNumOrUndefined(h, c.DelayID),
		Duration:      entityNumOrUndefined(h, c.DurationID),
		PowerAgeS:     powerAgeS,
		DeviceOffline: deviceOffline,

		ScheduleHours: entityNumOrUndefined(h, c.ScheduleDurationID),
	}
	if c.ScheduleArmID != "" {
		if st, ok := h.GetState(c.ScheduleArmID); ok && st.State == "on" {
			out.Scheduled = true
		}
	}
	if c.ScheduleStartID != "" {
		if st, ok := h.GetState(c.ScheduleStartID); ok {
			out.ScheduleStart = st.State
		}
	}
	if c.ScheduleTimerID != "" {
		if st, ok := h.GetState(c.ScheduleTimerID); ok && st.State == "active" {
			if secs, ok := timerRemainingSeconds(st); ok {
				out.TimerRemainingS = &secs
			}
		}
	}
	if powerKw != nil {
		out.PowerKw = *powerKw
		out.PowerKnown = true
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

// PVShareNow devuelve la fracción [0..1] del delta del cargador que pudo venir
// de excedente FV y si el reparto se puede calcular. El resto de casa sale del
// balance de la casa (producción + red - batería), el mismo que alimenta el
// consumo en vivo, y no de las pinzas, que no cubren la casa. Con fuente de red
// configurada y caída no hay balance: el reparto se declara no disponible en vez
// de calcularse sobre una base falsa.
func PVShareNow(h HA, store *install.Store, chargerPowerKw *float64, c Charger) (float64, bool) {
	t := store.Get()

	metersKw, metersDown := consumptionMetersKw(h, t)
	p := solar.ReadLivePower(t, h)
	house := p.HouseConsumption(metersKw, metersDown)
	if !house.OK {
		return 0, false
	}

	if chargerPowerKw != nil && *chargerPowerKw > 0.05 {
		other := restOfHouseKw(house, *chargerPowerKw, c.ChargerInHouseMeters)
		solarToCharger := math.Min(*chargerPowerKw, math.Max(0, p.Production-other))
		return math.Max(0, math.Min(1, solarToCharger/(*chargerPowerKw))), true
	}

	if house.Kw > 0.01 {
		return math.Max(0, math.Min(1, p.Production/house.Kw)), true
	}
	if p.Production > 0 {
		return 1, true
	}
	return 0, true
}

// restOfHouseKw es la casa sin el cargador. El balance es de casa entera y
// siempre incluye el cargador, así que se descuenta siempre. Las pinzas solo
// cubren los circuitos que miden, así que ahí solo se descuenta si el cargador
// está detrás de ellas.
func restOfHouseKw(house solar.HouseConsumption, chargerKw float64, chargerInMeters bool) float64 {
	other := house.Kw
	if house.FromGrid || chargerInMeters {
		other -= chargerKw
	}
	if other < 0 {
		other = 0
	}
	return other
}

// consumptionMetersKw suma las pinzas de consumo y dice si alguna está caída.
func consumptionMetersKw(h HA, t install.Topology) (float64, bool) {
	div := 1.0
	if t.Consumption.PowerUnit == install.UnitW {
		div = 1000
	}
	total := 0.0
	down := false
	for _, id := range t.Consumption.PowerIDs {
		v := entityNumOrUndefined(h, id)
		if v == nil {
			down = true
			continue
		}
		total += *v / div
	}
	return total, down
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

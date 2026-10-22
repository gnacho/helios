package extensions

import (
	"encoding/json"
	"math"
	"regexp"
	"time"
)

type BydConfig struct {
	Enabled           bool   `json:"enabled"`
	Name              string `json:"name"`
	SocID             string `json:"socId"`
	RangeID           string `json:"rangeId"`
	OdometerID        string `json:"odometerId"`
	BatteryPowerID    string `json:"batteryPowerId"`
	ChargingID        string `json:"chargingId"`
	PluggedID         string `json:"pluggedId"`
	OnlineID          string `json:"onlineId"`
	LockedID          string `json:"lockedId"`
	DoorsID           string `json:"doorsId"`
	WindowsID         string `json:"windowsId"`
	SentryID          string `json:"sentryId"`
	CabinTempID       string `json:"cabinTempId"`
	ExteriorTempID    string `json:"exteriorTempId"`
	TireFlID          string `json:"tireFlId"`
	TireFrID          string `json:"tireFrId"`
	TireRlID          string `json:"tireRlId"`
	TireRrID          string `json:"tireRrId"`
	LocationID        string `json:"locationId"`
	GpsAgeID          string `json:"gpsAgeId"`
	LastUpdateID      string `json:"lastUpdateId"`
	StartChargeID     string `json:"startChargeId"`
	StopChargeID      string `json:"stopChargeId"`
	ForcePollID       string `json:"forcePollId"`
	ChargeToFullID    string `json:"chargeToFullId"`
	ScheduleEnabledID string `json:"scheduleEnabledId"`
	ScheduleStartID   string `json:"scheduleStartId"`
	ScheduleEndID     string `json:"scheduleEndId"`
	RepeatDailyID     string `json:"repeatDailyId"`
	// Opcionales (v2: control + consumo + tiempo restante)
	LockID                *string `json:"lockId,omitempty"`
	AcSwitchID            *string `json:"acSwitchId,omitempty"`
	FlashLightsID         *string `json:"flashLightsId,omitempty"`
	FindCarID             *string `json:"findCarId,omitempty"`
	CloseWindowsID        *string `json:"closeWindowsId,omitempty"`
	RemainingHoursID      *string `json:"remainingHoursId,omitempty"`
	RemainingMinutesID    *string `json:"remainingMinutesId,omitempty"`
	ConsumptionRecentID   *string `json:"consumptionRecentId,omitempty"`
	Consumption50ID       *string `json:"consumption50Id,omitempty"`
	ConsumptionLifetimeID *string `json:"consumptionLifetimeId,omitempty"`
	ConsumptionTodayID    *string `json:"consumptionTodayId,omitempty"`
}

var LegacyByd = BydConfig{
	Enabled: false, Name: "BYD Atto 3",
	SocID:             "sensor.byd_atto_3_nivel_de_bateria",
	RangeID:           "sensor.byd_atto_3_autonomia",
	OdometerID:        "sensor.byd_atto_3_cuentakilometros",
	BatteryPowerID:    "sensor.byd_atto_3_battery_power",
	ChargingID:        "binary_sensor.byd_atto_3_cargando",
	PluggedID:         "binary_sensor.byd_atto_3_enchufe",
	OnlineID:          "binary_sensor.byd_atto_3_en_linea",
	LockedID:          "binary_sensor.byd_atto_3_bloqueado",
	DoorsID:           "binary_sensor.byd_atto_3_puertas_abiertas",
	WindowsID:         "binary_sensor.byd_atto_3_ventanas_abiertas",
	SentryID:          "binary_sensor.byd_atto_3_modo_centinela",
	CabinTempID:       "sensor.byd_atto_3_temperatura_de_la_cabina",
	ExteriorTempID:    "sensor.byd_atto_3_temperatura_exterior",
	TireFlID:          "sensor.byd_atto_3_presion_neumatico_delantero_izquierdo",
	TireFrID:          "sensor.byd_atto_3_presion_neumatico_delantero_derecho",
	TireRlID:          "sensor.byd_atto_3_presion_neumatico_trasero_izquierdo",
	TireRrID:          "sensor.byd_atto_3_presion_neumatico_trasero_derecho",
	LocationID:        "device_tracker.byd_atto_3_ubicacion",
	GpsAgeID:          "sensor.byd_atto_3_gps_ultima_actualizacion",
	LastUpdateID:      "sensor.byd_atto_3_ultima_actualizacion",
	StartChargeID:     "button.byd_atto_3_start_charging",
	StopChargeID:      "button.byd_atto_3_stop_charging",
	ForcePollID:       "button.byd_atto_3_forzar_consulta",
	ChargeToFullID:    "switch.byd_atto_3_charge_to_full",
	ScheduleEnabledID: "switch.byd_atto_3_schedule_enabled",
	ScheduleStartID:   "time.byd_atto_3_start_time",
	ScheduleEndID:     "time.byd_atto_3_end_time",
	RepeatDailyID:     "switch.byd_atto_3_repeat_daily",
}

var GenericByd = BydConfig{
	Enabled: false, Name: "BYD",
	SocID:             "sensor.byd_battery",
	RangeID:           "sensor.byd_range",
	OdometerID:        "sensor.byd_odometer",
	BatteryPowerID:    "sensor.byd_battery_power",
	ChargingID:        "binary_sensor.byd_charging",
	PluggedID:         "binary_sensor.byd_plug",
	OnlineID:          "binary_sensor.byd_online",
	LockedID:          "binary_sensor.byd_locked",
	DoorsID:           "binary_sensor.byd_doors_open",
	WindowsID:         "binary_sensor.byd_windows_open",
	SentryID:          "binary_sensor.byd_sentry",
	CabinTempID:       "sensor.byd_cabin_temp",
	ExteriorTempID:    "sensor.byd_exterior_temp",
	TireFlID:          "sensor.byd_tire_fl",
	TireFrID:          "sensor.byd_tire_fr",
	TireRlID:          "sensor.byd_tire_rl",
	TireRrID:          "sensor.byd_tire_rr",
	LocationID:        "device_tracker.byd_location",
	GpsAgeID:          "sensor.byd_gps_last_updated",
	LastUpdateID:      "sensor.byd_last_updated",
	StartChargeID:     "button.byd_start_charging",
	StopChargeID:      "button.byd_stop_charging",
	ForcePollID:       "button.byd_force_poll",
	ChargeToFullID:    "switch.byd_charge_to_full",
	ScheduleEnabledID: "switch.byd_schedule_enabled",
	ScheduleStartID:   "time.byd_start_time",
	ScheduleEndID:     "time.byd_end_time",
	RepeatDailyID:     "switch.byd_repeat_daily",
}

// NormalizeByd replica {...base.byd, ...rawByd}: el spread de JS sobrescribe
// cada campo presente en raw (aunque sea vacío) y hereda los ausentes.
func NormalizeByd(raw map[string]any, base BydConfig) BydConfig {
	// base -> JSON -> map; luego overlay raw; luego unmarshal al struct.
	baseMap := map[string]any{}
	if b, err := json.Marshal(base); err == nil {
		json.Unmarshal(b, &baseMap)
	}
	for k, v := range raw {
		baseMap[k] = v
	}
	out, _ := json.Marshal(baseMap)
	var byd BydConfig
	if err := json.Unmarshal(out, &byd); err != nil {
		return base
	}
	return byd
}

// BydActive: marco activo + extensión BYD encendida.
func BydActive(c Config) bool {
	return c.Enabled && c.Byd.Enabled
}

// BydEntities: IDs para subscribe_entities cuando el módulo está activo.
func BydEntities(c Config) []string {
	if !BydActive(c) {
		return nil
	}
	b := c.Byd
	ids := []string{
		b.SocID, b.RangeID, b.OdometerID, b.BatteryPowerID,
		b.ChargingID, b.PluggedID, b.OnlineID, b.LockedID, b.DoorsID, b.WindowsID, b.SentryID,
		b.CabinTempID, b.ExteriorTempID,
		b.TireFlID, b.TireFrID, b.TireRlID, b.TireRrID,
		b.LocationID, b.GpsAgeID, b.LastUpdateID,
		b.StartChargeID, b.StopChargeID, b.ForcePollID, b.ChargeToFullID,
		b.ScheduleEnabledID, b.ScheduleStartID, b.ScheduleEndID, b.RepeatDailyID,
		b.deref(b.LockID), b.deref(b.AcSwitchID), b.deref(b.FlashLightsID), b.deref(b.FindCarID), b.deref(b.CloseWindowsID),
		b.deref(b.RemainingHoursID), b.deref(b.RemainingMinutesID),
		b.deref(b.ConsumptionRecentID), b.deref(b.Consumption50ID), b.deref(b.ConsumptionLifetimeID), b.deref(b.ConsumptionTodayID),
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (b BydConfig) deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func entityBoolOrUndef(h HA, id string) *bool {
	s := entityStateOrUndefined(h, id)
	if s == nil {
		return nil
	}
	on := *s == "on"
	return &on
}

func entityAgeMinOrUndef(h HA, id string) *int {
	s := entityStateOrUndefined(h, id)
	if s == nil {
		return nil
	}
	ts, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil
	}
	mins := int(math.Round(time.Since(ts).Minutes()))
	if mins < 0 {
		mins = 0
	}
	return &mins
}

type BydTires struct {
	Fl *float64 `json:"fl"`
	Fr *float64 `json:"fr"`
	Rl *float64 `json:"rl"`
	Rr *float64 `json:"rr"`
}

type BydLive struct {
	Name                string   `json:"name"`
	Online              bool     `json:"online"`
	Soc                 *float64 `json:"soc"`
	RangeKm             *float64 `json:"rangeKm"`
	OdometerKm          *float64 `json:"odometerKm"`
	Charging            bool     `json:"charging"`
	Plugged             bool     `json:"plugged"`
	PowerKw             *float64 `json:"powerKw"`
	Locked              *bool    `json:"locked"`
	DoorsOpen           bool     `json:"doorsOpen"`
	WindowsOpen         bool     `json:"windowsOpen"`
	Sentry              *bool    `json:"sentry"`
	CabinTempC          *float64 `json:"cabinTempC"`
	ExteriorTempC       *float64 `json:"exteriorTempC"`
	Tires               BydTires `json:"tires"`
	Location            *string  `json:"location"`
	Lat                 *float64 `json:"lat"`
	Lon                 *float64 `json:"lon"`
	GpsAgeMin           *int     `json:"gpsAgeMin"`
	LastUpdateMin       *int     `json:"lastUpdateMin"`
	ChargeToFullOn      *bool    `json:"chargeToFullOn"`
	ScheduleEnabledOn   *bool    `json:"scheduleEnabledOn"`
	ScheduleStart       *string  `json:"scheduleStart"`
	ScheduleEnd         *string  `json:"scheduleEnd"`
	RepeatDailyOn       *bool    `json:"repeatDailyOn"`
	LockUnlocked        *bool    `json:"lockUnlocked"`
	AcOn                *bool    `json:"acOn"`
	RemainingMin        *int     `json:"remainingMin"`
	ConsumptionRecent   *float64 `json:"consumptionRecent"`
	Consumption50       *float64 `json:"consumption50"`
	ConsumptionLifetime *float64 `json:"consumptionLifetime"`
	ConsumptionToday    *float64 `json:"consumptionToday"`
}

// ComputeBydLive replica computeBydLive(). nil si el módulo está inactivo.
func ComputeBydLive(h HA, c Config) *BydLive {
	if !BydActive(c) {
		return nil
	}
	b := c.Byd

	powerW := entityNumOrUndefined(h, b.BatteryPowerID)
	var powerKw *float64
	if powerW != nil {
		v := round3(*powerW / 1000)
		powerKw = &v
	}

	var lat, lon *float64
	if b.LocationID != "" {
		if st, ok := h.GetState(b.LocationID); ok && st.Attributes != nil {
			la := parseFloatAny(st.Attributes["latitude"])
			lo := parseFloatAny(st.Attributes["longitude"])
			if la != nil && lo != nil {
				la2 := round3(*la)
				lon2 := round3(*lo)
				lat, lon = &la2, &lon2
			}
		}
	}

	out := &BydLive{
		Name:          b.Name,
		Soc:           entityNumOrUndefined(h, b.SocID),
		RangeKm:       entityNumOrUndefined(h, b.RangeID),
		OdometerKm:    entityNumOrUndefined(h, b.OdometerID),
		PowerKw:       powerKw,
		Locked:        entityBoolOrUndef(h, b.LockedID),
		Sentry:        entityBoolOrUndef(h, b.SentryID),
		CabinTempC:    entityNumOrUndefined(h, b.CabinTempID),
		ExteriorTempC: entityNumOrUndefined(h, b.ExteriorTempID),
		Tires: BydTires{
			Fl: entityNumOrUndefined(h, b.TireFlID),
			Fr: entityNumOrUndefined(h, b.TireFrID),
			Rl: entityNumOrUndefined(h, b.TireRlID),
			Rr: entityNumOrUndefined(h, b.TireRrID),
		},
		Location:          entityStateOrUndefined(h, b.LocationID),
		Lat:               lat,
		Lon:               lon,
		GpsAgeMin:         entityAgeMinOrUndef(h, b.GpsAgeID),
		LastUpdateMin:     entityAgeMinOrUndef(h, b.LastUpdateID),
		ChargeToFullOn:    entityBoolOrUndef(h, b.ChargeToFullID),
		ScheduleEnabledOn: entityBoolOrUndef(h, b.ScheduleEnabledID),
		ScheduleStart:     entityStateOrUndefined(h, b.ScheduleStartID),
		ScheduleEnd:       entityStateOrUndefined(h, b.ScheduleEndID),
		RepeatDailyOn:     entityBoolOrUndef(h, b.RepeatDailyID),
	}

	if v := entityBoolOrUndef(h, b.OnlineID); v != nil {
		out.Online = *v
	}
	if v := entityBoolOrUndef(h, b.ChargingID); v != nil {
		out.Charging = *v
	}
	if v := entityBoolOrUndef(h, b.PluggedID); v != nil {
		out.Plugged = *v
	}
	if v := entityBoolOrUndef(h, b.DoorsID); v != nil {
		out.DoorsOpen = *v
	}
	if v := entityBoolOrUndef(h, b.WindowsID); v != nil {
		out.WindowsOpen = *v
	}
	if s := entityStateOrUndefined(h, b.deref2(b.LockID)); s != nil {
		u := *s == "unlocked"
		out.LockUnlocked = &u
	}
	if s := entityStateOrUndefined(h, b.deref2(b.AcSwitchID)); s != nil {
		on := *s == "on"
		out.AcOn = &on
	}
	rh := entityNumOrUndefined(h, b.deref2(b.RemainingHoursID))
	rm := entityNumOrUndefined(h, b.deref2(b.RemainingMinutesID))
	if rh != nil || rm != nil {
		total := 0
		if rh != nil {
			total += int(*rh) * 60
		}
		if rm != nil {
			total += int(*rm)
		}
		out.RemainingMin = &total
	}
	out.ConsumptionRecent = entityNumOrUndefined(h, b.deref2(b.ConsumptionRecentID))
	out.Consumption50 = entityNumOrUndefined(h, b.deref2(b.Consumption50ID))
	out.ConsumptionLifetime = entityNumOrUndefined(h, b.deref2(b.ConsumptionLifetimeID))
	out.ConsumptionToday = entityNumOrUndefined(h, b.deref2(b.ConsumptionTodayID))

	return out
}

func (b BydConfig) deref2(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func parseFloatAny(v any) *float64 {
	switch x := v.(type) {
	case float64:
		return &x
	case string:
		return parseFloatStrict(x)
	}
	return nil
}

var scheduleRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d(:[0-5]\d)?$`)

// BydAction ejecuta una acción del módulo BYD con whitelist estricta.
// Devuelve error descriptivo: byd_disabled / unknown_action.
func BydAction(h HA, c Config, action string, value any) error {
	if !BydActive(c) {
		return &actionError{msg: "byd_disabled"}
	}
	b := c.Byd

	press := func(id string) error {
		return h.CallService("button", "press", map[string]any{"entity_id": id})
	}
	setSwitch := func(id string, on bool) error {
		svc := "turn_off"
		if on {
			svc = "turn_on"
		}
		return h.CallService("switch", svc, map[string]any{"entity_id": id})
	}

	boolValue, valueIsBool := value.(bool)
	strValue, valueIsStr := value.(string)

	switch action {
	case "start_charging":
		if b.StartChargeID == "" {
			return &actionError{"unknown_action"}
		}
		return press(b.StartChargeID)
	case "stop_charging":
		if b.StopChargeID == "" {
			return &actionError{"unknown_action"}
		}
		return press(b.StopChargeID)
	case "force_poll":
		if b.ForcePollID == "" {
			return &actionError{"unknown_action"}
		}
		return press(b.ForcePollID)
	case "charge_to_full":
		if b.ChargeToFullID == "" || !valueIsBool {
			return &actionError{"unknown_action"}
		}
		return setSwitch(b.ChargeToFullID, boolValue)
	case "repeat_daily":
		if b.RepeatDailyID == "" || !valueIsBool {
			return &actionError{"unknown_action"}
		}
		return setSwitch(b.RepeatDailyID, boolValue)
	case "schedule_enabled":
		if b.ScheduleEnabledID == "" || !valueIsBool {
			return &actionError{"unknown_action"}
		}
		return setSwitch(b.ScheduleEnabledID, boolValue)
	case "lock", "unlock":
		if b.LockID == nil || *b.LockID == "" {
			return &actionError{"unknown_action"}
		}
		svc := "unlock"
		if action == "lock" {
			svc = "lock"
		}
		return h.CallService("lock", svc, map[string]any{"entity_id": *b.LockID})
	case "ac":
		if b.AcSwitchID == nil || *b.AcSwitchID == "" || !valueIsBool {
			return &actionError{"unknown_action"}
		}
		return setSwitch(*b.AcSwitchID, boolValue)
	case "flash_lights":
		if b.FlashLightsID == nil || *b.FlashLightsID == "" {
			return &actionError{"unknown_action"}
		}
		return press(*b.FlashLightsID)
	case "find_car":
		if b.FindCarID == nil || *b.FindCarID == "" {
			return &actionError{"unknown_action"}
		}
		return press(*b.FindCarID)
	case "close_windows":
		if b.CloseWindowsID == nil || *b.CloseWindowsID == "" {
			return &actionError{"unknown_action"}
		}
		return press(*b.CloseWindowsID)
	case "schedule_start", "schedule_end":
		id := b.ScheduleStartID
		if action == "schedule_end" {
			id = b.ScheduleEndID
		}
		// HH:MM (o HH:MM:SS): validación estricta antes del servicio.
		if id == "" || !valueIsStr || !scheduleRe.MatchString(strValue) {
			return &actionError{"unknown_action"}
		}
		return h.CallService("time", "set_value", map[string]any{
			"entity_id": id, "time": strValue,
		})
	}
	return &actionError{"unknown_action"}
}

type actionError struct{ msg string }

func (e *actionError) Error() string { return e.msg }

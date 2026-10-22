package extensions

import (
	"encoding/json"
	"regexp"
	"strings"

	"helios-server/internal/db"
)

type PowerUnit string

const (
	UnitKW PowerUnit = "kW"
	UnitW  PowerUnit = "W"
)

type Charger struct {
	Key                  string    `json:"key"`
	Name                 string    `json:"name"`
	PowerID              string    `json:"powerId"`
	PowerUnit            PowerUnit `json:"powerUnit"`
	EnergyTotalID        string    `json:"energyTotalId"`
	EnergyDivisor        int       `json:"energyDivisor"`
	EnergySessionID      string    `json:"energySessionId"`
	StateID              string    `json:"stateId"`
	TempID               string    `json:"tempId"`
	SwitchID             string    `json:"switchId"`
	AmpID                string    `json:"ampId"`
	ImageURL             string    `json:"imageUrl,omitempty"`
	ChargerInHouseMeters bool      `json:"chargerInHouseMeters"`
	ChargingStates       []string  `json:"chargingStates"`
	ConnectedStates      []string  `json:"connectedStates"`
}

type CarChargerConfig struct {
	Enabled  bool      `json:"enabled"`
	Chargers []Charger `json:"chargers"`
}

type Config struct {
	Enabled    bool             `json:"enabled"`
	CarCharger CarChargerConfig `json:"carCharger"`
}

var GenericCharger = Charger{
	Key:                  "",
	Name:                 "Car charger",
	PowerID:              "sensor.charger_power",
	PowerUnit:            UnitKW,
	EnergyTotalID:        "sensor.charger_energy_total",
	EnergyDivisor:        1,
	StateID:              "sensor.charger_state",
	ChargerInHouseMeters: true,
	ChargingStates:       []string{"charging", "charger_charging"},
	ConnectedStates:      []string{"charger_insert", "connected", "charging", "charger_charging"},
}

var LegacyExtensions = Config{
	Enabled: false,
	CarCharger: CarChargerConfig{
		Enabled: false,
		Chargers: []Charger{
			{
				Key:                  "main",
				Name:                 "Cargador coche",
				PowerID:              "sensor.cargador_coche_potencia",
				PowerUnit:            UnitW,
				EnergyTotalID:        "sensor.cargador_coche_energia_total",
				EnergyDivisor:        100,
				EnergySessionID:      "sensor.cargador_coche_energia_sesion",
				StateID:              "sensor.cargador_coche_estado",
				TempID:               "sensor.cargador_coche_temperatura",
				SwitchID:             "switch.cargador_coche",
				AmpID:                "number.cargador_coche_corriente",
				ChargerInHouseMeters: false,
				ChargingStates:       []string{"charger_charging", "charging"},
				ConnectedStates:      []string{"charger_insert", "charger_charging", "charging"},
			},
		},
	},
}

var GenericExtensions = Config{
	Enabled: false,
	CarCharger: CarChargerConfig{
		Enabled:  false,
		Chargers: []Charger{withKey(GenericCharger, "main")},
	},
}

func withKey(c Charger, key string) Charger {
	c.Key = key
	return c
}

var keyRe = regexp.MustCompile(`^[a-z0-9-]{1,20}$`)

// NormalizeCharger replica normalizeCharger() del original. raw es el JSON del
// cargador tal cual (map) para distinguir "ausente" de "presente pero inválido".
func NormalizeCharger(raw map[string]any, base Charger, index int) Charger {
	out := Charger{}

	stringFields := []string{
		"key", "name", "powerId", "energyTotalId", "energySessionId",
		"stateId", "tempId", "switchId", "ampId", "imageUrl",
	}
	baseStrings := map[string]string{
		"key": base.Key, "name": base.Name, "powerId": base.PowerID,
		"energyTotalId": base.EnergyTotalID, "energySessionId": base.EnergySessionID,
		"stateId": base.StateID, "tempId": base.TempID,
		"switchId": base.SwitchID, "ampId": base.AmpID, "imageUrl": base.ImageURL,
	}

	vals := map[string]string{}
	for _, k := range stringFields {
		v, present := raw[k]
		if s, ok := v.(string); ok {
			vals[k] = s
			continue
		}
		if !present {
			vals[k] = baseStrings[k]
			continue
		}
		// Presente pero no string: key va a '' (no hereda 'main').
		vals[k] = ""
	}

	out.Key = vals["key"]
	out.PowerID = vals["powerId"]
	out.EnergyTotalID = vals["energyTotalId"]
	out.EnergySessionID = vals["energySessionId"]
	out.StateID = vals["stateId"]
	out.TempID = vals["tempId"]
	out.SwitchID = vals["switchId"]
	out.AmpID = vals["ampId"]
	out.ImageURL = vals["imageUrl"]

	rawKey := strings.ToLower(strings.TrimSpace(vals["key"]))
	if keyRe.MatchString(rawKey) {
		out.Key = rawKey
	} else if index == 0 {
		out.Key = "main"
	} else {
		out.Key = "c" + itoa(index+1)
	}

	if n := strings.TrimSpace(vals["name"]); n != "" {
		out.Name = n
	} else if bn := strings.TrimSpace(base.Name); bn != "" {
		out.Name = bn
	} else {
		out.Name = "Cargador " + itoa(index+1)
	}

	if pu, ok := raw["powerUnit"].(string); ok && (pu == "W" || pu == "kW") {
		out.PowerUnit = PowerUnit(pu)
	} else if base.PowerUnit == UnitW {
		out.PowerUnit = UnitW
	} else {
		out.PowerUnit = UnitKW
	}

	out.EnergyDivisor = 1
	if d, ok := intFromAny(raw["energyDivisor"]); ok && d >= 1 {
		out.EnergyDivisor = d
	} else if base.EnergyDivisor >= 1 {
		out.EnergyDivisor = base.EnergyDivisor
	}

	if ihm, present := raw["chargerInHouseMeters"]; !present || ihm == nil {
		out.ChargerInHouseMeters = base.ChargerInHouseMeters
	} else if b, ok := ihm.(bool); ok {
		out.ChargerInHouseMeters = b
	} else {
		out.ChargerInHouseMeters = true
	}

	out.ChargingStates = stringListField(raw, "chargingStates", base.ChargingStates)
	out.ConnectedStates = stringListField(raw, "connectedStates", base.ConnectedStates)

	return out
}

func stringListField(raw map[string]any, key string, base []string) []string {
	v, present := raw[key]
	if arr, ok := v.([]any); ok {
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	if !present {
		return append([]string{}, base...)
	}
	return []string{}
}

func intFromAny(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		i := int(x)
		if float64(i) == x {
			return i, true
		}
	case int:
		return x, true
	}
	return 0, false
}

// NormalizeConfig replica normalizeExtensions(): acepta forma antigua (campos
// sueltos en carCharger) y la migra a chargers[0].
func NormalizeConfig(raw map[string]any, base Config) Config {
	out := Config{CarCharger: CarChargerConfig{Chargers: []Charger{}}}

	if b, ok := raw["enabled"].(bool); ok {
		out.Enabled = b
	} else {
		out.Enabled = base.Enabled
	}

	inC, _ := raw["carCharger"].(map[string]any)
	if inC == nil {
		inC = map[string]any{}
	}

	if b, ok := inC["enabled"].(bool); ok {
		out.CarCharger.Enabled = b
	} else {
		out.CarCharger.Enabled = base.CarCharger.Enabled
	}

	var list []any
	if arr, ok := inC["chargers"].([]any); ok {
		list = arr
	} else {
		flat := map[string]any{}
		for k, v := range inC {
			if k != "enabled" {
				flat[k] = v
			}
		}
		if len(flat) > 0 {
			list = []any{flat}
		}
	}

	baseList := base.CarCharger.Chargers
	if len(baseList) == 0 {
		baseList = []Charger{GenericCharger}
	}

	for i, item := range list {
		m, _ := item.(map[string]any)
		if m == nil {
			m = map[string]any{}
		}
		baseIdx := i
		if baseIdx >= len(baseList)-1 {
			baseIdx = len(baseList) - 1
		}
		out.CarCharger.Chargers = append(out.CarCharger.Chargers, NormalizeCharger(m, baseList[baseIdx], i))
	}

	return out
}

// Store mantiene la configuración resuelta.
type Store struct {
	current Config
}

func NewStore() *Store {
	return &Store{current: GenericExtensions}
}

func (s *Store) Set(c Config) { s.current = c }

func (s *Store) Get() Config { return s.current }

// ResolveConfig lee kv extensions_config o cae al perfil legacy/genérico.
func ResolveConfig(database *db.DB) Config {
	if raw, err := database.KVGet("extensions_config"); err == nil && raw != "" {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil && parsed != nil {
			base := GenericExtensions
			if empty, err := database.DailyEmpty(); err == nil && !empty {
				base = LegacyExtensions
			}
			return NormalizeConfig(parsed, base)
		}
	}

	empty, err := database.DailyEmpty()
	if err == nil && empty {
		return GenericExtensions
	}
	return LegacyExtensions
}

// ChargerActive: marco activo + extensión encendida + cargadores.
func ChargerActive(c Config) bool {
	return c.Enabled && c.CarCharger.Enabled && len(c.CarCharger.Chargers) > 0
}

func GetChargerByKey(c Config, key string) *Charger {
	for i := range c.CarCharger.Chargers {
		if c.CarCharger.Chargers[i].Key == key {
			return &c.CarCharger.Chargers[i]
		}
	}
	return nil
}

// ChargerEntities son los IDs a suscribir en HAOS (deduplicados).
func ChargerEntities(c Config) []string {
	if !ChargerActive(c) {
		return nil
	}
	seen := map[string]struct{}{}
	out := []string{}
	for _, ch := range c.CarCharger.Chargers {
		for _, id := range []string{
			ch.PowerID, ch.EnergyTotalID, ch.EnergySessionID,
			ch.StateID, ch.TempID, ch.SwitchID, ch.AmpID,
		} {
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

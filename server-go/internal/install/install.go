package install

import (
	"encoding/json"
	"sync"

	"helios-server/internal/db"
)

type PowerUnit string

const (
	UnitKW PowerUnit = "kW"
	UnitW  PowerUnit = "W"
)

type EnergyAcc string

const (
	AccSum   EnergyAcc = "sum"
	AccState EnergyAcc = "state"
)

type Inverter struct {
	Key             string         `json:"key"`
	Name            string         `json:"name"`
	Model           string         `json:"model"`
	Kwp             float64        `json:"kwp"`
	Panels          string         `json:"panels"`
	TempC           float64        `json:"tempC"`
	HasBattery      bool           `json:"hasBattery"`
	BatteryKwh      float64        `json:"batteryKwh,omitempty"`
	PowerID         string         `json:"powerId"`
	PowerUnit       PowerUnit      `json:"powerUnit"`
	BackupPowerID   string         `json:"backupPowerId,omitempty"`
	BackupPowerUnit PowerUnit      `json:"backupPowerUnit,omitempty"`
	EnergyID        string         `json:"energyId"`
	EnergyAcc       EnergyAcc      `json:"energyAcc"`
	EnergyCap       float64        `json:"energyCap"`
	DeepIDs         []string       `json:"deepIds"`
	GlitchOffsets   map[string]float64 `json:"glitchOffsets"`
}

type BatteryConfig struct {
	Enabled           bool     `json:"enabled"`
	PowerID           string   `json:"powerId"`
	StateID           string   `json:"stateId"`
	SocID             string   `json:"socId"`
	CapacityKwh       float64  `json:"capacityKwh"`
	ChargingStates    []string `json:"chargingStates"`
	DischargingStates []string `json:"dischargingStates"`
}

type GridConfig struct {
	Mode      string  `json:"mode"` // "attrs" | "sensor"
	AttrsID   *string `json:"attrsId"`
	SensorID  *string `json:"sensorId"`
	ImportID  *string `json:"importId"`
	ExportID  *string `json:"exportId"`
}

type ConsumptionConfig struct {
	PowerIDs      []string  `json:"powerIds"`
	PowerUnit     PowerUnit `json:"powerUnit"`
	EnergyIDs     []string  `json:"energyIds"`
	RespaldoID    *string   `json:"respaldoId"`
	NoRespaldadaID *string  `json:"noRespaldadaId"`
}

type EnergyConfig struct {
	GridImportID  string `json:"gridImportId"`
	GridExportID  string `json:"gridExportId"`
	BatChargeID   string `json:"batChargeId"`
	BatDischargeID string `json:"batDischargeId"`
	ConsumptionID string `json:"consumptionId"`
}

type Topology struct {
	Inverters     []Inverter        `json:"inverters"`
	Battery       BatteryConfig     `json:"battery"`
	Grid          GridConfig        `json:"grid"`
	Consumption   ConsumptionConfig `json:"consumption"`
	Energy        EnergyConfig      `json:"energy"`
	Sun           string            `json:"sun"`
	Weather       string            `json:"weather"`
	WeatherTemp   string            `json:"weatherTemp"`
	StatusAttrsID *string           `json:"statusAttrsId"`
}

type EntitiesView struct {
	PvSolis            string  `json:"pvSolis"`
	PvFox              string  `json:"pvFox"`
	ConsRespaldo       string  `json:"consRespaldo"`
	ConsNoRespaldada   string  `json:"consNoRespaldada"`
	ConsAlmacen        string  `json:"consAlmacen"`
	ConsRespaldoEnergy string  `json:"consRespaldoEnergy"`
	ConsNoRespaldadaEnergy string `json:"consNoRespaldadaEnergy"`
	ConsAlmacenEnergy  string  `json:"consAlmacenEnergy"`
	BatteryPower       string  `json:"batteryPower"`
	BatteryState       string  `json:"batteryState"`
	BatterySoc         string  `json:"batterySoc"`
	Scraper            *string `json:"scraper"`
	StatusAttrsID      *string `json:"statusAttrsId"`
	Sun                string  `json:"sun"`
	Weather            string  `json:"weather"`
	WeatherTemp        string  `json:"weatherTemp"`
	ESolis             string  `json:"eSolis"`
	EFox               string  `json:"eFox"`
	EConsumption       string  `json:"eConsumption"`
	EGridImport        string  `json:"eGridImport"`
	EGridExport        string  `json:"eGridExport"`
	EBatCharge         string  `json:"eBatCharge"`
	EBatDischarge      string  `json:"eBatDischarge"`
}

type EnergyEntities struct {
	Solis         string `json:"solis"`
	Fox           string `json:"fox"`
	Consumption   string `json:"consumption"`
	GridImport    string `json:"gridImport"`
	GridExport    string `json:"gridExport"`
	BatCharge     string `json:"batCharge"`
	BatDischarge  string `json:"batDischarge"`
}

type DeepSource struct {
	IDs        []string  `json:"ids"`
	Acc        EnergyAcc `json:"acc"`
	Cap        float64   `json:"cap"`
	RequireAll bool      `json:"requireAll,omitempty"`
}

type DeepSources map[string]DeepSource

var foxGlitchOffsets = map[string]float64{
	"2025-06-15": 5.46,
	"2025-07-26": 13.88,
	"2025-08-20": 4.25,
	"2025-11-02": 206.58,
	"2026-03-13": 4.71,
}

func sp(s string) *string { return &s }

var LegacyTopology = Topology{
	Inverters: []Inverter{
		{
			Key: "solis", Name: "Solis", Model: "Solis S5-EH1P5K-L",
			Kwp: 4.4, Panels: "10 × 440 W", TempC: 46,
			HasBattery: true, BatteryKwh: 5,
			PowerID: "sensor.solis_potencia_actual", PowerUnit: UnitKW,
			EnergyID: "sensor.solis_energia_hoy", EnergyAcc: AccSum, EnergyCap: 35,
			DeepIDs:       []string{"sensor.energia_solis_diaria_um"},
			GlitchOffsets: map[string]float64{},
		},
		{
			Key: "fox", Name: "Fox", Model: "Fox H1-3.0-E",
			Kwp: 2.7, Panels: "6 × 450 W", TempC: 41,
			HasBattery: false,
			PowerID:    "sensor.almacen_pinza_power_b", PowerUnit: UnitW,
			BackupPowerID: "sensor.foxess_solar_power", BackupPowerUnit: UnitKW,
			EnergyID: "sensor.energia_fox_diaria", EnergyAcc: AccState, EnergyCap: 20,
			DeepIDs:       []string{"sensor.almacen_pinza_energy_produced_b"},
			GlitchOffsets: foxGlitchOffsets,
		},
	},
	Battery: BatteryConfig{
		Enabled: true,
		PowerID: "sensor.solis_bateria_potencia",
		StateID: "sensor.solis_bateria_estado",
		SocID:   "sensor.solis_bateria_soc",
		CapacityKwh: 5.12,
		ChargingStates:    []string{"Cargando"},
		DischargingStates: []string{"Descargando"},
	},
	Grid: GridConfig{
		Mode:    "attrs",
		AttrsID: sp("sensor.solis_scraper"),
	},
	StatusAttrsID: sp("sensor.solis_scraper"),
	Consumption: ConsumptionConfig{
		PowerIDs: []string{
			"sensor.medidor_respaldo_power",
			"sensor.vivienda_medidor_power",
			"sensor.almacen_pinza_power_a",
		},
		PowerUnit: UnitW,
		EnergyIDs: []string{
			"sensor.medidor_respaldo_energy",
			"sensor.vivienda_medidor_energy",
			"sensor.almacen_pinza_energy_a",
		},
		RespaldoID:     sp("sensor.medidor_respaldo_power"),
		NoRespaldadaID: sp("sensor.vivienda_medidor_power"),
	},
	Energy: EnergyConfig{
		GridImportID:  "sensor.energia_red_importada_solis",
		GridExportID:  "sensor.energia_red_exportada_solis",
		BatChargeID:   "sensor.energia_bateria_carga_diaria",
		BatDischargeID: "sensor.energia_bateria_descarga_diaria",
		ConsumptionID: "sensor.consumo_total_diario",
	},
	Sun:         "sun.sun",
	Weather:     "weather.forecast_casa",
	WeatherTemp: "sensor.sensor_temp_ext_temperature",
}

var GenericTopology = Topology{
	Inverters: []Inverter{
		{
			Key: "inv1", Name: "Inverter",
			PowerID: "sensor.inverter_power", PowerUnit: UnitKW,
			EnergyID: "sensor.inverter_energy_today", EnergyAcc: AccState, EnergyCap: 100,
			DeepIDs:       []string{"sensor.inverter_energy_total"},
			GlitchOffsets: map[string]float64{},
		},
	},
	Battery: BatteryConfig{
		Enabled: false,
		PowerID: "sensor.battery_power",
		StateID: "sensor.battery_state",
		SocID:   "sensor.battery_soc",
		ChargingStates:    []string{"charging", "Cargando"},
		DischargingStates: []string{"discharging", "Descargando"},
	},
	Grid: GridConfig{
		Mode:     "sensor",
		SensorID: sp("sensor.grid_power"),
		ImportID: sp("sensor.grid_import_power"),
		ExportID: sp("sensor.grid_export_power"),
	},
	Consumption: ConsumptionConfig{
		PowerIDs:  []string{"sensor.house_power"},
		PowerUnit: UnitW,
		EnergyIDs: []string{"sensor.house_energy"},
	},
	Energy: EnergyConfig{
		GridImportID:  "sensor.grid_import_energy",
		GridExportID:  "sensor.grid_export_energy",
		BatChargeID:   "sensor.battery_charge_energy",
		BatDischargeID: "sensor.battery_discharge_energy",
		ConsumptionID: "sensor.house_energy_today",
	},
	Sun:         "sun.sun",
	Weather:     "weather.forecast",
	WeatherTemp: "sensor.outdoor_temperature",
}

// Store holds the resolved topology (module-level state in the Node version).
type Store struct {
	mu      sync.RWMutex
	current Topology
}

func NewStore() *Store {
	return &Store{current: LegacyTopology}
}

func (s *Store) Set(t Topology) {
	s.mu.Lock()
	s.current = t
	s.mu.Unlock()
}

func (s *Store) Get() Topology {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// TopologyInput mirrors the admin-editable JSON (all fields optional).
type TopologyInput struct {
	Inverters     []InverterInput      `json:"inverters"`
	Battery       *BatteryConfig       `json:"battery"`
	Grid          *GridConfig          `json:"grid"`
	Consumption   *ConsumptionConfig   `json:"consumption"`
	Energy        *EnergyConfig        `json:"energy"`
	Sun           string               `json:"sun"`
	Weather       string               `json:"weather"`
	WeatherTemp   string               `json:"weatherTemp"`
	StatusAttrsID *string              `json:"statusAttrsId"`
}

type InverterInput struct {
	Key             string             `json:"key"`
	Name            string             `json:"name"`
	Model           string             `json:"model"`
	Kwp             float64            `json:"kwp"`
	Panels          string             `json:"panels"`
	TempC           float64            `json:"tempC"`
	HasBattery      bool               `json:"hasBattery"`
	BatteryKwh      float64            `json:"batteryKwh"`
	PowerID         string             `json:"powerId"`
	PowerUnit       PowerUnit          `json:"powerUnit"`
	BackupPowerID   string             `json:"backupPowerId"`
	BackupPowerUnit PowerUnit          `json:"backupPowerUnit"`
	EnergyID        string             `json:"energyId"`
	EnergyAcc       EnergyAcc          `json:"energyAcc"`
	EnergyCap       *float64           `json:"energyCap"`
	DeepIDs         []string           `json:"deepIds"`
	GlitchOffsets   map[string]float64 `json:"glitchOffsets"`
}

// ResolveInstall devuelve la topología efectiva: kv install_config.topology si
// existe y parsea, si no el perfil base (legacy si hay datos, genérico si no).
func ResolveInstall(database *db.DB) Topology {
	empty, err := database.DailyEmpty()
	base := LegacyTopology
	if err == nil && empty {
		base = GenericTopology
	}

	raw, err := database.KVGet("install_config")
	if err != nil || raw == "" {
		return base
	}

	var cfg struct {
		Topology *TopologyInput `json:"topology"`
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil || cfg.Topology == nil {
		return base
	}
	return Normalize(*cfg.Topology, base)
}

// Normalize completa una topología del admin con los huecos del perfil base.
func Normalize(cfg TopologyInput, base Topology) Topology {
	out := base

	if len(cfg.Inverters) > 0 {
		inverters := make([]Inverter, 0, len(cfg.Inverters))
		for i, inv := range cfg.Inverters {
			var b Inverter
			if i < len(base.Inverters) {
				b = base.Inverters[i]
			}
			if inv.Key != "" {
				b.Key = inv.Key
			} else {
				b.Key = "inv" + itoa(i+1)
			}
			if inv.Name != "" {
				b.Name = inv.Name
			} else if b.Name == "" {
				b.Name = "Inverter " + itoa(i+1)
			}
			// Campos escalares opcionales: solo sobrescriben si vienen con valor
			// (el original usa spread ciego; replicamos el efecto útil).
			if inv.Model != "" {
				b.Model = inv.Model
			}
			if inv.Kwp != 0 {
				b.Kwp = inv.Kwp
			}
			if inv.Panels != "" {
				b.Panels = inv.Panels
			}
			if inv.TempC != 0 {
				b.TempC = inv.TempC
			}
			b.HasBattery = inv.HasBattery
			if inv.BatteryKwh != 0 {
				b.BatteryKwh = inv.BatteryKwh
			}
			b.PowerID = inv.PowerID
			if inv.PowerUnit != "" {
				b.PowerUnit = inv.PowerUnit
			}
			b.BackupPowerID = inv.BackupPowerID
			if inv.BackupPowerUnit != "" {
				b.BackupPowerUnit = inv.BackupPowerUnit
			} else if b.PowerUnit != "" {
				b.BackupPowerUnit = b.PowerUnit
			}
			b.EnergyID = inv.EnergyID
			if inv.EnergyAcc != "" {
				b.EnergyAcc = inv.EnergyAcc
			} else {
				b.EnergyAcc = AccState
			}
			if inv.EnergyCap != nil {
				b.EnergyCap = *inv.EnergyCap
			} else if b.EnergyCap == 0 {
				b.EnergyCap = 100
			}
			if len(inv.DeepIDs) > 0 {
				b.DeepIDs = inv.DeepIDs
			} else if inv.EnergyID != "" {
				b.DeepIDs = []string{inv.EnergyID}
			}
			if inv.GlitchOffsets != nil {
				b.GlitchOffsets = inv.GlitchOffsets
			} else if b.GlitchOffsets == nil {
				b.GlitchOffsets = map[string]float64{}
			}
			inverters = append(inverters, b)
		}
		out.Inverters = inverters
	}

	if cfg.Battery != nil {
		out.Battery = *cfg.Battery
	}
	if cfg.Grid != nil {
		out.Grid = *cfg.Grid
	}
	if cfg.Consumption != nil {
		out.Consumption = *cfg.Consumption
	}
	if cfg.Energy != nil {
		out.Energy = *cfg.Energy
	}
	if cfg.Sun != "" {
		out.Sun = cfg.Sun
	}
	if cfg.Weather != "" {
		out.Weather = cfg.Weather
	}
	if cfg.WeatherTemp != "" {
		out.WeatherTemp = cfg.WeatherTemp
	}
	if cfg.StatusAttrsID != nil {
		out.StatusAttrsID = cfg.StatusAttrsID
	}
	return out
}

// EntitiesView replica el shape que consumían config.js y solar.js.
func (s *Store) Entities() EntitiesView {
	t := s.Get()
	var inv0, inv1 Inverter
	if len(t.Inverters) > 0 {
		inv0 = t.Inverters[0]
	}
	if len(t.Inverters) > 1 {
		inv1 = t.Inverters[1]
	}

	powerIDs := t.Consumption.PowerIDs
	energyIDs := t.Consumption.EnergyIDs
	at := func(xs []string, i int) string {
		if i < len(xs) {
			return xs[i]
		}
		return ""
	}

	return EntitiesView{
		PvSolis:                inv0.PowerID,
		PvFox:                  inv1.PowerID,
		ConsRespaldo:           at(powerIDs, 0),
		ConsNoRespaldada:       at(powerIDs, 1),
		ConsAlmacen:            at(powerIDs, 2),
		ConsRespaldoEnergy:     at(energyIDs, 0),
		ConsNoRespaldadaEnergy: at(energyIDs, 1),
		ConsAlmacenEnergy:      at(energyIDs, 2),
		BatteryPower:           t.Battery.PowerID,
		BatteryState:           t.Battery.StateID,
		BatterySoc:             t.Battery.SocID,
		Scraper:                t.Grid.AttrsID,
		StatusAttrsID:          t.StatusAttrsID,
		Sun:                    t.Sun,
		Weather:                t.Weather,
		WeatherTemp:            t.WeatherTemp,
		ESolis:                 inv0.EnergyID,
		EFox:                   inv1.EnergyID,
		EConsumption:           t.Energy.ConsumptionID,
		EGridImport:            t.Energy.GridImportID,
		EGridExport:            t.Energy.GridExportID,
		EBatCharge:             t.Energy.BatChargeID,
		EBatDischarge:          t.Energy.BatDischargeID,
	}
}

// LiveEntities devuelve los IDs a suscribir en HAOS (deduplicados).
func (s *Store) LiveEntities() []string {
	t := s.Get()
	seen := make(map[string]struct{})
	add := func(id string) {
		if id != "" {
			seen[id] = struct{}{}
		}
	}

	for _, inv := range t.Inverters {
		add(inv.PowerID)
		add(inv.EnergyID)
		for _, d := range inv.DeepIDs {
			add(d)
		}
	}
	for _, id := range t.Consumption.PowerIDs {
		add(id)
	}
	for _, id := range t.Consumption.EnergyIDs {
		add(id)
	}
	if t.Battery.Enabled {
		add(t.Battery.PowerID)
		add(t.Battery.StateID)
		add(t.Battery.SocID)
		add(t.Energy.BatChargeID)
		add(t.Energy.BatDischargeID)
	}
	if t.Grid.Mode == "attrs" && t.Grid.AttrsID != nil {
		add(*t.Grid.AttrsID)
	} else {
		if t.Grid.SensorID != nil {
			add(*t.Grid.SensorID)
		}
		if t.Grid.ImportID != nil {
			add(*t.Grid.ImportID)
		}
		if t.Grid.ExportID != nil {
			add(*t.Grid.ExportID)
		}
	}
	if t.StatusAttrsID != nil {
		if t.Grid.AttrsID == nil || *t.StatusAttrsID != *t.Grid.AttrsID {
			add(*t.StatusAttrsID)
		}
	}
	add(t.Energy.GridImportID)
	add(t.Energy.GridExportID)
	add(t.Energy.ConsumptionID)
	add(t.Sun)
	add(t.Weather)
	add(t.WeatherTemp)

	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	return out
}

func (s *Store) EnergyEntities() EnergyEntities {
	t := s.Get()
	var inv0, inv1 Inverter
	if len(t.Inverters) > 0 {
		inv0 = t.Inverters[0]
	}
	if len(t.Inverters) > 1 {
		inv1 = t.Inverters[1]
	}
	return EnergyEntities{
		Solis:        inv0.EnergyID,
		Fox:          inv1.EnergyID,
		Consumption:  t.Energy.ConsumptionID,
		GridImport:   t.Energy.GridImportID,
		GridExport:   t.Energy.GridExportID,
		BatCharge:    t.Energy.BatChargeID,
		BatDischarge: t.Energy.BatDischargeID,
	}
}

func (s *Store) DeepSources() DeepSources {
	t := s.Get()
	srcs := DeepSources{}
	for i, inv := range t.Inverters {
		ids := inv.DeepIDs
		if len(ids) == 0 {
			ids = []string{inv.EnergyID}
		}
		filtered := make([]string, 0, len(ids))
		for _, id := range ids {
			if id != "" {
				filtered = append(filtered, id)
			}
		}
		acc := inv.EnergyAcc
		if acc == "" {
			acc = AccState
		}
		cap := inv.EnergyCap
		if cap == 0 {
			cap = 100
		}
		srcs["inv"+itoa(i)] = DeepSource{IDs: filtered, Acc: acc, Cap: cap}
	}

	if len(t.Consumption.EnergyIDs) > 0 {
		srcs["consumption"] = DeepSource{
			IDs: t.Consumption.EnergyIDs, Acc: AccState, Cap: 100, RequireAll: true,
		}
	}
	if t.Energy.GridImportID != "" {
		srcs["gridImport"] = DeepSource{IDs: []string{t.Energy.GridImportID}, Acc: AccSum, Cap: 100}
	}
	if t.Energy.GridExportID != "" {
		srcs["gridExport"] = DeepSource{IDs: []string{t.Energy.GridExportID}, Acc: AccSum, Cap: 25}
	}
	if t.Battery.Enabled {
		if t.Energy.BatChargeID != "" {
			srcs["batCharge"] = DeepSource{IDs: []string{t.Energy.BatChargeID}, Acc: AccSum, Cap: 12}
		}
		if t.Energy.BatDischargeID != "" {
			srcs["batDischarge"] = DeepSource{IDs: []string{t.Energy.BatDischargeID}, Acc: AccSum, Cap: 12}
		}
	}
	return srcs
}

// DeepSourceDailyKeys mapea fuentes a columnas de daily (solis/fox/invN).
func (s *Store) DeepSourceDailyKeys() (DeepSources, map[string]string) {
	srcs := s.DeepSources()
	mapping := map[string]string{}
	i := 0
	for k := range srcs {
		if len(k) >= 3 && k[:3] == "inv" {
			switch i {
			case 0:
				mapping[k] = "solis"
			case 1:
				mapping[k] = "fox"
			default:
				mapping[k] = "inv" + itoa(i+1)
			}
			i++
		}
	}
	return srcs, mapping
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

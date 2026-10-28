package solar

import (
	"encoding/json"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/ha"
	"helios-server/internal/install"
)

// HAReader es lo que solar necesita de Home Assistant. Permite testear con un fake.
type HAReader interface {
	GetState(id string) (ha.EntityState, bool)
	IsConnected() bool
	StatisticsDuringPeriod(startTime, endTime string, statisticIDs []string, period string, types []string) (ha.Stats, error)
}

type LiveAlert struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Text     string `json:"text"`
}

type LiveInverter struct {
	Key  string  `json:"key"`
	Name string  `json:"name"`
	Kw   float64 `json:"kw"`
}

type SunData struct {
	State       string  `json:"state"`
	Elevation   float64 `json:"elevation"`
	NextRising  any     `json:"nextRising"`
	NextSetting any     `json:"nextSetting"`
}

type LiveData struct {
	Production     float64        `json:"production"`
	Consumption    *float64       `json:"consumption"`
	RespaldoKw     float64        `json:"respaldoKw"`
	NoRespaldadaKw float64        `json:"noRespaldadaKw"`
	BatteryPower   float64        `json:"batteryPower"`
	BatteryStatus  string         `json:"batteryStatus"`
	Soc            float64        `json:"soc"`
	Grid           *float64       `json:"grid"`
	Solis          float64        `json:"solis"`
	Fox            float64        `json:"fox"`
	Inverters      []LiveInverter `json:"inverters"`
	At             int            `json:"at"`
	Alerts         []LiveAlert    `json:"alerts"`
	Sun            SunData        `json:"sun"`
	Weather        string         `json:"weather"`
	WeatherTemp    float64        `json:"weatherTemp"`
	Station        string         `json:"station"`
	InverterOnline bool           `json:"inverterOnline"`
	Ts             string         `json:"ts"`
}

type Kpis struct {
	ProductionKwh        float64            `json:"productionKwh"`
	SolisKwh             float64            `json:"solisKwh"`
	FoxKwh               float64            `json:"foxKwh"`
	InvertersKwh         map[string]float64 `json:"invertersKwh"`
	ConsumptionKwh       float64            `json:"consumptionKwh"`
	GridImportKwh        float64            `json:"gridImportKwh"`
	GridExportKwh        float64            `json:"gridExportKwh"`
	BatteryChargedKwh    float64            `json:"batteryChargedKwh"`
	BatteryDischargedKwh float64            `json:"batteryDischargedKwh"`
}

type ConsumptionBaseline struct {
	Date   string              `json:"date"`
	Values map[string]*float64 `json:"values"`
}

var debugLive = os.Getenv("HELIOS_HA_DEBUG") == "1"

// Solar agrupa la lógica de cálculo con su topología y BD.
type Solar struct {
	Store *install.Store
	DB    *db.DB
	cache *seriesCache
}

func New(store *install.Store, database *db.DB) *Solar {
	return &Solar{Store: store, DB: database, cache: newSeriesCache()}
}

func Num(v any) float64 {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return 0
		}
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0
		}
		return f
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return 0
		}
		return f
	}
	return 0
}

func round(v float64, decimals int) float64 {
	p := math.Pow(10, float64(decimals))
	return math.Round(v*p) / p
}

func Round1(v float64) float64 { return round(v, 1) }
func Round2(v float64) float64 { return round(v, 2) }
func Round3(v float64) float64 { return round(v, 3) }

func TodayStr() string {
	return time.Now().Format("2006-01-02")
}

func entityNum(haReader HAReader, id string, divisor float64) float64 {
	if id == "" || divisor == 0 {
		if divisor == 0 {
			divisor = 1
		}
		if id == "" {
			return 0
		}
	}
	st, ok := haReader.GetState(id)
	if !ok {
		return 0
	}
	return Num(st.State) / divisor
}

func inverterPowerKw(haReader HAReader, inv install.Inverter) float64 {
	raw := entityNum(haReader, inv.PowerID, 1)
	if inv.PowerUnit == install.UnitW {
		return raw / 1000
	}
	return raw
}

func (s *Solar) consumptionKw(haReader HAReader) float64 {
	t := s.Store.Get()
	div := 1.0
	if t.Consumption.PowerUnit == install.UnitW {
		div = 1000
	}
	total := 0.0
	for _, id := range t.Consumption.PowerIDs {
		total += entityNum(haReader, id, div)
	}
	return total
}

// batteryPowerKw: positivo = cargando, negativo = descargando, según sinónimos configurados.
func batteryPowerKw(b install.BatteryConfig, haReader HAReader, batMag float64) float64 {
	if !b.Enabled {
		return 0
	}
	state := ""
	if st, ok := haReader.GetState(b.StateID); ok {
		state = st.State
	}
	if contains(b.ChargingStates, state) {
		return batMag
	}
	if contains(b.DischargingStates, state) {
		return -batMag
	}
	return 0
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

func isDown(st ha.EntityState, ok bool) bool {
	return !ok || st.State == "unavailable" || st.State == "unknown"
}

func attrNum(attrs map[string]any, key string) float64 {
	if attrs == nil {
		return 0
	}
	return Num(attrs[key])
}

func attrStr(attrs map[string]any, key string) string {
	if attrs == nil {
		return ""
	}
	if v, ok := attrs[key].(string); ok {
		return v
	}
	return ""
}

// ComputeLive replica computeLive() de solar.ts.
func (s *Solar) ComputeLive(haReader HAReader) LiveData {
	t := s.Store.Get()
	ents := s.Store.Entities()

	// Producción, red y batería en vivo con el mismo lector que usa el reparto de
	// FV del cargador: una sola implementación del balance.
	power := ReadLivePower(t, haReader)
	production := power.Production
	invLive := power.Inverters
	batteryPower := power.BatteryPower
	grid := power.Grid
	gridDown := power.GridDown

	clampKw := s.consumptionKw(haReader)

	connected := haReader.IsConnected()
	consDown := false
	if connected {
		for _, id := range t.Consumption.PowerIDs {
			st, ok := haReader.GetState(id)
			if isDown(st, ok) {
				consDown = true
				break
			}
		}
	}

	b := t.Battery
	soc := 0.0
	if b.Enabled {
		soc = entityNum(haReader, b.SocID, 1)
	}
	batState := ""
	if b.Enabled {
		if st, ok := haReader.GetState(b.StateID); ok {
			batState = st.State
		}
	}

	sunState, _ := haReader.GetState(t.Sun)
	weatherState, _ := haReader.GetState(t.Weather)
	elevation := attrNum(sunState.Attributes, "elevation")

	// Consumo con el balance de HA (producción + red - batería): las pinzas no
	// cubren la casa y dos pueden estar paradas, así que solo son detalle por
	// circuito. Con la fuente de red configurada pero caída no hay balance y el
	// consumo se sirve como no disponible antes que como una cifra varias veces
	// menor.
	house := power.HouseConsumption(clampKw, consDown)
	consumption := house.Kw
	if !power.GridConfigured && !gridDown && grid == 0 && consumption > 0.1 && production < 0.02 {
		// Sin balance (fuente de red sin configurar) se mantiene el respaldo
		// anterior: importación estimada a partir del consumo de las pinzas.
		grid = consumption
	}

	alerts := make([]LiveAlert, 0, 4)
	if !connected {
		alerts = append(alerts, LiveAlert{ID: "haos", Severity: "critical", Text: "Sin conexión con Home Assistant"})
	}
	if consDown {
		alerts = append(alerts, LiveAlert{ID: "consumo", Severity: "warning", Text: "Medidores de consumo sin datos"})
	}

	var statusAttrs map[string]any
	if t.StatusAttrsID != nil && *t.StatusAttrsID != "" {
		if st, ok := haReader.GetState(*t.StatusAttrsID); ok {
			statusAttrs = st.Attributes
		}
	}
	if debugLive {
		log.Printf("[live:debug] statusAttrsId=%v ok=%v attrs=%v", t.StatusAttrsID, statusAttrs != nil, statusAttrs)
		if t.Grid.AttrsID != nil {
			st, ok := haReader.GetState(*t.Grid.AttrsID)
			log.Printf("[live:debug] gridAttrs=%v ok=%v state=%q attrs=%v", *t.Grid.AttrsID, ok, st.State, st.Attributes)
		}
	}
	if t.StatusAttrsID != nil && statusAttrs != nil {
		if v, ok := statusAttrs["inverterOnline"]; ok && Num(v) == 0 {
			alerts = append(alerts, LiveAlert{ID: "inversor", Severity: "critical", Text: "Inversor offline"})
		}
		if lu, ok := statusAttrs["lastUpdate"]; ok {
			if ts, ok := parseHATime(lu); ok {
				mins := time.Since(ts).Minutes()
				if mins > 15 {
					alerts = append(alerts, LiveAlert{
						ID: "scraper", Severity: "warning",
						Text: "Datos de producción antiguos (hace " + itoa(int(math.Round(mins))) + " min)",
					})
				}
			}
		}
	}

	if soc > 0 && soc <= 20 {
		alerts = append(alerts, LiveAlert{
			ID: "bateria", Severity: "info",
			Text: "Batería en reserva (" + ftoa(Round1(soc)) + "%)",
		})
	}
	if elevation > 15 && production < 0.1 {
		alerts = append(alerts, LiveAlert{ID: "sin_pv", Severity: "warning", Text: "Sin producción solar a plena luz"})
	}

	now := time.Now()
	inv0 := LiveInverter{}
	if len(invLive) > 0 {
		inv0 = invLive[0]
	}
	inv1 := LiveInverter{}
	if len(invLive) > 1 {
		inv1 = invLive[1]
	}

	respaldoDiv := 1.0
	consDiv := 1.0
	if t.Consumption.PowerUnit == install.UnitW {
		respaldoDiv = 1000
		consDiv = 1000
	}
	respaldoID := ""
	if t.Consumption.RespaldoID != nil {
		respaldoID = *t.Consumption.RespaldoID
	}
	noRespID := ""
	if t.Consumption.NoRespaldadaID != nil {
		noRespID = *t.Consumption.NoRespaldadaID
	}

	// Con balance manda el balance aunque alguna pinza esté parada. Sin fuente
	// de red se sirve la suma de pinzas, salvo que estén caídas.
	var consumptionPtr *float64
	if house.OK {
		v := Round3(consumption)
		consumptionPtr = &v
	}
	var gridPtr *float64
	if !gridDown {
		v := Round3(grid)
		gridPtr = &v
	}

	batteryStatus := batState
	if batteryStatus == "" {
		batteryStatus = "Desconocido"
	}

	var nextRising, nextSetting any
	if sunState.Attributes != nil {
		nextRising = sunState.Attributes["next_rising"]
		nextSetting = sunState.Attributes["next_setting"]
	}
	sunText := sunState.State
	if sunText == "" {
		sunText = "unknown"
	}
	weatherText := weatherState.State
	if weatherText == "" {
		weatherText = "unknown"
	}

	station := ""
	inverterOnline := true
	if t.StatusAttrsID != nil && *t.StatusAttrsID != "" {
		station = attrStr(statusAttrs, "stationName")
		if statusAttrs != nil {
			if v, ok := statusAttrs["inverterOnline"]; ok {
				inverterOnline = Num(v) == 1
			} else {
				inverterOnline = false
			}
		} else {
			inverterOnline = false
		}
	}

	_ = respaldoDiv
	_ = consDiv
	_ = ents

	return LiveData{
		Production:     Round3(production),
		Consumption:    consumptionPtr,
		RespaldoKw:     Round3(entityNum(haReader, respaldoID, respaldoDiv)),
		NoRespaldadaKw: Round3(entityNum(haReader, noRespID, consDiv)),
		BatteryPower:   Round3(batteryPower),
		BatteryStatus:  batteryStatus,
		Soc:            Round1(soc),
		Grid:           gridPtr,
		Solis:          Round3(inv0.Kw),
		Fox:            Round3(inv1.Kw),
		Inverters:      invLive,
		At:             now.Hour()*60 + now.Minute(),
		Alerts:         alerts,
		Sun: SunData{
			State:       sunText,
			Elevation:   elevation,
			NextRising:  nextRising,
			NextSetting: nextSetting,
		},
		Weather:        weatherText,
		WeatherTemp:    entityNum(haReader, t.WeatherTemp, 1),
		Station:        station,
		InverterOnline: inverterOnline,
		Ts:             now.UTC().Format(time.RFC3339),
	}
}

// ConsumptionCounterIDs son los contadores de energía usados como baseline.
func (s *Solar) ConsumptionCounterIDs() []string {
	return s.Store.Get().Consumption.EnergyIDs
}

// EnsureConsumptionBaseline guarda el valor de cierre de ayer como baseline de hoy.
func (s *Solar) EnsureConsumptionBaseline(haReader HAReader) (*ConsumptionBaseline, error) {
	today := TodayStr()

	if raw, err := s.DB.KVGet("cons_baseline"); err == nil && raw != "" {
		var b ConsumptionBaseline
		if err := json.Unmarshal([]byte(raw), &b); err == nil && b.Date == today {
			return &b, nil
		}
	}

	now := time.Now()
	y := now.AddDate(0, 0, -1)
	start := time.Date(y.Year(), y.Month(), y.Day(), 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	ids := s.ConsumptionCounterIDs()
	stats, err := haReader.StatisticsDuringPeriod(
		start.Format(time.RFC3339), end.Format(time.RFC3339),
		ids, "day", []string{"state"},
	)
	if err != nil {
		return nil, err
	}

	values := make(map[string]*float64, len(ids))
	for _, id := range ids {
		rows := stats[id]
		if len(rows) == 0 {
			values[id] = nil
			continue
		}
		last := rows[len(rows)-1]
		if last.State == nil {
			values[id] = nil
			continue
		}
		v := *last.State
		values[id] = &v
	}

	b := &ConsumptionBaseline{Date: today, Values: values}
	if raw, err := json.Marshal(b); err == nil {
		s.DB.KVSet("cons_baseline", string(raw))
	}
	return b, nil
}

// ConsumptionTodayFromCounters devuelve el consumo de hoy por diferencia con el baseline.
func (s *Solar) ConsumptionTodayFromCounters(haReader HAReader, baseline *ConsumptionBaseline) *float64 {
	ids := s.ConsumptionCounterIDs()
	total := 0.0
	usable := false
	for _, id := range ids {
		cur := entityNum(haReader, id, 1)
		if baseline != nil {
			if base, ok := baseline.Values[id]; ok && base != nil && cur >= *base {
				total += cur - *base
				usable = true
			}
		}
	}
	if !usable {
		return nil
	}
	return &total
}

// ComputeTodayKpis replica computeTodayKpis() de solar.ts.
func (s *Solar) ComputeTodayKpis(haReader HAReader, consumptionOverride *float64) Kpis {
	ents := s.Store.EnergyEntities()
	t := s.Store.Get()

	g := func(id string) float64 {
		if id == "" {
			return 0
		}
		return entityNum(haReader, id, 1)
	}

	production := 0.0
	perInv := make(map[string]float64, len(t.Inverters))
	for _, inv := range t.Inverters {
		k := g(inv.EnergyID)
		perInv[inv.Key] = k
		production += k
	}

	gridImport := g(ents.GridImport)
	gridExport := g(ents.GridExport)
	batCharge := g(ents.BatCharge)
	batDischarge := g(ents.BatDischarge)

	medidores := 0.0
	if consumptionOverride != nil {
		medidores = *consumptionOverride
	}
	derived := production + gridImport - gridExport - batCharge + batDischarge
	// El balance de contadores es el mismo criterio que los días ya
	// consolidados (backfill). Si todos sus términos están a cero no hay balance
	// utilizable y se recurre al contador de las pinzas.
	balanceUsable := production != 0 || gridImport != 0 || gridExport != 0 || batCharge != 0 || batDischarge != 0

	consumption := 0.0
	switch {
	case balanceUsable:
		consumption = math.Max(derived, g(ents.Consumption))
	case medidores > 0:
		consumption = medidores
	default:
		consumption = g(ents.Consumption)
	}

	roundedInv := make(map[string]float64, len(perInv))
	for k, v := range perInv {
		roundedInv[k] = Round2(v)
	}

	solis := 0.0
	fox := 0.0
	if len(t.Inverters) > 0 {
		solis = perInv[t.Inverters[0].Key]
	}
	if len(t.Inverters) > 1 {
		fox = perInv[t.Inverters[1].Key]
	}

	return Kpis{
		ProductionKwh:        Round2(production),
		SolisKwh:             Round2(solis),
		FoxKwh:               Round2(fox),
		InvertersKwh:         roundedInv,
		ConsumptionKwh:       Round2(math.Min(100, consumption)),
		GridImportKwh:        Round2(gridImport),
		GridExportKwh:        Round2(gridExport),
		BatteryChargedKwh:    Round2(batCharge),
		BatteryDischargedKwh: Round2(batDischarge),
	}
}

func parseHATime(v any) (time.Time, bool) {
	switch x := v.(type) {
	case string:
		for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05.000Z07:00", "2006-01-02T15:04:05"} {
			if t, err := time.Parse(layout, x); err == nil {
				return t, true
			}
		}
	case float64:
		ms := int64(x)
		if ms > 1e12 {
			return time.UnixMilli(ms), true
		}
		return time.Unix(ms, 0), true
	}
	return time.Time{}, false
}

func itoa(n int) string { return strconv.Itoa(n) }

func ftoa(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

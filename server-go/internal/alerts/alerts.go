package alerts

import (
	"encoding/json"
	"log"
	"strconv"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/extensions"
	"helios-server/internal/ha"
	"helios-server/internal/install"
	"helios-server/internal/push"
)

const (
	ticksInversorOffline = 2
	ticksFoxOffline      = 3
	ticksCorteRed        = 3
	ticksRedRecuperada   = 2
	histeresisSoc        = 5.0

	HoraResumenDiario = 21
	ResumenOffsetMin  = 10
	recheckBoot       = 30 * time.Second
)

// HA es lo que el motor necesita de Home Assistant.
type HA interface {
	GetState(id string) (ha.EntityState, bool)
	IsConnected() bool
}

// SolarEngine abstrae solar.Solar (evita ciclo de imports).
type SolarEngine interface {
	ComputeLive(h HA) any
	TodayStr() string
}

// LiveSnapshot es la parte de LiveData que usan las alertas.
type LiveSnapshot struct {
	Soc            float64
	BatteryPower   float64
	RespaldoKw     float64
	NoRespaldadaKw float64
}

type NotifyFn func(tipo string, datos push.Datos, opts push.Options)

type Estado struct {
	InversorMal      int
	InversorAlertado bool
	FoxMal           int
	FoxAlertado      bool
	RedMal           int
	RedOk            int
	RedAlertado      bool
	BateriaAlertado  bool
	BateriaLlena     bool
	Chg              map[string]*ChgCargadorEstado
}

type Engine struct {
	DB     *db.DB
	HA     HA
	Store  *install.Store
	Notify NotifyFn
	Estado Estado

	// LiveFn calcula el snapshot en vivo (inyectado desde solar).
	LiveFn func(HA) LiveSnapshot
	// ChargersFn devuelve los cargadores con extensión activa (inyectada
	// desde extensions en el arranque); nil desactiva la sección.
	ChargersFn func() []extensions.Charger
	// TodayFn devuelve la fecha de hoy.
	TodayFn func() string
}

func New(database *db.DB, haReader HA, store *install.Store, notify NotifyFn) *Engine {
	if notify == nil {
		notify = func(tipo string, datos push.Datos, opts push.Options) {}
	}
	return &Engine{
		DB: database, HA: haReader, Store: store, Notify: notify,
		Estado:  Estado{Chg: map[string]*ChgCargadorEstado{}},
		TodayFn: func() string { return time.Now().Format("2006-01-02") },
	}
}

func (e *Engine) disparar(tipo string, datos push.Datos, opts push.Options) {
	e.Notify(tipo, datos, opts)
	severity := opts.Severity
	if severity == "" {
		severity = "normal"
	}
	detail := map[string]any{"severity": severity}
	for k, v := range datos {
		detail[k] = v
	}
	if err := e.DB.Audit("system", "", "alert:"+tipo, detail); err != nil {
		log.Printf("[alerts] audit: %v", err)
	}
}

func (e *Engine) installConfig() map[string]any {
	raw, err := e.DB.KVGet("install_config")
	if err != nil || raw == "" {
		return map[string]any{}
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return map[string]any{}
	}
	return cfg
}

func (e *Engine) reservaPct() float64 {
	cfg := e.installConfig()
	if v, ok := cfg["batteryReservePct"].(float64); ok {
		return v
	}
	return 20
}

func (e *Engine) corteRedEnabled() bool {
	cfg := e.installConfig()
	v, ok := cfg["corteRedEnabled"].(bool)
	return ok && v
}

// Tick evalúa todas las condiciones (llamar cada minuto).
func (e *Engine) Tick() {
	if e.HA == nil || !e.HA.IsConnected() {
		return
	}
	t := e.Store.Get()
	live := e.LiveFn(e.HA)

	var statusAttrs map[string]any
	if t.StatusAttrsID != nil && *t.StatusAttrsID != "" {
		if st, ok := e.HA.GetState(*t.StatusAttrsID); ok {
			statusAttrs = st.Attributes
		}
	}

	var lastUpd *time.Time
	if statusAttrs != nil {
		if v, ok := statusAttrs["lastUpdate"]; ok {
			if ts, ok := parseTime(v); ok {
				lastUpd = &ts
			}
		}
	}
	fresco := lastUpd != nil && time.Since(*lastUpd) < 15*time.Minute

	// Día/nocturno (fail-open si falta sun.sun).
	deDia := true
	if sun, ok := e.HA.GetState(t.Sun); ok {
		deDia = sun.State == "above_horizon"
	}

	// ---- Inversor offline ----
	if t.StatusAttrsID != nil && fresco && deDia && statusAttrs != nil {
		if v, ok := statusAttrs["inverterOnline"]; ok && toNum(v) == 0 {
			e.Estado.InversorMal++
			if !e.Estado.InversorAlertado && e.Estado.InversorMal >= ticksInversorOffline {
				e.Estado.InversorAlertado = true
				e.disparar("inversor_offline", push.Datos{}, push.Options{Severity: "critical"})
			}
		} else {
			if e.Estado.InversorAlertado {
				e.Estado.InversorAlertado = false
				e.disparar("inversor_ok", push.Datos{}, push.Options{Severity: "normal"})
			}
			e.Estado.InversorMal = 0
		}
	}

	// ---- Segundo inversor (fox): pinza unavailable, solo de día ----
	if deDia && len(t.Inverters) > 1 && t.Inverters[1].PowerID != "" {
		inv2 := t.Inverters[1]
		st, ok := e.HA.GetState(inv2.PowerID)
		datos := push.Datos{"nombre": inv2.Name}
		if !ok || st.State == "unavailable" || st.State == "unknown" {
			e.Estado.FoxMal++
			if !e.Estado.FoxAlertado && e.Estado.FoxMal >= ticksFoxOffline {
				e.Estado.FoxAlertado = true
				e.disparar("fox_offline", datos, push.Options{Severity: "high"})
			}
		} else {
			if e.Estado.FoxAlertado {
				e.Estado.FoxAlertado = false
				e.disparar("fox_ok", datos, push.Options{Severity: "normal"})
			}
			e.Estado.FoxMal = 0
		}
	}

	// ---- Corte de red (desactivado por defecto) ----
	if e.corteRedEnabled() && t.Grid.Mode == "attrs" && t.Consumption.RespaldoID != nil && t.Consumption.NoRespaldadaID != nil {
		gridMag := 0.0
		if statusAttrs != nil {
			if v, ok := statusAttrs["currentGridPower"]; ok {
				gridMag = abs(toNum(v))
			}
		}
		sinRed := fresco && gridMag < 0.05 && live.NoRespaldadaKw < 0.05 &&
			live.RespaldoKw > 0.03 && live.BatteryPower < -0.05

		datos := push.Datos{
			"gridMag": gridMag, "respaldoKw": live.RespaldoKw,
			"noRespaldadaKw": live.NoRespaldadaKw,
			"batteryPower":   live.BatteryPower, "fresco": fresco,
		}
		if sinRed {
			e.Estado.RedMal++
			e.Estado.RedOk = 0
			if !e.Estado.RedAlertado && e.Estado.RedMal >= ticksCorteRed {
				e.Estado.RedAlertado = true
				e.disparar("corte_red", datos, push.Options{Severity: "critical"})
			}
		} else {
			e.Estado.RedMal = 0
			if e.Estado.RedAlertado {
				e.Estado.RedOk++
				if e.Estado.RedOk >= ticksRedRecuperada {
					e.Estado.RedAlertado = false
					e.Estado.RedOk = 0
					e.disparar("corte_red_ok", datos, push.Options{Severity: "normal"})
				}
			}
		}
	}

	// ---- Batería baja ----
	reserva := e.reservaPct()
	if !e.Estado.BateriaAlertado && live.Soc > 0 && live.Soc <= reserva {
		e.Estado.BateriaAlertado = true
		e.disparar("bateria_baja", push.Datos{"soc": live.Soc, "reserva": reserva}, push.Options{Severity: "high"})
	} else if e.Estado.BateriaAlertado && live.Soc > reserva+histeresisSoc {
		e.Estado.BateriaAlertado = false
	}

	// ---- Batería llena ----
	if !e.Estado.BateriaLlena && live.Soc >= 100 {
		e.Estado.BateriaLlena = true
		e.disparar("bateria_llena", push.Datos{"soc": live.Soc}, push.Options{Severity: "normal"})
	} else if e.Estado.BateriaLlena && live.Soc < 100-histeresisSoc {
		e.Estado.BateriaLlena = false
	}

	// ---- Carga programada de los cargadores ----
	e.tickChargers()
}

// Resumen diario

type KpisResumen struct {
	ProductionKwh  float64
	ConsumptionKwh float64
	AutoconsumoPct float64
}

// KpisFn obtiene los KPIs del día (inyectado desde solar).
type KpisFn func() (KpisResumen, error)

func (e *Engine) EnviarResumenDiario(kpisFn KpisFn) error {
	k, err := kpisFn()
	if err != nil {
		return err
	}
	datos := push.Datos{
		"produccion":  k.ProductionKwh,
		"consumo":     k.ConsumptionKwh,
		"autoconsumo": round0(k.AutoconsumoPct),
	}
	e.Notify("resumen_diario", datos, push.Options{Severity: "normal"})
	if err := e.DB.Audit("system", "", "alert:resumen_diario", map[string]any{
		"severity": "normal", "produccion": datos["produccion"],
		"consumo": datos["consumo"], "autoconsumo": datos["autoconsumo"],
	}); err != nil {
		log.Printf("[alerts] audit resumen: %v", err)
	}
	return nil
}

// ProximoEnvioResumen: anochecer + offset, o hora fija como fallback.
func (e *Engine) ProximoEnvioResumen(ahora time.Time) time.Time {
	t := e.Store.Get()
	if st, ok := e.HA.GetState(t.Sun); ok && st.Attributes != nil {
		if v, ok := st.Attributes["next_setting"]; ok {
			if ts, ok := parseTime(v); ok {
				return ts.Add(ResumenOffsetMin * time.Minute)
			}
		}
	}
	fb := time.Date(ahora.Year(), ahora.Month(), ahora.Day(), HoraResumenDiario, 0, 0, 0, ahora.Location())
	if !fb.After(ahora) {
		fb = fb.AddDate(0, 0, 1)
	}
	return fb
}

const kvResumenFecha = "resumen_ultimo_envio"

func (e *Engine) EjecutarResumenSiToca(kpisFn KpisFn) (bool, error) {
	ahora := time.Now()
	hoy := ahora.Format("2006-01-02")
	if prev, err := e.DB.KVGet(kvResumenFecha); err == nil && prev == hoy {
		return false, nil
	}
	if err := e.EnviarResumenDiario(kpisFn); err != nil {
		return false, err
	}
	e.DB.KVSet(kvResumenFecha, hoy)
	return true, nil
}

// ScheduleResumenDiario programa el resumen y se reprograma.
func (e *Engine) ScheduleResumenDiario(kpisFn KpisFn) {
	ahora := time.Now()
	next := e.ProximoEnvioResumen(ahora)

	t := e.Store.Get()
	sunsetDisponible := false
	if st, ok := e.HA.GetState(t.Sun); ok && st.Attributes != nil {
		_, sunsetDisponible = st.Attributes["next_setting"]
	}

	delay := next.Sub(ahora)
	if !sunsetDisponible && delay > recheckBoot {
		time.AfterFunc(recheckBoot, func() { e.ScheduleResumenDiario(kpisFn) })
		return
	}
	if delay < time.Minute {
		delay = time.Minute
	}

	log.Printf("[helios] resumen diario programado para %s", next.Format(time.RFC3339))
	time.AfterFunc(delay, func() {
		enviado, err := e.EjecutarResumenSiToca(kpisFn)
		if err != nil {
			log.Printf("[helios] resumen diario error: %v", err)
		} else if enviado {
			log.Printf("[helios] resumen diario enviado")
		} else {
			log.Printf("[helios] resumen diario ya enviado hoy, omitido")
		}
		e.ScheduleResumenDiario(kpisFn)
	})
}

func parseTime(v any) (time.Time, bool) {
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

func toNum(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		if f, err := strconv.ParseFloat(x, 64); err == nil {
			return f
		}
	}
	return 0
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func round0(v float64) float64 {
	if v < 0 {
		return -float64(int64(-v + 0.5))
	}
	return float64(int64(v + 0.5))
}

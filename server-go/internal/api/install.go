package api

import (
	"encoding/json"
	"io"
	"net/http"

	"helios-server/internal/install"
)

func (s *Server) handleGetInstall(w http.ResponseWriter, r *http.Request) {
	t := s.Install.Get()
	raw, err := s.DB.KVGet("install_config")
	configured := false
	if err == nil && raw != "" {
		var cfg map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &cfg) == nil {
			if _, ok := cfg["topology"]; ok {
				configured = true
			}
		}
	}

	// entities: mismas filas que el original (roles compatibles).
	rows := make([]map[string]any, 0)
	for _, inv := range t.Inverters {
		rows = append(rows, map[string]any{"role": "inverter", "key": inv.Key, "entidad": inv.PowerID, "name": inv.Name})
		if inv.EnergyID != "" {
			rows = append(rows, map[string]any{"role": "inverter_energy", "key": inv.Key, "entidad": inv.EnergyID, "name": inv.Name})
		}
	}
	for _, id := range t.Consumption.PowerIDs {
		rows = append(rows, map[string]any{"role": "consumption", "entidad": id})
	}
	if t.Battery.Enabled {
		if t.Battery.PowerID != "" {
			rows = append(rows, map[string]any{"role": "battery_power", "entidad": t.Battery.PowerID})
		}
		if t.Battery.SocID != "" {
			rows = append(rows, map[string]any{"role": "battery_soc", "entidad": t.Battery.SocID})
		}
		if t.Battery.StateID != "" {
			rows = append(rows, map[string]any{"role": "battery_state", "entidad": t.Battery.StateID})
		}
	}
	if t.Grid.Mode == "attrs" && t.Grid.AttrsID != nil {
		rows = append(rows, map[string]any{"role": "grid_attrs", "entidad": *t.Grid.AttrsID})
	} else {
		if t.Grid.SensorID != nil {
			rows = append(rows, map[string]any{"role": "grid_sensor", "entidad": *t.Grid.SensorID})
		}
		if t.Grid.ImportID != nil {
			rows = append(rows, map[string]any{"role": "grid_import", "entidad": *t.Grid.ImportID})
		}
		if t.Grid.ExportID != nil {
			rows = append(rows, map[string]any{"role": "grid_export", "entidad": *t.Grid.ExportID})
		}
	}
	if t.StatusAttrsID != nil {
		rows = append(rows, map[string]any{"role": "inverter_status", "entidad": *t.StatusAttrsID})
	}
	rows = append(rows, map[string]any{"role": "sun", "entidad": t.Sun})
	if t.Weather != "" {
		rows = append(rows, map[string]any{"role": "weather", "entidad": t.Weather})
	}
	if t.WeatherTemp != "" {
		rows = append(rows, map[string]any{"role": "weather_temp", "entidad": t.WeatherTemp})
	}

	invViews := make([]map[string]any, 0, len(t.Inverters))
	for _, inv := range t.Inverters {
		invViews = append(invViews, map[string]any{
			"key": inv.Key, "name": inv.Name, "model": inv.Model,
			"kwp": inv.Kwp, "panels": inv.Panels,
			"hasBattery": inv.HasBattery, "batteryKwh": inv.BatteryKwh,
		})
	}

	entitiesView := s.Install.Entities()
	energyView := s.Install.EnergyEntities()

	writeJSON(w, 200, map[string]any{
		"configured":     configured,
		"topology":       t,
		"inverters":      invViews,
		"battery":        map[string]any{"enabled": t.Battery.Enabled, "capacityKwh": t.Battery.CapacityKwh},
		"grid":           map[string]any{"mode": t.Grid.Mode},
		"entities":       rows,
		"energyEntities": energyView,
		// EntitiesView plano (compat con el shape que consumía config.js).
		"entitiesView": entitiesView,
	})
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	raw, err := s.DB.KVGet("install_config")
	if err == nil && raw != "" {
		var cfg map[string]any
		if json.Unmarshal([]byte(raw), &cfg) == nil && cfg != nil {
			writeJSON(w, 200, cfg)
			return
		}
	}
	// Defaults del original (instalación sin config guardada).
	writeJSON(w, 200, map[string]any{
		"solisKwp":          4.4,
		"foxKwp":            2.7,
		"batteryKwh":        5.12,
		"batteryReservePct": 20,
		"priceImportEur":    s.Prices.PriceImport,
		"priceExportEur":    s.Prices.PriceExport,
		"co2KgPerKwh":       s.Prices.Co2PerKwh,
	})
}

func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	_, _, isAdmin := s.currentUser(r)
	if !isAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "requiere administrador"})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "body inválido"})
		return
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil || parsed == nil {
		writeJSON(w, 400, map[string]any{"error": "body inválido"})
		return
	}

	// Si trae topología, se normaliza contra el perfil base antes de guardar.
	if topo, ok := parsed["topology"]; ok {
		topoMap, _ := topo.(map[string]any)
		if topoMap == nil {
			writeJSON(w, 400, map[string]any{"error": "topología inválida en topology: debe ser un objeto"})
			return
		}
		base := s.Install.Get()
		input := install.TopologyInput{}
		rawTopo, _ := json.Marshal(topoMap)
		if err := json.Unmarshal(rawTopo, &input); err != nil {
			writeJSON(w, 400, map[string]any{"error": "topología inválida en topology: " + err.Error()})
			return
		}
		if err := install.ValidateTopologyInput(input); err != nil {
			writeJSON(w, 400, map[string]any{"error": err.Error()})
			return
		}
		normalized := install.Normalize(input, base)
		parsed["topology"] = normalized
	}

	out, err := json.Marshal(parsed)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "body inválido"})
		return
	}
	if err := s.DB.KVSet("install_config", string(out)); err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	// Re-resuelve en memoria (las suscripciones nuevas requieren reinicio).
	s.Install.Set(install.ResolveInstall(s.DB))

	username, userID, _ := s.currentUser(r)
	s.DB.Audit(username, userID, "config.change", map[string]any{"keys": keysOf(parsed)})

	writeJSON(w, 200, map[string]any{"ok": true, "restartNeeded": true})
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

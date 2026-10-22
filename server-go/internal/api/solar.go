package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"helios-server/internal/db"
	"helios-server/internal/solar"
)

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func validDate(s string) bool {
	if s == "" {
		return true
	}
	if !dateRe.MatchString(s) {
		return false
	}
	_, err := time.ParseInLocation("2006-01-02", s, time.Local)
	return err == nil
}

func (s *Server) handleSolarLive(w http.ResponseWriter, r *http.Request) {
	if s.HA == nil {
		writeJSON(w, 503, map[string]any{"error": "HAOS no configurado"})
		return
	}
	live := s.Solar.ComputeLive(s.HA)
	payload := map[string]any{"connected": s.HA.IsConnected()}
	// Mezcla el LiveData en el payload raíz (contrato del frontend).
	raw, err := json.Marshal(live)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	for k, v := range fields {
		payload[k] = v
	}
	writeJSON(w, 200, payload)
}

func (s *Server) handleSolarDay(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if !validDate(date) {
		writeJSON(w, 400, map[string]any{"error": "formato inválido, usa YYYY-MM-DD"})
		return
	}
	if s.HA == nil {
		writeJSON(w, 503, map[string]any{"error": "HAOS no configurado"})
		return
	}
	res, err := s.Solar.GetDaySeries(s.HA, date)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	key := date
	if key == "" {
		key = solar.TodayStr()
	}
	writeJSON(w, 200, map[string]any{"date": key, "points": res.Points, "estimated": res.Estimated})
}

func (s *Server) handleSolarKpis(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if !validDate(date) {
		writeJSON(w, 400, map[string]any{"error": "formato inválido, usa YYYY-MM-DD"})
		return
	}
	if s.HA == nil {
		writeJSON(w, 503, map[string]any{"error": "HAOS no configurado"})
		return
	}
	k, err := s.Solar.GetKpis(s.HA, date, s.Prices)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	key := date
	if key == "" {
		key = solar.TodayStr()
	}
	raw, err := json.Marshal(k)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	payload := map[string]any{"date": key}
	for kk, v := range fields {
		payload[kk] = v
	}
	writeJSON(w, 200, payload)
}

func (s *Server) handleSolarHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to := q.Get("from"), q.Get("to")
	if !validDate(from) || !validDate(to) {
		writeJSON(w, 400, map[string]any{"error": "formato inválido, usa YYYY-MM-DD"})
		return
	}

	limit, offset := 1000, 0
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			writeJSON(w, 400, map[string]any{"error": "paginación inválida"})
			return
		}
		limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeJSON(w, 400, map[string]any{"error": "paginación inválida"})
			return
		}
		offset = n
	}

	toFinal := to
	if toFinal == "" {
		toFinal = solar.TodayStr()
	}
	fromFinal := from
	if fromFinal == "" {
		base, err := time.ParseInLocation("2006-01-02", toFinal, time.Local)
		if err != nil {
			writeJSON(w, 400, map[string]any{"error": "fecha inválida"})
			return
		}
		fromFinal = base.AddDate(0, 0, -364).Format("2006-01-02")
	}

	total, err := s.DB.DailyCount(fromFinal, toFinal)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}
	rows, err := s.DB.DailyRange(fromFinal, toFinal, limit, offset)
	if err != nil {
		writeJSON(w, 500, map[string]any{"error": "error interno"})
		return
	}

	days := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		production := derefF(row.ProductionKwh)
		gridExport := derefF(row.GridExportKwh)
		autoconsumo := 0.0
		if production > 0 {
			pct := ((production - gridExport) / production) * 100
			if pct > 100 {
				pct = 100
			}
			autoconsumo = pct
		}
		days = append(days, map[string]any{
			"date":                 row.Date,
			"productionKwh":        row.ProductionKwh,
			"solisKwh":             derefF(row.SolisKwh),
			"foxKwh":               derefF(row.FoxKwh),
			"consumptionKwh":       row.ConsumptionKwh,
			"gridImportKwh":        row.GridImportKwh,
			"gridExportKwh":        row.GridExportKwh,
			"batteryChargedKwh":    row.BatteryChargedKwh,
			"batteryDischargedKwh": row.BatteryDischargedKwh,
			"autoconsumoPct":       autoconsumo,
		})
	}

	writeJSON(w, 200, map[string]any{
		"from": fromFinal, "to": toFinal,
		"total": total, "limit": limit, "offset": offset,
		"days": days,
	})
}

func derefF(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

var _ = db.DailyRow{}

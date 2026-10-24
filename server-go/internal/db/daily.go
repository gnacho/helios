package db

import (
	"database/sql"
	"encoding/json"
)

type DailyRow struct {
	Date                 string   `json:"date"`
	ProductionKwh        *float64 `json:"production_kwh"`
	ConsumptionKwh       *float64 `json:"consumption_kwh"`
	GridImportKwh        *float64 `json:"grid_import_kwh"`
	GridExportKwh        *float64 `json:"grid_export_kwh"`
	BatteryChargedKwh    *float64 `json:"battery_charged_kwh"`
	BatteryDischargedKwh *float64 `json:"battery_discharged_kwh"`
	SolisKwh             *float64 `json:"solis_kwh"`
	FoxKwh               *float64 `json:"fox_kwh"`
	InvertersKwh         *string  `json:"inverters_kwh"`
	ExtChargerKwh        *float64 `json:"ext_charger_kwh"`
	ExtChargerPvKwh      *float64 `json:"ext_charger_pv_kwh"`
}

type DaySeriesRow struct {
	Date       string `json:"date"`
	PointsJSON string `json:"points_json"`
	Estimated  int    `json:"estimated"`
	Source     string `json:"source"`
	UpdatedAt  int64  `json:"updated_at"`
}

func (d *DB) DailyRowByDate(date string) (*DailyRow, error) {
	var r DailyRow
	err := d.db.QueryRow(
		`SELECT date, production_kwh, consumption_kwh, grid_import_kwh, grid_export_kwh,
		        battery_charged_kwh, battery_discharged_kwh, solis_kwh, fox_kwh, inverters_kwh,
		        ext_charger_kwh, ext_charger_pv_kwh
		 FROM daily WHERE date = ?`, date,
	).Scan(
		&r.Date, &r.ProductionKwh, &r.ConsumptionKwh, &r.GridImportKwh, &r.GridExportKwh,
		&r.BatteryChargedKwh, &r.BatteryDischargedKwh, &r.SolisKwh, &r.FoxKwh, &r.InvertersKwh,
		&r.ExtChargerKwh, &r.ExtChargerPvKwh,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) DailyRange(from, to string, limit, offset int) ([]DailyRow, error) {
	rows, err := d.db.Query(
		`SELECT date, production_kwh, consumption_kwh, grid_import_kwh, grid_export_kwh,
		        battery_charged_kwh, battery_discharged_kwh, solis_kwh, fox_kwh, inverters_kwh,
		        ext_charger_kwh, ext_charger_pv_kwh
		 FROM daily WHERE date >= ? AND date <= ? ORDER BY date ASC LIMIT ? OFFSET ?`,
		from, to, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DailyRow
	for rows.Next() {
		var r DailyRow
		if err := rows.Scan(
			&r.Date, &r.ProductionKwh, &r.ConsumptionKwh, &r.GridImportKwh, &r.GridExportKwh,
			&r.BatteryChargedKwh, &r.BatteryDischargedKwh, &r.SolisKwh, &r.FoxKwh, &r.InvertersKwh,
			&r.ExtChargerKwh, &r.ExtChargerPvKwh,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) MinDailyDate() (string, error) {
	var date *string
	if err := d.db.QueryRow(`SELECT MIN(date) FROM daily`).Scan(&date); err != nil {
		return "", err
	}
	if date == nil {
		return "", nil
	}
	return *date, nil
}

// MaxDailyDate devuelve la última fecha consolidada ("" si no hay filas).
func (d *DB) MaxDailyDate() (string, error) {
	var date *string
	if err := d.db.QueryRow(`SELECT MAX(date) FROM daily`).Scan(&date); err != nil {
		return "", err
	}
	if date == nil {
		return "", nil
	}
	return *date, nil
}

func (d *DB) DailyCount(from, to string) (int, error) {
	var n int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM daily WHERE date >= ? AND date <= ?`, from, to).Scan(&n)
	return n, err
}

func (d *DB) GetDaySeriesRow(date string) (*DaySeriesRow, error) {
	var r DaySeriesRow
	err := d.db.QueryRow(
		`SELECT date, points_json, estimated, source, updated_at FROM day_series WHERE date = ?`, date,
	).Scan(&r.Date, &r.PointsJSON, &r.Estimated, &r.Source, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) UpsertDaySeries(date, pointsJSON string, estimated bool, source string, updatedAt int64) error {
	est := 0
	if estimated {
		est = 1
	}
	_, err := d.db.Exec(
		`INSERT INTO day_series (date, points_json, estimated, source, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(date) DO UPDATE SET
		   points_json=excluded.points_json,
		   estimated=excluded.estimated,
		   source=excluded.source,
		   updated_at=excluded.updated_at`,
		date, pointsJSON, est, source, updatedAt,
	)
	return err
}

func (d *DB) DaySeriesDates() ([]string, error) {
	rows, err := d.db.Query(`SELECT date FROM day_series`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (d *DB) UpsertDaily(r DailyRow) error {
	var invertersJSON *string = r.InvertersKwh
	_, err := d.db.Exec(
		`INSERT INTO daily (date, production_kwh, consumption_kwh, grid_import_kwh, grid_export_kwh,
		                    battery_charged_kwh, battery_discharged_kwh, solis_kwh, fox_kwh, inverters_kwh)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(date) DO UPDATE SET
		   production_kwh=excluded.production_kwh,
		   consumption_kwh=excluded.consumption_kwh,
		   grid_import_kwh=excluded.grid_import_kwh,
		   grid_export_kwh=excluded.grid_export_kwh,
		   battery_charged_kwh=excluded.battery_charged_kwh,
		   battery_discharged_kwh=excluded.battery_discharged_kwh,
		   solis_kwh=excluded.solis_kwh,
		   fox_kwh=excluded.fox_kwh,
		   inverters_kwh=excluded.inverters_kwh`,
		r.Date, r.ProductionKwh, r.ConsumptionKwh, r.GridImportKwh, r.GridExportKwh,
		r.BatteryChargedKwh, r.BatteryDischargedKwh, r.SolisKwh, r.FoxKwh, invertersJSON,
	)
	return err
}

// MarshalJSON de InvertersKwh helper para construir el JSON de la columna.
func InvertersJSON(m map[string]float64) *string {
	if m == nil {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

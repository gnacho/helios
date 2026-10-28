package db

import "database/sql"

type ChargerDayRow struct {
	Date  string   `json:"date"`
	Kwh   *float64 `json:"kwh"`
	PvKwh *float64 `json:"pvKwh"`
}

// AddChargerKwh suma kWh al día del cargador (crea la fila si no existe),
// clampeando la fracción solar al total.
func (d *DB) AddChargerKwh(date, charger string, kwh, pvKwh float64) error {
	pv := kwh
	if pvKwh < pv {
		pv = pvKwh
	}
	if pv < 0 {
		pv = 0
	}
	_, err := d.db.Exec(
		`INSERT INTO daily_charger (date, charger, kwh, pv_kwh)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(date, charger) DO UPDATE SET
		   kwh = COALESCE(kwh, 0) + excluded.kwh,
		   pv_kwh = MIN(COALESCE(pv_kwh, 0) + excluded.pv_kwh, COALESCE(kwh, 0) + excluded.kwh)`,
		date, charger, kwh, pv,
	)
	return err
}

// AddChargerKwhUnknownPV suma kWh al día del cargador sin tocar el reparto
// solar: cuando no hay balance la FV de ese tramo es desconocida y no se
// escribe una cifra inventada.
func (d *DB) AddChargerKwhUnknownPV(date, charger string, kwh float64) error {
	_, err := d.db.Exec(
		`INSERT INTO daily_charger (date, charger, kwh, pv_kwh)
		 VALUES (?, ?, ?, NULL)
		 ON CONFLICT(date, charger) DO UPDATE SET
		   kwh = COALESCE(kwh, 0) + excluded.kwh`,
		date, charger, kwh,
	)
	return err
}

// SetChargerKwhIfNull fija el valor SOLO si estaba a NULL, creando la fila.
func (d *DB) SetChargerKwhIfNull(date, charger string, kwh float64, pvKwh *float64) error {
	if _, err := d.db.Exec(
		`INSERT OR IGNORE INTO daily_charger (date, charger, kwh, pv_kwh) VALUES (?, ?, NULL, NULL)`,
		date, charger,
	); err != nil {
		return err
	}

	var pv any
	if pvKwh != nil {
		v := *pvKwh
		if v > kwh {
			v = kwh
		}
		if v < 0 {
			v = 0
		}
		pv = v
	}

	_, err := d.db.Exec(
		`UPDATE daily_charger SET kwh = ?, pv_kwh = ? WHERE date = ? AND charger = ? AND kwh IS NULL`,
		kwh, pv, date, charger,
	)
	return err
}

// ChargerHistoryRange devuelve las filas de un cargador en un rango.
func (d *DB) ChargerHistoryRange(from, to, charger string) ([]ChargerDayRow, error) {
	rows, err := d.db.Query(
		`SELECT date, kwh, pv_kwh FROM daily_charger
		 WHERE date >= ? AND date <= ? AND charger = ? ORDER BY date ASC`,
		from, to, charger,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ChargerDayRow
	for rows.Next() {
		var r ChargerDayRow
		if err := rows.Scan(&r.Date, &r.Kwh, &r.PvKwh); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ProductionConsumptionRange devuelve production_kwh/consumption_kwh de daily.
func (d *DB) ProductionConsumptionRange(from, to string) (map[string][2]float64, error) {
	rows, err := d.db.Query(
		`SELECT date, production_kwh, consumption_kwh FROM daily WHERE date >= ? AND date <= ?`,
		from, to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string][2]float64{}
	for rows.Next() {
		var date string
		var prod, cons sql.NullFloat64
		if err := rows.Scan(&date, &prod, &cons); err != nil {
			return nil, err
		}
		out[date] = [2]float64{prod.Float64, cons.Float64}
	}
	return out, rows.Err()
}

// MissingChargerDays: días con fila en daily (y sin fila en daily_charger).
func (d *DB) MissingChargerDays(from, to, charger string) ([]string, error) {
	rows, err := d.db.Query(
		`SELECT d.date FROM daily d
		 WHERE d.date >= ? AND d.date <= ?
		   AND NOT EXISTS (SELECT 1 FROM daily_charger dc WHERE dc.date = d.date AND dc.charger = ?)
		 ORDER BY d.date`,
		from, to, charger,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			return nil, err
		}
		out = append(out, date)
	}
	return out, rows.Err()
}

// HasDailyRow indica si existe la fila daily de esa fecha.
func (d *DB) HasDailyRow(date string) (bool, error) {
	var one int
	err := d.db.QueryRow(`SELECT 1 FROM daily WHERE date = ?`, date).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

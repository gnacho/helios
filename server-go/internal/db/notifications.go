package db

import "database/sql"

type PushSub struct {
	ID       string
	Endpoint string
	P256dh   string
	Auth     string
}

type Preference struct {
	Enabled     bool
	MinSeverity string
}

type QuietHours struct {
	QuietStart *int
	QuietEnd   *int
	Tz         string
}

type QueueGroup struct {
	UserID    string
	Tipo      string
	Severity  string
	Total     int
	DatosJSON string
}

func (d *DB) UserLanguage(userID string) (string, error) {
	var lang string
	err := d.db.QueryRow(`SELECT language FROM users WHERE id = ?`, userID).Scan(&lang)
	if err == sql.ErrNoRows {
		return "es", nil
	}
	return lang, err
}

func (d *DB) AllUserIDs() ([]string, error) {
	rows, err := d.db.Query(`SELECT id FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (d *DB) SubscriptionsByUser(userID string) ([]PushSub, error) {
	rows, err := d.db.Query(
		`SELECT id, endpoint, p256dh, auth FROM push_subscriptions WHERE user_id = ?`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PushSub
	for rows.Next() {
		var s PushSub
		if err := rows.Scan(&s.ID, &s.Endpoint, &s.P256dh, &s.Auth); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (d *DB) UpsertPushSubscription(id, userID, endpoint, p256dh, auth, userAgent string, now int64) error {
	_, err := d.db.Exec(
		`INSERT INTO push_subscriptions (id, user_id, endpoint, p256dh, auth, user_agent, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(endpoint) DO UPDATE SET
		   user_id = excluded.user_id,
		   p256dh = excluded.p256dh,
		   auth = excluded.auth,
		   user_agent = excluded.user_agent,
		   updated_at = excluded.updated_at`,
		id, userID, endpoint, p256dh, auth, userAgent, now, now,
	)
	return err
}

func (d *DB) DeletePushByEndpointAndUser(endpoint, userID string) error {
	_, err := d.db.Exec(`DELETE FROM push_subscriptions WHERE endpoint = ? AND user_id = ?`, endpoint, userID)
	return err
}

func (d *DB) DeletePushByEndpoint(endpoint string) error {
	_, err := d.db.Exec(`DELETE FROM push_subscriptions WHERE endpoint = ?`, endpoint)
	return err
}

func (d *DB) Preference(userID, tipo string) (*Preference, error) {
	var enabled int
	var minSev string
	err := d.db.QueryRow(
		`SELECT enabled, min_severity FROM notification_preferences WHERE user_id = ? AND tipo = ?`,
		userID, tipo,
	).Scan(&enabled, &minSev)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Preference{Enabled: enabled == 1, MinSeverity: minSev}, nil
}

func (d *DB) SetPreference(userID, tipo string, enabled bool, now int64) error {
	e := 0
	if enabled {
		e = 1
	}
	_, err := d.db.Exec(
		`INSERT INTO notification_preferences (user_id, tipo, enabled, min_severity, updated_at)
		 VALUES (?, ?, ?, 'normal', ?)
		 ON CONFLICT(user_id, tipo) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at`,
		userID, tipo, e, now,
	)
	return err
}

func (d *DB) PreferencesForUser(userID string) (map[string]bool, error) {
	rows, err := d.db.Query(`SELECT tipo, enabled FROM notification_preferences WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var tipo string
		var enabled int
		if err := rows.Scan(&tipo, &enabled); err != nil {
			return nil, err
		}
		out[tipo] = enabled == 1
	}
	return out, rows.Err()
}

func (d *DB) QuietHoursFor(userID string) (*QuietHours, error) {
	var q QuietHours
	var start, end sql.NullInt64
	err := d.db.QueryRow(
		`SELECT quiet_start, quiet_end, tz FROM notification_quiet_hours WHERE user_id = ?`, userID,
	).Scan(&start, &end, &q.Tz)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if start.Valid {
		v := int(start.Int64)
		q.QuietStart = &v
	}
	if end.Valid {
		v := int(end.Int64)
		q.QuietEnd = &v
	}
	return &q, nil
}

func (d *DB) EnqueueNotification(id, userID, tipo, severity, datosJSON string, now int64) error {
	_, err := d.db.Exec(
		`INSERT INTO notification_queue (id, user_id, tipo, severity, datos_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, userID, tipo, severity, datosJSON, now,
	)
	return err
}

func (d *DB) QueueGroups() ([]QueueGroup, error) {
	rows, err := d.db.Query(
		`SELECT user_id, tipo, severity, COUNT(*) AS total, MIN(datos_json) AS datos_json
		 FROM notification_queue GROUP BY user_id, tipo, severity`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QueueGroup
	for rows.Next() {
		var g QueueGroup
		if err := rows.Scan(&g.UserID, &g.Tipo, &g.Severity, &g.Total, &g.DatosJSON); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (d *DB) DeleteQueueFor(userID, tipo string) error {
	_, err := d.db.Exec(`DELETE FROM notification_queue WHERE user_id = ? AND tipo = ?`, userID, tipo)
	return err
}

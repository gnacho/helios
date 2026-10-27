package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"time"
)

type UserRow struct {
	ID           string
	Username     string
	PasswordHash string
	Email        *string
	Phone        *string
	Language     string
	Role         string
	DisplayName  *string
	CreatedAt    int64
}

type UserView struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"display_name"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
	Language    string  `json:"language"`
	Role        string  `json:"role"`
	CreatedAt   int64   `json:"created_at"`
}

type UserUpdates struct {
	DisplayName *string
	Email       *string
	Phone       *string
	Language    *string
}

type SessionRow struct {
	ID        string
	UserID    string
	CreatedAt int64
	ExpiresAt int64
	UA        *string
}

type AuditRow struct {
	ID     int64   `json:"id"`
	Ts     int64   `json:"ts"`
	Actor  string  `json:"actor"`
	UserID string  `json:"user_id"`
	Action string  `json:"action"`
	Detail *string `json:"detail"`
}

func (d *DB) CreateUser(username, passwordHash, language, role string) (*UserView, error) {
	id := cryptoRandomID()
	now := time.Now().UnixMilli()
	_, err := d.db.Exec(
		`INSERT INTO users (id, username, password_hash, language, role, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, username, passwordHash, language, role, now,
	)
	if err != nil {
		return nil, err
	}
	return d.GetUserByID(id)
}

func (d *DB) DeleteUserAndSessions(userID string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE id = ?`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) UpdateUserPasswordAndRevokeSessions(userID, passwordHash string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) GetUserByUsernameByID(id string) (*UserRow, error) {
	var u UserRow
	err := d.db.QueryRow(
		`SELECT id, username, password_hash, email, phone, language, role, display_name, created_at FROM users WHERE id = ?`,
		id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Email, &u.Phone, &u.Language, &u.Role, &u.DisplayName, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *DB) DailyEmpty() (bool, error) {
	var n int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM daily`).Scan(&n); err != nil {
		return false, err
	}
	return n == 0, nil
}

func (d *DB) ListUsers() ([]UserView, error) {
	rows, err := d.db.Query(
		`SELECT id, username, display_name, email, phone, language, role, created_at FROM users ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserView
	for rows.Next() {
		var u UserView
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Phone, &u.Language, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (d *DB) QueryRow(query string, args ...any) *sql.Row {
	return d.db.QueryRow(query, args...)
}

func (d *DB) Exec(query string, args ...any) (sql.Result, error) {
	return d.db.Exec(query, args...)
}

func (d *DB) GetUserByUsername(username string) (*UserRow, error) {
	var u UserRow
	err := d.db.QueryRow(
		`SELECT id, username, password_hash, email, phone, language, role, display_name, created_at FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Email, &u.Phone, &u.Language, &u.Role, &u.DisplayName, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *DB) GetUserByID(id string) (*UserView, error) {
	var u UserView
	err := d.db.QueryRow(
		`SELECT id, username, display_name, email, phone, language, role, created_at FROM users WHERE id = ?`,
		id,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Phone, &u.Language, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *DB) UpdateUser(id string, updates UserUpdates) (*UserView, error) {
	query := `UPDATE users SET `
	args := []any{}
	set := false

	if updates.DisplayName != nil {
		query += `display_name = ?, `
		args = append(args, *updates.DisplayName)
		set = true
	}
	if updates.Email != nil {
		query += `email = ?, `
		args = append(args, *updates.Email)
		set = true
	}
	if updates.Phone != nil {
		query += `phone = ?, `
		args = append(args, *updates.Phone)
		set = true
	}
	if updates.Language != nil {
		query += `language = ?, `
		args = append(args, *updates.Language)
		set = true
	}
	if !set {
		return d.GetUserByID(id)
	}

	query = query[:len(query)-2] + ` WHERE id = ?`
	args = append(args, id)

	if _, err := d.db.Exec(query, args...); err != nil {
		return nil, err
	}
	return d.GetUserByID(id)
}

func (d *DB) CreateSession(userID string, ttlMs int64, ua string) (string, error) {
	id := cryptoRandomID()
	now := time.Now().UnixMilli()
	_, err := d.db.Exec(
		`INSERT INTO sessions (id, user_id, created_at, expires_at, ua) VALUES (?, ?, ?, ?, ?)`,
		id, userID, now, now+ttlMs, nullIfEmpty(ua),
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (d *DB) GetSession(id string) (*SessionRow, error) {
	var s SessionRow
	err := d.db.QueryRow(
		`SELECT id, user_id, created_at, expires_at, ua FROM sessions WHERE id = ?`,
		id,
	).Scan(&s.ID, &s.UserID, &s.CreatedAt, &s.ExpiresAt, &s.UA)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if s.ExpiresAt < time.Now().UnixMilli() {
		d.DeleteSession(id)
		return nil, nil
	}
	return &s, nil
}

func (d *DB) DeleteSession(id string) error {
	_, err := d.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (d *DB) DeleteUserSessionsExcept(userID, keepSessionID string) error {
	_, err := d.db.Exec(`DELETE FROM sessions WHERE user_id = ? AND id != ?`, userID, keepSessionID)
	return err
}

func (d *DB) RenewSessionIfDue(id string, ttlMs int64) (bool, error) {
	var expiresAt int64
	err := d.db.QueryRow(`SELECT expires_at FROM sessions WHERE id = ?`, id).Scan(&expiresAt)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	now := time.Now().UnixMilli()
	if expiresAt-now >= ttlMs/2 {
		return false, nil
	}

	_, err = d.db.Exec(`UPDATE sessions SET expires_at = ? WHERE id = ?`, now+ttlMs, id)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (d *DB) Audit(actor, userID, action string, detail any) error {
	var detailStr *string
	if detail != nil {
		b, err := json.Marshal(detail)
		if err != nil {
			return err
		}
		s := string(b)
		if len(s) > 500 {
			s = s[:500]
		}
		detailStr = &s
	}
	_, err := d.db.Exec(
		`INSERT INTO audit_log (ts, actor, user_id, action, detail) VALUES (?, ?, ?, ?, ?)`,
		time.Now().UnixMilli(), actor, nullIfEmpty(userID), action, detailStr,
	)
	return err
}

func (d *DB) AuditRange(limit, offset int) ([]AuditRow, error) {
	// COALESCE: las filas con user_id vacío se guardan como NULL
	// (nullIfEmpty) y Scan a string fallaría con "converting NULL to
	// string" (bug latente: la API devolvía 500 con el audit lleno).
	rows, err := d.db.Query(
		`SELECT id, ts, actor, COALESCE(user_id, ''), action, detail FROM audit_log ORDER BY ts DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AuditRow
	for rows.Next() {
		var a AuditRow
		if err := rows.Scan(&a.ID, &a.Ts, &a.Actor, &a.UserID, &a.Action, &a.Detail); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (d *DB) KVGet(key string) (string, error) {
	var value string
	err := d.db.QueryRow(`SELECT value FROM kv WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

func (d *DB) KVSet(key, value string) error {
	_, err := d.db.Exec(
		`INSERT INTO kv (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value,
	)
	return err
}

func cryptoRandomID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

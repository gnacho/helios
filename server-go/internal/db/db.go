package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps *sql.DB with Helios-specific helpers.
type DB struct {
	db *sql.DB
}

func Open(path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("mkdir data dir: %w", err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return nil, fmt.Errorf("chmod data dir: %w", err)
		}
	}
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", path)
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// single writer, like better-sqlite3
	sqldb.SetMaxOpenConns(1)

	if err := migrate(sqldb); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err := restrictFileMode(path); err != nil {
		return nil, err
	}

	return &DB{db: sqldb}, nil
}

// restrictFileMode keeps the database and its WAL sidecars readable only by the
// service account, including files created before this hardening.
func restrictFileMode(path string) error {
	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Chmod(name, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("chmod %s: %w", name, err)
		}
	}
	return nil
}

func (d *DB) Close() error { return d.db.Close() }

func (d *DB) Ping() error { return d.db.Ping() }

func migrate(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS daily (
  date TEXT PRIMARY KEY,
  production_kwh REAL,
  consumption_kwh REAL,
  grid_import_kwh REAL,
  grid_export_kwh REAL,
  battery_charged_kwh REAL,
  battery_discharged_kwh REAL,
  solis_kwh REAL,
  fox_kwh REAL,
  inverters_kwh TEXT,
  ext_charger_kwh REAL,
  ext_charger_pv_kwh REAL
);

CREATE TABLE IF NOT EXISTS daily_charger (
  date TEXT NOT NULL,
  charger TEXT NOT NULL,
  kwh REAL,
  pv_kwh REAL,
  PRIMARY KEY (date, charger)
);

CREATE TABLE IF NOT EXISTS day_series (
  date TEXT PRIMARY KEY,
  points_json TEXT NOT NULL,
  estimated INTEGER DEFAULT 0,
  source TEXT DEFAULT 'live',
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  username TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  email TEXT,
  phone TEXT,
  language TEXT DEFAULT 'es',
  role TEXT DEFAULT 'user',
  display_name TEXT,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  ua TEXT
);

CREATE TABLE IF NOT EXISTS kv (
  key TEXT PRIMARY KEY,
  value TEXT
);

CREATE TABLE IF NOT EXISTS login_attempts (
  ip TEXT PRIMARY KEY,
  attempts INTEGER DEFAULT 0,
  locked_until INTEGER DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_login_attempts_locked_until ON login_attempts(locked_until);

CREATE TABLE IF NOT EXISTS audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  actor TEXT NOT NULL,
  user_id TEXT,
  action TEXT NOT NULL,
  detail TEXT
);
CREATE INDEX IF NOT EXISTS idx_audit_ts ON audit_log(ts);

CREATE TABLE IF NOT EXISTS push_subscriptions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  endpoint TEXT UNIQUE NOT NULL,
  p256dh TEXT NOT NULL,
  auth TEXT NOT NULL,
  user_agent TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS notification_preferences (
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  tipo TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  min_severity TEXT NOT NULL DEFAULT 'normal',
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (user_id, tipo)
);

CREATE TABLE IF NOT EXISTS notification_quiet_hours (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  quiet_start INTEGER,
  quiet_end INTEGER,
  tz TEXT NOT NULL DEFAULT 'Europe/Madrid',
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS notification_queue (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  tipo TEXT NOT NULL,
  severity TEXT NOT NULL DEFAULT 'normal',
  datos_json TEXT NOT NULL DEFAULT '{}',
  created_at INTEGER NOT NULL
);
`
	_, err := db.Exec(schema)
	return err
}

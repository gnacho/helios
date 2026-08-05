import Database from 'better-sqlite3'
import fs from 'node:fs'
import path from 'node:path'

export function initSchema(db) {
  db.pragma('journal_mode = WAL')
  db.exec(`
    CREATE TABLE IF NOT EXISTS daily (
      date TEXT PRIMARY KEY,
      production_kwh REAL,
      consumption_kwh REAL,
      grid_import_kwh REAL,
      grid_export_kwh REAL,
      battery_charged_kwh REAL,
      battery_discharged_kwh REAL
    );
    CREATE TABLE IF NOT EXISTS users (
      id TEXT PRIMARY KEY,
      username TEXT UNIQUE NOT NULL,
      password_hash TEXT NOT NULL,
      email TEXT,
      phone TEXT,
      language TEXT DEFAULT 'es',
      role TEXT DEFAULT 'user',
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
  `)
  migrate(db)
  return db
}

// Migraciones incrementales con PRAGMA user_version. Los CREATE IF NOT EXISTS
// de arriba son idempotentes y cubren el esquema completo; aquí solo va lo que
// requiere transformar datos o tablas ya existentes. Regla: NUNCA editar una
// migración ya aplicada en producción; se añade una nueva al final.
function migrate(db) {
  const version = db.pragma('user_version', { simple: true })

  if (version < 1) {
    // v1: daily gana desglose por inversor.
    const cols = db.prepare('PRAGMA table_info(daily)').all().map((c) => c.name)
    if (!cols.includes('solis_kwh')) db.exec('ALTER TABLE daily ADD COLUMN solis_kwh REAL')
    if (!cols.includes('fox_kwh')) db.exec('ALTER TABLE daily ADD COLUMN fox_kwh REAL')
    db.pragma('user_version = 1')
  }

  if (version < 2) {
    // v2: sessions del esquema viejo (sin user_id) se recrea. Se pierden las
    // sesiones activas (relogin), aceptable: antes rompía el login con 500.
    const cols = db.prepare('PRAGMA table_info(sessions)').all().map((c) => c.name)
    if (cols.length > 0 && !cols.includes('user_id')) {
      db.exec(`
        DROP TABLE sessions;
        CREATE TABLE sessions (
          id TEXT PRIMARY KEY,
          user_id TEXT NOT NULL,
          created_at INTEGER NOT NULL,
          expires_at INTEGER NOT NULL,
          ua TEXT
        );
      `)
    }
    db.pragma('user_version = 2')
  }

  if (version < 3) {
    const cols = db.prepare('PRAGMA table_info(users)').all().map((c) => c.name)
    if (!cols.includes('display_name')) db.exec('ALTER TABLE users ADD COLUMN display_name TEXT')
    db.pragma('user_version = 3')
  }
}

export function openDb(dataDir) {
  fs.mkdirSync(dataDir, { recursive: true })
  const db = new Database(path.join(dataDir, 'helios.db'))
  return initSchema(db)
}

export function upsertDaily(db, row) {
  db.prepare(
    `INSERT INTO daily (date, production_kwh, consumption_kwh, grid_import_kwh, grid_export_kwh, battery_charged_kwh, battery_discharged_kwh, solis_kwh, fox_kwh)
     VALUES (@date, @production_kwh, @consumption_kwh, @grid_import_kwh, @grid_export_kwh, @battery_charged_kwh, @battery_discharged_kwh, @solis_kwh, @fox_kwh)
     ON CONFLICT(date) DO UPDATE SET
       production_kwh=excluded.production_kwh,
       consumption_kwh=excluded.consumption_kwh,
       grid_import_kwh=excluded.grid_import_kwh,
       grid_export_kwh=excluded.grid_export_kwh,
       battery_charged_kwh=excluded.battery_charged_kwh,
       battery_discharged_kwh=excluded.battery_discharged_kwh,
       solis_kwh=excluded.solis_kwh,
       fox_kwh=excluded.fox_kwh`
  ).run(row)
}

export function dailyRange(db, from, to, limit = 1000, offset = 0) {
  return db
    .prepare('SELECT * FROM daily WHERE date >= ? AND date <= ? ORDER BY date ASC LIMIT ? OFFSET ?')
    .all(from, to, limit, offset)
}

export function dailyCount(db, from, to) {
  return db.prepare('SELECT COUNT(*) AS n FROM daily WHERE date >= ? AND date <= ?').get(from, to).n
}

export function dailyEmpty(db) {
  return db.prepare('SELECT COUNT(*) AS n FROM daily').get().n === 0
}

export function createUser(db, username, passwordHash, language = 'es', role = 'user') {
  const id = cryptoRandomId()
  const now = Date.now()
  db.prepare('INSERT INTO users (id, username, password_hash, language, role, created_at) VALUES (?, ?, ?, ?, ?, ?)').run(
    id,
    username,
    passwordHash,
    language,
    role,
    now
  )
  return { id, username, language, role }
}

export function getUserByUsername(db, username) {
  return db.prepare('SELECT * FROM users WHERE username = ?').get(username)
}

export function getUserById(db, id) {
  return db.prepare('SELECT id, username, display_name, email, phone, language, role, created_at FROM users WHERE id = ?').get(id)
}

export function updateUser(db, id, updates) {
  const fields = []
  const values = []
  if (updates.display_name !== undefined) {
    fields.push('display_name = ?')
    values.push(updates.display_name)
  }
  if (updates.email !== undefined) {
    fields.push('email = ?')
    values.push(updates.email)
  }
  if (updates.phone !== undefined) {
    fields.push('phone = ?')
    values.push(updates.phone)
  }
  if (updates.language !== undefined) {
    fields.push('language = ?')
    values.push(updates.language)
  }
  if (fields.length === 0) return null
  values.push(id)
  db.prepare(`UPDATE users SET ${fields.join(', ')} WHERE id = ?`).run(...values)
  return getUserById(db, id)
}

export function createSession(db, userId, ttlMs, ua) {
  const id = cryptoRandomId()
  const now = Date.now()
  db.prepare('INSERT INTO sessions (id, user_id, created_at, expires_at, ua) VALUES (?, ?, ?, ?, ?)').run(
    id,
    userId,
    now,
    now + ttlMs,
    ua || null
  )
  return id
}

export function getSession(db, id) {
  const row = db.prepare('SELECT * FROM sessions WHERE id = ?').get(id)
  if (!row) return null
  if (row.expires_at < Date.now()) {
    db.prepare('DELETE FROM sessions WHERE id = ?').run(id)
    return null
  }
  return row
}

export function deleteSession(db, id) {
  db.prepare('DELETE FROM sessions WHERE id = ?').run(id)
}

export function cleanSessions(db) {
  db.prepare('DELETE FROM sessions WHERE expires_at < ?').run(Date.now())
}

// Audit log: toda mutación autenticada queda registrada. detail NUNCA lleva passwords/tokens.
export function audit(db, actor, userId, action, detail) {
  db.prepare('INSERT INTO audit_log (ts, actor, user_id, action, detail) VALUES (?, ?, ?, ?, ?)').run(
    Date.now(),
    actor,
    userId || null,
    action,
    detail ? JSON.stringify(detail).slice(0, 500) : null
  )
}

export function auditRange(db, limit = 50, offset = 0) {
  return db.prepare('SELECT * FROM audit_log ORDER BY ts DESC LIMIT ? OFFSET ?').all(limit, offset)
}

export function auditCount(db) {
  return db.prepare('SELECT COUNT(*) AS n FROM audit_log').get().n
}

export function purgeAudit(db, retentionMs = 90 * 24 * 3600 * 1000) {
  db.prepare('DELETE FROM audit_log WHERE ts < ?').run(Date.now() - retentionMs)
}

export function kvGet(db, key) {  const row = db.prepare('SELECT value FROM kv WHERE key = ?').get(key)
  return row ? row.value : null
}

export function kvSet(db, key, value) {
  db.prepare('INSERT INTO kv (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value').run(
    key,
    value
  )
}

function cryptoRandomId() {
  const bytes = new Uint8Array(32)
  crypto.getRandomValues(bytes)
  return Buffer.from(bytes).toString('hex')
}

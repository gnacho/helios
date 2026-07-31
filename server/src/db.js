import Database from 'better-sqlite3'
import fs from 'node:fs'
import path from 'node:path'

export function openDb(dataDir) {
  fs.mkdirSync(dataDir, { recursive: true })
  const db = new Database(path.join(dataDir, 'helios.db'))
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
  `)
  const cols = db.prepare('PRAGMA table_info(daily)').all().map((c) => c.name)
  if (!cols.includes('solis_kwh')) db.exec('ALTER TABLE daily ADD COLUMN solis_kwh REAL')
  if (!cols.includes('fox_kwh')) db.exec('ALTER TABLE daily ADD COLUMN fox_kwh REAL')
  return db
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
  return db.prepare('SELECT id, username, email, phone, language, role, created_at FROM users WHERE id = ?').get(id)
}

export function updateUser(db, id, updates) {
  const fields = []
  const values = []
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

export function kvGet(db, key) {
  const row = db.prepare('SELECT value FROM kv WHERE key = ?').get(key)
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

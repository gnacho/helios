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
    CREATE TABLE IF NOT EXISTS sessions (
      id TEXT PRIMARY KEY,
      created_at INTEGER NOT NULL,
      expires_at INTEGER NOT NULL,
      ua TEXT
    );
    CREATE TABLE IF NOT EXISTS kv (
      key TEXT PRIMARY KEY,
      value TEXT
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

export function dailyRange(db, from, to) {
  return db
    .prepare('SELECT * FROM daily WHERE date >= ? AND date <= ? ORDER BY date ASC')
    .all(from, to)
}

export function dailyEmpty(db) {
  return db.prepare('SELECT COUNT(*) AS n FROM daily').get().n === 0
}

export function createSession(db, ttlMs, ua) {
  const id = cryptoRandomId()
  const now = Date.now()
  db.prepare('INSERT INTO sessions (id, created_at, expires_at, ua) VALUES (?, ?, ?, ?)').run(
    id,
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

import Database from 'better-sqlite3'
import fs from 'node:fs'
import path from 'node:path'

export type DB = Database.Database

export interface DailyRow {
  date: string
  production_kwh: number | null
  consumption_kwh: number | null
  grid_import_kwh: number | null
  grid_export_kwh: number | null
  battery_charged_kwh: number | null
  battery_discharged_kwh: number | null
  solis_kwh: number | null
  fox_kwh: number | null
  inverters_kwh: string | null
  ext_charger_kwh: number | null
  ext_charger_pv_kwh: number | null
}

export interface DailyInput {
  date: string
  production_kwh: number
  consumption_kwh: number
  grid_import_kwh: number
  grid_export_kwh: number
  battery_charged_kwh: number
  battery_discharged_kwh: number
  solis_kwh: number
  fox_kwh: number
  inverters_kwh: string
}

export interface DaySeriesRow {
  date: string
  points_json: string
  estimated: number
  source: string
  updated_at: number
}

export interface DaySeriesInput {
  date: string
  points_json: string
  estimated: number
  source: string
  updated_at: number
}

export interface UserRow {
  id: string
  username: string
  password_hash: string
  email: string | null
  phone: string | null
  language: string
  role: string
  display_name: string | null
  created_at: number
}

export interface UserView {
  id: string
  username: string
  display_name: string | null
  email: string | null
  phone: string | null
  language: string
  role: string
  created_at: number
}

export interface UserUpdates {
  display_name?: string | null
  email?: string | null
  phone?: string | null
  language?: string
}

export interface SessionRow {
  id: string
  user_id: string
  created_at: number
  expires_at: number
  ua: string | null
}

export interface AuditRow {
  id: number
  ts: number
  actor: string
  user_id: string | null
  action: string
  detail: string | null
}

export function initSchema(db: DB): DB {
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
function migrate(db: DB): void {
  const version = db.pragma('user_version', { simple: true }) as number

  if (version < 1) {
    // v1: daily gana desglose por inversor.
    const cols = (db.prepare('PRAGMA table_info(daily)').all() as { name: string }[]).map((c) => c.name)
    if (!cols.includes('solis_kwh')) db.exec('ALTER TABLE daily ADD COLUMN solis_kwh REAL')
    if (!cols.includes('fox_kwh')) db.exec('ALTER TABLE daily ADD COLUMN fox_kwh REAL')
    db.pragma('user_version = 1')
  }

  if (version < 2) {
    // v2: sessions del esquema viejo (sin user_id) se recrea. Se pierden las
    // sesiones activas (relogin), aceptable: antes rompía el login con 500.
    const cols = (db.prepare('PRAGMA table_info(sessions)').all() as { name: string }[]).map((c) => c.name)
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
    const cols = (db.prepare('PRAGMA table_info(users)').all() as { name: string }[]).map((c) => c.name)
    if (!cols.includes('display_name')) db.exec('ALTER TABLE users ADD COLUMN display_name TEXT')
    db.pragma('user_version = 3')
  }

  if (version < 4) {
    // v4: daily gana inverters_kwh (JSON "clave inversor" → kWh) para topologías
    // con N inversores (issue #37). solis_kwh/fox_kwh se siguen escribiendo
    // (los 2 primeros inversores) por compatibilidad con el frontend existente.
    const cols = (db.prepare('PRAGMA table_info(daily)').all() as { name: string }[]).map((c) => c.name)
    if (!cols.includes('inverters_kwh')) db.exec('ALTER TABLE daily ADD COLUMN inverters_kwh TEXT')
    db.pragma('user_version = 4')
  }

  if (version < 5) {
    // v5: daily gana ext_charger_kwh (kWh cargados por día, extensión cargador,
    // issue #94). La acumula el propio Helios por deltas del contador total:
    // los cargadores locales suelen exponer energía sin state_class (sin
    // statistics en HAOS). upsertDaily NO la toca → sobrevive a la
    // consolidación nocturna.
    const cols = (db.prepare('PRAGMA table_info(daily)').all() as { name: string }[]).map((c) => c.name)
    if (!cols.includes('ext_charger_kwh')) db.exec('ALTER TABLE daily ADD COLUMN ext_charger_kwh REAL')
    db.pragma('user_version = 5')
  }

  if (version < 6) {
    // v6: daily gana ext_charger_pv_kwh (fracción solar de la carga, issue #94
    // iteración 4). La atribuye accumulateChargerDaily con el balance
    // instantáneo (excedente FV vs resto); el resto = red y batería.
    const cols = (db.prepare('PRAGMA table_info(daily)').all() as { name: string }[]).map((c) => c.name)
    if (!cols.includes('ext_charger_pv_kwh')) db.exec('ALTER TABLE daily ADD COLUMN ext_charger_pv_kwh REAL')
    db.pragma('user_version = 6')
  }

  if (version < 7) {
    // v7 (issue 146): energia del cargador por dia Y por cargador (varios
    // cargadores configurables). La clave del cargador 1 es 'main'; las
    // columnas daily.ext_charger_kwh/pv_kwh quedan congeladas y todo lo
    // acumulado hasta ahora se copia a 'main' (el cargador original).
    db.exec(`CREATE TABLE IF NOT EXISTS daily_charger (
      date TEXT NOT NULL,
      charger TEXT NOT NULL,
      kwh REAL,
      pv_kwh REAL,
      PRIMARY KEY (date, charger)
    )`)
    db.exec(`INSERT OR IGNORE INTO daily_charger (date, charger, kwh, pv_kwh)
             SELECT date, 'main', ext_charger_kwh, ext_charger_pv_kwh FROM daily
             WHERE ext_charger_kwh IS NOT NULL`)
    db.pragma('user_version = 7')
  }
}

export function openDb(dataDir: string): DB {
  fs.mkdirSync(dataDir, { recursive: true })
  const db = new Database(path.join(dataDir, 'helios.db'))
  return initSchema(db)
}

export function upsertDaily(db: DB, row: DailyInput): void {
  db.prepare(
    `INSERT INTO daily (date, production_kwh, consumption_kwh, grid_import_kwh, grid_export_kwh, battery_charged_kwh, battery_discharged_kwh, solis_kwh, fox_kwh, inverters_kwh)
     VALUES (@date, @production_kwh, @consumption_kwh, @grid_import_kwh, @grid_export_kwh, @battery_charged_kwh, @battery_discharged_kwh, @solis_kwh, @fox_kwh, @inverters_kwh)
     ON CONFLICT(date) DO UPDATE SET
       production_kwh=excluded.production_kwh,
       consumption_kwh=excluded.consumption_kwh,
       grid_import_kwh=excluded.grid_import_kwh,
       grid_export_kwh=excluded.grid_export_kwh,
       battery_charged_kwh=excluded.battery_charged_kwh,
       battery_discharged_kwh=excluded.battery_discharged_kwh,
       solis_kwh=excluded.solis_kwh,
       fox_kwh=excluded.fox_kwh,
       inverters_kwh=excluded.inverters_kwh`
  ).run(row)
}

export function dailyRange(db: DB, from: string, to: string, limit = 1000, offset = 0): DailyRow[] {
  return db
    .prepare('SELECT * FROM daily WHERE date >= ? AND date <= ? ORDER BY date ASC LIMIT ? OFFSET ?')
    .all(from, to, limit, offset) as DailyRow[]
}

export function dailyCount(db: DB, from: string, to: string): number {
  return (db.prepare('SELECT COUNT(*) AS n FROM daily WHERE date >= ? AND date <= ?').get(from, to) as { n: number }).n
}

export function dailyEmpty(db: DB): boolean {
  return (db.prepare('SELECT COUNT(*) AS n FROM daily').get() as { n: number }).n === 0
}

export function upsertDaySeries(db: DB, row: DaySeriesInput): void {
  db.prepare(
    `INSERT INTO day_series (date, points_json, estimated, source, updated_at)
     VALUES (@date, @points_json, @estimated, @source, @updated_at)
     ON CONFLICT(date) DO UPDATE SET
       points_json=excluded.points_json,
       estimated=excluded.estimated,
       source=excluded.source,
       updated_at=excluded.updated_at`
  ).run(row)
}

export function getDaySeriesRow(db: DB, date: string): DaySeriesRow | undefined {
  return db.prepare('SELECT * FROM day_series WHERE date = ?').get(date) as DaySeriesRow | undefined
}

export function daySeriesDates(db: DB): string[] {
  return (db.prepare('SELECT date FROM day_series').all() as { date: string }[]).map((r) => r.date)
}

export function createUser(db: DB, username: string, passwordHash: string, language = 'es', role = 'user'): { id: string; username: string; language: string; role: string } {
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

export function getUserByUsername(db: DB, username: string): UserRow | undefined {
  return db.prepare('SELECT * FROM users WHERE username = ?').get(username) as UserRow | undefined
}

export function getUserById(db: DB, id: string): UserView | undefined {
  return db.prepare('SELECT id, username, display_name, email, phone, language, role, created_at FROM users WHERE id = ?').get(id) as UserView | undefined
}

export function updateUser(db: DB, id: string, updates: UserUpdates): UserView | null {
  const fields: string[] = []
  const values: (string | null)[] = []
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
  return getUserById(db, id) ?? null
}

export function createSession(db: DB, userId: string, ttlMs: number, ua: string | null | undefined): string {
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

export function getSession(db: DB, id: string): SessionRow | null {
  const row = db.prepare('SELECT * FROM sessions WHERE id = ?').get(id) as SessionRow | undefined
  if (!row) return null
  if (row.expires_at < Date.now()) {
    db.prepare('DELETE FROM sessions WHERE id = ?').run(id)
    return null
  }
  return row
}

export function deleteSession(db: DB, id: string): void {
  db.prepare('DELETE FROM sessions WHERE id = ?').run(id)
}

// Expiración deslizante: si a la sesión le queda menos de la mitad del TTL,
// extiende expires_at un TTL completo desde ahora. Un usuario activo no pierde
// la sesión en el límite fijo. Devuelve true si renovó (el caller re-emite la
// cookie con maxAge fresco).
export function renewSessionIfDue(db: DB, id: string, ttlMs: number): boolean {
  const row = db.prepare('SELECT expires_at FROM sessions WHERE id = ?').get(id) as { expires_at: number } | undefined
  if (!row) return false
  const now = Date.now()
  if (row.expires_at - now >= ttlMs / 2) return false
  db.prepare('UPDATE sessions SET expires_at = ? WHERE id = ?').run(now + ttlMs, id)
  return true
}

export function cleanSessions(db: DB): void {
  db.prepare('DELETE FROM sessions WHERE expires_at < ?').run(Date.now())
  const oneHourAgo = Date.now() - 3600 * 1000
  db.prepare('DELETE FROM login_attempts WHERE locked_until > 0 AND locked_until < ?').run(oneHourAgo)
}

// Audit log: toda mutación autenticada queda registrada. detail NUNCA lleva passwords/tokens.
export function audit(db: DB, actor: string, userId: string | null, action: string, detail?: unknown): void {
  db.prepare('INSERT INTO audit_log (ts, actor, user_id, action, detail) VALUES (?, ?, ?, ?, ?)').run(
    Date.now(),
    actor,
    userId || null,
    action,
    detail ? JSON.stringify(detail).slice(0, 500) : null
  )
}

export function auditRange(db: DB, limit = 50, offset = 0): AuditRow[] {
  return db.prepare('SELECT * FROM audit_log ORDER BY ts DESC LIMIT ? OFFSET ?').all(limit, offset) as AuditRow[]
}

export function auditCount(db: DB): number {
  return (db.prepare('SELECT COUNT(*) AS n FROM audit_log').get() as { n: number }).n
}

export function purgeAudit(db: DB, retentionMs = 90 * 24 * 3600 * 1000): void {
  db.prepare('DELETE FROM audit_log WHERE ts < ?').run(Date.now() - retentionMs)
}

export function kvGet(db: DB, key: string): string | null {
  const row = db.prepare('SELECT value FROM kv WHERE key = ?').get(key) as { value: string | null } | undefined
  return row ? row.value : null
}

export function kvSet(db: DB, key: string, value: string): void {
  db.prepare('INSERT INTO kv (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value').run(
    key,
    value
  )
}

function cryptoRandomId(): string {
  const bytes = new Uint8Array(32)
  crypto.getRandomValues(bytes)
  return Buffer.from(bytes).toString('hex')
}

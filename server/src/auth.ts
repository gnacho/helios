import crypto from 'node:crypto'
import bcrypt from 'bcryptjs'
import { getCookie, setCookie, deleteCookie } from 'hono/cookie'
import type { Context, Next } from 'hono'
import type Database from 'better-sqlite3'
import { createSession, getSession, deleteSession, renewSessionIfDue, createUser, getUserByUsername, getUserById, updateUser } from './db.js'
import { config } from './config.js'

type Db = Database.Database

interface AppEnv {
  Bindings: { incoming?: { socket?: { remoteAddress?: string } } }
  Variables: { userId: string }
}

interface PublicUser {
  id: string
  username: string
  email: string | null
  phone: string | null
  language: string
  role: string
}

const COOKIE_NAME = 'helios_session'

function secret(db: Db) {
  if (config.sessionSecret) return config.sessionSecret
  const row = db.prepare('SELECT value FROM kv WHERE key = ?').get('session_secret') as { value: string } | undefined
  if (row) return row.value
  const generated = crypto.randomBytes(32).toString('hex')
  db.prepare('INSERT INTO kv (key, value) VALUES (?, ?)').run('session_secret', generated)
  return generated
}

function sign(db: Db, id: string) {
  return crypto.createHmac('sha256', secret(db)).update(id).digest('hex')
}

function safeEqual(a: string, b: string) {
  const ba = Buffer.from(String(a))
  const bb = Buffer.from(String(b))
  if (ba.length !== bb.length) return false
  return crypto.timingSafeEqual(ba, bb)
}

function clientIp(c: Context<AppEnv>) {
  return c.req.header('x-forwarded-for')?.split(',')[0].trim() || c.env?.incoming?.socket?.remoteAddress || 'unknown'
}

export function loginRateLimited(db: Db, c: Context<AppEnv>) {
  const ip = clientIp(c)
  const row = db.prepare('SELECT * FROM login_attempts WHERE ip = ?').get(ip) as { locked_until: number } | undefined
  if (!row) return false
  if (row.locked_until > Date.now()) return true
  return false
}

export function registerLoginFail(db: Db, c: Context<AppEnv>) {
  const ip = clientIp(c)
  db.prepare(`
    INSERT INTO login_attempts (ip, attempts, locked_until)
    VALUES (?, 1, 0)
    ON CONFLICT(ip) DO UPDATE SET
      attempts = attempts + 1,
      locked_until = CASE
        WHEN attempts >= 4 THEN ?
        ELSE locked_until
      END
  `).run(ip, Date.now() + 5 * 60 * 1000)
}

export function loginOk(db: Db, c: Context<AppEnv>) {
  const ip = clientIp(c)
  db.prepare('DELETE FROM login_attempts WHERE ip = ?').run(ip)
}

// Crea el admin inicial desde .env (AUTH_USER/AUTH_PASS) si no existe. Idempotente.
export async function ensureBootstrapAdmin(db: Db) {
  const existing = getUserByUsername(db, config.authUser)
  if (existing) return
  const hash = await bcrypt.hash(config.authPass, 10)
  createUser(db, config.authUser, hash, 'es', 'admin')
  console.log(`[helios] admin bootstrap creado: ${config.authUser}`)
}

// Cambio de contraseña del propio usuario: exige la actual (bcrypt) y cierra el resto de sesiones.
export async function changeOwnPassword(db: Db, userId: string, currentPassword: string, newPassword: string, keepSessionId?: string) {
  const row = db.prepare('SELECT password_hash FROM users WHERE id = ?').get(userId) as { password_hash: string } | undefined
  if (!row) return { ok: false, reason: 'not_found' }
  const valid = await bcrypt.compare(currentPassword, row.password_hash)
  if (!valid) return { ok: false, reason: 'wrong_current' }
  const hash = await bcrypt.hash(newPassword, 10)
  db.prepare('UPDATE users SET password_hash = ? WHERE id = ?').run(hash, userId)
  if (keepSessionId) {
    db.prepare('DELETE FROM sessions WHERE user_id = ? AND id != ?').run(userId, keepSessionId)
  }
  return { ok: true }
}

export async function registerUser(db: Db, username: string, password: string, language = 'es', role = 'user') {
  if (!username || !password) return null
  const existing = getUserByUsername(db, username)
  if (existing) return null
  const hash = await bcrypt.hash(password, 10)
  return createUser(db, username, hash, language, role)
}

const DUMMY_HASH = '$2b$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012'

export async function handleLogin(db: Db, c: Context<AppEnv>, body: { username?: string; password?: string } | null) {
  const { username, password } = body || {}
  if (!username || !password) return null

  const user = getUserByUsername(db, username)
  if (!user) {
    await bcrypt.compare(password, DUMMY_HASH)
    return null
  }

  const valid = await bcrypt.compare(password, user.password_hash)
  if (!valid) return null

  // Rotación de sesión: invalida la sesión previa de la cookie (anti session-fixation)
  const previous = sessionIdFromCookie(db, c)
  if (previous) deleteSession(db, previous.id)

  const id = createSession(db, user.id, config.sessionTtlMs, c.req.header('user-agent'))
  const value = `${id}.${sign(db, id)}`
  const isHttps = c.req.header('x-forwarded-proto') === 'https' || c.req.url.startsWith('https://')
  setCookie(c, COOKIE_NAME, value, {
    httpOnly: true,
    sameSite: 'Lax',
    secure: isHttps,
    maxAge: Math.floor(config.sessionTtlMs / 1000),
    path: '/',
  })
  return { user: { id: user.id, username: user.username, email: user.email, phone: user.phone, language: user.language, role: user.role } as PublicUser }
}

export function handleLogout(db: Db, c: Context<AppEnv>) {
  const session = sessionIdFromCookie(db, c)
  if (session) deleteSession(db, session.id)
  deleteCookie(c, COOKIE_NAME, { path: '/' })
}

export function sessionIdFromCookie(db: Db, c: Context<AppEnv>) {
  const raw = getCookie(c)[COOKIE_NAME]
  if (!raw) return null
  const dot = raw.lastIndexOf('.')
  if (dot < 0) return null
  const id = raw.slice(0, dot)
  const sig = raw.slice(dot + 1)
  if (!safeEqual(sig, sign(db, id))) return null
  const session = getSession(db, id)
  if (!session) return null
  return { id: session.id, userId: session.user_id }
}

export function requireAuth(db: Db): (c: Context<AppEnv>, next: Next) => Promise<Response | void> {
  return async (c: Context<AppEnv>, next: Next) => {
    const session = sessionIdFromCookie(db, c)
    if (!session) return c.json({ error: 'no autorizado' }, 401)
    // Expiración deslizante (#130): si la sesión queda por debajo de la mitad
    // del TTL, extender su vida y re-emitir la cookie con maxAge fresco.
    if (renewSessionIfDue(db, session.id, config.sessionTtlMs)) {
      const isHttps = c.req.header('x-forwarded-proto') === 'https' || c.req.url.startsWith('https://')
      setCookie(c, COOKIE_NAME, `${session.id}.${sign(db, session.id)}`, {
        httpOnly: true,
        sameSite: 'Lax',
        secure: isHttps,
        maxAge: Math.floor(config.sessionTtlMs / 1000),
        path: '/',
      })
    }
    c.set('userId', session.userId)
    return next()
  }
}

export { updateUser }

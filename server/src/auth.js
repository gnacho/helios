import crypto from 'node:crypto'
import bcrypt from 'bcrypt'
import { getCookie, setCookie, deleteCookie } from 'hono/cookie'
import { createSession, getSession, deleteSession, createUser, getUserByUsername, getUserById, updateUser } from './db.js'
import { config } from './config.js'

const COOKIE_NAME = 'helios_session'

function secret(db) {
  if (config.sessionSecret) return config.sessionSecret
  const row = db.prepare('SELECT value FROM kv WHERE key = ?').get('session_secret')
  if (row) return row.value
  const generated = crypto.randomBytes(32).toString('hex')
  db.prepare('INSERT INTO kv (key, value) VALUES (?, ?)').run('session_secret', generated)
  return generated
}

function sign(db, id) {
  return crypto.createHmac('sha256', secret(db)).update(id).digest('hex')
}

function safeEqual(a, b) {
  const ba = Buffer.from(String(a))
  const bb = Buffer.from(String(b))
  if (ba.length !== bb.length) return false
  return crypto.timingSafeEqual(ba, bb)
}

function clientIp(c) {
  return c.req.header('x-forwarded-for')?.split(',')[0].trim() || c.env?.incoming?.socket?.remoteAddress || 'unknown'
}

export function loginRateLimited(db, c) {
  const ip = clientIp(c)
  const row = db.prepare('SELECT * FROM login_attempts WHERE ip = ?').get(ip)
  if (!row) return false
  if (row.locked_until > Date.now()) return true
  return false
}

export function registerLoginFail(db, c) {
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

export function loginOk(db, c) {
  const ip = clientIp(c)
  db.prepare('DELETE FROM login_attempts WHERE ip = ?').run(ip)
}

// Crea el admin inicial desde .env (AUTH_USER/AUTH_PASS) si no existe. Idempotente.
export async function ensureBootstrapAdmin(db) {
  const existing = getUserByUsername(db, config.authUser)
  if (existing) return
  const hash = await bcrypt.hash(config.authPass, 10)
  createUser(db, config.authUser, hash, 'es', 'admin')
  console.log(`[helios] admin bootstrap creado: ${config.authUser}`)
}

// Cambio de contraseña del propio usuario: exige la actual (bcrypt) y cierra el resto de sesiones.
export async function changeOwnPassword(db, userId, currentPassword, newPassword, keepSessionId) {
  const row = db.prepare('SELECT password_hash FROM users WHERE id = ?').get(userId)
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

export async function registerUser(db, username, password, language = 'es', role = 'user') {
  if (!username || !password) return null
  const existing = getUserByUsername(db, username)
  if (existing) return null
  const hash = await bcrypt.hash(password, 10)
  return createUser(db, username, hash, language, role)
}

export async function handleLogin(db, c, body) {
  const { username, password } = body || {}
  if (!username || !password) return null
  
  const user = getUserByUsername(db, username)
  if (!user) return null
  
  const valid = await bcrypt.compare(password, user.password_hash)
  if (!valid) return null
  
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
  return { user: { id: user.id, username: user.username, email: user.email, phone: user.phone, language: user.language, role: user.role } }
}

export function handleLogout(db, c) {
  const session = sessionIdFromCookie(db, c)
  if (session) deleteSession(db, session.id)
  deleteCookie(c, COOKIE_NAME, { path: '/' })
}

export function sessionIdFromCookie(db, c) {
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

export function requireAuth(db) {
  return async (c, next) => {
    const session = sessionIdFromCookie(db, c)
    if (!session) return c.json({ error: 'no autorizado' }, 401)
    c.set('userId', session.userId)
    return next()
  }
}

export { updateUser }

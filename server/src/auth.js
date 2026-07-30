import crypto from 'node:crypto'
import { getCookie, setCookie, deleteCookie } from 'hono/cookie'
import { createSession, getSession, deleteSession } from './db.js'
import { config } from './config.js'

const COOKIE_NAME = 'helios_session'
const loginAttempts = new Map()

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

export function loginRateLimited(c) {
  const ip = clientIp(c)
  const rec = loginAttempts.get(ip)
  if (rec && rec.until > Date.now()) return true
  return false
}

export function registerLoginFail(c) {
  const ip = clientIp(c)
  const rec = loginAttempts.get(ip) || { fails: 0, until: 0 }
  rec.fails += 1
  if (rec.fails >= 5) {
    rec.until = Date.now() + 5 * 60 * 1000
    rec.fails = 0
  }
  loginAttempts.set(ip, rec)
}

export function loginOk(c) {
  loginAttempts.delete(clientIp(c))
}

export function handleLogin(db, c, body) {
  const { username, password } = body || {}
  if (!username || !password) return null
  if (!safeEqual(username, config.authUser) || !safeEqual(password, config.authPass)) return null
  const id = createSession(db, config.sessionTtlMs, c.req.header('user-agent'))
  const value = `${id}.${sign(db, id)}`
  const isHttps =
    c.req.header('x-forwarded-proto') === 'https' || c.req.url.startsWith('https://')
  setCookie(c, COOKIE_NAME, value, {
    httpOnly: true,
    sameSite: 'Lax',
    secure: isHttps,
    maxAge: Math.floor(config.sessionTtlMs / 1000),
    path: '/',
  })
  return { user: config.authUser }
}

export function handleLogout(db, c) {
  const id = sessionIdFromCookie(db, c)
  if (id) deleteSession(db, id)
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
  if (!getSession(db, id)) return null
  return id
}

export function requireAuth(db) {
  return async (c, next) => {
    const id = sessionIdFromCookie(db, c)
    if (!id) return c.json({ error: 'no autorizado' }, 401)
    return next()
  }
}

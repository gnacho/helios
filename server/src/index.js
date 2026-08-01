import fs from 'node:fs'
import crypto from 'node:crypto'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { z } from 'zod'
import bcrypt from 'bcryptjs'

const dirname = path.dirname(fileURLToPath(import.meta.url))
const envPath = path.join(dirname, '..', '.env')
if (fs.existsSync(envPath)) {
  for (const line of fs.readFileSync(envPath, 'utf8').split('\n')) {
    const m = line.match(/^\s*([A-Z0-9_]+)\s*=\s*(.*)\s*$/)
    if (m && process.env[m[1]] === undefined) process.env[m[1]] = m[2]
  }
}

const { serve } = await import('@hono/node-server')
const { serveStatic } = await import('@hono/node-server/serve-static')
const { Hono } = await import('hono')
const { streamSSE } = await import('hono/streaming')
const { config, LIVE_ENTITIES } = await import('./config.js')
const { HAClient } = await import('./ha.js')
const dbModule = await import('./db.js')
const solar = await import('./solar.js')
const auth = await import('./auth.js')

const db = dbModule.openDb(config.dataDir)
const { dailyRange, dailyCount, cleanSessions, kvGet, kvSet } = dbModule
await auth.ensureBootstrapAdmin(db)

const ha = new HAClient(config.haosUrl, config.haosToken)
let haReady = false
let backfillState = 'pending'

ha.on('connected', async () => {
  console.log('[helios] HAOS conectado')
  await ha.subscribeEntities(LIVE_ENTITIES)
  haReady = true
  if (backfillState === 'pending') {
    backfillState = 'running'
    try {
      const r = await solar.maybeBackfill(ha, db)
      if (r.ran) console.log(`[helios] backfill inicial: ${r.rows} días`)
      await solar.ensureConsumptionBaseline(ha, db)
      backfillState = 'done'
    } catch (err) {
      backfillState = 'error'
      console.error('[helios] backfill error:', err.message)
    }
  }
})
ha.on('disconnected', () => {
  haReady = false
  console.log('[helios] HAOS desconectado, reintentando...')
})
ha.on('fatal', (msg) => {
  console.error('[helios] FATAL:', msg)
})
ha.on('wsError', (msg) => {
  console.error('[helios] ws:', msg)
})
ha.start()

const app = new Hono()

app.get('/health', (c) => {
  let dbOk = true
  try {
    db.prepare('SELECT 1').get()
  } catch {
    dbOk = false
  }
  const mem = process.memoryUsage()
  return c.json(
    {
      status: dbOk ? 'ok' : 'degraded',
      uptime: Math.round(process.uptime()),
      memory: { rss: mem.rss, heapUsed: mem.heapUsed },
      db: dbOk ? 'connected' : 'error',
      haos: haReady ? 'connected' : 'disconnected',
    },
    dbOk ? 200 : 503
  )
})

// Security headers middleware
app.use('*', async (c, next) => {
  c.header('X-Content-Type-Options', 'nosniff')
  c.header('X-Frame-Options', 'DENY')
  c.header('Referrer-Policy', 'strict-origin-when-cross-origin')
  c.header('Permissions-Policy', 'geolocation=(), microphone=(), camera=()')
  c.header('Strict-Transport-Security', 'max-age=31536000; includeSubDomains')
  c.header('Content-Security-Policy', "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
  await next()
})
const sseClients = new Set()
const MAX_SSE_CLIENTS = 10

let lastPush = 0
ha.on('entity', () => {
  const now = Date.now()
  if (now - lastPush < 1000 || sseClients.size === 0) return
  lastPush = now
  const payload = `data: ${JSON.stringify({ type: 'live', data: solar.computeLive(ha) })}\n\n`
  for (const c of sseClients) {
    try {
      c.write(payload)
    } catch {}
  }
})

// Validation schemas
const dateSchema = z.string().regex(/^\d{4}-\d{2}-\d{2}$/).optional()
const loginSchema = z.object({
  username: z.string().min(1),
  password: z.string().min(1)
})
const registerSchema = z.object({
  username: z.string().min(3).max(50),
  password: z.string().min(6),
  language: z.string().regex(/^(es|en|zh-CN)$/).optional(),
  role: z.enum(['user', 'admin']).optional()
})
const profileSchema = z.object({
  email: z.string().email().optional().nullable(),
  phone: z.string().optional().nullable(),
  language: z.string().regex(/^(es|en|zh-CN)$/).optional()
})
const passwordSchema = z.object({
  current: z.string().min(1),
  password: z.string().min(6)
})

app.post('/api/auth/login', async (c) => {
  if (auth.loginRateLimited(db, c)) return c.json({ error: 'demasiados intentos, espera 5 minutos' }, 429)
  const body = await c.req.json().catch(() => null)
  const parsed = loginSchema.safeParse(body)
  if (!parsed.success) {
    return c.json({ error: 'formato inválido' }, 400)
  }
  const res = await auth.handleLogin(db, c, parsed.data)
  if (!res) {
    auth.registerLoginFail(db, c)
    return c.json({ error: 'usuario o contraseña incorrectos' }, 401)
  }
  auth.loginOk(db, c)
  return c.json({ ok: true, user: res.user })
})

app.post('/api/auth/register', async (c) => {
  const session = auth.sessionIdFromCookie(db, c)
  if (!session) return c.json({ authenticated: false }, 401)
  
  const currentUser = dbModule.getUserById(db, session.userId)
  if (!currentUser || currentUser.role !== 'admin') {
    return c.json({ error: 'solo administradores pueden crear usuarios' }, 403)
  }
  
  const body = await c.req.json().catch(() => null)
  const parsed = registerSchema.safeParse(body)
  if (!parsed.success) {
    return c.json({ error: 'formato inválido' }, 400)
  }
  const res = await auth.registerUser(db, parsed.data.username, parsed.data.password, parsed.data.language || 'es', parsed.data.role || 'user')
  if (!res) {
    return c.json({ error: 'usuario ya existe' }, 409)
  }
  return c.json({ ok: true, user: res })
})

app.post('/api/auth/logout', (c) => {
  auth.handleLogout(db, c)
  return c.json({ ok: true })
})

app.get('/api/auth/me', (c) => {
  const session = auth.sessionIdFromCookie(db, c)
  if (!session) return c.json({ authenticated: false }, 401)
  const user = dbModule.getUserById(db, session.userId)
  if (!user) return c.json({ authenticated: false }, 401)
  return c.json({ authenticated: true, user })
})

app.get('/api/auth/profile', (c) => {
  const session = auth.sessionIdFromCookie(db, c)
  if (!session) return c.json({ authenticated: false }, 401)
  const user = dbModule.getUserById(db, session.userId)
  if (!user) return c.json({ authenticated: false }, 401)
  return c.json({ user })
})

app.put('/api/auth/profile', async (c) => {
  const session = auth.sessionIdFromCookie(db, c)
  if (!session) return c.json({ authenticated: false }, 401)
  const body = await c.req.json().catch(() => null)
  const parsed = profileSchema.safeParse(body)
  if (!parsed.success) {
    return c.json({ error: 'formato inválido' }, 400)
  }
  const updated = auth.updateUser(db, session.userId, parsed.data)
  if (!updated) return c.json({ error: 'no se pudo actualizar' }, 500)
  return c.json({ ok: true, user: updated })
})

app.put('/api/auth/password', async (c) => {
  const session = auth.sessionIdFromCookie(db, c)
  if (!session) return c.json({ authenticated: false }, 401)
  const body = await c.req.json().catch(() => null)
  const parsed = passwordSchema.safeParse(body)
  if (!parsed.success) return c.json({ error: 'formato inválido' }, 400)
  const res = await auth.changeOwnPassword(db, session.userId, parsed.data.current, parsed.data.password, session.id)
  if (!res.ok) {
    return c.json({ error: res.reason === 'wrong_current' ? 'la contraseña actual no es correcta' : 'no se pudo cambiar' }, 400)
  }
  return c.json({ ok: true })
})

app.get('/api/auth/users', (c) => {
  const session = auth.sessionIdFromCookie(db, c)
  if (!session) return c.json({ authenticated: false }, 401)
  const user = dbModule.getUserById(db, session.userId)
  if (!user || user.role !== 'admin') {
    return c.json({ error: 'solo administradores pueden ver usuarios' }, 403)
  }
  const users = db.prepare('SELECT id, username, email, phone, language, role, created_at FROM users').all()
  return c.json({ users })
})

app.put('/api/auth/users/:id/password', async (c) => {
  const session = auth.sessionIdFromCookie(db, c)
  if (!session) return c.json({ authenticated: false }, 401)
  const user = dbModule.getUserById(db, session.userId)
  if (!user || user.role !== 'admin') {
    return c.json({ error: 'solo administradores pueden cambiar contraseñas' }, 403)
  }
  
  const userId = c.req.param('id')
  const body = await c.req.json().catch(() => null)
  const parsed = z.object({ password: z.string().min(6) }).safeParse(body)
  if (!parsed.success) return c.json({ error: 'formato inválido' }, 400)
  
  const hash = await bcrypt.hash(parsed.data.password, 10)
  db.prepare('UPDATE users SET password_hash = ? WHERE id = ?').run(hash, userId)
  return c.json({ ok: true })
})

app.put('/api/auth/users/:id/language', async (c) => {
  const session = auth.sessionIdFromCookie(db, c)
  if (!session) return c.json({ authenticated: false }, 401)
  const user = dbModule.getUserById(db, session.userId)
  if (!user || user.role !== 'admin') {
    return c.json({ error: 'solo administradores pueden cambiar idioma' }, 403)
  }
  
  const userId = c.req.param('id')
  const body = await c.req.json().catch(() => null)
  const parsed = z.object({ language: z.string().regex(/^(es|en|zh-CN)$/) }).safeParse(body)
  if (!parsed.success) return c.json({ error: 'formato inválido' }, 400)
  
  db.prepare('UPDATE users SET language = ? WHERE id = ?').run(parsed.data.language, userId)
  return c.json({ ok: true })
})

app.put('/api/auth/users/:id/role', async (c) => {
  const session = auth.sessionIdFromCookie(db, c)
  if (!session) return c.json({ authenticated: false }, 401)
  const user = dbModule.getUserById(db, session.userId)
  if (!user || user.role !== 'admin') {
    return c.json({ error: 'solo administradores pueden cambiar roles' }, 403)
  }
  
  const userId = c.req.param('id')
  if (userId === session.userId) {
    return c.json({ error: 'no puedes cambiar tu propio rol' }, 400)
  }
  const body = await c.req.json().catch(() => null)
  const parsed = z.object({ role: z.enum(['user', 'admin']) }).safeParse(body)
  if (!parsed.success) return c.json({ error: 'formato inválido' }, 400)
  
  db.prepare('UPDATE users SET role = ? WHERE id = ?').run(parsed.data.role, userId)
  return c.json({ ok: true })
})

app.delete('/api/auth/users/:id', (c) => {
  const session = auth.sessionIdFromCookie(db, c)
  if (!session) return c.json({ authenticated: false }, 401)
  const user = dbModule.getUserById(db, session.userId)
  if (!user || user.role !== 'admin') {
    return c.json({ error: 'solo administradores pueden eliminar usuarios' }, 403)
  }
  
  const userId = c.req.param('id')
  if (userId === session.userId) {
    return c.json({ error: 'no puedes eliminarte a ti mismo' }, 400)
  }
  
  db.prepare('DELETE FROM users WHERE id = ?').run(userId)
  db.prepare('DELETE FROM sessions WHERE user_id = ?').run(userId)
  return c.json({ ok: true })
})

// Recovery de admin: SOLO conexión directa desde localhost (sin cabeceras de proxy).
// Uso: curl -X POST http://127.0.0.1:<puerto>/api/auth/recover desde SSH en el host.
app.post('/api/auth/recover', async (c) => {
  const ip = c.env?.incoming?.socket?.remoteAddress
  const proxied = c.req.header('x-forwarded-for') || c.req.header('x-real-ip')
  if (proxied || (ip !== '127.0.0.1' && ip !== '::1' && ip !== '::ffff:127.0.0.1')) {
    return c.json({ error: 'solo desde localhost' }, 403)
  }
  const temp = crypto.randomBytes(9).toString('base64url')
  const hash = await bcrypt.hash(temp, 10)
  db.prepare('UPDATE users SET password_hash = ? WHERE role = ?').run(hash, 'admin')
  console.log('[helios] recovery: contraseña de admins reseteada (temporal)')
  return c.json({ ok: true, tempPassword: temp })
})

const guarded = new Hono()
guarded.use('*', auth.requireAuth(db))

guarded.get('/solar/live', (c) => {
  return c.json({ connected: haReady, ...solar.computeLive(ha) })
})

guarded.get('/solar/day', async (c) => {
  try {
    const date = c.req.query('date')
    const parsed = dateSchema.safeParse(date)
    if (!parsed.success) {
      return c.json({ error: 'formato inválido, usa YYYY-MM-DD' }, 400)
    }
    const result = await solar.getDaySeries(ha, parsed.data, db)
    return c.json({ date: parsed.data || solar.todayStr(), points: result.points, estimated: result.estimated })
  } catch (err) {
    return c.json({ error: err.message }, 400)
  }
})

guarded.get('/solar/kpis', async (c) => {
  try {
    const date = c.req.query('date')
    const parsed = dateSchema.safeParse(date)
    if (!parsed.success) {
      return c.json({ error: 'formato inválido, usa YYYY-MM-DD' }, 400)
    }
    const kpis = await solar.getKpis(ha, parsed.data, db)
    return c.json({ date: parsed.data || solar.todayStr(), ...kpis })
  } catch (err) {
    return c.json({ error: err.message }, 400)
  }
})

guarded.get('/solar/history', (c) => {
  const to = c.req.query('to')
  const from = c.req.query('from')
  const toParsed = dateSchema.safeParse(to)
  const fromParsed = dateSchema.safeParse(from)
  if (!toParsed.success || !fromParsed.success) {
    return c.json({ error: 'formato inválido, usa YYYY-MM-DD' }, 400)
  }
  const pageParsed = z
    .object({
      limit: z.coerce.number().int().min(1).max(1000).default(1000),
      offset: z.coerce.number().int().min(0).default(0),
    })
    .safeParse({ limit: c.req.query('limit'), offset: c.req.query('offset') })
  if (!pageParsed.success) {
    return c.json({ error: 'paginación inválida' }, 400)
  }
  const toFinal = toParsed.data || solar.todayStr()
  const fromFinal = fromParsed.data || shiftDays(toFinal, -364)
  const total = dailyCount(db, fromFinal, toFinal)
  const rows = dailyRange(db, fromFinal, toFinal, pageParsed.data.limit, pageParsed.data.offset)
  const days = rows.map((r) => ({
    date: r.date,
    productionKwh: r.production_kwh,
    solisKwh: r.solis_kwh ?? 0,
    foxKwh: r.fox_kwh ?? 0,
    consumptionKwh: r.consumption_kwh,
    gridImportKwh: r.grid_import_kwh,
    gridExportKwh: r.grid_export_kwh,
    batteryChargedKwh: r.battery_charged_kwh,
    batteryDischargedKwh: r.battery_discharged_kwh,
    autoconsumoPct:
      r.production_kwh > 0 ? Math.min(100, ((r.production_kwh - r.grid_export_kwh) / r.production_kwh) * 100) : 0,
  }))
  return c.json({ from: fromFinal, to: toFinal, backfill: backfillState, total, limit: pageParsed.data.limit, offset: pageParsed.data.offset, days })
})

guarded.get('/solar/stream', (c) => {
  if (sseClients.size >= MAX_SSE_CLIENTS) {
    return c.json({ error: 'demasiados clientes conectados' }, 429)
  }
  c.header('X-Accel-Buffering', 'no')
  c.header('Cache-Control', 'no-cache')
  return streamSSE(c, async (stream) => {
    const client = {
      write: (payload) => stream.write(payload),
    }
    sseClients.add(client)
    await stream.write(`data: ${JSON.stringify({ type: 'live', data: solar.computeLive(ha) })}\n\n`)
    const heartbeat = setInterval(() => {
      stream.write(`: ping\n\n`).catch(() => {})
    }, 30000)
    stream.onAbort(() => {
      clearInterval(heartbeat)
      sseClients.delete(client)
    })
    await new Promise(() => {})
  })
})

guarded.get('/config', (c) => {
  const raw = kvGet(db, 'install_config')
  return c.json(
    raw
      ? JSON.parse(raw)
      : {
          solisKwp: 4.4,
          foxKwp: 2.7,
          batteryKwh: 5.12,
          batteryReservePct: 20,
          priceImportEur: config.priceImport,
          priceExportEur: config.priceExport,
          co2KgPerKwh: config.co2PerKwh,
        }
  )
})

guarded.put('/config', async (c) => {
  const body = await c.req.json().catch(() => null)
  if (!body || typeof body !== 'object') return c.json({ error: 'body inválido' }, 400)
  kvSet(db, 'install_config', JSON.stringify(body))
  return c.json({ ok: true })
})

app.route('/api', guarded)

app.onError((err, c) => {
  console.error('[helios] error:', err.message)
  return c.json({ error: 'error interno' }, 500)
})

app.use('/*', serveStatic({ root: config.staticDir }))

app.get('*', (c) => {
  if (c.req.path.startsWith('/api/')) return c.json({ error: 'no encontrado' }, 404)
  if (c.req.path.startsWith('/assets/')) return c.text('no encontrado', 404)
  // Se lee en cada petición: permite redesplegar el frontend sin reiniciar el server
  try {
    return c.html(fs.readFileSync(path.join(config.staticDir, 'index.html'), 'utf8'))
  } catch {
    return c.text('frontend no desplegado', 404)
  }
})

serve({ fetch: app.fetch, port: config.port, hostname: config.host }, (info) => {
  console.log(`[helios] escuchando en http://${config.host}:${info.port}`)
})

setInterval(() => cleanSessions(db), 3600 * 1000).unref()
setInterval(() => {
  try {
    db.pragma('wal_checkpoint(TRUNCATE)')
  } catch (err) {
    console.error('[helios] wal_checkpoint error:', err.message)
  }
}, 3600 * 1000).unref()

function scheduleNightly() {
  const now = new Date()
  const next = new Date(now)
  next.setDate(next.getDate() + 1)
  next.setHours(0, 10, 0, 0)
  setTimeout(async () => {
    try {
      const n = await solar.backfillHistory(ha, db)
      await solar.ensureConsumptionBaseline(ha, db)
      console.log(`[helios] consolidación nocturna: ${n} días`)
    } catch (err) {
      console.error('[helios] consolidación nocturna error:', err.message)
    }
    scheduleNightly()
  }, next.getTime() - now.getTime()).unref()
}
scheduleNightly()

// Graceful shutdown
function gracefulShutdown(signal) {
  console.log(`[helios] ${signal} recibido, cerrando...`)
  ha.stop()
  for (const client of sseClients) {
    try { client.write('event: shutdown\ndata: {}\n\n') } catch {}
  }
  db.close()
  process.exit(0)
}
process.on('SIGTERM', () => gracefulShutdown('SIGTERM'))
process.on('SIGINT', () => gracefulShutdown('SIGINT'))

function shiftDays(dateStr, days) {
  const d = new Date(dateStr + 'T00:00:00')
  d.setDate(d.getDate() + days)
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

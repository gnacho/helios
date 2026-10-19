// routes-push.js — endpoints de suscripción y preferencias Web Push (Helios).
// Auth: sesión por cookie HttpOnly SameSite=Lax mismo-origen (middleware
// requireAuth del router `guarded`; no hay cabecera CSRF separada en Helios).
// El endpoint push es una capability URL SECRETA: nunca se loguea.
import crypto from 'node:crypto'
import { z } from 'zod'
import { isPushConfigured, pushPublicKey, TIPOS_ALERTA } from './push.js'

const subscribeSchema = z.object({
  endpoint: z.url().max(1000),
  expirationTime: z.number().nullable().optional(),
  keys: z.object({
    p256dh: z.string().min(1).max(200),
    auth: z.string().min(1).max(200),
  }),
})

const unsubscribeSchema = z.object({
  endpoint: z.url().max(1000),
})

const prefSchema = z.object({
  tipo: z.enum(TIPOS_ALERTA),
  enabled: z.boolean(),
})

// Registra las rutas en un router que YA lleva requireAuth (c.get('userId')).
export function registerPushRoutes(router, db) {
  // Clave pública VAPID para applicationServerKey. No es secreta, pero solo
  // se sirve con sesión. Sin claves configuradas la UI muestra el aviso.
  router.get('/push/vapid-public-key', (c) => {
    if (!isPushConfigured()) return c.json({ error: 'push no configurado en el servidor' }, 503)
    return c.json({ publicKey: pushPublicKey() })
  })

  // Upsert por endpoint: re-suscripción o cambio de usuario en el mismo
  // navegador = UPDATE (el endpoint es único por dispositivo/navegador).
  router.post('/push/subscribe', async (c) => {
    if (!isPushConfigured()) return c.json({ error: 'push no configurado en el servidor' }, 503)
    const body = await c.req.json().catch(() => null)
    const parsed = subscribeSchema.safeParse(body)
    if (!parsed.success) return c.json({ error: 'suscripción inválida' }, 400)
    const now = Date.now()
    db.prepare(
      `INSERT INTO push_subscriptions (id, user_id, endpoint, p256dh, auth, user_agent, created_at, updated_at)
       VALUES (?, ?, ?, ?, ?, ?, ?, ?)
       ON CONFLICT(endpoint) DO UPDATE SET
         user_id = excluded.user_id,
         p256dh = excluded.p256dh,
         auth = excluded.auth,
         user_agent = excluded.user_agent,
         updated_at = excluded.updated_at`
    ).run(
      crypto.randomUUID(),
      c.get('userId'),
      parsed.data.endpoint,
      parsed.data.keys.p256dh,
      parsed.data.keys.auth,
      c.req.header('user-agent') || null,
      now,
      now
    )
    return c.json({ ok: true }, 201)
  })

  // Borra SOLO si la suscripción pertenece al usuario de la sesión.
  router.delete('/push/unsubscribe', async (c) => {
    const body = await c.req.json().catch(() => null)
    const parsed = unsubscribeSchema.safeParse(body)
    if (!parsed.success) return c.json({ error: 'petición inválida' }, 400)
    db.prepare('DELETE FROM push_subscriptions WHERE endpoint = ? AND user_id = ?').run(
      parsed.data.endpoint,
      c.get('userId')
    )
    return c.json({ ok: true })
  })

  // Preferencias por tipo de alerta del usuario de la sesión.
  // Sin fila = activado (defecto). Se devuelven todos los tipos conocidos.
  router.get('/push/preferences', (c) => {
    const filas = db
      .prepare('SELECT tipo, enabled FROM notification_preferences WHERE user_id = ?')
      .all(c.get('userId'))
    const mapa = Object.fromEntries(filas.map((f) => [f.tipo, f.enabled === 1]))
    const prefs = Object.fromEntries(TIPOS_ALERTA.map((t) => [t, mapa[t] !== false]))
    return c.json({ prefs })
  })

  router.put('/push/preferences', async (c) => {
    const body = await c.req.json().catch(() => null)
    const parsed = prefSchema.safeParse(body)
    if (!parsed.success) return c.json({ error: 'preferencia inválida' }, 400)
    db.prepare(
      `INSERT INTO notification_preferences (user_id, tipo, enabled, min_severity, updated_at)
       VALUES (?, ?, ?, 'normal', ?)
       ON CONFLICT(user_id, tipo) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at`
    ).run(c.get('userId'), parsed.data.tipo, parsed.data.enabled ? 1 : 0, Date.now())
    return c.json({ ok: true })
  })
}

// push.js — Web Push (VAPID) para Helios: configuración, motor de envío con
// i18n server-side (es/en/zh-CN), preferencias por usuario, quiet hours con
// cola consolidada y borrado de suscripciones muertas (404/410).
// Patrón: skill web-push-alerts, adaptado de Deltos (deltos/server/src/push.js).
//
// Decisiones propias de Helios:
// - SIN fail-fast sin claves VAPID: en LAN HTTP el push está dormido por
//   secure context; la app arranca igual y la UI muestra "Requiere HTTPS".
// - Helios NO tiene modo demo en server: no hay flag demo.
// - Tiempos en epoch ms (convención del resto del esquema).
// - Las alertas son de SISTEMA (energía): se notifican a TODOS los usuarios;
//   las preferencias por tipo filtran por usuario.
import crypto from 'node:crypto'
import webpush from 'web-push'

let vapidOk = false
let sendFn = (sub, payload, opts) => webpush.sendNotification(sub, payload, opts)

// Configura VAPID una vez al arrancar. Devuelve true si el push queda activo.
export function configurePush({ publicKey, privateKey, subject } = {}) {
  if (!publicKey || !privateKey || !subject) {
    vapidOk = false
    console.warn('[push] sin VAPID_PUBLIC_KEY/PRIVATE_KEY/SUBJECT: notificaciones push desactivadas')
    return false
  }
  webpush.setVapidDetails(subject, publicKey, privateKey)
  vapidOk = true
  console.log('[push] VAPID configurado: notificaciones push activas')
  return true
}

export function isPushConfigured() {
  return vapidOk
}

export function pushPublicKey() {
  return vapidOk ? process.env.VAPID_PUBLIC_KEY : null
}

// --- Tests: inyectar un sender falso y resetear estado ----------------------
export function _setSendFn(fn) {
  sendFn = fn || ((sub, payload, opts) => webpush.sendNotification(sub, payload, opts))
}
export function _resetForTests() {
  vapidOk = false
  _setSendFn(null)
  stmtCache = new WeakMap()
}

// Catálogo i18n (texto FINAL compuesto en servidor; el SW no traduce). Textos
// sin marcas de inversor (issue #39): el nombre real sale de la topología y se
// pasa en `datos.nombre` donde aplica (fox_offline/fox_ok usan el 2º inversor).
const CATALOGO = {
  es: {
    inversor_offline: { titulo: 'Inversor offline', cuerpo: () => 'El inversor no responde' },
    inversor_ok: { titulo: 'Inversor recuperado', cuerpo: () => 'El inversor vuelve a estar online' },
    fox_offline: { titulo: 'Inversor offline', cuerpo: (d) => `El inversor ${d.nombre || 'secundario'} no reporta datos (de día)` },
    fox_ok: { titulo: 'Inversor recuperado', cuerpo: (d) => `El inversor ${d.nombre || 'secundario'} vuelve a reportar` },
    corte_red: { titulo: 'Corte de red', cuerpo: () => 'Posible corte de red eléctrica: consumo desde batería/PV' },
    corte_red_ok: { titulo: 'Red recuperada', cuerpo: () => 'La red eléctrica vuelve a estar presente' },
    bateria_baja: { titulo: 'Batería baja', cuerpo: (d) => `SOC en ${d.soc}% (reserva ${d.reserva}%)` },
    resumen_diario: {
      titulo: 'Resumen del día',
      cuerpo: (d) => `Producción ${d.produccion} kWh · Consumo ${d.consumo} kWh · Autoconsumo ${d.autoconsumo}%`,
    },
    resumen: { titulo: 'Actividad en Helios', cuerpo: (d) => `${d.total} alertas durante las horas de silencio` },
  },
  en: {
    inversor_offline: { titulo: 'Inverter offline', cuerpo: () => 'The inverter is not responding' },
    inversor_ok: { titulo: 'Inverter recovered', cuerpo: () => 'The inverter is back online' },
    fox_offline: { titulo: 'Inverter offline', cuerpo: (d) => `The ${d.nombre || 'secondary'} inverter is not reporting (daytime)` },
    fox_ok: { titulo: 'Inverter recovered', cuerpo: (d) => `The ${d.nombre || 'secondary'} inverter is reporting again` },
    corte_red: { titulo: 'Grid outage', cuerpo: () => 'Possible grid outage: running on battery/PV' },
    corte_red_ok: { titulo: 'Grid restored', cuerpo: () => 'Grid power is back' },
    bateria_baja: { titulo: 'Low battery', cuerpo: (d) => `SOC at ${d.soc}% (reserve ${d.reserva}%)` },
    resumen_diario: {
      titulo: 'Daily summary',
      cuerpo: (d) => `Production ${d.produccion} kWh · Consumption ${d.consumo} kWh · Self-consumption ${d.autoconsumo}%`,
    },
    resumen: { titulo: 'Helios activity', cuerpo: (d) => `${d.total} alerts during your quiet hours` },
  },
  zh: {
    inversor_offline: { titulo: '逆变器离线', cuerpo: () => '逆变器无响应' },
    inversor_ok: { titulo: '逆变器已恢复', cuerpo: () => '逆变器已重新上线' },
    fox_offline: { titulo: '逆变器离线', cuerpo: (d) => `${d.nombre || '辅助'} 逆变器白天无数据` },
    fox_ok: { titulo: '逆变器已恢复', cuerpo: (d) => `${d.nombre || '辅助'} 逆变器已恢复数据` },
    corte_red: { titulo: '电网停电', cuerpo: () => '可能电网停电：正在使用电池/光伏供电' },
    corte_red_ok: { titulo: '电网已恢复', cuerpo: () => '电网供电已恢复' },
    bateria_baja: { titulo: '电池电量低', cuerpo: (d) => `SOC ${d.soc}%（保留 ${d.reserva}%）` },
    resumen_diario: {
      titulo: '每日总结',
      cuerpo: (d) => `发电 ${d.produccion} kWh · 用电 ${d.consumo} kWh · 自用 ${d.autoconsumo}%`,
    },
    resumen: { titulo: 'Helios 动态', cuerpo: (d) => `免打扰期间有 ${d.total} 条提醒` },
  },
}

// Tipos de alerta conocidos (para validación de preferencias).
export const TIPOS_ALERTA = [
  'inversor_offline',
  'inversor_ok',
  'fox_offline',
  'fox_ok',
  'corte_red',
  'corte_red_ok',
  'bateria_baja',
  'resumen_diario',
]

const SEVERIDADES = ['normal', 'high', 'critical']

// --- Statements cacheados por BD (WeakMap: permite :memory: en tests) -------
let stmtCache = new WeakMap()
function stmts(db) {
  let s = stmtCache.get(db)
  if (!s) {
    s = {
      subsPorUsuario: db.prepare('SELECT id, endpoint, p256dh, auth FROM push_subscriptions WHERE user_id = ?'),
      idioma: db.prepare('SELECT language FROM users WHERE id = ?'),
      pref: db.prepare('SELECT enabled, min_severity FROM notification_preferences WHERE user_id = ? AND tipo = ?'),
      quiet: db.prepare('SELECT quiet_start, quiet_end, tz FROM notification_quiet_hours WHERE user_id = ?'),
      borrarPorEndpoint: db.prepare('DELETE FROM push_subscriptions WHERE endpoint = ?'),
      encolar: db.prepare('INSERT INTO notification_queue (id, user_id, tipo, severity, datos_json, created_at) VALUES (?, ?, ?, ?, ?, ?)'),
      colaAgrupada: db.prepare(
        `SELECT user_id, tipo, severity, COUNT(*) AS total, MIN(datos_json) AS datos_json
         FROM notification_queue GROUP BY user_id, tipo, severity`
      ),
      todosUsuarios: db.prepare('SELECT id FROM users'),
    }
    stmtCache.set(db, s)
  }
  return s
}

function idiomaDe(db, userId) {
  const lang = stmts(db).idioma.get(userId)?.language
  if (lang === 'en') return 'en'
  if (lang === 'zh-CN' || lang === 'zh') return 'zh'
  return 'es' // 'auto' y desconocidos → es (defecto de la casa)
}

// Quiet hours en la zona horaria del usuario (Intl, sin dependencias).
function enQuietHours(db, userId) {
  const q = stmts(db).quiet.get(userId)
  if (!q || q.quiet_start === null || q.quiet_end === null) return false
  const hora = Number(
    new Intl.DateTimeFormat('en-GB', { hour: '2-digit', hour12: false, timeZone: q.tz }).format(new Date())
  )
  if (q.quiet_start <= q.quiet_end) return hora >= q.quiet_start && hora < q.quiet_end
  return hora >= q.quiet_start || hora < q.quiet_end // cruza medianoche
}

function componerPayload(lang, tipo, datos, url) {
  const entrada = CATALOGO[lang][tipo] || CATALOGO[lang].resumen
  const title = entrada.titulo
  const body = entrada.cuerpo(datos)
  return JSON.stringify({
    // Campos planos → handler push del SW (Chrome/Firefox/Safari)
    title,
    body,
    url,
    tag: tipo, // coalescing: mismo tag reemplaza la notificación anterior
    // Declarative Web Push (Safari/iOS 18.4+, sin ejecutar el SW)
    web_push: 8030,
    notification: { title, body, navigate: url },
  })
}

// Envío a UNA suscripción: 404/410 = muerta (borrar); 429/5xx = reintentar
// con backoff + jitter (máx 3); otros status = bug nuestro (log sin endpoint).
async function enviarAUna(db, sub, json, opciones) {
  const destino = { endpoint: sub.endpoint, keys: { p256dh: sub.p256dh, auth: sub.auth } }
  for (let intento = 1; intento <= 3; intento++) {
    try {
      await sendFn(destino, json, { ...opciones, contentEncoding: 'aes128gcm' })
      return 'ok'
    } catch (err) {
      const status = err?.statusCode
      if (status === 404 || status === 410) {
        stmts(db).borrarPorEndpoint.run(sub.endpoint)
        return 'borrada'
      }
      if (status === 429 || (status !== undefined && status >= 500)) {
        if (intento < 3) {
          await new Promise((r) => setTimeout(r, 500 * 2 ** intento + Math.floor(Math.random() * 250)))
          continue
        }
        return 'fallido'
      }
      console.error(`[push] error status=${status} sub=${sub.id}: ${err?.message}`)
      return 'fallido'
    }
  }
  return 'fallido'
}

/**
 * Notifica por push a usuarios (app CERRADA; con la app abierta ya se enteran
 * por SSE). Respeta preferencias y quiet hours. Devuelve contadores.
 */
export async function notifyUsers(db, userIds, tipo, datos = {}, opciones = {}) {
  const { severity = 'normal', url = '/', ttl = severity === 'critical' ? 3600 : 21600 } = opciones
  const res = { enviados: 0, borrados: 0, fallidos: 0, pospuestos: 0, omitidos: 0 }
  const s = stmts(db)

  for (const userId of [...new Set(userIds)]) {
    const pref = s.pref.get(userId, tipo)
    if (pref) {
      if (!pref.enabled || SEVERIDADES.indexOf(severity) < SEVERIDADES.indexOf(pref.min_severity)) {
        res.omitidos++
        continue
      }
    }
    if (severity !== 'critical' && enQuietHours(db, userId)) {
      s.encolar.run(crypto.randomUUID(), userId, tipo, severity, JSON.stringify(datos), Date.now())
      res.pospuestos++
      continue
    }
    const lang = idiomaDe(db, userId)
    const json = componerPayload(lang, tipo, datos, url)
    if (!vapidOk) {
      res.omitidos++
      continue
    }
    const subs = s.subsPorUsuario.all(userId)
    if (subs.length === 0) {
      res.omitidos++
      continue
    }
    const urgency = severity === 'critical' ? 'high' : severity === 'high' ? 'normal' : 'low'
    const resultados = await Promise.allSettled(subs.map((sub) => enviarAUna(db, sub, json, { TTL: ttl, urgency, topic: tipo })))
    for (const r of resultados) {
      if (r.status === 'fulfilled' && r.value === 'ok') res.enviados++
      else if (r.status === 'fulfilled' && r.value === 'borrada') res.borrados++
      else res.fallidos++
    }
  }
  return res
}

// Atajo para alertas de sistema (energía): avisa a TODOS los usuarios.
// Fire-and-forget desde el motor (nunca bloquea el tick del motor).
export function notifyAll(db, tipo, datos, opciones = {}) {
  const ids = stmts(db).todosUsuarios.all().map((r) => r.id)
  if (ids.length === 0) return
  notifyUsers(db, ids, tipo, datos, opciones).catch((err) => console.error('[push] error en notifyAll:', err))
}

// Mantenimiento horario: consolida la cola de quiet hours en UN resumen por
// usuario+tipo y vacía la cola. Llamado desde el intervalo horario de index.js.
export async function flushNotificationQueue(db) {
  const s = stmts(db)
  const grupos = s.colaAgrupada.all()
  if (grupos.length === 0) return
  for (const g of grupos) {
    // Fuera de quiet hours ya: se entrega el resumen. Si sigue en ventana, se
    // queda en cola para el próximo tick.
    if (enQuietHours(db, g.user_id)) continue
    await notifyUsers(db, [g.user_id], 'resumen', { total: g.total }, { severity: g.severity })
    db.prepare('DELETE FROM notification_queue WHERE user_id = ? AND tipo = ?').run(g.user_id, g.tipo)
  }
}

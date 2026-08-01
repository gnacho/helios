// push.test.js — motor Web Push (notifyUsers, preferencias, quiet hours,
// borrado 404/410) y motor de alertas (flancos, anti-rebote, resumen diario).
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import Database from 'better-sqlite3'
import webpush from 'web-push'
import { initSchema } from '../src/db.js'
import { configurePush, notifyUsers, flushNotificationQueue, _setSendFn, _resetForTests } from '../src/push.js'
import { createAlertsEngine, enviarResumenDiario } from '../src/alerts.js'

// Par VAPID real (setVapidDetails valida formato; claves fake no pasan).
const KEYS = webpush.generateVAPIDKeys()
function configura() {
  process.env.VAPID_PUBLIC_KEY = KEYS.publicKey
  configurePush({ publicKey: KEYS.publicKey, privateKey: KEYS.privateKey, subject: 'mailto:test@example.com' })
}

let db
beforeEach(() => {
  _resetForTests()
  db = initSchema(new Database(':memory:'))
})
afterEach(() => _resetForTests())

function insertUser(username, language = 'es') {
  const id = crypto.randomUUID()
  db.prepare("INSERT INTO users (id, username, password_hash, language, role, created_at) VALUES (?, ?, 'x', ?, 'user', ?)").run(
    id,
    username,
    language,
    Date.now()
  )
  return id
}

function insertSub(userId, endpoint = `https://push.example.com/${crypto.randomUUID()}`) {
  const now = Date.now()
  db.prepare(
    'INSERT INTO push_subscriptions (id, user_id, endpoint, p256dh, auth, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)'
  ).run(crypto.randomUUID(), userId, endpoint, 'p', 'a', now, now)
  return endpoint
}

describe('schema', () => {
  it('crea las tablas push sin tocar las existentes', () => {
    const tables = db.prepare("SELECT name FROM sqlite_master WHERE type='table'").all().map((r) => r.name)
    for (const t of ['push_subscriptions', 'notification_preferences', 'notification_quiet_hours', 'notification_queue']) {
      expect(tables).toContain(t)
    }
    expect(tables).toContain('daily')
    expect(tables).toContain('users')
  })
})

describe('motor notifyUsers', () => {
  it('envía con el idioma del usuario (es/en/zh) y borra suscripciones muertas (410)', async () => {
    const u1 = insertUser('ana', 'es')
    const u2 = insertUser('bob', 'en')
    const u3 = insertUser('chen', 'zh-CN')
    insertSub(u1, 'https://push.example.com/ana')
    insertSub(u2, 'https://push.example.com/bob')
    insertSub(u3, 'https://push.example.com/chen')

    const enviados = []
    _setSendFn(async (sub, payload) => {
      if (sub.endpoint.includes('bob')) {
        const err = new Error('Gone')
        err.statusCode = 410
        throw err
      }
      enviados.push({ sub, payload: JSON.parse(payload) })
    })
    configura()

    const res = await notifyUsers(db, [u1, u2, u3], 'inversor_offline', {}, { severity: 'critical' })
    expect(res.enviados).toBe(2)
    expect(res.borrados).toBe(1)
    const porIdioma = Object.fromEntries(enviados.map((e) => [e.sub.endpoint.split('/').pop(), e.payload.title]))
    expect(porIdioma.ana).toBe('Inversor offline')
    expect(porIdioma.chen).toBe('逆变器离线')
    const restantes = db.prepare('SELECT endpoint FROM push_subscriptions').all()
    expect(restantes).toHaveLength(2)
    expect(restantes.map((r) => r.endpoint).join()).not.toContain('bob')
  })

  it('respeta preferencias: tipo desactivado = omitido', async () => {
    const u1 = insertUser('ana')
    insertSub(u1)
    db.prepare(
      'INSERT INTO notification_preferences (user_id, tipo, enabled, min_severity, updated_at) VALUES (?, ?, 0, ?, ?)'
    ).run(u1, 'resumen_diario', 'normal', Date.now())
    configura()
    _setSendFn(async () => {
      throw new Error('no debería enviarse')
    })
    const res = await notifyUsers(db, [u1], 'resumen_diario', { produccion: 1, consumo: 2, autoconsumo: 50 })
    expect(res.omitidos).toBe(1)
    expect(res.enviados).toBe(0)
  })

  it('quiet hours: pospone (encola) y el flush consolida al salir de la ventana', async () => {
    const u1 = insertUser('ana')
    insertSub(u1)
    // Ventana que SIEMPRE cubre la hora actual (incluso a las 23 h): empieza
    // ahora y cruza medianoche (start > end = ventana válida que cruza).
    const hora = Number(
      new Intl.DateTimeFormat('en-GB', { hour: '2-digit', hour12: false, timeZone: 'Europe/Madrid' }).format(new Date())
    )
    db.prepare(
      'INSERT INTO notification_quiet_hours (user_id, quiet_start, quiet_end, tz, updated_at) VALUES (?, ?, ?, ?, ?)'
    ).run(u1, hora, (hora + 2) % 24, 'Europe/Madrid', Date.now())
    configura()

    const res = await notifyUsers(db, [u1], 'bateria_baja', { soc: 18, reserva: 20 }, { severity: 'high' })
    expect(res.pospuestos).toBe(1)
    expect(db.prepare('SELECT * FROM notification_queue').all()).toHaveLength(1)

    // Con la ventana activa, el flush NO entrega (sigue en cola)
    await flushNotificationQueue(db)
    expect(db.prepare('SELECT * FROM notification_queue').all()).toHaveLength(1)

    // Las críticas NO se posponen aunque haya quiet hours
    const crit = await notifyUsers(db, [u1], 'corte_red', {}, { severity: 'critical' })
    expect(crit.pospuestos).toBe(0)

    // Cerramos la ventana y el flush consolida en UN resumen
    db.prepare('DELETE FROM notification_quiet_hours').run()
    const enviados = []
    _setSendFn(async (sub, payload) => {
      enviados.push(JSON.parse(payload))
    })
    await flushNotificationQueue(db)
    expect(db.prepare('SELECT * FROM notification_queue').all()).toHaveLength(0)
    expect(enviados).toHaveLength(1)
    expect(enviados[0].title).toBe('Actividad en Helios')
    expect(enviados[0].body).toContain('1 alertas')
  })

  it('sin VAPID configurado omite el envío real', async () => {
    const u1 = insertUser('ana')
    insertSub(u1)
    const res = await notifyUsers(db, [u1], 'inversor_offline', {}, { severity: 'critical' })
    expect(res.enviados).toBe(0)
    expect(res.omitidos).toBe(1)
  })
})

// --- Motor de alertas (flancos y anti-rebote) --------------------------------

function mockHa({ inverterOnline = 1, gridPower = 0.5, lastUpdate = new Date().toISOString() } = {}) {
  return {
    connected: true,
    getState: (id) => {
      if (id.endsWith('scraper')) {
        return { state: 'ok', attributes: { inverterOnline, currentGridPower: gridPower, lastUpdate } }
      }
      return null
    },
  }
}

function mockSolar(live = {}) {
  return {
    computeLive: () => ({ consumption: 0.5, production: 0, soc: 50, batteryPower: 0, ...live }),
    todayStr: () => '2026-08-02',
    getKpis: async () => ({ productionKwh: 12.3, consumptionKwh: 8.4, autoconsumoPct: 61.4 }),
  }
}

function capturaNotifs() {
  const llamadas = []
  return { llamadas, notifyFn: (db, tipo, datos, opciones) => llamadas.push({ tipo, datos, opciones }) }
}

describe('motor de alertas', () => {
  it('inversor offline: dispara al 2º tick, no repite, y avisa al recuperar', () => {
    const { llamadas, notifyFn } = capturaNotifs()
    const engine = createAlertsEngine({ db, ha: mockHa({ inverterOnline: 0 }), solar: mockSolar(), notifyFn })
    engine.tick()
    expect(llamadas).toHaveLength(0) // anti-rebote: 1 tick no basta
    engine.tick()
    expect(llamadas).toHaveLength(1)
    expect(llamadas[0].tipo).toBe('inversor_offline')
    expect(llamadas[0].opciones.severity).toBe('critical')
    engine.tick()
    engine.tick()
    expect(llamadas).toHaveLength(1) // no reenvía mientras persiste
  })

  it('inversor recuperado: avisa una sola vez al volver online', () => {
    const { llamadas, notifyFn } = capturaNotifs()
    const haOff = mockHa({ inverterOnline: 0 })
    const engine = createAlertsEngine({ db, ha: haOff, solar: mockSolar(), notifyFn })
    engine.tick()
    engine.tick()
    expect(llamadas.map((l) => l.tipo)).toEqual(['inversor_offline'])
    // Recupera: mismo engine, ha ahora online
    haOff.getState = mockHa({ inverterOnline: 1 }).getState
    engine.tick()
    expect(llamadas.map((l) => l.tipo)).toEqual(['inversor_offline', 'inversor_ok'])
    engine.tick()
    expect(llamadas).toHaveLength(2)
  })

  it('corte de red: firma sostenida 3 ticks dispara crítica y recupera a los 2', () => {
    const { llamadas, notifyFn } = capturaNotifs()
    // Noche: grid 0, consumo 0,5 kW, batería descargando
    const ha = mockHa({ gridPower: 0 })
    const solar = mockSolar({ consumption: 0.5, batteryPower: -0.3, production: 0 })
    const engine = createAlertsEngine({ db, ha, solar, notifyFn })
    engine.tick()
    engine.tick()
    expect(llamadas).toHaveLength(0)
    engine.tick()
    expect(llamadas.map((l) => l.tipo)).toEqual(['corte_red'])
    expect(llamadas[0].opciones.severity).toBe('critical')
    // Vuelve la red
    ha.getState = mockHa({ gridPower: 0.8 }).getState
    engine.tick()
    expect(llamadas).toHaveLength(1)
    engine.tick()
    expect(llamadas.map((l) => l.tipo)).toEqual(['corte_red', 'corte_red_ok'])
  })

  it('corte de red: NO dispara con scraper antiguo (>15 min)', () => {
    const { llamadas, notifyFn } = capturaNotifs()
    const viejo = new Date(Date.now() - 30 * 60000).toISOString()
    const ha = mockHa({ gridPower: 0, lastUpdate: viejo })
    const engine = createAlertsEngine({ db, ha, solar: mockSolar({ consumption: 0.5, batteryPower: -0.3 }), notifyFn })
    engine.tick()
    engine.tick()
    engine.tick()
    engine.tick()
    expect(llamadas).toHaveLength(0)
  })

  it('batería baja: dispara al cruzar la reserva y se rearma con histéresis', () => {
    const { llamadas, notifyFn } = capturaNotifs()
    const solar = mockSolar({ soc: 19 })
    const engine = createAlertsEngine({ db, ha: mockHa(), solar, notifyFn })
    engine.tick()
    expect(llamadas.map((l) => l.tipo)).toEqual(['bateria_baja'])
    expect(llamadas[0].datos).toEqual({ soc: 19, reserva: 20 })
    engine.tick()
    expect(llamadas).toHaveLength(1) // no repite
    // Sube a 22 (reserva 20 + histeresis 5 = rearma a >25): sigue alertado
    solar.computeLive = mockSolar({ soc: 22 }).computeLive
    engine.tick()
    expect(llamadas).toHaveLength(1)
    // Cae otra vez sin haber superado 25: NO vuelve a disparar
    solar.computeLive = mockSolar({ soc: 19 }).computeLive
    engine.tick()
    expect(llamadas).toHaveLength(1)
    // Supera 25: rearma; nueva caída dispara de nuevo
    solar.computeLive = mockSolar({ soc: 60 }).computeLive
    engine.tick()
    solar.computeLive = mockSolar({ soc: 18 }).computeLive
    engine.tick()
    expect(llamadas).toHaveLength(2)
  })

  it('batería: respeta la reserva configurada en install_config', () => {
    const { llamadas, notifyFn } = capturaNotifs()
    db.prepare('INSERT INTO kv (key, value) VALUES (?, ?)').run('install_config', JSON.stringify({ batteryReservePct: 30 }))
    const engine = createAlertsEngine({ db, ha: mockHa(), solar: mockSolar({ soc: 25 }), notifyFn })
    engine.tick()
    expect(llamadas).toHaveLength(1)
    expect(llamadas[0].datos.reserva).toBe(30)
  })

  it('sin conexión HAOS no evalúa nada', () => {
    const { llamadas, notifyFn } = capturaNotifs()
    const ha = { ...mockHa({ inverterOnline: 0 }), connected: false }
    const engine = createAlertsEngine({ db, ha, solar: mockSolar(), notifyFn })
    engine.tick()
    engine.tick()
    engine.tick()
    expect(llamadas).toHaveLength(0)
  })

  it('resumen diario: compone con los KPI del día', async () => {
    const llamadas = []
    const notifyFn = async (db, tipo, datos, opciones) => llamadas.push({ tipo, datos, opciones })
    await enviarResumenDiario(db, mockHa(), mockSolar(), notifyFn)
    expect(llamadas).toHaveLength(1)
    expect(llamadas[0].tipo).toBe('resumen_diario')
    expect(llamadas[0].datos).toEqual({ produccion: 12.3, consumo: 8.4, autoconsumo: 61 })
    expect(llamadas[0].opciones.severity).toBe('normal')
  })
})

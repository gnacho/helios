// push.test.js — motor Web Push (notifyUsers, preferencias, quiet hours,
// borrado 404/410) y motor de alertas (flancos, anti-rebote, resumen diario).
import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import Database from 'better-sqlite3'
import webpush from 'web-push'
import { initSchema } from '../src/db.js'
import { kvGet, kvSet } from '../src/db.js'
import { configurePush, notifyUsers, flushNotificationQueue, _setSendFn, _resetForTests } from '../src/push.js'
import { createAlertsEngine, enviarResumenDiario, proximoEnvioResumen, ejecutarResumenSiToca, RESUMEN_OFFSET_MIN, HORA_RESUMEN_DIARIO } from '../src/alerts.js'

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

function mockHa({ inverterOnline = 1, gridPower = 0.5, lastUpdate = new Date().toISOString(), sunState = 'above_horizon', foxState = '123.4' } = {}) {
  return {
    connected: true,
    getState: (id) => {
      if (id.endsWith('scraper')) {
        return { state: 'ok', attributes: { inverterOnline, currentGridPower: gridPower, lastUpdate } }
      }
      if (id === 'sun.sun') return { state: sunState, attributes: {} }
      if (id === 'sensor.almacen_pinza_power_b') return { state: foxState, attributes: {} }
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

// corte_red está desactivado por defecto (issue #19): los tests de la heurística
// lo habilitan vía install_config.corteRedEnabled.
function habilitaCorteRed() {
  const prev = (() => {
    try {
      return JSON.parse(kvGet(db, 'install_config') || '{}')
    } catch {
      return {}
    }
  })()
  kvSet(db, 'install_config', JSON.stringify({ ...prev, corteRedEnabled: true }))
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

  it('fox offline: dispara al 3er tick de día y avisa al recuperar', () => {
    const { llamadas, notifyFn } = capturaNotifs()
    const haFox = mockHa({ foxState: 'unavailable' })
    const engine = createAlertsEngine({ db, ha: haFox, solar: mockSolar(), notifyFn })
    engine.tick()
    engine.tick()
    expect(llamadas).toHaveLength(0) // 2 ticks no bastan (anti-rebote)
    engine.tick()
    expect(llamadas.map((l) => l.tipo)).toEqual(['fox_offline'])
    expect(llamadas[0].opciones.severity).toBe('high')
    engine.tick()
    expect(llamadas).toHaveLength(1) // no reenvía mientras persiste
    haFox.getState = mockHa({ foxState: '120.0' }).getState
    engine.tick()
    expect(llamadas.map((l) => l.tipo)).toEqual(['fox_offline', 'fox_ok'])
  })

  it('fox offline: NO se evalúa de noche (sun below_horizon)', () => {
    const { llamadas, notifyFn } = capturaNotifs()
    const engine = createAlertsEngine({
      db,
      ha: mockHa({ foxState: 'unavailable', sunState: 'below_horizon' }),
      solar: mockSolar(),
      notifyFn,
    })
    engine.tick()
    engine.tick()
    engine.tick()
    engine.tick()
    expect(llamadas).toHaveLength(0)
  })

  it('inversor: NO dispara de noche (sun below_horizon) aunque esté offline', () => {
    const { llamadas, notifyFn } = capturaNotifs()
    const engine = createAlertsEngine({
      db,
      ha: mockHa({ inverterOnline: 0, sunState: 'below_horizon' }),
      solar: mockSolar(),
      notifyFn,
    })
    engine.tick()
    engine.tick()
    engine.tick()
    engine.tick()
    expect(llamadas).toHaveLength(0)
  })

  it('inversor: NO dispara con scraper antiguo (>15 min) ni reevalúa hasta que refresca', () => {
    const { llamadas, notifyFn } = capturaNotifs()
    const viejo = new Date(Date.now() - 30 * 60000).toISOString()
    const ha = mockHa({ inverterOnline: 0, lastUpdate: viejo })
    const engine = createAlertsEngine({ db, ha, solar: mockSolar(), notifyFn })
    engine.tick()
    engine.tick()
    engine.tick()
    expect(llamadas).toHaveLength(0)
    // El scraper refresca y sigue offline de día: ahora sí, a los 2 ticks
    ha.getState = mockHa({ inverterOnline: 0 }).getState
    engine.tick()
    engine.tick()
    expect(llamadas.map((l) => l.tipo)).toEqual(['inversor_offline'])
  })

  it('inversor: la recuperación nocturna se notifica al reanudarse la evaluación (amanecer)', () => {
    const { llamadas, notifyFn } = capturaNotifs()
    const ha = mockHa({ inverterOnline: 0 })
    const engine = createAlertsEngine({ db, ha, solar: mockSolar(), notifyFn })
    engine.tick()
    engine.tick()
    expect(llamadas.map((l) => l.tipo)).toEqual(['inversor_offline'])
    // Anochece y el inversor vuelve: de noche no se evalúa
    ha.getState = mockHa({ inverterOnline: 1, sunState: 'below_horizon' }).getState
    engine.tick()
    engine.tick()
    expect(llamadas).toHaveLength(1)
    // Amanece: la evaluación se reanuda y se notifica la recuperación
    ha.getState = mockHa({ inverterOnline: 1 }).getState
    engine.tick()
    expect(llamadas.map((l) => l.tipo)).toEqual(['inversor_offline', 'inversor_ok'])
  })

  it('corte de red: firma sostenida 3 ticks dispara crítica y recupera a los 2', () => {
    habilitaCorteRed()
    const { llamadas, notifyFn } = capturaNotifs()
    // Corte real: el circuito NO respaldado cae (~0), el respaldado sigue por EPS+batería
    const ha = mockHa({ gridPower: 0 })
    const solar = mockSolar({ respaldoKw: 0.4, noRespaldadaKw: 0.02, consumption: 0.42, batteryPower: -0.3 })
    const engine = createAlertsEngine({ db, ha, solar, notifyFn })
    engine.tick()
    engine.tick()
    expect(llamadas).toHaveLength(0)
    engine.tick()
    expect(llamadas.map((l) => l.tipo)).toEqual(['corte_red'])
    expect(llamadas[0].opciones.severity).toBe('critical')
    // Vuelve la red: el circuito no respaldado recupera suministro
    solar.computeLive = () => ({ production: 0, consumption: 1.4, respaldoKw: 0.4, noRespaldadaKw: 1.0, soc: 50, batteryPower: 0 })
    engine.tick()
    expect(llamadas).toHaveLength(1)
    engine.tick()
    expect(llamadas.map((l) => l.tipo)).toEqual(['corte_red', 'corte_red_ok'])
  })

  it('corte de red: NO dispara con scraper antiguo (>15 min)', () => {
    habilitaCorteRed()
    const { llamadas, notifyFn } = capturaNotifs()
    const viejo = new Date(Date.now() - 30 * 60000).toISOString()
    const ha = mockHa({ gridPower: 0, lastUpdate: viejo })
    const engine = createAlertsEngine({
      db,
      ha,
      solar: mockSolar({ respaldoKw: 0.4, noRespaldadaKw: 0.02, consumption: 0.42, batteryPower: -0.3 }),
      notifyFn,
    })
    engine.tick()
    engine.tick()
    engine.tick()
    engine.tick()
    expect(llamadas).toHaveLength(0)
  })

  it('corte de red: NO dispara en autoconsumo (grid=0 pero ambas pinzas >0)', () => {
    // El falso positivo canónico: casa autoabasteciéndose con red presente
    habilitaCorteRed()
    const { llamadas, notifyFn } = capturaNotifs()
    const ha = mockHa({ gridPower: 0 })
    const engine = createAlertsEngine({
      db,
      ha,
      solar: mockSolar({ respaldoKw: 0.4, noRespaldadaKw: 1.4, consumption: 1.85, production: 1.5, batteryPower: 0.4 }),
      notifyFn,
    })
    for (let i = 0; i < 6; i++) engine.tick()
    expect(llamadas).toHaveLength(0)
  })

  it('corte de red: NO dispara en peak-shaving (batería descargando, grid=0, red presente)', () => {
    // El inversor descarga batería para no comprar en franja cara: grid≈0,
    // batería descargando — antes disparaba, ahora no
    habilitaCorteRed()
    const { llamadas, notifyFn } = capturaNotifs()
    const ha = mockHa({ gridPower: 0 })
    const engine = createAlertsEngine({
      db,
      ha,
      solar: mockSolar({ respaldoKw: 0.3, noRespaldadaKw: 0.7, consumption: 1.0, production: 0.2, batteryPower: -0.4 }),
      notifyFn,
    })
    for (let i = 0; i < 6; i++) engine.tick()
    expect(llamadas).toHaveLength(0)
  })

  it('corte de red: NO dispara cuando el Fox pierde Modbus (incidente tipo 12-Jul-2025)', () => {
    // El Fox reporta r_volt≈0 durante horas, pero la vivienda sigue consumiendo:
    // caída de comms del Fox, no corte eléctrico. La firma diferencial lo descarta.
    habilitaCorteRed()
    const { llamadas, notifyFn } = capturaNotifs()
    const ha = mockHa({ gridPower: 0 })
    const engine = createAlertsEngine({
      db,
      ha,
      solar: mockSolar({ respaldoKw: 0.3, noRespaldadaKw: 1.5, consumption: 1.8, production: 0.5, batteryPower: 0.1 }),
      notifyFn,
    })
    for (let i = 0; i < 6; i++) engine.tick()
    expect(llamadas).toHaveLength(0)
  })

  it('corte de red: NO dispara en apagón total sin EPS (ambas pinzas a 0)', () => {
    // Sin EPS, todo se apaga; esta alerta no es la responsable (la cubre inversor_offline)
    habilitaCorteRed()
    const { llamadas, notifyFn } = capturaNotifs()
    const ha = mockHa({ gridPower: 0 })
    const engine = createAlertsEngine({
      db,
      ha,
      solar: mockSolar({ respaldoKw: 0.0, noRespaldadaKw: 0.0, consumption: 0.0, production: 0, batteryPower: 0 }),
      notifyFn,
    })
    for (let i = 0; i < 6; i++) engine.tick()
    expect(llamadas).toHaveLength(0)
  })

  it('corte de red: NO dispara cuando la pinza no respaldada está en dropout pero la batería carga', () => {
    // Caso real 9-Ago-2026: pinza Zigbee (vivienda_medidor_power) en dropout
    // reportando 0 W mientras la red está perfecta (Fox on-grid, calentador a
    // 232 V) y la batería carga desde FV. La firma diferencial por sí sola
    // dispararía un crítico falso; la cláusula batteryPower < -0.05 lo filtra.
    habilitaCorteRed()
    const { llamadas, notifyFn } = capturaNotifs()
    const ha = mockHa({ gridPower: 0 })
    const engine = createAlertsEngine({
      db,
      ha,
      solar: mockSolar({ respaldoKw: 1.7, noRespaldadaKw: 0.0, consumption: 1.7, production: 1.4, batteryPower: 0.3 }),
      notifyFn,
    })
    for (let i = 0; i < 6; i++) engine.tick()
    expect(llamadas).toHaveLength(0)
  })

  it('corte de red: DESACTIVADO por defecto (issue #19), no dispara aunque la firma se cumpla', () => {
    // Sin install_config.corteRedEnabled → la heurística no se evalúa: ni con la
    // firma de "corte real" ni con la del falso positivo. Sin flags → 0 alarmas.
    const { llamadas, notifyFn } = capturaNotifs()
    const ha = mockHa({ gridPower: 0 })
    const solar = mockSolar({ respaldoKw: 0.4, noRespaldadaKw: 0.02, consumption: 0.42, batteryPower: -0.3 })
    const engine = createAlertsEngine({ db, ha, solar, notifyFn })
    for (let i = 0; i < 8; i++) engine.tick()
    expect(llamadas).toHaveLength(0)
  })

  it('corte de red: habilitado, dispara con los datos del disparo en el audit', () => {
    habilitaCorteRed()
    const { llamadas, notifyFn } = capturaNotifs()
    const ha = mockHa({ gridPower: 0 })
    const solar = mockSolar({ respaldoKw: 0.4, noRespaldadaKw: 0.02, consumption: 0.42, batteryPower: -0.3 })
    const engine = createAlertsEngine({ db, ha, solar, notifyFn })
    engine.tick()
    engine.tick()
    engine.tick()
    expect(llamadas.map((l) => l.tipo)).toEqual(['corte_red'])
    expect(llamadas[0].datos).toMatchObject({ gridMag: 0, respaldoKw: 0.4, noRespaldadaKw: 0.02, batteryPower: -0.3, fresco: true })
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

// --- Scheduler del resumen: anochecer vs fallback -----------------------------

function haConSun(nextSetting) {
  return {
    connected: true,
    getState: (id) => (id === 'sun.sun' ? { state: 'above_horizon', attributes: { next_setting: nextSetting } } : null),
  }
}
const haSinSun = { connected: true, getState: () => null }

describe('proximoEnvioResumen (anochecer con fallback)', () => {
  it('con sunset HOY → sunset + offset', () => {
    const mediodia = new Date('2026-08-08T12:00:00+02:00')
    const next = proximoEnvioResumen(haConSun('2026-08-08T21:30:00+02:00'), mediodia)
    const esperado = new Date(`2026-08-08T21:30:00+02:00`).getTime() + RESUMEN_OFFSET_MIN * 60000
    expect(next.getTime()).toBe(esperado)
  })

  it('con sunset MAÑANA (ya anocheció hoy) → sunset de mañana + offset (sigue al próximo anochecer)', () => {
    const noche = new Date('2026-08-08T22:00:00+02:00')
    const next = proximoEnvioResumen(haConSun('2026-08-09T21:14:00+02:00'), noche)
    expect(next.getTime()).toBe(new Date('2026-08-09T21:14:00+02:00').getTime() + RESUMEN_OFFSET_MIN * 60000)
  })

  it('sin dato solar (HAOS caído) de día → fallback 21:00 local hoy', () => {
    // Constructor Date(y,m,d,h) = hora LOCAL del runner: portable entre TZ.
    const mediodia = new Date(2026, 7, 8, 12, 0, 0)
    const next = proximoEnvioResumen(haSinSun, mediodia)
    const esperado = new Date(2026, 7, 8, HORA_RESUMEN_DIARIO, 0, 0)
    expect(next.getTime()).toBe(esperado.getTime())
  })

  it('sin dato solar pasado 21:00 local → fallback 21:00 local de mañana', () => {
    const noche = new Date(2026, 7, 8, 22, 0, 0)
    const next = proximoEnvioResumen(haSinSun, noche)
    const esperado = new Date(2026, 7, 9, HORA_RESUMEN_DIARIO, 0, 0)
    expect(next.getTime()).toBe(esperado.getTime())
  })

  it('el offset de asentamiento respeta RESUMEN_OFFSET_MIN y HORA_RESUMEN_DIARIO exportados', () => {
    expect(RESUMEN_OFFSET_MIN).toBeGreaterThan(0)
    expect(HORA_RESUMEN_DIARIO).toBe(21)
  })
})

describe('ejecutarResumenSiToca (guarda anti-doble-envío)', () => {
  it('envía una vez y la segunda llamada el mismo día se omite', async () => {
    const llamadas = []
    const notifyFn = async (db, tipo, datos, opciones) => llamadas.push({ tipo, datos, opciones })
    const r1 = await ejecutarResumenSiToca(db, mockHa(), mockSolar(), notifyFn)
    const r2 = await ejecutarResumenSiToca(db, mockHa(), mockSolar(), notifyFn)
    expect(r1).toBe(true)
    expect(r2).toBe(false)
    expect(llamadas).toHaveLength(1)
  })
})

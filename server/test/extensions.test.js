// extensions.test.js — marco de extensiones (issue #94) y varios cargadores
// (issue #146): resolución kv>legacy>generic, migración de la config plana,
// live por cargador, acumulación diaria por deltas del contador (tabla
// daily_charger), atribución solar y backfill desde el recorder.
import { describe, it, expect, afterEach } from 'vitest'
import Database from 'better-sqlite3'
import { z } from 'zod'
import { initSchema, kvSet, kvGet } from '../src/db.js'
import { createSchemas } from '../../shared/schemas.js'
import { _setForTests as _setInstallForTests, LEGACY_TOPOLOGY } from '../src/install.js'
import {
  resolveExtensions,
  normalizeExtensions,
  chargerActive,
  chargerEntities,
  computeChargerLive,
  getChargerByKey,
  accumulateChargerDaily,
  pvShareNow,
  dailyDeltasFromHistory,
  chargerPvFromCurves,
  backfillChargerHistory,
  chargerDayCurve,
  addChargerKwh,
  setChargerKwhIfNull,
  chargerHistory,
  _setForTests,
  LEGACY_EXTENSIONS,
  GENERIC_EXTENSIONS,
} from '../src/extensions.js'

const { extensionsSchema } = createSchemas(z)

afterEach(() => {
  _setForTests(GENERIC_EXTENSIONS)
  _setInstallForTests(LEGACY_TOPOLOGY)
})

function emptyDb() {
  return initSchema(new Database(':memory:'))
}

function dbWithDaily() {
  const db = initSchema(new Database(':memory:'))
  db.prepare(
    `INSERT INTO daily (date, production_kwh, consumption_kwh, solis_kwh, fox_kwh)
     VALUES ('2026-08-01', 10, 8, 6, 4)`
  ).run()
  return db
}

/** Extensión de test: un cargador 'main' con entidades cortas. */
function chargerCfg(overrides = {}) {
  return normalizeExtensions({
    enabled: true,
    carCharger: {
      enabled: true,
      chargers: [
        {
          key: 'main',
          name: 'Pared',
          powerId: 'sensor.chg_power',
          powerUnit: 'kW',
          energyTotalId: 'sensor.chg_total',
          energySessionId: 'sensor.chg_session',
          stateId: 'sensor.chg_state',
          tempId: 'sensor.chg_temp',
          switchId: 'switch.chg',
          ampId: 'number.chg_amp',
          chargingStates: ['charging'],
          connectedStates: ['connected', 'charging'],
          ...overrides,
        },
      ],
    },
  })
}

/** Dos cargadores: 'main' (Pared) y 'c2' (Portátil). */
function twoChargersCfg() {
  return normalizeExtensions({
    enabled: true,
    carCharger: {
      enabled: true,
      chargers: [
        {
          key: 'main',
          name: 'Pared',
          powerId: 'sensor.chg_power',
          powerUnit: 'kW',
          energyTotalId: 'sensor.chg_total',
          stateId: 'sensor.chg_state',
          switchId: 'switch.chg',
          ampId: 'number.chg_amp',
          chargingStates: ['charging'],
          connectedStates: ['connected', 'charging'],
        },
        {
          key: 'c2',
          name: 'Portátil',
          powerId: 'sensor.chg2_power',
          powerUnit: 'W',
          energyTotalId: 'sensor.chg2_total',
          energyDivisor: 100,
          stateId: 'sensor.chg2_state',
          switchId: 'switch.chg2',
          ampId: 'select.chg2_amp',
          chargingStates: ['charging'],
          connectedStates: ['connected', 'charging'],
        },
      ],
    },
  })
}

function mockHa(states) {
  return {
    connected: true,
    getState: (id) => states[id],
  }
}

/** Fecha local YYYY-MM-DD de hoy (como todayStr del server). */
function kvDate() {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

/** Fecha local YYYY-MM-DD desplazada n días desde hoy. */
function dayOffset(n) {
  const d = new Date()
  d.setDate(d.getDate() + n)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

describe('resolveExtensions', () => {
  it('instalación con datos SIN kv → perfil LEGACY (entidades prefabricadas, apagado)', () => {
    const ext = resolveExtensions(dbWithDaily())
    expect(ext.enabled).toBe(false)
    expect(ext.carCharger.enabled).toBe(false)
    expect(ext.carCharger.chargers[0].powerId).toBe(LEGACY_EXTENSIONS.carCharger.chargers[0].powerId)
    expect(ext.carCharger.chargers[0].key).toBe('main')
  })

  it('instalación nueva SIN kv → perfil GENERIC', () => {
    const ext = resolveExtensions(emptyDb())
    expect(ext.carCharger.chargers[0].powerId).toBe(GENERIC_EXTENSIONS.carCharger.chargers[0].powerId)
  })

  it('extensions_config en kv gana sobre el perfil base', () => {
    const db = dbWithDaily()
    kvSet(db, 'extensions_config', JSON.stringify({
      enabled: true,
      carCharger: { enabled: true, powerId: 'sensor.mi_cargador' },
    }))
    const ext = resolveExtensions(db)
    expect(ext.enabled).toBe(true)
    expect(ext.carCharger.enabled).toBe(true)
    expect(ext.carCharger.chargers[0].powerId).toBe('sensor.mi_cargador')
    // el resto de campos vienen del perfil base
    expect(ext.carCharger.chargers[0].powerUnit).toBe('kW')
  })

  it('kv corrupto → fallback al perfil base', () => {
    const db = dbWithDaily()
    kvSet(db, 'extensions_config', '{no json')
    const ext = resolveExtensions(db)
    expect(ext.carCharger.chargers[0].powerId).toBe(LEGACY_EXTENSIONS.carCharger.chargers[0].powerId)
  })

  it('el schema zod valida la forma del PUT (lista de cargadores)', () => {
    const ok = extensionsSchema.safeParse({ enabled: true, carCharger: { enabled: true, chargers: [{ powerId: 'sensor.x' }] } })
    expect(ok.success).toBe(true)
    const bad = extensionsSchema.safeParse({ carCharger: { chargers: [{ powerUnit: 'MW' }] } })
    expect(bad.success).toBe(false)
    const badKey = extensionsSchema.safeParse({ carCharger: { chargers: [{ key: 'A B!' }] } })
    expect(badKey.success).toBe(false)
  })
})

describe('normalizeExtensions', () => {
  it('coerción de tipos inesperados a valores seguros', () => {
    const ext = normalizeExtensions({
      enabled: 'si',
      carCharger: { enabled: 1, name: '  ', powerId: 42, chargingStates: 'charging', powerUnit: 'W' },
    })
    expect(ext.enabled).toBe(false)
    expect(ext.carCharger.enabled).toBe(false)
    expect(ext.carCharger.chargers).toHaveLength(1)
    expect(ext.carCharger.chargers[0].name).toBe(GENERIC_EXTENSIONS.carCharger.chargers[0].name)
    expect(ext.carCharger.chargers[0].powerId).toBe('')
    expect(ext.carCharger.chargers[0].chargingStates).toEqual([])
    expect(ext.carCharger.chargers[0].powerUnit).toBe('W')
  })

  it('config plana antigua → migrada al primer cargador (main) sin perder nada', () => {
    const ext = normalizeExtensions({
      enabled: true,
      carCharger: {
        enabled: true,
        name: 'Pared',
        powerId: 'sensor.p',
        powerUnit: 'W',
        energyDivisor: 100,
        energyTotalId: 'sensor.t',
        switchId: 'switch.s',
        ampId: 'number.a',
        chargerInHouseMeters: false,
      },
    })
    const c0 = ext.carCharger.chargers[0]
    expect(c0.key).toBe('main')
    expect(c0.name).toBe('Pared')
    expect(c0.powerId).toBe('sensor.p')
    expect(c0.powerUnit).toBe('W')
    expect(c0.energyDivisor).toBe(100)
    expect(c0.ampId).toBe('number.a')
    expect(c0.chargerInHouseMeters).toBe(false)
  })

  it('lista de cargadores: claves auto (main, c2...) y claves inválidas reemplazadas', () => {
    const ext = normalizeExtensions({
      enabled: true,
      carCharger: {
        enabled: true,
        chargers: [
          { key: 'PARED!', name: 'Pared' },
          { name: 'Portátil' },
          { key: 'ok-key', name: 'Tercero' },
        ],
      },
    })
    expect(ext.carCharger.chargers.map((c) => c.key)).toEqual(['main', 'c2', 'ok-key'])
    expect(ext.carCharger.chargers.map((c) => c.name)).toEqual(['Pared', 'Portátil', 'Tercero'])
  })

  it('sin cargadores en la lista → la extensión del cargador no está activa', () => {
    const ext = normalizeExtensions({ enabled: true, carCharger: { enabled: true, chargers: [] } })
    expect(chargerActive(ext)).toBe(false)
  })
})

describe('chargerActive / chargerEntities', () => {
  it('marco apagado → sin entidades aunque el cargador esté encendido', () => {
    const ext = normalizeExtensions({ enabled: false, carCharger: { enabled: true } })
    expect(chargerActive(ext)).toBe(false)
    expect(chargerEntities(ext)).toEqual([])
  })

  it('activo → entidades deduplicadas sin huecos', () => {
    const ext = chargerCfg({ tempId: '', switchId: '', ampId: '', energySessionId: 'sensor.chg_power' })
    expect(chargerEntities(ext)).toEqual(['sensor.chg_power', 'sensor.chg_total', 'sensor.chg_state'])
  })

  it('varios cargadores → unión de entidades de todos', () => {
    const ext = twoChargersCfg()
    expect(chargerEntities(ext)).toEqual([
      'sensor.chg_power', 'sensor.chg_total', 'sensor.chg_state', 'switch.chg', 'number.chg_amp',
      'sensor.chg2_power', 'sensor.chg2_total', 'sensor.chg2_state', 'switch.chg2', 'select.chg2_amp',
    ])
  })
})

describe('computeChargerLive', () => {
  it('extensión inactiva → lista vacía (la clave no viaja en el SSE)', () => {
    expect(computeChargerLive(mockHa({}), GENERIC_EXTENSIONS)).toEqual([])
  })

  it('cargando por estado + kW directos', () => {
    _setForTests(chargerCfg())
    const live = computeChargerLive(
      mockHa({
        'sensor.chg_power': { state: '2.3' },
        'sensor.chg_state': { state: 'charging' },
        'sensor.chg_session': { state: '12.5' },
        'sensor.chg_total': { state: '469' },
        'sensor.chg_temp': { state: '42' },
        'switch.chg': { state: 'on' },
        'number.chg_amp': { state: '16' },
      })
    )[0]
    expect(live.key).toBe('main')
    expect(live.name).toBe('Pared')
    expect(live.charging).toBe(true)
    expect(live.connected).toBe(true)
    expect(live.powerKw).toBe(2.3)
    expect(live.sessionKwh).toBe(12.5)
    expect(live.totalKwh).toBe(469)
    expect(live.tempC).toBe(42)
    expect(live.switchOn).toBe(true)
    expect(live.amps).toBe(16)
  })

  it('fallback: potencia significativa cuenta como cargando sin estado', () => {
    _setForTests(chargerCfg())
    const live = computeChargerLive(mockHa({ 'sensor.chg_power': { state: '3.1' } }))[0]
    expect(live.charging).toBe(true)
    expect(live.connected).toBe(true)
    expect(live.state).toBeUndefined()
  })

  it('conectado sin cargar (enchufado en espera)', () => {
    _setForTests(chargerCfg())
    const live = computeChargerLive(
      mockHa({ 'sensor.chg_power': { state: '0' }, 'sensor.chg_state': { state: 'connected' } })
    )[0]
    expect(live.charging).toBe(false)
    expect(live.connected).toBe(true)
  })

  it('unidades W → kW y unavailable → undefined por campo', () => {
    _setForTests(chargerCfg({ powerUnit: 'W' }))
    const live = computeChargerLive(
      mockHa({
        'sensor.chg_power': { state: '2300' },
        'sensor.chg_temp': { state: 'unavailable' },
        'sensor.chg_state': { state: 'unknown' },
      })
    )[0]
    expect(live.powerKw).toBe(2.3)
    expect(live.tempC).toBeUndefined()
    expect(live.state).toBeUndefined()
  })

  it('dos cargadores → snapshots independientes con divisor propio', () => {
    _setForTests(twoChargersCfg())
    const live = computeChargerLive(
      mockHa({
        'sensor.chg_power': { state: '7.2' },
        'sensor.chg2_power': { state: '1380' },
        'sensor.chg2_total': { state: '1510' },
      })
    )
    expect(live).toHaveLength(2)
    expect(live[0].powerKw).toBe(7.2)
    expect(live[1].powerKw).toBe(1.38)
    expect(live[1].totalKwh).toBe(15.1)
    expect(live[1].name).toBe('Portátil')
  })
})

describe('accumulateChargerDaily', () => {
  it('primer tick solo siembra la base; los siguientes acumulan el delta', () => {
    const db = emptyDb()
    _setForTests(chargerCfg())
    const ha = mockHa({ 'sensor.chg_total': { state: '469' } })
    accumulateChargerDaily(ha, db)
    expect(kvGet(db, 'charger_counter:main')).toBe(JSON.stringify({ date: kvDate(), total: 469 }))
    expect(chargerHistory(db, '2000-01-01', '2999-12-31', 'main')).toHaveLength(0)
    // segundo tick: +2 kWh
    accumulateChargerDaily(mockHa({ 'sensor.chg_total': { state: '471' } }), db)
    const days = chargerHistory(db, '2000-01-01', '2999-12-31', 'main')
    expect(days).toHaveLength(1)
    expect(days[0].kwh).toBe(2)
  })

  it('base legacy (charger_counter plano) la hereda main sin duplicar hueco', () => {
    const db = emptyDb()
    kvSet(db, 'charger_counter', JSON.stringify({ date: kvDate(), total: 500 }))
    _setForTests(chargerCfg())
    accumulateChargerDaily(mockHa({ 'sensor.chg_total': { state: '501' } }), db)
    const days = chargerHistory(db, '2000-01-01', '2999-12-31', 'main')
    expect(days[0].kwh).toBe(1)
    expect(kvGet(db, 'charger_counter:main')).toBe(JSON.stringify({ date: kvDate(), total: 501 }))
  })

  it('reset del contador (baja) → re-basa sin sumar el hueco', () => {
    const db = emptyDb()
    _setForTests(chargerCfg())
    accumulateChargerDaily(mockHa({ 'sensor.chg_total': { state: '900' } }), db)
    accumulateChargerDaily(mockHa({ 'sensor.chg_total': { state: '10' } }), db) // reset
    expect(chargerHistory(db, '2000-01-01', '2999-12-31', 'main')).toHaveLength(0)
    accumulateChargerDaily(mockHa({ 'sensor.chg_total': { state: '12' } }), db)
    expect(chargerHistory(db, '2000-01-01', '2999-12-31', 'main')[0].kwh).toBe(2)
  })

  it('salto imposible (>50 kWh) se ignora y re-basa', () => {
    const db = emptyDb()
    _setForTests(chargerCfg())
    accumulateChargerDaily(mockHa({ 'sensor.chg_total': { state: '100' } }), db)
    accumulateChargerDaily(mockHa({ 'sensor.chg_total': { state: '999' } }), db) // glitch
    expect(chargerHistory(db, '2000-01-01', '2999-12-31', 'main')).toHaveLength(0)
    expect(JSON.parse(kvGet(db, 'charger_counter:main')).total).toBe(999)
  })

  it('extensión inactiva → no toca nada', () => {
    const db = emptyDb()
    accumulateChargerDaily(mockHa({ 'sensor.chg_total': { state: '5' } }), db)
    expect(kvGet(db, 'charger_counter:main')).toBeNull()
  })

  it('dos cargadores acumulan por separado (daily_charger por key)', () => {
    const db = emptyDb()
    _setForTests(twoChargersCfg())
    accumulateChargerDaily(
      mockHa({ 'sensor.chg_total': { state: '100' }, 'sensor.chg2_total': { state: '5000' } }),
      db
    )
    accumulateChargerDaily(
      mockHa({ 'sensor.chg_total': { state: '103' }, 'sensor.chg2_total': { state: '5300' } }),
      db
    )
    const main = chargerHistory(db, '2000-01-01', '2999-12-31', 'main')
    const c2 = chargerHistory(db, '2000-01-01', '2999-12-31', 'c2')
    expect(main[0].kwh).toBe(3)
    expect(c2[0].kwh).toBe(3) // 300 centésimas
    expect(db.prepare('SELECT COUNT(*) AS n FROM daily_charger').get().n).toBe(2)
  })

  it('contador unavailable → tick sin efecto', () => {
    const db = emptyDb()
    _setForTests(chargerCfg())
    accumulateChargerDaily(mockHa({ 'sensor.chg_total': { state: 'unavailable' } }), db)
    expect(kvGet(db, 'charger_counter:main')).toBeNull()
  })
})

describe('dailyDeltasFromHistory', () => {
  it('deltas por día con estados no numéricos saltados', () => {
    const rows = [
      { state: '100', last_changed: '2026-08-01T10:00:00Z' },
      { state: 'unavailable', last_changed: '2026-08-01T10:01:00Z' },
      { state: '103', last_changed: '2026-08-01T11:00:00Z' },
      { state: '103', last_changed: '2026-08-02T10:00:00Z' },
      { state: '106.5', last_changed: '2026-08-02T11:00:00Z' },
    ]
    const perDay = dailyDeltasFromHistory(rows)
    expect(perDay.get('2026-08-01')).toBe(3)
    expect(perDay.get('2026-08-02')).toBe(3.5)
  })

  it('suma incrementos positivos: multi-paso, unavailable y reset intra-día', () => {
    const rows = [
      { state: '10', last_changed: '2026-08-01T10:00:00Z' },
      { state: '12.5', last_changed: '2026-08-01T10:05:00Z' },
      { state: 'unknown', last_changed: '2026-08-01T10:06:00Z' },
      { state: '14', last_changed: '2026-08-01T10:07:00Z' },
    ]
    expect(dailyDeltasFromHistory(rows, 100).get('2026-08-01')).toBeCloseTo(0.04, 5)
  })

  it('reset del contador entre días: el día previo queda solo con su base', () => {
    const rows = [
      { state: '900', last_changed: '2026-08-01T10:00:00Z' },
      { state: '5', last_changed: '2026-08-02T10:00:00Z' },
      { state: '7', last_changed: '2026-08-02T11:00:00Z' },
    ]
    const perDay = dailyDeltasFromHistory(rows)
    expect(perDay.get('2026-08-01')).toBe(0)
    expect(perDay.get('2026-08-02')).toBe(2)
  })

  it('salto imposible dentro de un día se ignora como glitch', () => {
    const rows = [
      { state: '100', last_changed: '2026-08-01T10:00:00Z' },
      { state: '1000', last_changed: '2026-08-01T10:05:00Z' },
      { state: '1001', last_changed: '2026-08-01T10:10:00Z' },
    ]
    expect(dailyDeltasFromHistory(rows).get('2026-08-01')).toBe(1)
  })

  it('divisor del contador: centésimas de kWh → kWh', () => {
    const rows = [
      { state: '1000', last_changed: '2026-08-01T10:00:00Z' },
      { state: '1250', last_changed: '2026-08-01T11:00:00Z' },
    ]
    expect(dailyDeltasFromHistory(rows, 100).get('2026-08-01')).toBeCloseTo(2.5, 5)
  })
})

describe('backfillChargerHistory', () => {
  it('backfill escribe días a NULL (hoy incluida) y marca 0 los sin carga', async () => {
    const db = emptyDb()
    // filas diarias preexistentes (las crea la consolidación solar)
    const d2 = dayOffset(-2)
    const d1 = dayOffset(-1)
    const d0 = dayOffset(0)
    for (const d of [d2, d1, d0]) {
      db.prepare('INSERT INTO daily (date) VALUES (?)').run(d)
    }
    _setForTests(chargerCfg())
    const ha = {
      historyDuringPeriod: async ({ entityId }) => {
        expect(entityId).toBe('sensor.chg_total')
        return [
          { state: '10', last_changed: `${d2}T10:00:00` },
          { state: '15', last_changed: `${d2}T12:00:00` },
          { state: '15', last_changed: `${d0}T10:00:00` },
          { state: '17', last_changed: `${d0}T11:00:00` },
        ]
      },
      statisticsDuringPeriod: async () => ({}),
    }
    const n = await backfillChargerHistory(ha, db)
    expect(n).toBe(3)
    const days = chargerHistory(db, d2, d0, 'main')
    expect(days[0].kwh).toBe(5)
    expect(days[1].kwh).toBe(0) // día sin estados dentro de la ventana → 0
    expect(days[2].kwh).toBe(2)
  })

  it('hoy sin fila daily: siembra la base del acumulador, no escribe la fila', async () => {
    const db = emptyDb()
    _setForTests(chargerCfg())
    const today = dayOffset(0)
    const ha = {
      historyDuringPeriod: async () => [
        { state: '10', last_changed: `${today}T07:00:00` },
        { state: '14', last_changed: `${today}T08:00:00` },
      ],
      statisticsDuringPeriod: async () => ({}),
    }
    const n = await backfillChargerHistory(ha, db)
    expect(n).toBe(0)
    expect(chargerHistory(db, today, today, 'main')).toHaveLength(0)
    expect(JSON.parse(kvGet(db, 'charger_counter:main')).total).toBe(10)
  })

  it('backfill de dos cargadores: huecos independientes por key', async () => {
    const db = emptyDb()
    const d1 = dayOffset(-1)
    db.prepare('INSERT INTO daily (date) VALUES (?)').run(d1)
    _setForTests(twoChargersCfg())
    const calls = []
    const ha = {
      historyDuringPeriod: async ({ entityId }) => {
        calls.push(entityId)
        if (entityId === 'sensor.chg_total') {
          return [
            { state: '100', last_changed: `${d1}T09:00:00` },
            { state: '103', last_changed: `${d1}T10:00:00` },
          ]
        }
        return [{ state: '500', last_changed: `${d1}T09:00:00` }]
      },
      statisticsDuringPeriod: async () => ({}),
    }
    const n = await backfillChargerHistory(ha, db)
    expect(n).toBe(2)
    expect(chargerHistory(db, d1, d1, 'main')[0].kwh).toBe(3)
    expect(chargerHistory(db, d1, d1, 'c2')[0].kwh).toBe(0)
    expect(calls).toEqual(['sensor.chg_total', 'sensor.chg2_total'])
  })
})

describe('energía diaria y divisores', () => {
  it('addChargerKwh suma sobre lo existente; setChargerKwhIfNull no pisa valores', () => {
    const db = emptyDb()
    setChargerKwhIfNull(db, 'main', '2026-08-01', 5, 3)
    setChargerKwhIfNull(db, 'main', '2026-08-01', 9, 9) // ya no es NULL → no pisa
    addChargerKwh(db, 'main', '2026-08-01', 0.5, 0.5)
    addChargerKwh(db, 'main', '2026-08-01', 0.25, 0.25)
    const days = chargerHistory(db, '2026-08-01', '2026-08-01', 'main')
    expect(days[0].kwh).toBeCloseTo(5.75, 5)
    expect(days[0].pvKwh).toBeCloseTo(3.75, 5)
  })

  it('addChargerKwh por cargador: filas independientes', () => {
    const db = emptyDb()
    addChargerKwh(db, 'main', '2026-08-01', 4)
    addChargerKwh(db, 'c2', '2026-08-01', 1)
    expect(chargerHistory(db, '2026-08-01', '2026-08-01', 'main')[0].kwh).toBe(4)
    expect(chargerHistory(db, '2026-08-01', '2026-08-01', 'c2')[0].kwh).toBe(1)
  })

  it('acumulador y live aplican el divisor del contador', () => {
    const db = emptyDb()
    _setForTests(chargerCfg({ energyDivisor: 100, powerUnit: 'W' }))
    const ha = mockHa({
      'sensor.chg_total': { state: '46900' },
      'sensor.chg_power': { state: '2300' },
    })
    accumulateChargerDaily(ha, db)
    accumulateChargerDaily(mockHa({ 'sensor.chg_total': { state: '47150' } }), db)
    expect(chargerHistory(db, '2000-01-01', '2999-12-31', 'main')[0].kwh).toBeCloseTo(2.5, 5)
    const live = computeChargerLive(ha)[0]
    expect(live.totalKwh).toBe(469)
    expect(live.powerKw).toBe(2.3)
  })
})

describe('atribución solar', () => {
  const TOPO = {
    inverters: [{ key: 'solis', powerId: 'sensor.pv', powerUnit: 'kW' }],
    consumption: { powerIds: ['sensor.house'], powerUnit: 'W' },
  }

  afterEach(() => _setInstallForTests(LEGACY_TOPOLOGY))

  it('pvShareNow: excedente FV parcial tras servir el resto de la casa (en casa)', () => {
    _setInstallForTests(TOPO)
    const ext = chargerCfg({ chargerInHouseMeters: true })
    const ha = mockHa({ 'sensor.pv': { state: '5' }, 'sensor.house': { state: '7000' } })
    // carga 3 kW: resto de casa = 7 - 3 = 4 → excedente 1 → 1/3
    expect(pvShareNow(ha, 3, ext, ext.carCharger.chargers[0])).toBeCloseTo(1 / 3, 5)
  })

  it('pvShareNow: circuito APARTE (legacy) → no se descuenta del consumo', () => {
    _setInstallForTests(TOPO)
    const ext = chargerCfg({ chargerInHouseMeters: false })
    const ha = mockHa({ 'sensor.pv': { state: '5' }, 'sensor.house': { state: '7000' } })
    // resto de casa = 7 (el cargador no está incluido) → excedente 0 → 0
    expect(pvShareNow(ha, 3, ext, ext.carCharger.chargers[0])).toBe(0)
  })

  it('pvShareNow: sin potencia del cargador → reparto producción/consumo', () => {
    _setInstallForTests(TOPO)
    const ext = chargerCfg()
    const ha = mockHa({ 'sensor.pv': { state: '4' }, 'sensor.house': { state: '8000' } })
    expect(pvShareNow(ha, undefined, ext, ext.carCharger.chargers[0])).toBe(0.5)
  })

  it('pvShareNow: excedente total → 1; noche → 0', () => {
    _setInstallForTests(TOPO)
    const ext = chargerCfg()
    const day = mockHa({ 'sensor.pv': { state: '5' }, 'sensor.house': { state: '2000' } })
    const night = mockHa({ 'sensor.pv': { state: '0' }, 'sensor.house': { state: '3000' } })
    expect(pvShareNow(day, 3, ext, ext.carCharger.chargers[0])).toBe(1)
    expect(pvShareNow(night, 3, ext, ext.carCharger.chargers[0])).toBe(0)
  })

  it('accumulateChargerDaily atribuye la fracción solar del delta', () => {
    _setInstallForTests(TOPO)
    const db = emptyDb()
    const ext = chargerCfg({ chargerInHouseMeters: false })
    _setForTests(ext)
    accumulateChargerDaily(
      mockHa({ 'sensor.chg_total': { state: '100' }, 'sensor.pv': { state: '0' }, 'sensor.house': { state: '0' } }),
      db
    )
    // delta 2 kWh con carga 2 kW y excedente 0 → pv 0
    accumulateChargerDaily(
      mockHa({ 'sensor.chg_total': { state: '102' }, 'sensor.chg_power': { state: '2' }, 'sensor.pv': { state: '0' }, 'sensor.house': { state: '3000' } }),
      db
    )
    const days = chargerHistory(db, '2000-01-01', '2999-12-31', 'main')
    expect(days[0].kwh).toBe(2)
    expect(days[0].pvKwh).toBe(0)
  })

  it('addChargerKwh clampa el PV al total del día', () => {
    const db = emptyDb()
    addChargerKwh(db, 'main', '2026-08-01', 1, 1)
    addChargerKwh(db, 'main', '2026-08-01', 0.2, 0.2)
    expect(chargerHistory(db, '2026-08-01', '2026-08-01', 'main')[0].pvKwh).toBeCloseTo(1.2, 5)
  })

  it('chargerPvFromCurves: unavailable del sensor NO fabrica carga fantasma', async () => {
    _setInstallForTests(TOPO)
    const ext = chargerCfg({ chargerInHouseMeters: true })
    const ha = {
      statisticsDuringPeriod: async () => ({
        'sensor.pv': [{ start: 0, mean: 5 }],
        'sensor.house': [{ start: 0, mean: 7000 }],
      }),
      historyDuringPeriod: async () => [
        { state: 'unavailable', last_changed: new Date().toISOString() },
      ],
    }
    const out = await chargerPvFromCurves(ha, new Date(Date.now() - 3600e3).toISOString(), new Date().toISOString(), ext.carCharger.chargers[0])
    expect(out.size).toBe(0)
  })

  it('chargerPvFromCurves: integra min(cargador, excedente) por buckets de 5 min', async () => {
    _setInstallForTests(TOPO)
    const ext = chargerCfg({ chargerInHouseMeters: true })
    const BUCKET = 5 * 60e3
    const base = Math.floor(Date.now() / BUCKET) * BUCKET
    const ha = {
      statisticsDuringPeriod: async () => ({
        'sensor.pv': [
          { start: base - BUCKET, mean: 5 },
          { start: base, mean: 5 },
        ],
        'sensor.house': [
          { start: base - BUCKET, mean: 3000 },
          { start: base, mean: 3000 },
        ],
      }),
      historyDuringPeriod: async () => [
        { state: '2', last_changed: new Date(base - BUCKET).toISOString() },
        { state: '4', last_changed: new Date(base).toISOString() },
      ],
    }
    const out = await chargerPvFromCurves(ha, new Date(base - BUCKET).toISOString(), new Date(base + 1).toISOString(), ext.carCharger.chargers[0])
    const total = [...out.values()].reduce((a, b) => a + b, 0)
    // bucket 1: carga 2 kW, resto de casa 1 kW, excedente 4 kW -> todo solar
    // bucket 2: carga 4 kW, resto de casa 0 (3-4), excedente 5 kW -> todo solar
    // (2 kW + 4 kW) x 5 min = 0.5 kWh, todo de origen solar
    expect(total).toBeCloseTo(0.5, 2)
  })

  it('chargerHistory estima el PV por balance diario cuando falta (pv NULL)', () => {
    const db = emptyDb()
    db.prepare("INSERT INTO daily (date, production_kwh, consumption_kwh) VALUES ('2026-08-01', 10, 8)").run()
    setChargerKwhIfNull(db, 'main', '2026-08-01', 3, null)
    _setForTests(chargerCfg({ chargerInHouseMeters: false }))
    const days = chargerHistory(db, '2026-08-01', '2026-08-01', 'main')
    // balance: min(3, max(0, 10 - 8)) = 2
    expect(days[0].pvKwh).toBe(2)
  })
})

// ── Módulo BYD (#100) ────────────────────────────────────────────────────────
import { bydActive, bydEntities, computeBydLive, bydAction, LEGACY_BYD } from '../src/extensions.js'

function bydExt(overrides = {}) {
  return normalizeExtensions({
    enabled: true,
    carCharger: { enabled: false },
    byd: {
      enabled: true,
      name: 'Test BYD',
      socId: 'sensor.byd_soc',
      rangeId: 'sensor.byd_range',
      odometerId: 'sensor.byd_odo',
      batteryPowerId: 'sensor.byd_power',
      chargingId: 'binary_sensor.byd_charging',
      pluggedId: 'binary_sensor.byd_plug',
      onlineId: 'binary_sensor.byd_online',
      lockedId: 'binary_sensor.byd_locked',
      doorsId: 'binary_sensor.byd_doors',
      windowsId: 'binary_sensor.byd_windows',
      sentryId: 'binary_sensor.byd_sentry',
      cabinTempId: 'sensor.byd_cabin',
      exteriorTempId: 'sensor.byd_out',
      tireFlId: 'sensor.byd_fl',
      tireFrId: 'sensor.byd_fr',
      tireRlId: 'sensor.byd_rl',
      tireRrId: 'sensor.byd_rr',
      locationId: 'device_tracker.byd_loc',
      gpsAgeId: 'sensor.byd_gps_ts',
      lastUpdateId: 'sensor.byd_last_ts',
      startChargeId: 'button.byd_start',
      stopChargeId: 'button.byd_stop',
      forcePollId: 'button.byd_poll',
      chargeToFullId: 'switch.byd_ctf',
      scheduleEnabledId: 'switch.byd_sched',
      scheduleStartId: 'time.byd_start',
      scheduleEndId: 'time.byd_end',
      repeatDailyId: 'switch.byd_repeat',
      lockId: 'lock.byd_lock',
      acSwitchId: 'switch.byd_ac',
      flashLightsId: 'button.byd_flash',
      findCarId: 'button.byd_find',
      closeWindowsId: 'button.byd_windows',
      remainingHoursId: 'sensor.byd_rem_h',
      remainingMinutesId: 'sensor.byd_rem_m',
      consumptionRecentId: 'sensor.byd_cons_recent',
      consumption50Id: 'sensor.byd_cons_50',
      consumptionLifetimeId: 'sensor.byd_cons_life',
      consumptionTodayId: 'sensor.byd_cons_today',
      ...overrides,
    },
  })
}

function bydHa(states = {}) {
  return mockHa({
    'sensor.byd_soc': { state: '97' },
    'sensor.byd_range': { state: '410' },
    'sensor.byd_odo': { state: '13508' },
    'sensor.byd_power': { state: '6800' },
    'binary_sensor.byd_charging': { state: 'on' },
    'binary_sensor.byd_plug': { state: 'on' },
    'binary_sensor.byd_online': { state: 'on' },
    'binary_sensor.byd_locked': { state: 'on' },
    'binary_sensor.byd_doors': { state: 'off' },
    'binary_sensor.byd_windows': { state: 'off' },
    'binary_sensor.byd_sentry': { state: 'on' },
    'sensor.byd_cabin': { state: '28' },
    'sensor.byd_out': { state: '31' },
    'sensor.byd_fl': { state: '232' },
    'sensor.byd_fr': { state: '232' },
    'sensor.byd_rl': { state: '230' },
    'sensor.byd_rr': { state: '232' },
    'device_tracker.byd_loc': {
      state: 'home',
      attributes: { latitude: 39.559, longitude: -0.512 },
    },
    'lock.byd_lock': { state: 'unlocked' },
    'switch.byd_ac': { state: 'off' },
    'sensor.byd_rem_h': { state: '1' },
    'sensor.byd_rem_m': { state: '25' },
    'sensor.byd_cons_recent': { state: '14.4' },
    'sensor.byd_cons_50': { state: '14.4' },
    'sensor.byd_cons_life': { state: '17.3' },
    'sensor.byd_cons_today': { state: '14.4' },
    ...states,
  })
}

describe('bydActive / bydEntities', () => {
  it('inactivo por defecto; entidades solo con el módulo encendido', () => {
    expect(bydActive(GENERIC_EXTENSIONS)).toBe(false)
    const ext = bydExt()
    expect(bydActive(ext)).toBe(true)
    expect(bydEntities(ext)).toContain('sensor.byd_soc')
    expect(bydEntities(bydExt({ enabled: false }))).toEqual([])
  })
})

describe('computeBydLive', () => {
  it('snapshot completo: soc, kW, binarios, neumáticos, ubicación', () => {
    const ext = bydExt()
    _setForTests(ext)
    const live = computeBydLive(bydHa(), ext)
    expect(live.soc).toBe(97)
    expect(live.rangeKm).toBe(410)
    expect(live.powerKw).toBe(6.8)
    expect(live.charging).toBe(true)
    expect(live.plugged).toBe(true)
    expect(live.locked).toBe(true)
    expect(live.doorsOpen).toBe(false)
    expect(live.tires).toEqual({ fl: 232, fr: 232, rl: 230, rr: 232 })
    expect(live.location).toBe('home')
  })

  it('unavailable/unknown → undefined (no 0), y W → kW', () => {
    const ext = bydExt()
    _setForTests(ext)
    const live = computeBydLive(bydHa({ 'sensor.byd_soc': { state: 'unavailable' }, 'sensor.byd_power': { state: 'unknown' } }), ext)
    expect(live.soc).toBeUndefined()
    expect(live.powerKw).toBeUndefined()
    expect(live.online).toBe(true) // binary sigue presente
  })

  it('timestamps ISO → antigüedad en minutos; offline sin datos', () => {
    const ext = bydExt()
    _setForTests(ext)
    const ts = new Date(Date.now() - 5 * 60000).toISOString()
    const live = computeBydLive(bydHa({ 'sensor.byd_gps_ts': { state: ts }, 'binary_sensor.byd_online': { state: 'off' } }), ext)
    expect(live.gpsAgeMin).toBe(5)
    expect(live.online).toBe(false)
  })

  it('sin el módulo activo devuelve undefined', () => {
    _setForTests(GENERIC_EXTENSIONS)
    expect(computeBydLive(bydHa(), GENERIC_EXTENSIONS)).toBeUndefined()
  })
})

function recordingHa() {
  const calls = []
  return {
    calls,
    call: async (msg) => {
      calls.push(msg)
      return {}
    },
  }
}

describe('bydAction (whitelist)', () => {
  it('start_charging → button.press sobre la entidad configurada', async () => {
    const ha = recordingHa()
    await bydAction(ha, bydExt(), 'start_charging')
    expect(ha.calls[0]).toEqual({
      type: 'call_service',
      domain: 'button',
      service: 'press',
      target: { entity_id: 'button.byd_start' },
    })
  })

  it('charge_to_full → switch turn_on/off según value', async () => {
    const ha = recordingHa()
    await bydAction(ha, bydExt(), 'charge_to_full', true)
    expect(ha.calls[0].service).toBe('turn_on')
    await bydAction(ha, bydExt(), 'charge_to_full', false)
    expect(ha.calls[1].service).toBe('turn_off')
  })

  it('schedule_start valida HH:MM y usa time.set_value', async () => {
    const ha = recordingHa()
    await bydAction(ha, bydExt(), 'schedule_start', '08:30')
    expect(ha.calls[0]).toEqual({
      type: 'call_service',
      domain: 'time',
      service: 'set_value',
      target: { entity_id: 'time.byd_start' },
      time: '08:30',
    })
    await expect(bydAction(ha, bydExt(), 'schedule_start', '99:99')).rejects.toThrow('unknown_action')
    await expect(bydAction(ha, bydExt(), 'schedule_start', 'nope')).rejects.toThrow('unknown_action')
  })

  it('acción desconocida o módulo apagado → error', async () => {
    const ha = recordingHa()
    await expect(bydAction(ha, bydExt(), 'open_frunk')).rejects.toThrow('unknown_action')
    await expect(bydAction(ha, bydExt({ enabled: false }), 'start_charging')).rejects.toThrow('byd_disabled')
  })
})

describe('normalizeExtensions: módulo byd', () => {
  it('campos no-string revierten a string vacío; legacy trae los IDs reales', () => {
    const ext = normalizeExtensions({
      enabled: true,
      byd: { enabled: true, name: 'Mi BYD', socId: 42, rangeId: null },
    })
    expect(ext.byd.enabled).toBe(true)
    expect(ext.byd.name).toBe('Mi BYD')
    expect(ext.byd.socId).toBe('')
    expect(ext.byd.rangeId).toBe('')
    expect(LEGACY_BYD.socId).toBe('sensor.byd_atto_3_nivel_de_bateria')
    expect(LEGACY_EXTENSIONS.byd).toBe(LEGACY_BYD)
  })

  it('el schema zod acepta el módulo byd completo', () => {
    const parsed = extensionsSchema.safeParse({
      enabled: true,
      carCharger: { enabled: false },
      byd: { enabled: true, name: 'X', socId: 'sensor.x', unknownField: true },
    })
    expect(parsed.success).toBe(true)
    expect(parsed.data.byd.socId).toBe('sensor.x')
    expect(parsed.data.byd.enabled).toBe(true)
  })
})


describe('computeBydLive v2 (geo, control, consumo, restante)', () => {
  it('lat/lon desde attributes del device_tracker', () => {
    const ext = bydExt()
    _setForTests(ext)
    const live = computeBydLive(bydHa(), ext)
    expect(live.lat).toBeCloseTo(39.559, 2)
    expect(live.lon).toBeCloseTo(-0.512, 2)
  })

  it('device_tracker sin coordenadas → lat/lon undefined (no rompe)', () => {
    const ext = bydExt()
    _setForTests(ext)
    const live = computeBydLive(bydHa({ 'device_tracker.byd_loc': { state: 'home', attributes: {} } }), ext)
    expect(live.lat).toBeUndefined()
    expect(live.lon).toBeUndefined()
  })

  it('lock/ac normalizados y tiempo restante h+min combinado', () => {
    const ext = bydExt()
    _setForTests(ext)
    const live = computeBydLive(bydHa(), ext)
    expect(live.lockUnlocked).toBe(true)
    expect(live.acOn).toBe(false)
    expect(live.remainingMin).toBe(85)
    expect(live.consumptionRecent).toBe(14.4)
    expect(live.consumptionLifetime).toBe(17.3)
  })
})

describe('bydAction v2 (lock/ac/luces/pitido/ventanas)', () => {
  it('lock/unlock → lock.lock | lock.unlock', async () => {
    const ha = recordingHa()
    await bydAction(ha, bydExt(), 'lock')
    expect(ha.calls[0]).toEqual({
      type: 'call_service',
      domain: 'lock',
      service: 'lock',
      target: { entity_id: 'lock.byd_lock' },
    })
    await bydAction(ha, bydExt(), 'unlock')
    expect(ha.calls[1].service).toBe('unlock')
  })

  it('ac → switch turn_on/off', async () => {
    const ha = recordingHa()
    await bydAction(ha, bydExt(), 'ac', true)
    expect(ha.calls[0].service).toBe('turn_on')
    await bydAction(ha, bydExt(), 'ac', false)
    expect(ha.calls[1].service).toBe('turn_off')
    await expect(bydAction(ha, bydExt(), 'ac', 'si')).rejects.toThrow('unknown_action')
  })

  it('flash_lights / find_car / close_windows → button.press', async () => {
    const ha = recordingHa()
    await bydAction(ha, bydExt(), 'flash_lights')
    await bydAction(ha, bydExt(), 'find_car')
    await bydAction(ha, bydExt(), 'close_windows')
    for (const c of ha.calls) {
      expect(c.domain).toBe('button')
      expect(c.service).toBe('press')
    }
    expect(ha.calls.map((c) => c.target.entity_id)).toEqual(['button.byd_flash', 'button.byd_find', 'button.byd_windows'])
  })

  it('sin la entidad configurada → unknown_action (botón oculto)', async () => {
    const ha = recordingHa()
    await expect(bydAction(ha, bydExt({ findCarId: '' }), 'find_car')).rejects.toThrow('unknown_action')
  })
})

describe('chargerDayCurve', () => {
  const midnight = new Date()
  midnight.setHours(0, 0, 0, 0)

  it('hoy: la curva termina en el minuto actual, no en medianoche', async () => {
    const nowMin = Math.floor((Date.now() - midnight.getTime()) / 60000)
    const res = await chargerDayCurve(
      { historyDuringPeriod: async () => [{ state: '2', last_changed: new Date(midnight.getTime()).toISOString() }] },
      undefined,
      chargerCfg(),
      chargerCfg().carCharger.chargers[0]
    )
    expect(res.points.length).toBe(Math.ceil(nowMin / 5))
    expect(res.points[0].kw).toBe(2)
  })

  it('día pasado: curva completa de 24 h (288 buckets)', async () => {
    const res = await chargerDayCurve(
      { historyDuringPeriod: async () => [{ state: '2', last_changed: '2026-08-01T00:00:00' }] },
      '2026-08-01',
      chargerCfg(),
      chargerCfg().carCharger.chargers[0]
    )
    expect(res.points).toHaveLength(288)
    expect(res.points[0].label).toBe('00:00')
    expect(res.points[res.points.length - 1].label).toBe('23:55')
  })

  it('curvas por cargador: cache keys separados (no se mezclan)', async () => {
    const cfg = twoChargersCfg()
    const cMain = getChargerByKey(cfg, 'main')
    const c2 = getChargerByKey(cfg, 'c2')
    const ha = {
      historyDuringPeriod: async ({ entityId }) => [
        { state: entityId === 'sensor.chg_power' ? '2' : '1380', last_changed: '2026-08-01T00:00:00' },
      ],
    }
    const r1 = await chargerDayCurve(ha, '2026-08-01', cfg, cMain)
    const r2 = await chargerDayCurve(ha, '2026-08-01', cfg, c2)
    expect(r1.points[0].kw).toBe(2)
    expect(r2.points[0].kw).toBe(1.38) // W → kW
  })

  it('sin potencia configurada → curva vacía', async () => {
    const cfg = chargerCfg({ powerId: '' })
    const res = await chargerDayCurve(mockHa({}), '2026-07-01', cfg, cfg.carCharger.chargers[0])
    expect(res.points).toEqual([])
  })
})

describe('getChargerByKey', () => {
  it('devuelve el cargador pedido y null si no existe', () => {
    const cfg = twoChargersCfg()
    expect(getChargerByKey(cfg, 'c2').name).toBe('Portátil')
    expect(getChargerByKey(cfg, 'nope')).toBeNull()
  })
})

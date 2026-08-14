// extensions.test.js — marco de extensiones (issue #94): resolución
// kv>legacy>generic, live del cargador, acumulación diaria por deltas del
// contador y backfill desde el historial del recorder.
import { describe, it, expect, afterEach } from 'vitest'
import Database from 'better-sqlite3'
import { z } from 'zod'
import { initSchema, kvSet } from '../src/db.js'
import { createSchemas } from '../../shared/schemas.js'
import { _setForTests as _setInstallForTests, LEGACY_TOPOLOGY } from '../src/install.js'
import {
  resolveExtensions,
  normalizeExtensions,
  chargerActive,
  chargerEntities,
  computeChargerLive,
  accumulateChargerDaily,
  pvShareNow,
  dailyDeltasFromHistory,
  chargerPvFromCurves,
  backfillChargerHistory,
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

/** Topología de test del cargador: entidades cortas y estados. */
function chargerExt(overrides = {}) {
  return normalizeExtensions({
    enabled: true,
    carCharger: {
      enabled: true,
      powerId: 'sensor.chg_power',
      powerUnit: 'kW',
      energyTotalId: 'sensor.chg_total',
      energySessionId: 'sensor.chg_session',
      stateId: 'sensor.chg_state',
      tempId: 'sensor.chg_temp',
      switchId: 'switch.chg',
      chargingStates: ['charging'],
      connectedStates: ['connected', 'charging'],
      ...overrides,
    },
  })
}

function mockHa(states) {
  return {
    connected: true,
    getState: (id) => states[id],
  }
}

describe('resolveExtensions', () => {
  it('instalación con datos SIN kv → perfil LEGACY (entidades prefabricadas, apagado)', () => {
    const ext = resolveExtensions(dbWithDaily())
    expect(ext.enabled).toBe(false)
    expect(ext.carCharger.enabled).toBe(false)
    expect(ext.carCharger.powerId).toBe(LEGACY_EXTENSIONS.carCharger.powerId)
  })

  it('instalación nueva SIN kv → perfil GENERIC', () => {
    const ext = resolveExtensions(emptyDb())
    expect(ext.carCharger.powerId).toBe(GENERIC_EXTENSIONS.carCharger.powerId)
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
    expect(ext.carCharger.powerId).toBe('sensor.mi_cargador')
    // el resto de campos vienen del perfil base
    expect(ext.carCharger.powerUnit).toBe('kW')
  })

  it('kv corrupto → fallback al perfil base', () => {
    const db = dbWithDaily()
    kvSet(db, 'extensions_config', '{no json')
    const ext = resolveExtensions(db)
    expect(ext.carCharger.powerId).toBe(LEGACY_EXTENSIONS.carCharger.powerId)
  })

  it('el schema zod valida la forma del PUT', () => {
    const ok = extensionsSchema.safeParse({ enabled: true, carCharger: { enabled: true, powerId: 'sensor.x' } })
    expect(ok.success).toBe(true)
    const bad = extensionsSchema.safeParse({ carCharger: { powerUnit: 'MW' } })
    expect(bad.success).toBe(false)
  })
})

describe('normalizeExtensions', () => {
  it('coerce tipos inesperados a valores seguros', () => {
    const ext = normalizeExtensions({
      enabled: 'si',
      carCharger: { enabled: 1, name: '  ', powerId: 42, chargingStates: 'charging', powerUnit: 'W' },
    })
    expect(ext.enabled).toBe(false)
    expect(ext.carCharger.enabled).toBe(false)
    expect(ext.carCharger.name).toBe(GENERIC_EXTENSIONS.carCharger.name)
    expect(ext.carCharger.powerId).toBe('')
    expect(ext.carCharger.chargingStates).toEqual([])
    expect(ext.carCharger.powerUnit).toBe('W')
  })
})

describe('chargerActive / chargerEntities', () => {
  it('marco apagado → sin entidades aunque el cargador esté encendido', () => {
    const ext = normalizeExtensions({ enabled: false, carCharger: { enabled: true } })
    expect(chargerActive(ext)).toBe(false)
    expect(chargerEntities(ext)).toEqual([])
  })

  it('activo → entidades deduplicadas sin huecos', () => {
    const ext = chargerExt({ tempId: '', switchId: '', energySessionId: 'sensor.chg_power' })
    expect(chargerEntities(ext)).toEqual(['sensor.chg_power', 'sensor.chg_total', 'sensor.chg_state'])
  })
})

describe('computeChargerLive', () => {
  it('extensión inactiva → undefined (la clave no viaja en el SSE)', () => {
    expect(computeChargerLive(mockHa({}), GENERIC_EXTENSIONS)).toBeUndefined()
  })

  it('cargando por estado + kW directos', () => {
    _setForTests(chargerExt())
    const live = computeChargerLive(
      mockHa({
        'sensor.chg_power': { state: '2.3' },
        'sensor.chg_state': { state: 'charging' },
        'sensor.chg_session': { state: '12.5' },
        'sensor.chg_total': { state: '469' },
        'sensor.chg_temp': { state: '42' },
        'switch.chg': { state: 'on' },
      })
    )
    expect(live.charging).toBe(true)
    expect(live.connected).toBe(true)
    expect(live.powerKw).toBe(2.3)
    expect(live.sessionKwh).toBe(12.5)
    expect(live.totalKwh).toBe(469)
    expect(live.tempC).toBe(42)
    expect(live.switchOn).toBe(true)
  })

  it('fallback: potencia significativa cuenta como cargando sin estado', () => {
    _setForTests(chargerExt())
    const live = computeChargerLive(mockHa({ 'sensor.chg_power': { state: '3.1' } }))
    expect(live.charging).toBe(true)
    expect(live.connected).toBe(true)
    expect(live.state).toBeUndefined()
  })

  it('conectado sin cargar (enchufado en espera)', () => {
    _setForTests(chargerExt())
    const live = computeChargerLive(
      mockHa({ 'sensor.chg_power': { state: '0' }, 'sensor.chg_state': { state: 'connected' } })
    )
    expect(live.charging).toBe(false)
    expect(live.connected).toBe(true)
  })

  it('unidades W → kW y unavailable → undefined por campo', () => {
    _setForTests(chargerExt({ powerUnit: 'W' }))
    const live = computeChargerLive(
      mockHa({
        'sensor.chg_power': { state: '2300' },
        'sensor.chg_temp': { state: 'unavailable' },
        'sensor.chg_state': { state: 'unknown' },
      })
    )
    expect(live.powerKw).toBe(2.3)
    expect(live.tempC).toBeUndefined()
    expect(live.state).toBeUndefined()
    expect(live.charging).toBe(true) // por potencia
  })
})

describe('accumulateChargerDaily', () => {
  it('primer tick solo siembra la base; los siguientes acumulan el delta', () => {
    const db = emptyDb()
    _setForTests(chargerExt())
    const ha = mockHa({ 'sensor.chg_total': { state: '100' } })
    accumulateChargerDaily(ha, db)
    // sin fila todavía: solo kv sembrado
    expect(chargerHistory(db, '2000-01-01', '2999-12-31')).toEqual([])
    // sube 1.5 kWh
    ha.getState = () => ({ state: '101.5' })
    accumulateChargerDaily(ha, db)
    const today = chargerHistory(db, '2000-01-01', '2999-12-31')[0]
    expect(today.kwh).toBe(1.5)
  })

  it('reset del contador (baja) → re-basa sin sumar el hueco', () => {
    const db = emptyDb()
    _setForTests(chargerExt())
    let total = 100
    const ha = mockHa({})
    ha.getState = () => ({ state: String(total) })
    accumulateChargerDaily(ha, db)
    total = 101
    accumulateChargerDaily(ha, db)
    total = 5 // el cargador se resetea: 96 kWh "perdidos" NO se suman
    accumulateChargerDaily(ha, db)
    total = 6
    accumulateChargerDaily(ha, db)
    const days = chargerHistory(db, '2000-01-01', '2999-12-31')
    // 1 kWh pre-reset + 1 kWh post-reset (delta legítimo de la nueva base)
    expect(days[0].kwh).toBe(2)
  })

  it('salto imposible (>50 kWh) se ignora y re-basa', () => {
    const db = emptyDb()
    _setForTests(chargerExt())
    let total = 100
    const ha = mockHa({})
    ha.getState = () => ({ state: String(total) })
    accumulateChargerDaily(ha, db)
    total = 200 // glitch
    accumulateChargerDaily(ha, db)
    expect(chargerHistory(db, '2000-01-01', '2999-12-31')).toEqual([])
    total = 201
    accumulateChargerDaily(ha, db)
    expect(chargerHistory(db, '2000-01-01', '2999-12-31')[0].kwh).toBe(1)
  })

  it('extensión inactiva → no toca nada', () => {
    const db = emptyDb()
    _setForTests(GENERIC_EXTENSIONS)
    accumulateChargerDaily(mockHa({ 'sensor.chg_total': { state: '100' } }), db)
    expect(chargerHistory(db, '2000-01-01', '2999-12-31')).toEqual([])
  })
})

describe('dailyDeltasFromHistory / backfillChargerHistory', () => {
  it('deltas por día con estados no numéricos saltados', () => {
    // Fechas LOCALES (lección TZ: los runners CI van en UTC): el agrupado
    // por día del módulo usa la hora local del servidor.
    const L = (y, m, d, h, min = 0) => new Date(y, m - 1, d, h, min).toISOString()
    const rows = [
      { state: '100', last_changed: L(2026, 8, 13, 0, 0) },
      { state: '102', last_changed: L(2026, 8, 13, 7, 0) },
      { state: 'unavailable', last_changed: L(2026, 8, 13, 12, 0) },
      { state: '105', last_changed: L(2026, 8, 13, 20, 0) },
      { state: '106', last_changed: L(2026, 8, 14, 8, 0) },
      { state: '108', last_changed: L(2026, 8, 14, 18, 0) },
    ]
    const perDay = dailyDeltasFromHistory(rows)
    expect(perDay.size).toBe(2)
    expect(perDay.get('2026-08-13')).toBe(5)
    expect(perDay.get('2026-08-14')).toBe(2)
  })

  it('suma incrementos positivos: multi-paso, unavailable y reset intra-día', () => {
    const L = (y, m, d, h, min = 0) => new Date(y, m - 1, d, h, min).toISOString()
    const rows = [
      { state: '100', last_changed: L(2026, 8, 12, 8) },
      { state: '145', last_changed: L(2026, 8, 12, 10) },
      { state: 'unavailable', last_changed: L(2026, 8, 12, 11) },
      { state: '190', last_changed: L(2026, 8, 12, 12) },
      { state: '5', last_changed: L(2026, 8, 12, 20) }, // reset del equipo
      { state: '25', last_changed: L(2026, 8, 12, 21) },
      { state: '25', last_changed: L(2026, 8, 13, 9) },
      { state: '60', last_changed: L(2026, 8, 13, 15) },
    ]
    const perDay = dailyDeltasFromHistory(rows)
    expect(perDay.get('2026-08-12')).toBe(110) // 45 + 45 + (reset no resta) 20
    expect(perDay.get('2026-08-13')).toBe(35)
  })

  it('divisor del contador: centésimas de kWh → kWh', () => {
    const L = (y, m, d, h) => new Date(y, m - 1, d, h).toISOString()
    const rows = [
      { state: '0', last_changed: L(2026, 8, 11, 14) },
      { state: '118', last_changed: L(2026, 8, 11, 15) },
      { state: '369', last_changed: L(2026, 8, 11, 16) },
    ]
    const perDay = dailyDeltasFromHistory(rows, 100)
    expect(perDay.get('2026-08-11')).toBe(3.69)
  })

  it('backfill escribe días a NULL (hoy incluida) y marca 0 los sin carga', async () => {
    const db = emptyDb()
    const t = new Date()
    const iso = (offsetDays) => {
      const d = new Date(t.getFullYear(), t.getMonth(), t.getDate() - offsetDays)
      return {
        key: d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0'),
        at: (h) => new Date(d.getFullYear(), d.getMonth(), d.getDate(), h).toISOString(),
      }
    }
    const d2 = iso(2) // preset
    const d1 = iso(1) // NULL con carga en el recorder
    const d0 = iso(0) // HOY NULL sin estados → 0
    db.prepare(
      'INSERT INTO daily (date, production_kwh, ext_charger_kwh) VALUES (?, 10, 7.0), (?, 10, NULL), (?, 10, NULL)'
    ).run(d2.key, d1.key, d0.key)
    _setForTests(chargerExt())
    const rows = [
      { state: '100', last_changed: d2.at(12) },
      { state: '103', last_changed: d2.at(20) },
      { state: '103', last_changed: d1.at(10) },
      { state: '107', last_changed: d1.at(20) },
    ]
    const ha = { historyDuringPeriod: async () => rows }
    const n = await backfillChargerHistory(ha, db)
    expect(n).toBe(2)
    const days = chargerHistory(db, d1.key, d0.key)
    expect(days[0].kwh).toBe(4) // NULL → delta del recorder
    expect(days[1].kwh).toBe(0) // hoy sin estados → 0 (no NULL)
  })

  it('hoy sin fila daily: siembra la base del acumulador, no escribe la fila', async () => {
    const db = emptyDb()
    const t = new Date()
    const iso = (offsetDays) => {
      const d = new Date(t.getFullYear(), t.getMonth(), t.getDate() - offsetDays)
      return {
        key: d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0'),
        at: (h) => new Date(d.getFullYear(), d.getMonth(), d.getDate(), h).toISOString(),
      }
    }
    const d1 = iso(1)
    const d0 = iso(0)
    db.prepare('INSERT INTO daily (date, production_kwh, ext_charger_kwh) VALUES (?, 10, NULL)').run(d1.key)
    _setForTests(chargerExt({ energyDivisor: 100 }))
    const rows = [
      { state: '100', last_changed: d1.at(12) },
      { state: '300', last_changed: d1.at(20) },
      { state: '14', last_changed: d0.at(14) }, // primer estado de HOY
      { state: '469', last_changed: d0.at(16) },
    ]
    const ha = { historyDuringPeriod: async () => rows }
    const n = await backfillChargerHistory(ha, db)
    expect(n).toBe(1) // solo ayer (2 kWh)
    expect(chargerHistory(db, d0.key, d0.key)).toEqual([]) // hoy sin fila
    // base sembrada con el primer estado de hoy (14/100 kWh)
    const { kvGet } = await import('../src/db.js')
    expect(JSON.parse(kvGet(db, 'charger_counter'))).toEqual({ date: d0.key, total: 0.14 })
    // el acumulador aplica el delta de hoy y crea la fila
    const haLive = { getState: (id) => ({ 'sensor.chg_total': { state: '469' } })[id] }
    accumulateChargerDaily(haLive, db)
    expect(chargerHistory(db, d0.key, d0.key)[0].kwh).toBe(4.55)
  })

  it('acumulador y live aplican el divisor del contador', () => {
    const db = emptyDb()
    _setForTests(chargerExt({ energyDivisor: 100, powerUnit: 'W', powerId: 'sensor.chg_power' }))
    let total = 36900 // unidades crudas = 369 kWh
    const ha = mockHa({})
    ha.getState = (id) => ({ 'sensor.chg_total': { state: String(total) }, 'sensor.chg_power': { state: '2605' } })[id]
    accumulateChargerDaily(ha, db)
    total = 36955 // +55 unidades = 0,55 kWh
    accumulateChargerDaily(ha, db)
    const day = chargerHistory(db, '2000-01-01', '2999-12-31')[0]
    expect(day.kwh).toBe(0.55)
    const live = computeChargerLive(ha)
    expect(live.totalKwh).toBe(369.55)
    expect(live.powerKw).toBe(2.605)
  })

  it('addChargerKwh suma sobre lo existente; setChargerKwhIfNull no pisa ni crea filas', () => {
    const db = emptyDb()
    addChargerKwh(db, '2026-08-12', 2)
    addChargerKwh(db, '2026-08-12', 1.5)
    setChargerKwhIfNull(db, '2026-08-12', 99)
    expect(chargerHistory(db, '2026-08-12', '2026-08-12')[0].kwh).toBe(3.5)
    setChargerKwhIfNull(db, '2026-08-14', 4) // sin fila → no crea (solo backfill de filas)
    expect(chargerHistory(db, '2026-08-14', '2026-08-14')).toEqual([])
  })
})

describe('atribución solar (iteración 4)', () => {
  // Topología simple: 1 inversor kW + consumo W (incluye el circuito del cargador).
  const TOPO = {
    ...LEGACY_TOPOLOGY,
    inverters: [
      { ...LEGACY_TOPOLOGY.inverters[0], key: 'inv1', name: 'Inv', powerId: 'sensor.prod', powerUnit: 'kW' },
    ],
    consumption: {
      ...LEGACY_TOPOLOGY.consumption,
      powerIds: ['sensor.cons'],
      powerUnit: 'W',
    },
  }

  function haWith(prod, consW, chg) {
    const map = {
      'sensor.prod': { state: String(prod) },
      'sensor.cons': { state: String(consW) },
      'sensor.chg_total': { state: '100' },
    }
    if (chg !== undefined) map['sensor.chg_power'] = { state: String(chg) }
    return { connected: true, getState: (id) => map[id] }
  }

  it('pvShareNow: excedente FV parcial tras servir el resto de la casa', () => {
    _setInstallForTests(TOPO)
    // producción 5 kW, consumo 6 kW con cargador a 3 → resto casa 3 → solar al cargador 2 → 2/3
    expect(pvShareNow(haWith(5, 6000, 3), 3)).toBeCloseTo(2 / 3, 5)
  })

  it('pvShareNow: cargador en circuito APARTE (legacy) → no se descuenta del consumo', () => {
    _setInstallForTests(TOPO)
    const ext = chargerExt({ chargerInHouseMeters: false })
    // producción 5, casa 3, cargador 3: sin descontar → surplus 2 → 2/3
    // (con descontar sería 5-0=5 → 1, demasiado optimista)
    expect(pvShareNow(haWith(5, 3000, 3), 3, ext)).toBeCloseTo(2 / 3, 5)
  })

  it('pvShareNow: sin cargador, reparto producción/consumo', () => {
    _setInstallForTests(TOPO)
    expect(pvShareNow(haWith(4, 8000, undefined), undefined)).toBeCloseTo(0.5, 5)
  })

  it('pvShareNow: excedente total → 1; noche → 0', () => {
    _setInstallForTests(TOPO)
    expect(pvShareNow(haWith(8, 6000, 3), 3)).toBe(1)
    expect(pvShareNow(haWith(0, 6000, 3), 3)).toBe(0)
  })

  it('accumulateChargerDaily escribe la fracción solar del delta', () => {
    const db = emptyDb()
    _setForTests(chargerExt())
    _setInstallForTests(TOPO)
    const ha = haWith(5, 6000, 3)
    accumulateChargerDaily(ha, db) // siembra base 100
    ha.getState = (id) => ({ 'sensor.prod': { state: '5' }, 'sensor.cons': { state: '6000' }, 'sensor.chg_total': { state: '100.9' }, 'sensor.chg_power': { state: '3' } })[id]
    accumulateChargerDaily(ha, db)
    const row = chargerHistory(db, '2000-01-01', '2999-12-31')[0]
    expect(row.kwh).toBe(0.9)
    expect(row.pvKwh).toBeCloseTo(0.9 * (2 / 3), 2)
  })

  it('addChargerKwh clampa el PV al total del día', () => {
    const db = emptyDb()
    addChargerKwh(db, '2026-08-12', 5, 9) // pv > kwh → 5
    expect(chargerHistory(db, '2026-08-12', '2026-08-12')[0].pvKwh).toBe(5)
    addChargerKwh(db, '2026-08-12', 2, 1)
    const row = chargerHistory(db, '2026-08-12', '2026-08-12')[0]
    expect(row.kwh).toBe(7)
    expect(row.pvKwh).toBe(6) // 5 + 1, sin superar 7
  })

  it('chargerPvFromCurves: unavailable del sensor de potencia NO fabrica carga fantasma', async () => {
    _setInstallForTests(TOPO)
    _setForTests(chargerExt({ powerId: 'sensor.chg_power', chargerInHouseMeters: false }))
    const start = new Date(2026, 7, 11, 15, 0, 0)
    const stats = {
      'sensor.prod': [0, 1, 2].map((i) => ({ start: new Date(start.getTime() + i * 300000).toISOString(), mean: 5 })),
      'sensor.cons': [0, 1, 2].map((i) => ({ start: new Date(start.getTime() + i * 300000).toISOString(), mean: 500 })),
    }
    // Carga real en el bucket 0; después el sensor cae a unavailable (dropout):
    // el hold-last del viejo algoritmo seguía "cargando" 2 kW con el sol alto.
    const powerRows = [
      { state: '2', last_changed: new Date(start.getTime() - 60000).toISOString() },
      { state: 'unavailable', last_changed: new Date(start.getTime() + 240000).toISOString() },
    ]
    const ha = {
      statisticsDuringPeriod: async () => stats,
      historyDuringPeriod: async () => powerRows,
    }
    const pv = await chargerPvFromCurves(ha, start.toISOString(), new Date(start.getTime() + 900000).toISOString())
    // solo el bucket 0 con carga real: min(2, 5-0.5)=2 kW × 5 min ≈ 0.167 kWh
    expect(pv.get('2026-08-11')).toBeCloseTo(2 * (5 / 60), 2)
  })

  it('chargerPvFromCurves: integra min(cargador, excedente) por buckets de 5 min', async () => {
    _setInstallForTests(TOPO)
    _setForTests(chargerExt({ powerId: 'sensor.chg_power', chargerInHouseMeters: false }))
    // 3 buckets de 5 min: cargador 2 kW constante; producción 4 kW, casa 1 kW
    // → surplus 3 → solar = min(2, 3) = 2 kW → 2 kW × 0.25 h = 0.5 kWh
    const start = new Date(2026, 7, 12, 12, 0, 0)
    const stats = {
      'sensor.prod': [0, 1, 2].map((i) => ({ start: new Date(start.getTime() + i * 300000).toISOString(), mean: 4 })),
      'sensor.cons': [0, 1, 2].map((i) => ({ start: new Date(start.getTime() + i * 300000).toISOString(), mean: 1000 })),
    }
    const powerRows = [
      { state: '2', last_changed: new Date(start.getTime() - 3600000).toISOString() }, // 2 kW
    ]
    const ha = {
      statisticsDuringPeriod: async () => stats,
      historyDuringPeriod: async () => powerRows,
    }
    const pv = await chargerPvFromCurves(ha, start.toISOString(), new Date(start.getTime() + 900000).toISOString())
    expect(pv.get('2026-08-12')).toBeCloseTo(0.5, 2)
  })

  it('chargerHistory estima el PV por balance diario cuando falta (pv NULL)', () => {
    const db = emptyDb()
    // producción 20, consumo 15 (incluye cargador 6) → resto 9 → solar al cargador 11 → clamp 6
    db.prepare(
      `INSERT INTO daily (date, production_kwh, consumption_kwh, ext_charger_kwh)
       VALUES ('2026-08-12', 20, 15, 6), ('2026-08-13', 10, 30, 4)`
    ).run()
    const days = chargerHistory(db, '2026-08-12', '2026-08-13')
    expect(days[0].pvKwh).toBe(6) // excedente 11 > carga 6 → todo solar
    expect(days[1].pvKwh).toBe(0) // producción < consumo del resto → 0
  })
})

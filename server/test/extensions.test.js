// extensions.test.js — marco de extensiones (issue #94): resolución
// kv>legacy>generic, live del cargador, acumulación diaria por deltas del
// contador y backfill desde el historial del recorder.
import { describe, it, expect, afterEach } from 'vitest'
import Database from 'better-sqlite3'
import { z } from 'zod'
import { initSchema, kvSet } from '../src/db.js'
import { createSchemas } from '../../shared/schemas.js'
import {
  resolveExtensions,
  normalizeExtensions,
  chargerActive,
  chargerEntities,
  computeChargerLive,
  accumulateChargerDaily,
  dailyDeltasFromHistory,
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

  it('backfill solo escribe días a NULL y anteriores a hoy', async () => {
    const db = emptyDb()
    db.prepare(
      `INSERT INTO daily (date, production_kwh, ext_charger_kwh) VALUES ('2026-08-12', 10, 7.0), ('2026-08-13', 10, NULL)`
    ).run()
    _setForTests(chargerExt())
    const L = (y, m, d, h) => new Date(y, m - 1, d, h).toISOString()
    const rows = [
      { state: '100', last_changed: L(2026, 8, 12, 12) },
      { state: '103', last_changed: L(2026, 8, 12, 20) },
      { state: '103', last_changed: L(2026, 8, 13, 10) },
      { state: '107', last_changed: L(2026, 8, 13, 20) },
    ]
    const ha = { historyDuringPeriod: async () => rows }
    const n = await backfillChargerHistory(ha, db)
    expect(n).toBe(1)
    const days = chargerHistory(db, '2026-08-12', '2026-08-13')
    expect(days[0].kwh).toBe(7.0) // ya tenía dato: intacto
    expect(days[1].kwh).toBe(4)
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

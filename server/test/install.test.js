// install.test.js — topología configurable (issue #37): perfiles legacy/generic,
// N inversores, grid sin scraper, estados de batería en varios idiomas.
import { describe, it, expect, afterEach } from 'vitest'
import Database from 'better-sqlite3'
import { initSchema, kvSet } from '../src/db.js'
import {
  resolveInstall,
  normalizeTopology,
  liveEntities,
  deepSources,
  deepSourceDailyKeys,
  getEntities,
  _setForTests,
  LEGACY_TOPOLOGY,
  GENERIC_TOPOLOGY,
} from '../src/install.js'

afterEach(() => {
  // Volver al perfil legacy por defecto (los tests existentes dependen de él).
  _setForTests(LEGACY_TOPOLOGY)
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

describe('resolveInstall', () => {
  it('instalación con datos SIN install_config → perfil LEGACY (scraper + 2 inversores)', () => {
    const t = resolveInstall(dbWithDaily())
    expect(t.grid.source).toBe('scraper')
    expect(t.inverters).toHaveLength(2)
    expect(t.battery.enabled).toBe(true)
  })

  it('instalación nueva SIN install_config → perfil GENERIC (sin scraper)', () => {
    const t = resolveInstall(emptyDb())
    expect(t.grid.source).toBe('sensor')
    expect(t.grid.scraperId).toBeNull()
    expect(t.inverters).toHaveLength(1)
    expect(t.battery.enabled).toBe(false)
  })

  it('install_config con topology → se usa la topología del admin', () => {
    const db = dbWithDaily()
    kvSet(db, 'install_config', JSON.stringify({
      topology: {
        inverters: [
          { key: 'a', name: 'Alpha', powerId: 'sensor.alpha_power', powerUnit: 'W' },
          { key: 'b', name: 'Beta', powerId: 'sensor.beta_power' },
          { key: 'c', name: 'Gamma', powerId: 'sensor.gamma_power' },
        ],
        grid: { source: 'sensor', sensorId: 'sensor.grid_net' },
        battery: { enabled: false },
      },
    }))
    const t = resolveInstall(db)
    expect(t.inverters).toHaveLength(3)
    expect(t.inverters[0].powerUnit).toBe('W')
    expect(t.inverters[2].name).toBe('Gamma')
    expect(t.grid.source).toBe('sensor')
    expect(t.grid.sensorId).toBe('sensor.grid_net')
    expect(t.battery.enabled).toBe(false)
  })

  it('install_config corrupto → fallback al perfil base', () => {
    const db = dbWithDaily()
    kvSet(db, 'install_config', '{no-json')
    const t = resolveInstall(db)
    expect(t.grid.source).toBe('scraper')
  })
})

describe('normalizeTopology', () => {
  it('completa huecos con el perfil base y numera inversores sin key', () => {
    const t = normalizeTopology({ inverters: [{ powerId: 'sensor.uno' }] }, LEGACY_TOPOLOGY)
    expect(t.inverters[0].key).toBe('inv1')
    expect(t.inverters[0].name).toBe('Inverter 1')
    expect(t.inverters[0].powerUnit).toBe('kW')
    expect(t.grid).toEqual(LEGACY_TOPOLOGY.grid)
  })
})

describe('liveEntities', () => {
  it('legacy: suscribe sensores de potencia, energía, batería, scraper y sun', () => {
    _setForTests(LEGACY_TOPOLOGY)
    const ids = liveEntities()
    expect(ids).toContain('sensor.solis_potencia_actual')
    expect(ids).toContain('sensor.solis_scraper')
    expect(ids).toContain('sun.sun')
    expect(ids).toContain('sensor.solis_bateria_soc')
  })

  it('generic sin batería: NO suscribe sensores de batería ni scraper', () => {
    _setForTests(GENERIC_TOPOLOGY)
    const ids = liveEntities()
    expect(ids).not.toContain('sensor.solis_scraper')
    expect(ids).not.toContain('sensor.solis_bateria_soc')
    expect(ids).toContain('sensor.grid_import_power')
  })
})

describe('deepSources', () => {
  it('legacy: mapea los 2 inversores a solis/fox y mantiene consumo/grid/batería', () => {
    _setForTests(LEGACY_TOPOLOGY)
    const { srcs, map } = deepSourceDailyKeys()
    expect(map.inv0).toBe('solis')
    expect(map.inv1).toBe('fox')
    expect(srcs.consumption.requireAll).toBe(true)
    expect(srcs.gridImport.acc).toBe('sum')
    expect(srcs.batCharge).toBeDefined()
  })

  it('3 inversores: mapea inv0→solis, inv1→fox, inv2→inv3', () => {
    const t = normalizeTopology(
      {
        inverters: [
          { key: 'a', energyId: 'sensor.a_en', deepIds: ['sensor.a_deep'] },
          { key: 'b', energyId: 'sensor.b_en', deepIds: ['sensor.b_deep'] },
          { key: 'c', energyId: 'sensor.c_en', deepIds: ['sensor.c_deep'] },
        ],
      },
      GENERIC_TOPOLOGY
    )
    _setForTests(t)
    const { srcs, map } = deepSourceDailyKeys()
    expect(map.inv0).toBe('solis')
    expect(map.inv1).toBe('fox')
    expect(map.inv2).toBe('inv3')
    expect(srcs.inv2.ids).toEqual(['sensor.c_deep'])
  })

  it('sin batería: no genera batCharge/batDischarge', () => {
    _setForTests(GENERIC_TOPOLOGY)
    const { srcs } = deepSourceDailyKeys()
    expect(srcs.batCharge).toBeUndefined()
    expect(srcs.batDischarge).toBeUndefined()
  })
})

describe('getEntities (shape compat con el frontend)', () => {
  it('legacy: expone los IDs de la casa', () => {
    _setForTests(LEGACY_TOPOLOGY)
    const e = getEntities()
    expect(e.pvSolis).toBe('sensor.solis_potencia_actual')
    expect(e.scraper).toBe('sensor.solis_scraper')
    expect(e.eSolis).toBe('sensor.solis_energia_hoy')
  })
})

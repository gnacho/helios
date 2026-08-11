import { describe, it, expect, afterEach } from 'vitest'
import { computeLive, cachedCollector } from '../src/solar.js'
import { _setForTests } from '../src/install.js'
import { LEGACY_TOPOLOGY, GENERIC_TOPOLOGY } from '../src/install.js'

afterEach(() => {
  _setForTests(LEGACY_TOPOLOGY)
})

const emptyHa = { getState: () => undefined, connected: false }

describe('computeLive', () => {
  it('devuelve ceros y alerta crítica sin conexión HAOS', () => {
    const live = computeLive(emptyHa)
    expect(live.production).toBe(0)
    expect(live.consumption).toBe(0)
    expect(live.alerts.some((a) => a.id === 'haos' && a.severity === 'critical')).toBe(true)
  })
})

describe('computeLive — topología genérica (issue #37)', () => {
  function ha() {
    return {
      connected: true,
      getState: (id) => {
        const map = {
          'sensor.inv1_power': { state: '3.2' },
          'sensor.inv2_power': { state: '1500' }, // W
          'sensor.house_power': { state: '800' }, // W
          'sensor.grid_net': { state: '-1.2' }, // import positivo / export negativo
          'sensor.battery_power': { state: '1.0' },
          'sensor.battery_soc': { state: '55' },
          'sun.sun': { state: 'above_horizon', attributes: { elevation: 40 } },
        }
        return map[id] || undefined
      },
    }
  }

  it('N inversores con unidades distintas: suma producción y expone inverters[]', () => {
    const t = {
      ...GENERIC_TOPOLOGY,
      inverters: [
        { ...GENERIC_TOPOLOGY.inverters[0], key: 'inv1', powerId: 'sensor.inv1_power', powerUnit: 'kW' },
        { ...GENERIC_TOPOLOGY.inverters[0], key: 'inv2', powerId: 'sensor.inv2_power', powerUnit: 'W' },
      ],
    }
    _setForTests(t)
    const live = computeLive(ha())
    // 3.2 + 1500W/1000 = 4.7
    expect(live.production).toBeCloseTo(4.7, 2)
    expect(live.inverters).toHaveLength(2)
    expect(live.inverters[0].kw).toBeCloseTo(3.2, 2)
    expect(live.inverters[1].kw).toBeCloseTo(1.5, 2)
    // solis/fox = compat de los 2 primeros inversores
    expect(live.solis).toBeCloseTo(3.2, 2)
    expect(live.fox).toBeCloseTo(1.5, 2)
  })

  it('grid sin scraper (sensor plano con signo): import = positivo, export = negativo', () => {
    const t = { ...GENERIC_TOPOLOGY, grid: { ...GENERIC_TOPOLOGY.grid, source: 'sensor', sensorId: 'sensor.grid_net' } }
    _setForTests(t)
    const live = computeLive(ha())
    expect(live.grid).toBeCloseTo(-1.2, 2)
  })

  it('sin batería: soc=0, batteryPower=0 y no genera alerta de batería', () => {
    const t = { ...GENERIC_TOPOLOGY, battery: { ...GENERIC_TOPOLOGY.battery, enabled: false } }
    _setForTests(t)
    const live = computeLive(ha())
    expect(live.soc).toBe(0)
    expect(live.batteryPower).toBe(0)
    expect(live.alerts.some((a) => a.id === 'bateria')).toBe(false)
  })

  it('estados de batería en INGLÉS: charging/discharging se interpretan por sinónimos', () => {
    const t = {
      ...GENERIC_TOPOLOGY,
      battery: {
        ...GENERIC_TOPOLOGY.battery,
        enabled: true,
        powerId: 'sensor.battery_power',
        stateId: 'sensor.battery_state',
        socId: 'sensor.battery_soc',
        chargingStates: ['charging', 'Cargando'],
        dischargingStates: ['discharging', 'Descargando'],
      },
    }
    const haDischarging = {
      ...ha(),
      getState: (id) => {
        const base = ha().getState(id)
        if (id === 'sensor.battery_state') return { state: 'discharging' }
        return base
      },
    }
    _setForTests(t)
    const live = computeLive(haDischarging)
    expect(live.batteryPower).toBeCloseTo(-1.0, 2)
  })
})

describe('cachedCollector', () => {
  it('single-flight: N llamadas concurrentes = 1 ejecución', async () => {
    let calls = 0
    const fn = async () => {
      calls++
      await new Promise((r) => setTimeout(r, 20))
      return 42
    }
    const cached = cachedCollector(() => 'k', () => 1000, fn)
    const [a, b, c] = await Promise.all([cached(), cached(), cached()])
    expect(a).toBe(42)
    expect(b).toBe(42)
    expect(c).toBe(42)
    expect(calls).toBe(1)
  })

  it('sirve de caché dentro del TTL y re-ejecuta al expirar', async () => {
    let calls = 0
    const cached = cachedCollector(() => 'k', () => 30, async () => ++calls)
    expect(await cached()).toBe(1)
    expect(await cached()).toBe(1)
    await new Promise((r) => setTimeout(r, 50))
    expect(await cached()).toBe(2)
  })

  it('no cachea errores', async () => {
    let calls = 0
    const cached = cachedCollector(() => 'k', () => 1000, async () => {
      calls++
      if (calls === 1) throw new Error('fallo')
      return 'ok'
    })
    await expect(cached()).rejects.toThrow('fallo')
    expect(await cached()).toBe('ok')
  })
})

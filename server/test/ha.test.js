// ha.test.js — caché de entidades del cliente HAOS (subscribe_entities).
// El bug que motivó estos tests (issue 100): los eventos incrementales 'c'
// traen un diff PARCIAL de attributes y el código los reemplazaba enteros
// → lat/lon del device_tracker se evaporaban tras el primer update.
import { describe, it, expect } from 'vitest'
import { HAClient } from '../src/ha.js'

/** Instancia sin conexión: solo ejercita _applyEntityEvent/getState. */
function clientWithCache() {
  const c = new HAClient('http://x', 'tok')
  c.entities = new Map()
  return c
}

describe('HAClient._applyEntityEvent', () => {
  it('snapshot (a) instala estado + attributes completos', () => {
    const c = clientWithCache()
    c._applyEntityEvent({
      a: { 'device_tracker.coche': { s: 'home', a: { latitude: 39.5, longitude: -0.5, friendly_name: 'Coche' }, lu: 1 } },
    })
    expect(c.getState('device_tracker.coche')).toEqual({
      state: 'home',
      attributes: { latitude: 39.5, longitude: -0.5, friendly_name: 'Coche' },
      lastUpdated: 1,
    })
  })

  it('diff parcial (c.+) hace MERGE de attributes, no los reemplaza', () => {
    const c = clientWithCache()
    c._applyEntityEvent({
      a: { 'device_tracker.coche': { s: 'home', a: { latitude: 39.5, longitude: -0.5 }, lu: 1 } },
    })
    // Update incremental que solo cambia latitude y añade gps_speed
    c._applyEntityEvent({
      c: { 'device_tracker.coche': { '+': { s: 'not_home', a: { latitude: 39.6, gps_speed: 0 }, lu: 2 } } },
    })
    const e = c.getState('device_tracker.coche')
    expect(e.state).toBe('not_home')
    // longitude SOBREVIVE al diff parcial y latitude se actualiza
    expect(e.attributes).toEqual({ latitude: 39.6, longitude: -0.5, gps_speed: 0 })
  })

  it('diff solo de estado (c.+ sin a) conserva los attributes', () => {
    const c = clientWithCache()
    c._applyEntityEvent({
      a: { 'sensor.soc': { s: '50', a: { unit_of_measurement: '%' }, lu: 1 } },
    })
    c._applyEntityEvent({ c: { 'sensor.soc': { '+': { s: '51', lu: 2 } } } })
    const e = c.getState('sensor.soc')
    expect(e.state).toBe('51')
    expect(e.attributes).toEqual({ unit_of_measurement: '%' })
  })

  it('diff de borrado (c.-) elimina la entidad', () => {
    const c = clientWithCache()
    c._applyEntityEvent({ a: { 'sensor.x': { s: '1', a: {}, lu: 1 } } })
    c._applyEntityEvent({ c: { 'sensor.x': { '-': {} } } })
    expect(c.getState('sensor.x')).toBeUndefined()
  })
})

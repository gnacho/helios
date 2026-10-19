// callService: construye el mensaje websocket de HAOS correctamente
import { describe, it, expect } from 'vitest'
import { HAClient } from '../src/ha.js'

describe('HAClient.callService', () => {
  it('switch on/off: target sin service_data', async () => {
    const ha = Object.create(HAClient.prototype)
    let capturado
    ha.call = async (msg) => { capturado = msg; return { ok: true } }
    await ha.callService('switch', 'turn_on', { entity_id: 'switch.x' })
    expect(capturado).toEqual({ type: 'call_service', domain: 'switch', service: 'turn_on', target: { entity_id: 'switch.x' } })
  })

  it('number set_value: service_data con el valor', async () => {
    const ha = Object.create(HAClient.prototype)
    let capturado
    ha.call = async (msg) => { capturado = msg; return { ok: true } }
    await ha.callService('number', 'set_value', { entity_id: 'number.a', value: 16 })
    expect(capturado.domain).toBe('number')
    expect(capturado.service).toBe('set_value')
    expect(capturado.target).toEqual({ entity_id: 'number.a' })
    expect(capturado.service_data).toEqual({ value: 16 })
  })

  it('select option: service_data con la opcion', async () => {
    const ha = Object.create(HAClient.prototype)
    let capturado
    ha.call = async (msg) => { capturado = msg; return { ok: true } }
    await ha.callService('input_select', 'select_option', { entity_id: 'input_select.b', option: '16' })
    expect(capturado.service_data).toEqual({ option: '16' })
  })
})

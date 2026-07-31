import { describe, it, expect } from 'vitest'
import { computeLive, cachedCollector } from '../src/solar.js'

const emptyHa = { getState: () => undefined, connected: false }

describe('computeLive', () => {
  it('devuelve ceros y alerta crítica sin conexión HAOS', () => {
    const live = computeLive(emptyHa)
    expect(live.production).toBe(0)
    expect(live.consumption).toBe(0)
    expect(live.alerts.some((a) => a.id === 'haos' && a.severity === 'critical')).toBe(true)
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

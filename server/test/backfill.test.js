import { describe, it, expect, vi, afterEach } from 'vitest'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

// ESM hoists imports: config.js se evalúa al importar solar.js. El env se
// inyecta ANTES del import dinámico para que la validación zod no falle.
process.env.HAOS_TOKEN = 'test'
process.env.AUTH_PASS = 'testpass'
process.env.DATA_DIR = '/tmp/helios-bf-datadir'

const { openDb } = await import('../src/db.js')
const { backfillHistory } = await import('../src/solar.js')

function makeHa(stats) {
  return { statisticsDuringPeriod: vi.fn(async () => stats) }
}

describe('backfillHistory — huecos de datos (caza de bugs 6-Ago)', () => {
  afterEach(() => vi.useRealTimers())

  it('un día sin fila reparte el delta entre los días del hueco (no todo al posterior)', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-08-04T12:00:00Z'))

    // FOX (acc:'state', acumulador creciente): día1=10, día2 SIN datos, día3=30.
    // El delta 10→30 = 20 kWh cubre 2 días → 10/día, NO 20 concentrados en el día 3.
    const stats = {
      'sensor.almacen_pinza_energy_produced_b': [
        { start: '2026-08-01T00:00:00Z', state: 10 },
        { start: '2026-08-03T00:00:00Z', state: 30 },
      ],
    }
    const ha = makeHa(stats)
    const dir = mkdtempSync(join(tmpdir(), 'helios-bf-'))
    const db = openDb(dir)

    const n = await backfillHistory(ha, db)
    expect(n).toBeGreaterThan(0)

    const day1 = db.prepare('SELECT fox_kwh FROM daily WHERE date = ?').get('2026-08-01')
    const day2 = db.prepare('SELECT fox_kwh FROM daily WHERE date = ?').get('2026-08-02')
    const day3 = db.prepare('SELECT fox_kwh FROM daily WHERE date = ?').get('2026-08-03')
    expect(day1.fox_kwh).toBe(10) // primer día: delta desde 0
    expect(day2.fox_kwh).toBe(10) // día del hueco: su parte
    expect(day3.fox_kwh).toBe(10) // día con fila: solo su parte, no 20

    db.close()
    rmSync(dir, { recursive: true, force: true })
  })
})

import { describe, it, expect, vi, afterEach } from 'vitest'
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

// ESM hoists imports: config.js se evalúa al importar solar.js. El env se
// inyecta ANTES del import dinámico para que la validación zod no falle.
process.env.HAOS_TOKEN = 'test'
process.env.AUTH_PASS = 'testpass'
process.env.DATA_DIR = '/tmp/helios-bf-datadir'

const { openDb } = await import('../src/db.js')
const { backfillHistory, backfillDaySeries, todayStr } = await import('../src/solar.js')
const { _setForTests, LEGACY_TOPOLOGY } = await import('../src/install.js')

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

describe('backfillDaySeries — curvas persistidas en day_series (issue #114)', () => {
  afterEach(() => _setForTests(LEGACY_TOPOLOGY))

  it('persiste curvas de días del backup de HAOS en day_series', async () => {
    // Backup de HAOS antiguo (formato statisticsDuringPeriod): pinza_b es el
    // inversor Fox (LEGACY). Un solo día (2026-07-20) con 3 buckets de 5 min.
    const stats = {
      'sensor.almacen_pinza_power_b': [
        { start: '2026-07-20T10:00:00Z', mean: 2000 },
        { start: '2026-07-20T10:05:00Z', mean: 2200 },
        { start: '2026-07-20T10:10:00Z', mean: 1800 },
      ],
    }
    const dir = mkdtempSync(join(tmpdir(), 'helios-bf-'))
    const db = openDb(dir)
    db.prepare("INSERT INTO daily (date, production_kwh) VALUES ('2026-07-20', 10)").run()
    const backupFile = join(dir, 'day-series-backup.json')
    writeFileSync(backupFile, JSON.stringify(stats))

    const ha = makeHa({}) // HAOS vivo sin datos; la fuente es el backup
    const n = await backfillDaySeries(ha, db, backupFile)
    expect(n.rows).toBeGreaterThan(0)

    const row = db.prepare('SELECT * FROM day_series WHERE date = ?').get('2026-07-20')
    expect(row).toBeTruthy()
    expect(row.source).toBe('backup')
    expect(row.estimated).toBe(0)
    const points = JSON.parse(row.points_json)
    expect(points.length).toBe(3)
    expect(points[0].production).toBe(2.0)
    expect(points[0].solis).toBe(0)
    expect(points[0].fox).toBe(2.0)

    db.close()
    rmSync(dir, { recursive: true, force: true })
  })

  it('getDaySeries sirve días pasados desde day_series sin tocar HAOS', async () => {
    const { getDaySeries } = await import('../src/solar.js')
    const dir = mkdtempSync(join(tmpdir(), 'helios-bf-'))
    const db = openDb(dir)
    const points = [
      { t: 600, label: '10:00', solis: 1, fox: 2, production: 3, consumption: 1, batteryPower: 0, soc: 50, grid: -2 },
    ]
    db.prepare(
      `INSERT INTO day_series (date, points_json, estimated, source, updated_at)
       VALUES ('2026-07-20', ?, 0, 'backup', ?)`
    ).run(JSON.stringify(points), Date.now())

    const ha = { statisticsDuringPeriod: vi.fn() } // no debe llamarse
    const res = await getDaySeries(ha, '2026-07-20', db)
    expect(res.points.length).toBe(1)
    expect(res.points[0].production).toBe(3)
    expect(ha.statisticsDuringPeriod).not.toHaveBeenCalled()

    db.close()
    rmSync(dir, { recursive: true, force: true })
  })
})

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
const { backfillHistory, backfillDaySeries, todayStr, getDaySeries } = await import('../src/solar.js')
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

describe('backup power source per inverter (issue #124)', () => {
  afterEach(() => _setForTests(LEGACY_TOPOLOGY))

  // Día pasado SIN fila en day_series: getDaySeries computa desde HAOS y usa
  // la fila `daily` del día como total de referencia para escalar el respaldo.
  it('rellena el hueco del sensor principal con backupPowerId y escala al total daily', async () => {
    const dir = mkdtempSync(join(tmpdir(), 'helios-bf-'))
    const db = openDb(dir)
    // LEGACY: fox.powerId = almacen_pinza_power_b (W), fox.backupPowerId =
    // foxess_solar_power (kW). El día tiene 2 tramos de 1 h: 07:00-08:00 SOLO
    // en el backup (hueco de la pinza) y 10:00-11:00 SOLO en la pinza.
    const hourBuckets = (iso, mean) =>
      Array.from({ length: 12 }, (_, i) => ({
        start: new Date(new Date(iso).getTime() + i * 5 * 60000).toISOString(),
        mean,
      }))
    const stats = {
      'sensor.foxess_solar_power': hourBuckets('2026-07-20T07:00:00Z', 1.5),
      'sensor.almacen_pinza_power_b': hourBuckets('2026-07-20T10:00:00Z', 2000),
    }
    const ha = {
      statisticsDuringPeriod: vi.fn(async () => stats),
      getState: vi.fn(),
    }
    // Total real del Fox del día según la fuente profunda (pinza lifetime):
    // 2 kWh medidos + 1.7 kWh en el hueco (el backup crudo da 1.5, ~4% bajo).
    db.prepare(
      "INSERT INTO daily (date, production_kwh, solis_kwh, fox_kwh) VALUES ('2026-07-20', 3.7, 0, 3.7)"
    ).run()

    const res = await getDaySeries(ha, '2026-07-20', db)
    expect(res.estimated).toBe(true)

    // El hueco (07:00Z local) usa el backup: fox = 1.5 kW escalado por
    // (3.7 - 2.0) / 1.5 = 1.133 → ~1.7 kW. La parte de la pinza (10:00Z) NO se
    // toca: fox = 2.0 kW. Los labels son hora LOCAL del runner.
    const lab = (iso) => {
      const d = new Date(iso)
      return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
    }
    const p07 = res.points.find((p) => p.label === lab('2026-07-20T07:00:00Z'))
    const p10 = res.points.find((p) => p.label === lab('2026-07-20T10:00:00Z'))
    expect(p07.fox).toBeCloseTo(1.7, 1)
    expect(p10.fox).toBeCloseTo(2.0, 1)

    // La curva integra el total daily real del Fox.
    const dtH = 5 / 60
    const integ = res.points.reduce((acc, p) => acc + p.fox * dtH, 0)
    expect(integ).toBeCloseTo(3.7, 1)

    db.close()
    rmSync(dir, { recursive: true, force: true })
  })

  it('sin backupPowerId no inventa datos (comportamiento previo)', async () => {
    const dir = mkdtempSync(join(tmpdir(), 'helios-bf-'))
    const db = openDb(dir)
    const t = {
      ...LEGACY_TOPOLOGY,
      inverters: LEGACY_TOPOLOGY.inverters.map((inv, i) =>
        i === 1 ? { ...inv, backupPowerId: '', backupPowerUnit: 'kW' } : inv
      ),
    }
    _setForTests(t)
    const stats = {
      'sensor.almacen_pinza_power_b': [{ start: '2026-07-21T10:00:00Z', mean: 2000 }],
    }
    const ha = { statisticsDuringPeriod: vi.fn(async () => stats), getState: vi.fn() }
    db.prepare(
      "INSERT INTO daily (date, production_kwh, solis_kwh, fox_kwh) VALUES ('2026-07-21', 2.0, 0, 2.0)"
    ).run()

    const res = await getDaySeries(ha, '2026-07-21', db)
    expect(res.estimated).toBe(false)
    const lab = (iso) => {
      const d = new Date(iso)
      return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
    }
    const p10 = res.points.find((p) => p.label === lab('2026-07-21T10:00:00Z'))
    expect(p10.fox).toBeCloseTo(2.0, 1)
    // Solo existe el bucket de la pinza: sin hueco rellenado por backup.
    expect(res.points.some((p) => p.label === lab('2026-07-21T07:00:00Z'))).toBe(false)

    db.close()
    rmSync(dir, { recursive: true, force: true })
  })
})

describe('consumption gap filled from previous-day pattern (issue 125)', () => {
  afterEach(() => _setForTests(LEGACY_TOPOLOGY))

  it('rellena un hueco sin NINGÚN medidor con la forma del día anterior escalada', async () => {
    const dir = mkdtempSync(join(tmpdir(), 'helios-bf-'))
    const db = openDb(dir)

    const hourBuckets = (iso, mean) =>
      Array.from({ length: 12 }, (_, i) => ({
        start: new Date(new Date(iso).getTime() + i * 5 * 60000).toISOString(),
        mean,
      }))
    // Solis (inv0) produce 07-08 y 10-11 (los buckets existen).
    const stats = {
      'sensor.solis_potencia_actual': [
        ...hourBuckets('2026-07-22T07:00:00Z', 1.5),
        ...hourBuckets('2026-07-22T10:00:00Z', 2.0),
      ],
    }
    // 07-08 NO tiene medidores de consumo (hueco); 10-11 los 3 miden 1000 W.
    for (const id of ['sensor.medidor_respaldo_power', 'sensor.vivienda_medidor_power', 'sensor.almacen_pinza_power_a']) {
      stats[id] = hourBuckets('2026-07-22T10:00:00Z', 1000)
    }
    const ha = { statisticsDuringPeriod: vi.fn(async () => stats), getState: vi.fn() }

    // Fila daily del día: consumo real total 3.5 kWh (3 medidos + 0.5 hueco).
    db.prepare(
      "INSERT INTO daily (date, production_kwh, consumption_kwh) VALUES ('2026-07-22', 5, 3.5)"
    ).run()

    // Patrón del día anterior: el hueco (07-08) consumía 0.5 kW de forma.
    const prevPts = [
      ...hourBuckets('2026-07-21T07:00:00Z', 500).map((r, i) => {
        const d = new Date(r.start)
        return { t: d.getHours() * 60 + d.getMinutes(), label: '07:00', consumption: 0.5 }
      }),
    ]
    db.prepare(
      `INSERT INTO day_series (date, points_json, estimated, source, updated_at)
       VALUES ('2026-07-21', ?, 0, 'haos', ?)`
    ).run(JSON.stringify(prevPts), Date.now())

    const res = await getDaySeries(ha, '2026-07-22', db)
    expect(res.estimated).toBe(true)

    const dtH = 5 / 60
    const integ = res.points.reduce((acc, p) => acc + (p.consumption || 0) * dtH, 0)
    expect(integ).toBeCloseTo(3.5, 1)

    db.close()
    rmSync(dir, { recursive: true, force: true })
  })

  it('HOY sin fila daily: rellena el hueco con el patrón del día anterior sin escalar', async () => {
    const dir = mkdtempSync(join(tmpdir(), 'helios-bf-'))
    const db = openDb(dir)

    const hourBuckets = (iso, mean) =>
      Array.from({ length: 12 }, (_, i) => ({
        start: new Date(new Date(iso).getTime() + i * 5 * 60000).toISOString(),
        mean,
      }))
    const stats = {
      'sensor.solis_potencia_actual': hourBuckets('2026-07-23T07:00:00Z', 1.5),
    }
    // Hueco: 07-08 sin NINGÚN medidor. Sin fila daily (caso HOY en vivo).
    const ha = { statisticsDuringPeriod: vi.fn(async () => stats), getState: vi.fn() }

    const prevPts = [
      ...hourBuckets('2026-07-22T07:00:00Z', 500).map((r) => {
        const d = new Date(r.start)
        return { t: d.getHours() * 60 + d.getMinutes(), label: '07:00', consumption: 0.5 }
      }),
    ]
    db.prepare(
      `INSERT INTO day_series (date, points_json, estimated, source, updated_at)
       VALUES ('2026-07-22', ?, 0, 'haos', ?)`
    ).run(JSON.stringify(prevPts), Date.now())

    const res = await getDaySeries(ha, '2026-07-23', db)
    expect(res.estimated).toBe(true)
    const p = res.points.find((x) => x.consumption > 0)
    expect(p.consumption).toBeCloseTo(0.5, 1)

    db.close()
    rmSync(dir, { recursive: true, force: true })
  })
})

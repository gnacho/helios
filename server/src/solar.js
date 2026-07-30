import { ENTITIES, ENERGY_ENTITIES, config } from './config.js'
import { upsertDaily, dailyEmpty, kvGet, kvSet } from './db.js'

const num = (v) => {
  const n = parseFloat(v)
  return Number.isFinite(n) ? n : 0
}

const entityNum = (ha, id, divisor = 1) => {
  const e = ha.getState(id)
  if (!e) return 0
  return num(e.state) / divisor
}

const CONSUMPTION_SOURCES = [ENTITIES.consRespaldo, ENTITIES.consNoRespaldada, ENTITIES.consAlmacen]

const consumptionKw = (ha) => {
  let total = 0
  for (const id of CONSUMPTION_SOURCES) total += entityNum(ha, id, 1000)
  return total
}

export function computeLive(ha) {
  const solis = entityNum(ha, ENTITIES.pvSolis)
  let fox = entityNum(ha, ENTITIES.pvFox, 1000)
  if (fox < 0.02) fox = 0
  const consumption = consumptionKw(ha)

  const batMag = entityNum(ha, ENTITIES.batteryPower)
  const batState = ha.getState(ENTITIES.batteryState)?.state || ''
  const batteryPower = batState === 'Cargando' ? batMag : batState === 'Descargando' ? -batMag : 0
  const soc = entityNum(ha, ENTITIES.batterySoc)

  const scraper = ha.getState(ENTITIES.scraper)
  const gridMag = Math.abs(num(scraper?.attributes?.currentGridPower))
  const gridDir = scraper?.attributes?.gridDirection || 'none'
  let grid = gridDir === 'import' ? gridMag : gridDir === 'export' ? -gridMag : 0

  const sun = ha.getState(ENTITIES.sun)
  const weather = ha.getState(ENTITIES.weather)
  const elevation = num(sun?.attributes?.elevation)
  const production = round3(solis + fox)

  // Sanity check: de noche sin sol, no puede haber export solar
  if (elevation < 0 && grid < -0.03 && production < 0.02) grid = -grid

  // Fallback: si no hay datos de grid pero hay consumo y no hay producción, estimar import
  if (grid === 0 && consumption > 0.1 && production < 0.02) grid = consumption

  const alerts = []
  if (!ha.connected) alerts.push({ id: 'haos', severity: 'critical', text: 'Sin conexión con Home Assistant' })
  if (scraper?.attributes?.inverterOnline === 0)
    alerts.push({ id: 'inversor', severity: 'critical', text: 'Inversor Solis offline' })
  const lastUpd = scraper?.attributes?.lastUpdate ? new Date(scraper.attributes.lastUpdate).getTime() : null
  if (lastUpd && (Date.now() - lastUpd) / 60000 > 15)
    alerts.push({
      id: 'scraper',
      severity: 'warning',
      text: `Datos del Solis antiguos (hace ${Math.round((Date.now() - lastUpd) / 60000)} min)`,
    })
  if (soc > 0 && soc <= 20) alerts.push({ id: 'bateria', severity: 'info', text: `Batería en reserva (${round1(soc)}%)` })
  if (elevation > 15 && production < 0.1)
    alerts.push({ id: 'sin_pv', severity: 'warning', text: 'Sin producción solar a plena luz' })

  const now = new Date()
  return {
    production,
    consumption: round3(consumption),
    batteryPower: round3(batteryPower),
    batteryStatus: batState || 'Desconocido',
    soc: round1(soc),
    grid: round3(grid),
    solis: round3(solis),
    fox: round3(fox),
    at: now.getHours() * 60 + now.getMinutes(),
    alerts,
    sun: {
      state: sun?.state || 'unknown',
      elevation,
      nextRising: sun?.attributes?.next_rising || null,
      nextSetting: sun?.attributes?.next_setting || null,
    },
    weather: weather?.state || 'unknown',
    weatherTemp: entityNum(ha, ENTITIES.weatherTemp),
    station: scraper?.attributes?.stationName || '',
    inverterOnline: scraper?.attributes?.inverterOnline === 1,
    ts: now.toISOString(),
  }
}

const CONSUMPTION_COUNTER_IDS = () => [
  ENTITIES.consRespaldoEnergy,
  ENTITIES.consNoRespaldadaEnergy,
  ENTITIES.consAlmacenEnergy,
]

export async function ensureConsumptionBaseline(ha, db) {
  const today = todayStr()
  const raw = kvGet(db, 'cons_baseline')
  if (raw) {
    try {
      const b = JSON.parse(raw)
      if (b.date === today) return b
    } catch {}
  }
  const y = new Date()
  y.setDate(y.getDate() - 1)
  const start = new Date(y.getFullYear(), y.getMonth(), y.getDate())
  const end = new Date(start)
  end.setDate(end.getDate() + 1)
  const stats = await ha.statisticsDuringPeriod({
    startTime: start.toISOString(),
    endTime: end.toISOString(),
    statisticIds: CONSUMPTION_COUNTER_IDS(),
    period: 'day',
    types: ['state'],
  })
  const values = {}
  for (const id of CONSUMPTION_COUNTER_IDS()) {
    const rows = stats[id] || []
    values[id] = rows.length ? num(rows[rows.length - 1].state) : null
  }
  const b = { date: today, values }
  kvSet(db, 'cons_baseline', JSON.stringify(b))
  return b
}

export function consumptionTodayFromCounters(ha, baseline) {
  let total = 0
  let usable = false
  for (const id of CONSUMPTION_COUNTER_IDS()) {
    const base = baseline?.values?.[id]
    const cur = entityNum(ha, id)
    if (base !== null && base !== undefined && cur >= base) {
      total += cur - base
      usable = true
    }
  }
  return usable ? total : null
}

export function computeTodayKpis(ha, consumptionOverride) {
  const g = (id) => entityNum(ha, id)
  const production = g(ENTITIES.eSolis) + g(ENTITIES.eFox)
  const gridImport = g(ENTITIES.eGridImport)
  const gridExport = g(ENTITIES.eGridExport)
  const batCharge = g(ENTITIES.eBatCharge)
  const batDischarge = g(ENTITIES.eBatDischarge)
  const medidores = consumptionOverride !== null && consumptionOverride !== undefined ? consumptionOverride : 0
  const derived = production + gridImport - gridExport - batCharge + batDischarge
  const consumption = Math.max(medidores, derived, medidores > 0 ? 0 : g(ENTITIES.eConsumption))
  return {
    productionKwh: round2(production),
    solisKwh: round2(g(ENTITIES.eSolis)),
    foxKwh: round2(g(ENTITIES.eFox)),
    consumptionKwh: round2(Math.min(100, consumption)),
    gridImportKwh: round2(gridImport),
    gridExportKwh: round2(gridExport),
    batteryChargedKwh: round2(batCharge),
    batteryDischargedKwh: round2(batDischarge),
  }
}

export async function getDaySeries(ha, dateStr, db) {
  const day = dateStr ? new Date(dateStr + 'T00:00:00') : new Date()
  if (Number.isNaN(day.getTime())) throw new Error('fecha inválida')
  const start = new Date(day)
  start.setHours(0, 0, 0, 0)
  const end = new Date(start)
  end.setDate(end.getDate() + 1)

  const ids = [
    ENTITIES.pvSolis,
    ENTITIES.pvFox,
    ENTITIES.consRespaldo,
    ENTITIES.consNoRespaldada,
    ENTITIES.consAlmacen,
    ENTITIES.batteryPower,
    ENTITIES.batterySoc,
  ]
  const stats = await ha.statisticsDuringPeriod({
    startTime: start.toISOString(),
    endTime: end.toISOString(),
    statisticIds: ids,
    period: '5minute',
    types: ['mean'],
  })

  const buckets = new Map()
  for (const id of ids) {
    for (const row of stats[id] || []) {
      if (row.mean === null || row.mean === undefined) continue
      const t = new Date(row.start)
      const min = t.getHours() * 60 + t.getMinutes()
      if (!buckets.has(min)) buckets.set(min, {})
      buckets.get(min)[id] = row.mean
    }
  }

  const points = []
  let prevSoc = null
  const minutes = [...buckets.keys()].sort((a, b) => a - b)
  for (const min of minutes) {
    const b = buckets.get(min)
    const solis = num(b[ENTITIES.pvSolis])
    let fox = num(b[ENTITIES.pvFox]) / 1000
    if (fox < 0.02) fox = 0
    const consumption =
      (num(b[ENTITIES.consRespaldo]) + num(b[ENTITIES.consNoRespaldada]) + num(b[ENTITIES.consAlmacen])) / 1000
    const batMag = num(b[ENTITIES.batteryPower])
    const soc = num(b[ENTITIES.batterySoc])

    let batteryPower = 0
    if (prevSoc !== null && batMag > 0.01) {
      const socDelta = soc - prevSoc
      if (socDelta > 0.02) batteryPower = batMag
      else if (socDelta < -0.02) batteryPower = -batMag
    }
    if (soc > 0) prevSoc = soc

    const production = solis + fox
    let grid = consumption - production + batteryPower
    if (Math.abs(grid) < 0.03) grid = 0

    const hh = String(Math.floor(min / 60)).padStart(2, '0')
    const mm = String(min % 60).padStart(2, '0')
    points.push({
      t: min,
      label: `${hh}:${mm}`,
      solis: round3(solis),
      fox: round3(fox),
      production: round3(production),
      consumption: round3(consumption),
      batteryPower: round3(batteryPower),
      soc: round1(soc),
      grid: round3(grid),
    })
  }

  const dtH = 5 / 60
  const sumSolis = points.reduce((acc, p) => acc + p.solis * dtH, 0)
  const sumFox = points.reduce((acc, p) => acc + p.fox * dtH, 0)
  let estimated = false
  if (sumSolis < 0.5 && sumFox > 1 && db) {
    const row = db.prepare('SELECT solis_kwh, fox_kwh FROM daily WHERE date = ?').get(dateStr || todayStr())
    if (row && row.solis_kwh > 1 && sumFox > 1) {
      const ratio = row.solis_kwh / sumFox
      for (const p of points) {
        p.solis = round3(p.fox * ratio)
        p.production = round3(p.solis + p.fox)
        let grid = p.consumption - p.production + p.batteryPower
        if (Math.abs(grid) < 0.03) grid = 0
        p.grid = round3(grid)
      }
      estimated = true
    }
  }
  return { points, estimated }
}

export async function getKpis(ha, dateStr, db) {
  const today = todayStr()
  if (!dateStr || dateStr === today) {
    const baseline = await ensureConsumptionBaseline(ha, db).catch(() => null)
    const consToday = consumptionTodayFromCounters(ha, baseline)
    const k = computeTodayKpis(ha, consToday)
    const live = computeLive(ha)
    const { points: series } = await getDaySeries(ha, today, db).catch(() => ({ points: [] }))
    let peakProductionKw = 0
    let peakAt = 14 * 60
    for (const p of series) {
      if (p.production > peakProductionKw) {
        peakProductionKw = p.production
        peakAt = p.t
      }
    }
    return decorateKpis({ ...k, soc: live.soc, peakProductionKw: round3(peakProductionKw), peakAt })
  }

  const day = new Date(dateStr + 'T00:00:00')
  if (Number.isNaN(day.getTime())) throw new Error('fecha inválida')

  const row = db.prepare('SELECT * FROM daily WHERE date = ?').get(dateStr)
  const { points: series } = await getDaySeries(ha, dateStr, db).catch(() => ({ points: [] }))
  let peakProductionKw = 0
  let peakAt = 14 * 60
  let soc = 0
  for (const p of series) {
    if (p.production > peakProductionKw) {
      peakProductionKw = p.production
      peakAt = p.t
    }
    if (p.soc > 0) soc = p.soc
  }

  if (row) {
    return decorateKpis({
      productionKwh: round2(row.production_kwh || 0),
      solisKwh: round2(row.solis_kwh || 0),
      foxKwh: round2(row.fox_kwh || 0),
      consumptionKwh: round2(row.consumption_kwh || 0),
      gridImportKwh: round2(row.grid_import_kwh || 0),
      gridExportKwh: round2(row.grid_export_kwh || 0),
      batteryChargedKwh: round2(row.battery_charged_kwh || 0),
      batteryDischargedKwh: round2(row.battery_discharged_kwh || 0),
      soc,
      peakProductionKw: round3(peakProductionKw),
      peakAt,
    })
  }

  const start = new Date(day)
  const end = new Date(day)
  end.setDate(end.getDate() + 1)

  const ids = Object.values(ENERGY_ENTITIES)
  const stats = await ha.statisticsDuringPeriod({
    startTime: start.toISOString(),
    endTime: end.toISOString(),
    statisticIds: ids,
    period: 'day',
    types: ['state'],
  })

  const change = (id) => {    const rows = stats[id] || []
    let total = 0
    for (const r of rows) total += num(r.state)
    return total
  }

  const solisKwh = change(ENERGY_ENTITIES.solis)
  const foxKwh = change(ENERGY_ENTITIES.fox)

  return decorateKpis({
    productionKwh: round2(solisKwh + foxKwh),
    solisKwh: round2(solisKwh),
    foxKwh: round2(foxKwh),
    consumptionKwh: round2(change(ENERGY_ENTITIES.consumption)),
    gridImportKwh: round2(change(ENERGY_ENTITIES.gridImport)),
    gridExportKwh: round2(change(ENERGY_ENTITIES.gridExport)),
    batteryChargedKwh: round2(change(ENERGY_ENTITIES.batCharge)),
    batteryDischargedKwh: round2(change(ENERGY_ENTITIES.batDischarge)),
    soc,
    peakProductionKw: round3(peakProductionKw),
    peakAt,
  })
}

function decorateKpis(k) {
  const autoconsumoPct =
    k.productionKwh > 0 ? Math.min(100, ((k.productionKwh - k.gridExportKwh) / k.productionKwh) * 100) : 0
  const autosuficienciaPct =
    k.consumptionKwh > 0 ? Math.min(100, ((k.consumptionKwh - k.gridImportKwh) / k.consumptionKwh) * 100) : 0
  const directFvKwh = Math.max(0, Math.min(k.productionKwh, k.consumptionKwh))
  const ahorroEur =
    (directFvKwh + k.batteryDischargedKwh) * config.priceImport + k.gridExportKwh * config.priceExport
  return {
    ...k,
    autoconsumoPct: round1(autoconsumoPct),
    autosuficienciaPct: round1(autosuficienciaPct),
    ahorroEur: round2(ahorroEur),
    co2EvitadoKg: round2(k.productionKwh * config.co2PerKwh),
  }
}

const DEEP_SOURCES = {
  solis: { ids: ['sensor.energia_solis_diaria_um'], acc: 'sum', cap: 35 },
  fox: { ids: ['sensor.almacen_pinza_energy_produced_b'], acc: 'state', cap: 20 },
  consumption: {
    ids: ['sensor.medidor_respaldo_energy', 'sensor.vivienda_medidor_energy', 'sensor.almacen_pinza_energy_a'],
    acc: 'state',
    cap: 100,
    requireAll: true,
  },
  gridImport: { ids: ['sensor.energia_red_importada_solis'], acc: 'sum', cap: 100 },
  gridExport: { ids: ['sensor.energia_red_exportada_solis'], acc: 'sum', cap: 25 },
  batCharge: { ids: ['sensor.energia_bateria_carga_diaria'], acc: 'sum', cap: 12 },
  batDischarge: { ids: ['sensor.energia_bateria_descarga_diaria'], acc: 'sum', cap: 12 },
}

const FOX_GLITCH_OFFSETS = {
  '2025-06-15': 5.46,
  '2025-07-26': 13.88,
  '2025-08-20': 4.25,
  '2025-11-02': 206.58,
  '2026-03-13': 4.71,
}

export async function backfillHistory(ha, db) {
  const end = new Date()
  end.setDate(end.getDate() + 1)
  const start = new Date('2024-01-01T00:00:00')
  const queryStart = new Date(start)
  queryStart.setDate(queryStart.getDate() - 2)
  const ids = Object.values(DEEP_SOURCES).flatMap((s) => s.ids)
  const stats = await ha.statisticsDuringPeriod({
    startTime: queryStart.toISOString(),
    endTime: end.toISOString(),
    statisticIds: ids,
    period: 'day',
    types: ['state', 'sum'],
  })

  const byDate = new Map()
  const ensure = (key) => {
    if (!byDate.has(key)) byDate.set(key, {})
    return byDate.get(key)
  }
  const minKey = dateKey(start)

  for (const [field, src] of Object.entries(DEEP_SOURCES)) {
    const perIdDaily = new Map()
    for (const id of src.ids) {
      const rows = (stats[id] || []).filter((r) => (src.acc === 'sum' ? r.sum : r.state) !== null && (src.acc === 'sum' ? r.sum : r.state) !== undefined)
      rows.sort((a, b) => a.start - b.start)
      const daily = new Map()
      let prev = 0
      for (const row of rows) {
        const key = dateKey(new Date(row.start))
        const raw = src.acc === 'sum' ? row.sum : row.state
        if (key === todayStr()) {
          prev = raw
          continue
        }
        let value
        if (src.acc === 'sum') {
          value = Math.max(0, raw - prev)
        } else {
          const glitch = field === 'fox' ? FOX_GLITCH_OFFSETS[key] || 0 : 0
          value = Math.max(0, raw - prev - glitch)
        }
        prev = raw
        if (key < minKey) continue
        daily.set(key, value)
      }
      perIdDaily.set(id, daily)
    }

    const allKeys = new Set()
    for (const daily of perIdDaily.values()) for (const key of daily.keys()) allKeys.add(key)

    for (const key of allKeys) {
      if (src.requireAll && ![...perIdDaily.values()].every((d) => d.has(key))) continue
      let value = 0
      for (const daily of perIdDaily.values()) value += daily.get(key) || 0
      if (src.cap && value > src.cap) {
        console.warn(`[helios] ${field} diario anómalo ${key}: ${value} kWh, capado a ${src.cap}`)
        value = src.cap
      }
      const cur = ensure(key)[field]
      ensure(key)[field] = Math.max(cur === undefined ? -Infinity : cur, value)
    }
  }

  let n = 0
  for (const [date, f] of [...byDate.entries()].sort()) {
    const production = (f.solis || 0) + (f.fox || 0)
    const medidores = f.consumption || 0
    const derived = production + (f.gridImport || 0) - (f.gridExport || 0) - (f.batCharge || 0) + (f.batDischarge || 0)
    const consumption = medidores > 0 ? Math.min(100, Math.max(medidores, derived)) : 0
    upsertDaily(db, {
      date,
      production_kwh: round2(production),
      consumption_kwh: round2(consumption),
      grid_import_kwh: round2(f.gridImport || 0),
      grid_export_kwh: round2(f.gridExport || 0),
      battery_charged_kwh: round2(f.batCharge || 0),
      battery_discharged_kwh: round2(f.batDischarge || 0),
      solis_kwh: round2(f.solis || 0),
      fox_kwh: round2(f.fox || 0),
    })
    n++
  }
  return n
}

export async function maybeBackfill(ha, db) {
  if (dailyEmpty(db)) {
    const n = await backfillHistory(ha, db)
    return { ran: true, rows: n }
  }
  return { ran: false, rows: 0 }
}

export function todayStr() {
  return dateKey(new Date())
}

function dateKey(d) {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

const round1 = (n) => Math.round(n * 10) / 10
const round2 = (n) => Math.round(n * 100) / 100
const round3 = (n) => Math.round(n * 1000) / 1000

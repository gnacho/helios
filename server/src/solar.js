// solar.js — computación de datos solares a partir de la topología resuelta
// (install.js). La topología (N inversores, batería opcional, grid source) vive
// en install_config; aquí solo se consume con getInstall()/getEntities().
//
// Contrato hacia el frontend: computeLive y las series mantienen solis/fox como
// claves de los 2 PRIMEROS inversores (compatibilidad con el frontend actual)
// y añaden inverters[] para topologías con N inversores o nombres distintos.
import { config } from './config.js'
import { upsertDaily, dailyEmpty, kvGet, kvSet } from './db.js'
import { getInstall, getEntities, getEnergyEntities, deepSources, deepSourceDailyKeys } from './install.js'

const num = (v) => {
  const n = parseFloat(v)
  return Number.isFinite(n) ? n : 0
}

// Caché TTL + single-flight para colectores caros (recorder/statistics de HAOS):
// N requests concurrentes a la misma clave = UNA query a HAOS.
export function cachedCollector(keyFn, ttlFn, fn) {
  const cache = new Map()
  const inflight = new Map()
  return (...args) => {
    const key = keyFn(...args)
    const hit = cache.get(key)
    if (hit && Date.now() - hit.at < hit.ttl) return Promise.resolve(hit.value)
    if (inflight.has(key)) return inflight.get(key)
    const p = Promise.resolve()
      .then(() => fn(...args))
      .then((value) => {
        if (cache.size > 300) cache.delete(cache.keys().next().value)
        cache.set(key, { value, at: Date.now(), ttl: ttlFn(...args) })
        return value
      })
      .finally(() => inflight.delete(key))
    inflight.set(key, p)
    return p
  }
}

const entityNum = (ha, id, divisor = 1) => {
  const e = ha.getState(id)
  if (!e) return 0
  return num(e.state) / divisor
}

// Potencia de cada inversor en kW, respetando la unidad del sensor (kW o W).
function inverterPowerKw(ha, inv) {
  const raw = entityNum(ha, inv.powerId)
  return inv.powerUnit === 'W' ? raw / 1000 : raw
}

function consumptionKw(ha) {
  const t = getInstall()
  let total = 0
  for (const id of t.consumption.powerIds) total += entityNum(ha, id, t.consumption.powerUnit === 'W' ? 1000 : 1)
  return total
}

// Potencia de la batería en kW con signo (positivo=cargan, negativo=descargan).
// El estado se interpreta por SINÓNIMOS configurables (topología.battery.
// chargingStates/dischargingStates): HAOS puede reportar 'Cargando'/'charging'
// según el idioma de la instalación. Sin estado reconocible → 0.
function batteryPowerKw(ha, batMag) {
  const b = getInstall().battery
  if (!b.enabled) return 0
  const state = ha.getState(b.stateId)?.state || ''
  if (b.chargingStates.includes(state)) return batMag
  if (b.dischargingStates.includes(state)) return -batMag
  return 0
}

export function computeLive(ha) {
  const t = getInstall()
  const ENTITIES = getEntities()

  let production = 0
  const invLive = []
  for (const inv of t.inverters) {
    let kw = inverterPowerKw(ha, inv)
    if (kw < 0.02) kw = 0
    production += kw
    invLive.push({ key: inv.key, name: inv.name, kw: round3(kw) })
  }
  const consumption = consumptionKw(ha)

  const b = t.battery
  const batMag = b.enabled ? entityNum(ha, b.powerId) : 0
  const batteryPower = batteryPowerKw(ha, batMag)
  const soc = b.enabled ? entityNum(ha, b.socId) : 0
  const batState = b.enabled ? (ha.getState(b.stateId)?.state || '') : ''

  // Grid: fuente configurable. 'attrs' → atributos de un sensor HAOS con
  // potencia/dirección en attrs (caso del sensor del scraper Solis). 'sensor'
  // → estados: sensor plano con signo o pares import/export.
  let grid = 0
  if (t.grid.mode === 'attrs' && t.grid.attrsId) {
    const attrs = ha.getState(t.grid.attrsId)
    const gridMag = Math.abs(num(attrs?.attributes?.currentGridPower))
    const gridDir = attrs?.attributes?.gridDirection || 'none'
    grid = gridDir === 'import' ? gridMag : gridDir === 'export' ? -gridMag : 0
  } else {
    if (t.grid.sensorId) {
      grid = entityNum(ha, t.grid.sensorId) // signo: + import / - export
    } else {
      const imp = entityNum(ha, t.grid.importId)
      const exp = entityNum(ha, t.grid.exportId)
      grid = imp > 0 ? imp : -exp
    }
  }

  const sun = ha.getState(t.sun)
  const weather = ha.getState(t.weather)
  const elevation = num(sun?.attributes?.elevation)

  // Sanity check: de noche sin sol, no puede haber export solar
  if (elevation < 0 && grid < -0.03 && production < 0.02) grid = -grid

  // Fallback: si no hay datos de grid pero hay consumo y no hay producción, estimar import
  if (grid === 0 && consumption > 0.1 && production < 0.02) grid = consumption

  const alerts = []
  if (!ha.connected) alerts.push({ id: 'haos', severity: 'critical', text: 'Sin conexión con Home Assistant' })

  // Estado del inversor: desde statusAttrsId (opcional). Si no hay sensor con
  // esos atributos, no hay alerta de inversor/scraper (genérico). Textos sin
  // marca: el nombre real vive en la topología, no aquí (issue #39).
  const statusAttrs = t.statusAttrsId ? ha.getState(t.statusAttrsId)?.attributes || {} : {}
  if (t.statusAttrsId && statusAttrs.inverterOnline === 0)
    alerts.push({ id: 'inversor', severity: 'critical', text: 'Inversor offline' })
  const lastUpd = statusAttrs.lastUpdate ? new Date(statusAttrs.lastUpdate).getTime() : null
  if (t.statusAttrsId && lastUpd && (Date.now() - lastUpd) / 60000 > 15)
    alerts.push({
      id: 'scraper',
      severity: 'warning',
      text: `Datos de producción antiguos (hace ${Math.round((Date.now() - lastUpd) / 60000)} min)`,
    })

  if (soc > 0 && soc <= 20) alerts.push({ id: 'bateria', severity: 'info', text: `Batería en reserva (${round1(soc)}%)` })
  if (elevation > 15 && production < 0.1)
    alerts.push({ id: 'sin_pv', severity: 'warning', text: 'Sin producción solar a plena luz' })

  const now = new Date()
  const inv0 = invLive[0] || { kw: 0 }
  const inv1 = invLive[1] || { kw: 0 }
  return {
    production: round3(production),
    consumption: round3(consumption),
    respaldoKw: round3(entityNum(ha, t.consumption.respaldoId, t.consumption.powerUnit === 'W' ? 1000 : 1)),
    noRespaldadaKw: round3(entityNum(ha, t.consumption.noRespaldadaId, t.consumption.powerUnit === 'W' ? 1000 : 1)),
    batteryPower: round3(batteryPower),
    batteryStatus: batState || 'Desconocido',
    soc: round1(soc),
    grid: round3(grid),
    solis: round3(inv0.kw),
    fox: round3(inv1.kw),
    inverters: invLive,
    at: now.getHours() * 60 + now.getMinutes(),
    alerts,
    sun: {
      state: sun?.state || 'unknown',
      elevation,
      nextRising: sun?.attributes?.next_rising || null,
      nextSetting: sun?.attributes?.next_setting || null,
    },
    weather: weather?.state || 'unknown',
    weatherTemp: entityNum(ha, t.weatherTemp),
    station: t.statusAttrsId ? ha.getState(t.statusAttrsId)?.attributes?.stationName || '' : '',
    inverterOnline: t.statusAttrsId ? ha.getState(t.statusAttrsId)?.attributes?.inverterOnline === 1 : true,
    ts: now.toISOString(),
  }
}

const CONSUMPTION_COUNTER_IDS = () => getInstall().consumption.energyIds

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
  const ids = CONSUMPTION_COUNTER_IDS()
  const stats = await ha.statisticsDuringPeriod({
    startTime: start.toISOString(),
    endTime: end.toISOString(),
    statisticIds: ids,
    period: 'day',
    types: ['state'],
  })
  const values = {}
  for (const id of ids) {
    const rows = stats[id] || []
    values[id] = rows.length ? num(rows[rows.length - 1].state) : null
  }
  const b = { date: today, values }
  kvSet(db, 'cons_baseline', JSON.stringify(b))
  return b
}

export function consumptionTodayFromCounters(ha, baseline) {
  const ids = CONSUMPTION_COUNTER_IDS()
  let total = 0
  let usable = false
  for (const id of ids) {
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
  const E = getEnergyEntities()
  const t = getInstall()
  const g = (id) => (id ? entityNum(ha, id) : 0)
  let production = 0
  const perInv = {}
  for (const inv of t.inverters) {
    const k = g(inv.energyId)
    perInv[inv.key] = k
    production += k
  }
  const gridImport = g(E.gridImport)
  const gridExport = g(E.gridExport)
  const batCharge = g(E.batCharge)
  const batDischarge = g(E.batDischarge)
  const medidores = consumptionOverride !== null && consumptionOverride !== undefined ? consumptionOverride : 0
  const derived = production + gridImport - gridExport - batCharge + batDischarge
  const consumption = Math.max(medidores, derived, medidores > 0 ? 0 : g(E.consumption))
  return {
    productionKwh: round2(production),
    solisKwh: round2(perInv[t.inverters[0]?.key] || 0),
    foxKwh: round2(perInv[t.inverters[1]?.key] || 0),
    invertersKwh: Object.fromEntries(Object.entries(perInv).map(([k, v]) => [k, round2(v)])),
    consumptionKwh: round2(Math.min(100, consumption)),
    gridImportKwh: round2(gridImport),
    gridExportKwh: round2(gridExport),
    batteryChargedKwh: round2(batCharge),
    batteryDischargedKwh: round2(batDischarge),
  }
}

async function getDaySeriesUncached(ha, dateStr, db) {
  const t = getInstall()
  const day = dateStr ? new Date(dateStr + 'T00:00:00') : new Date()
  if (Number.isNaN(day.getTime())) throw new Error('fecha inválida')
  const start = new Date(day)
  start.setHours(0, 0, 0, 0)
  const end = new Date(start)
  end.setDate(end.getDate() + 1)

  const invIds = t.inverters.map((inv) => inv.powerId).filter(Boolean)
  const ids = [...invIds, ...t.consumption.powerIds]
  if (t.battery.enabled) {
    if (t.battery.powerId) ids.push(t.battery.powerId)
    if (t.battery.socId) ids.push(t.battery.socId)
  }
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
      const tm = new Date(row.start)
      const min = tm.getHours() * 60 + tm.getMinutes()
      if (!buckets.has(min)) buckets.set(min, {})
      buckets.get(min)[id] = row.mean
    }
  }

  const points = []
  let prevSoc = null
  const minutes = [...buckets.keys()].sort((a, b) => a - b)
  const consDiv = t.consumption.powerUnit === 'W' ? 1000 : 1
  for (const min of minutes) {
    const b = buckets.get(min)
    let production = 0
    const perInv = {}
    for (const inv of t.inverters) {
      let kw = num(b[inv.powerId]) / (inv.powerUnit === 'W' ? 1000 : 1)
      if (kw < 0.02) kw = 0
      perInv[inv.key] = kw
      production += kw
    }
    const consumption = t.consumption.powerIds.reduce((acc, id) => acc + num(b[id]) / consDiv, 0)

    let batteryPower = 0
    if (t.battery.enabled) {
      const batMag = num(b[t.battery.powerId])
      const soc = num(b[t.battery.socId])
      if (prevSoc !== null && batMag > 0.01) {
        const socDelta = soc - prevSoc
        if (socDelta > 0.02) batteryPower = batMag
        else if (socDelta < -0.02) batteryPower = -batMag
      }
      if (soc > 0) prevSoc = soc
    }
    const soc = t.battery.enabled ? num(b[t.battery.socId]) : 0

    let grid = consumption - production + batteryPower
    if (Math.abs(grid) < 0.03) grid = 0

    const hh = String(Math.floor(min / 60)).padStart(2, '0')
    const mm = String(min % 60).padStart(2, '0')
    const inv0 = perInv[t.inverters[0]?.key] || 0
    const inv1 = perInv[t.inverters[1]?.key] || 0
    points.push({
      t: min,
      label: `${hh}:${mm}`,
      solis: round3(inv0),
      fox: round3(inv1),
      inverters: Object.fromEntries(Object.entries(perInv).map(([k, v]) => [k, round3(v)])),
      production: round3(production),
      consumption: round3(consumption),
      batteryPower: round3(batteryPower),
      soc: round1(soc),
      grid: round3(grid),
    })
  }

  const dtH = 5 / 60
  const sumInv = (key) => points.reduce((acc, p) => acc + (p.inverters[key] || 0) * dtH, 0)
  let estimated = false
  // Estimación de curva: si el inversor 0 no tiene statistics pero el total
  // diario sí, se escala la curva del resto (fallback típico de sensores sin
  // state_class). Se mantiene el heurístico original Solis→Fox.
  if (db && points.length) {
    const inv0Key = t.inverters[0]?.key
    const inv1Key = t.inverters[1]?.key
    const sum0 = sumInv(inv0Key)
    const sum1 = inv1Key ? sumInv(inv1Key) : 0
    if (sum0 < 0.5 && sum1 > 1 && inv0Key && inv1Key) {
      const row = db.prepare('SELECT solis_kwh, fox_kwh FROM daily WHERE date = ?').get(dateStr || todayStr())
      if (row && row.solis_kwh > 1 && sum1 > 1) {
        const ratio = row.solis_kwh / sum1
        for (const p of points) {
          p.inverters[inv0Key] = round3(p.inverters[inv1Key] * ratio)
          p.solis = p.inverters[inv0Key]
          p.production = round3(p.inverters[inv0Key] + (p.inverters[inv1Key] || 0))
          let grid = p.consumption - p.production + p.batteryPower
          if (Math.abs(grid) < 0.03) grid = 0
          p.grid = round3(grid)
        }
        estimated = true
      }
    }
  }
  return { points, estimated }
}

// Serie del día cacheada: hoy TTL 60 s (cambia con cada estadística de 5 min),
// días pasados TTL 6 h (inmutables salvo recálculo de backfill nocturno).
export const getDaySeries = cachedCollector(
  (_ha, dateStr) => dateStr || todayStr(),
  (_ha, dateStr) => ((dateStr || todayStr()) === todayStr() ? 60_000 : 6 * 3600_000),
  getDaySeriesUncached
)

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
    const invertersKwh = parseInvertersKwh(row)
    return decorateKpis({
      productionKwh: round2(row.production_kwh || 0),
      solisKwh: round2(row.solis_kwh || 0),
      foxKwh: round2(row.fox_kwh || 0),
      invertersKwh,
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

  const E = getEnergyEntities()
  const t = getInstall()
  const ids = Object.values(E).filter(Boolean)
  const stats = await ha.statisticsDuringPeriod({
    startTime: start.toISOString(),
    endTime: end.toISOString(),
    statisticIds: ids,
    period: 'day',
    types: ['state'],
  })

  const change = (id) => {
    const rows = stats[id] || []
    let total = 0
    for (const r of rows) total += num(r.state)
    return total
  }

  const inv0Kwh = change(E.solis)
  const inv1Kwh = change(E.fox)
  const invertersKwh = {}
  t.inverters.forEach((inv, i) => {
    invertersKwh[inv.key] = i === 0 ? inv0Kwh : i === 1 ? inv1Kwh : change(inv.energyId)
  })

  return decorateKpis({
    productionKwh: round2(inv0Kwh + inv1Kwh),
    solisKwh: round2(inv0Kwh),
    foxKwh: round2(inv1Kwh),
    invertersKwh,
    consumptionKwh: round2(change(E.consumption)),
    gridImportKwh: round2(change(E.gridImport)),
    gridExportKwh: round2(change(E.gridExport)),
    batteryChargedKwh: round2(change(E.batCharge)),
    batteryDischargedKwh: round2(change(E.batDischarge)),
    soc,
    peakProductionKw: round3(peakProductionKw),
    peakAt,
  })
}

function parseInvertersKwh(row) {
  if (row.inverters_kwh) {
    try {
      return JSON.parse(row.inverters_kwh)
    } catch {}
  }
  const out = {}
  if (row.solis_kwh !== null && row.solis_kwh !== undefined) out.solis = row.solis_kwh
  if (row.fox_kwh !== null && row.fox_kwh !== undefined) out.fox = row.fox_kwh
  return out
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

export async function backfillHistory(ha, db) {
  const t = getInstall()
  const end = new Date()
  end.setDate(end.getDate() + 1)
  const start = new Date('2024-01-01T00:00:00')
  const queryStart = new Date(start)
  queryStart.setDate(queryStart.getDate() - 2)
  const { srcs, map } = deepSourceDailyKeys()
  const ids = Object.values(srcs).flatMap((s) => s.ids)
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

  for (const [field, src] of Object.entries(srcs)) {
    const perIdDaily = new Map()
    for (const id of src.ids) {
      const rows = (stats[id] || []).filter((r) => (src.acc === 'sum' ? r.sum : r.state) !== null && (src.acc === 'sum' ? r.sum : r.state) !== undefined)
      rows.sort((a, b) => a.start - b.start)
      const daily = new Map()
      let prev = 0
      let prevKey = null // fecha de la fila anterior (para detectar huecos)
      for (const row of rows) {
        const key = dateKey(new Date(row.start))
        const raw = src.acc === 'sum' ? row.sum : row.state
        if (key === todayStr()) {
          prev = raw
          prevKey = key
          continue
        }
        if (src.acc === 'sum') {
          const value = Math.max(0, raw - prev)
          prev = raw
          prevKey = key
          if (key < minKey) continue
          daily.set(key, value)
          continue
        }
        // acc:'state': acumulador creciente. El delta cubre desde prevKey
        // hasta key; si faltan días entre ambos (hueco de datos en HAOS), el
        // delta se REPARTE uniformemente entre los días sin fila + el día de
        // la fila. Sin esto, un hueco de N días concentra N días de consumo
        // en el día posterior (bug de backfill, caza de bugs 6-Ago-2026).
        const invKey = map[field]
        const glitch = invKey && t.inverters.find((i) => i.key === invKey)?.glitchOffsets?.[key] || 0
        let delta = Math.max(0, raw - prev - glitch)
        const gapDays = gapBetween(prevKey, key) // días intermedios sin fila
        if (prevKey && gapDays > 0) {
          const share = delta / (gapDays + 1)
          for (let d = 1; d <= gapDays; d++) {
            const gk = dateKey(addDays(new Date(key + 'T00:00:00'), -d))
            if (gk >= minKey && gk !== todayStr()) daily.set(gk, Math.max(daily.get(gk) || 0, share))
          }
          delta = share
        }
        prev = raw
        prevKey = key
        if (key < minKey) continue
        daily.set(key, Math.max(daily.get(key) || 0, delta))
      }
      perIdDaily.set(id, daily)
    }

    const allKeys = new Set()
    for (const daily of perIdDaily.values()) for (const key of daily.keys()) allKeys.add(key)

    const outKey = map[field] || field
    for (const key of allKeys) {
      if (src.requireAll && ![...perIdDaily.values()].every((d) => d.has(key))) continue
      let value = 0
      for (const daily of perIdDaily.values()) value += daily.get(key) || 0
      if (src.cap && value > src.cap) {
        console.warn(`[helios] ${outKey} diario anómalo ${key}: ${value} kWh, capado a ${src.cap}`)
        value = src.cap
      }
      const cur = ensure(key)[outKey]
      ensure(key)[outKey] = Math.max(cur === undefined ? -Infinity : cur, value)
    }
  }

  let n = 0
  // Columna por inversor (índice): los 2 primeros se escriben en solis/fox
  // (compat); el resto en invN. deepSourceDailyKeys mapea igual.
  const colForIndex = (i) => (i === 0 ? 'solis' : i === 1 ? 'fox' : `inv${i + 1}`)
  for (const [date, f] of [...byDate.entries()].sort()) {
    const perInv = {}
    let production = 0
    for (const [k, src] of Object.entries(srcs)) {
      if (!k.startsWith('inv')) continue
      const col = map[k]
      perInv[col] = f[col] || 0
      production += perInv[col]
    }
    const medidores = f.consumption || 0
    const derived = production + (f.gridImport || 0) - (f.gridExport || 0) - (f.batCharge || 0) + (f.batDischarge || 0)
    const consumption = medidores > 0 ? Math.min(100, Math.max(medidores, derived)) : 0
    // inverters_kwh guarda por clave real del inversor (issue #37).
    const invertersKwh = {}
    t.inverters.forEach((inv, i) => {
      invertersKwh[inv.key] = round2(perInv[colForIndex(i)] || 0)
    })
    upsertDaily(db, {
      date,
      production_kwh: round2(production),
      consumption_kwh: round2(consumption),
      grid_import_kwh: round2(f.gridImport || 0),
      grid_export_kwh: round2(f.gridExport || 0),
      battery_charged_kwh: round2(f.batCharge || 0),
      battery_discharged_kwh: round2(f.batDischarge || 0),
      solis_kwh: round2(perInv.solis || 0),
      fox_kwh: round2(perInv.fox || 0),
      inverters_kwh: JSON.stringify(invertersKwh),
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

// gapBetween devuelve los días intermedios SIN fila entre dos fechas
// consecutivas del acumulador (prevKey < key). null/igual → 0.
function gapBetween(prevKey, key) {
  if (!prevKey || prevKey >= key) return 0
  const a = new Date(prevKey + 'T00:00:00')
  const b = new Date(key + 'T00:00:00')
  return Math.round((b - a) / 86400000) - 1
}

// addDays suma días a una fecha (para repartir el delta del hueco hacia atrás).
function addDays(d, n) {
  const out = new Date(d)
  out.setDate(out.getDate() + n)
  return out
}

const round1 = (n) => Math.round(n * 10) / 10
const round2 = (n) => Math.round(n * 100) / 100
const round3 = (n) => Math.round(n * 1000) / 1000

// extensions.js — marco de extensiones (issue #94).
//
// Las extensiones son módulos opcionales que amplían la app con vistas y
// sensores propios. La primera: el cargador del coche eléctrico.
//
// Resolución (misma filosofía que install.js):
//   1. `extensions_config` en kv → configuración explícita del admin
//      (editable por API/UI desde Ajustes).
//   2. Instalación existente (daily con filas) → perfil LEGACY con las
//      entidades típicas de un cargador local, extensión APAGADA.
//   3. Instalación nueva (daily vacío) → placeholders genéricos, APAGADA.
//
// El interruptor maestro (`enabled`) gobierna todo el marco: si está apagado,
// ninguna extensión corre y no aparecen menús extra. Cada extensión tiene su
// propio `enabled` (se enciende con el marco activo).
//
// El estado resuelto se instala con resolveAndSet(db) al arrancar. Guardar
// cambios (PUT /api/extensions) re-resuelve en memoria, pero las suscripciones
// de entidades con HAOS se fijan al arrancar → el cambio pleno requiere
// reinicio (mismo contrato que la topología).

import { kvGet, kvSet } from './db.js'
import { getInstall } from './install.js'
import { cachedCollector } from './solar.js'

const num = (v) => {
  const n = parseFloat(v)
  return Number.isFinite(n) ? n : 0
}

const round3 = (v) => Math.round(v * 1000) / 1000

const CHARGER_GLITCH_KWH = 50 // salto imposible entre ticks de 60 s
const STEP_MIN = 5

// ── Perfil LEGACY: instalación existente → cargador local típico (localtuya).
export const LEGACY_EXTENSIONS = {
  enabled: false,
  carCharger: {
    enabled: false,
    name: 'Cargador coche',
    powerId: 'sensor.cargador_coche_potencia',
    // ⚠ Verificado contra el recorder (14-Ago-2026): el DPS reporta W aunque
    // la entidad localtuya diga kW (2605 "kW" durante carga = 2,6 kW reales).
    powerUnit: 'W',
    energyTotalId: 'sensor.cargador_coche_energia_total',
    // El contador va en centésimas de kWh (DPS 1 qccdz): 469 = 4,69 kWh.
    energyDivisor: 100,
    energySessionId: 'sensor.cargador_coche_energia_sesion',
    stateId: 'sensor.cargador_coche_estado',
    tempId: 'sensor.cargador_coche_temperatura',
    switchId: 'switch.cargador_coche',
    chargingStates: ['charger_charging', 'charging'],
    connectedStates: ['charger_insert', 'charger_charging', 'charging'],
  },
}

// ── Perfil GENÉRICO: instalación nueva. El admin lo completa en Ajustes.
export const GENERIC_EXTENSIONS = {
  enabled: false,
  carCharger: {
    enabled: false,
    name: 'Car charger',
    powerId: 'sensor.charger_power',
    powerUnit: 'kW',
    energyTotalId: 'sensor.charger_energy_total',
    energyDivisor: 1,
    energySessionId: '',
    stateId: 'sensor.charger_state',
    tempId: '',
    switchId: '',
    chargingStates: ['charging', 'charger_charging'],
    connectedStates: ['charger_insert', 'connected', 'charging', 'charger_charging'],
  },
}

// ── Estado resuelto (módulo) ────────────────────────────────────────────────
let current = GENERIC_EXTENSIONS

export function resolveAndSet(db) {
  current = resolveExtensions(db)
  return current
}

export function getExtensions() {
  return current
}

// --- Tests: inyectar extensiones sin BD --------------------------------------
export function _setForTests(ext) {
  current = ext
  return current
}

export function resolveExtensions(db) {
  const raw = kvGet(db, 'extensions_config')
  if (raw) {
    try {
      const cfg = JSON.parse(raw)
      return normalizeExtensions(cfg)
    } catch {
      /* config corrupta → fallback al perfil base */
    }
  }
  const hasDaily = db.prepare('SELECT COUNT(*) AS n FROM daily').get().n > 0
  return hasDaily ? LEGACY_EXTENSIONS : GENERIC_EXTENSIONS
}

// Normaliza una config escrita por el admin sobre el perfil genérico. No
// lanza: tipos inesperados revierten a valores seguros.
export function normalizeExtensions(cfg, base = GENERIC_EXTENSIONS) {
  const out = {
    enabled: typeof cfg.enabled === 'boolean' ? cfg.enabled : base.enabled,
    carCharger: { ...base.carCharger, ...(cfg.carCharger || {}) },
  }
  const c = out.carCharger
  c.enabled = typeof c.enabled === 'boolean' ? c.enabled : false
  c.name = typeof c.name === 'string' && c.name.trim() ? c.name.trim() : base.carCharger.name
  c.powerUnit = c.powerUnit === 'W' ? 'W' : 'kW'
  c.energyDivisor = Number.isInteger(c.energyDivisor) && c.energyDivisor >= 1 ? c.energyDivisor : 1
  c.chargingStates = Array.isArray(c.chargingStates) ? c.chargingStates.filter((s) => typeof s === 'string') : []
  c.connectedStates = Array.isArray(c.connectedStates) ? c.connectedStates.filter((s) => typeof s === 'string') : []
  for (const k of ['powerId', 'energyTotalId', 'energySessionId', 'stateId', 'tempId', 'switchId']) {
    if (typeof c[k] !== 'string') c[k] = ''
  }
  return out
}

// ── Cargador: estado y suscripciones ─────────────────────────────────────────

/** ¿El marco está activo Y la extensión del cargador encendida? */
export function chargerActive(ext = getExtensions()) {
  return !!(ext.enabled && ext.carCharger && ext.carCharger.enabled)
}

/** IDs para subscribe_entities cuando el cargador está activo. */
export function chargerEntities(ext = getExtensions()) {
  if (!chargerActive(ext)) return []
  const c = ext.carCharger
  return [...new Set([c.powerId, c.energyTotalId, c.energySessionId, c.stateId, c.tempId, c.switchId].filter(Boolean))]
}

// Lectura numérica de un estado HA: undefined si no hay entidad o está
// unavailable/unknown (así la UI distingue "sin dato" de "0").
function entityNumOrUndef(ha, id) {
  if (!id) return undefined
  const e = ha.getState(id)
  if (!e || e.state === 'unavailable' || e.state === 'unknown') return undefined
  const n = parseFloat(e.state)
  return Number.isFinite(n) ? n : undefined
}

/** Divide un valor leído por el divisor del contador (undefined lo respeta). */
function divOpt(v, divisor) {
  return v === undefined ? undefined : round3(v / (divisor || 1))
}

function entityStateOrUndef(ha, id) {
  if (!id) return undefined
  const e = ha.getState(id)
  if (!e || e.state === 'unavailable' || e.state === 'unknown') return undefined
  return e.state
}

/** Snapshot en vivo del cargador (viaja en el SSE `live`). */
export function computeChargerLive(ha, ext = getExtensions()) {
  if (!chargerActive(ext)) return undefined
  const c = ext.carCharger
  const rawPower = entityNumOrUndef(ha, c.powerId)
  const powerKw =
    rawPower === undefined ? undefined : c.powerUnit === 'W' ? round3(rawPower / 1000) : round3(rawPower)
  const state = entityStateOrUndef(ha, c.stateId)
  // "Cargando": estado reconocido O potencia significativa (fallback robusto
  // ante enums distintos por instalación; mismo umbral que la batería).
  const charging = (state !== undefined && c.chargingStates.includes(state)) || (powerKw !== undefined && powerKw > 0.05)
  const connected = charging || (state !== undefined && c.connectedStates.includes(state))
  const sw = entityStateOrUndef(ha, c.switchId)
  return {
    name: c.name,
    charging,
    connected,
    state,
    powerKw: powerKw ?? 0,
    sessionKwh: divOpt(entityNumOrUndef(ha, c.energySessionId), c.energyDivisor),
    totalKwh: divOpt(entityNumOrUndef(ha, c.energyTotalId), c.energyDivisor),
    tempC: entityNumOrUndef(ha, c.tempId),
    switchOn: sw === undefined ? undefined : sw === 'on',
  }
}

// ── Cargador: energía diaria propia ──────────────────────────────────────────
// Los contadores de energía de cargadores locales suelen exponerse SIN
// state_class → sin statistics en HAOS. Helios acumula sus propios deltas por
// día en daily.ext_charger_kwh (v5) a partir del contador total:
//   - La base del contador persiste en kv (`charger_counter`) → los reinicios
//     del servidor no pierden el hilo.
//   - Bajada del contador (reset del equipo) → nueva base, sin sumar.
//   - Salto imposible (> CHARGER_GLITCH_KWH entre ticks) → se ignora y se
//     re-basa (los huecos los cubre el backfill desde el recorder de HAOS).
//
// Además atribuye el ORIGEN de cada delta (v6, ext_charger_pv_kwh): la fracción
// que en ese instante pudo venir de excedente FV. Balance por slot: solar al
// cargador = min(potencia cargador, max(0, producción − consumoSinCargador)).
// Lo que no es FV directo se etiqueta "red y batería" (una carga nocturna desde
// batería cuenta como no solar: honesto y simple).

function todayStr() {
  const d = new Date()
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function dateKey(d) {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

/** Suma `kwh` al día (crea la fila con ceros si no existe). `pvKwh` (0..kwh)
 *  es la fracción de origen solar; se acumula clampeada al total del día. */
export function addChargerKwh(db, date, kwh, pvKwh = 0) {
  const pv = round3(Math.max(0, Math.min(pvKwh, kwh)))
  db.prepare(
    `INSERT INTO daily (date, production_kwh, consumption_kwh, grid_import_kwh, grid_export_kwh, battery_charged_kwh, battery_discharged_kwh, ext_charger_kwh, ext_charger_pv_kwh)
     VALUES (?, 0, 0, 0, 0, 0, 0, ?, ?)
     ON CONFLICT(date) DO UPDATE SET
       ext_charger_kwh = COALESCE(ext_charger_kwh, 0) + excluded.ext_charger_kwh,
       ext_charger_pv_kwh = MIN(COALESCE(ext_charger_pv_kwh, 0) + excluded.ext_charger_pv_kwh, COALESCE(ext_charger_kwh, 0) + excluded.ext_charger_kwh)`
  ).run(date, round3(kwh), pv)
}

/** Fija (sin sumar) el kWh de un día SOLO si estaba a NULL (backfill). */
export function setChargerKwhIfNull(db, date, kwh) {
  db.prepare('UPDATE daily SET ext_charger_kwh = ? WHERE date = ? AND ext_charger_kwh IS NULL').run(round3(kwh), date)
}

/** Fracción [0..1] del delta que pudo venir de excedente FV en este instante.
 *  Balance por slot (la topología de consumo incluye el circuito del cargador):
 *  solar → cargador = min(potCargador, max(0, producción − (consumo − potCargador))).
 *  Sin lectura de potencia del cargador → reparto proporcional producción/consumo. */
export function pvShareNow(ha, chargerPowerKw) {
  const t = getInstall()
  let production = 0
  for (const inv of t.inverters) {
    const raw = entityNumOrUndef(ha, inv.powerId)
    if (raw === undefined) continue
    production += inv.powerUnit === 'W' ? raw / 1000 : raw
  }
  const consDiv = t.consumption.powerUnit === 'W' ? 1000 : 1
  let consumption = 0
  for (const id of t.consumption.powerIds) {
    const raw = entityNumOrUndef(ha, id)
    if (raw !== undefined) consumption += raw / consDiv
  }
  if (chargerPowerKw !== undefined && chargerPowerKw > 0.05) {
    const other = Math.max(0, consumption - chargerPowerKw)
    const solarToCharger = Math.min(chargerPowerKw, Math.max(0, production - other))
    return Math.max(0, Math.min(1, solarToCharger / chargerPowerKw))
  }
  if (consumption > 0.01) return Math.max(0, Math.min(1, production / consumption))
  return production > 0 ? 1 : 0
}

/** Tick de acumulación (llamar cada ~60 s con HAOS conectado). */
export function accumulateChargerDaily(ha, db, ext = getExtensions()) {
  if (!chargerActive(ext)) return
  const c = ext.carCharger
  if (!c.energyTotalId) return
  const total = divOpt(entityNumOrUndef(ha, c.energyTotalId), c.energyDivisor)
  if (total === undefined) return

  let saved = null
  const raw = kvGet(db, 'charger_counter')
  if (raw) {
    try {
      saved = JSON.parse(raw)
    } catch {
      /* kv corrupto → re-basar */
    }
  }

  if (!saved || typeof saved.total !== 'number' || saved.total > total) {
    // Primera vez, kv perdido o reset del contador: nueva base, sin sumar.
    kvSet(db, 'charger_counter', JSON.stringify({ date: todayStr(), total }))
    return
  }

  const delta = total - saved.total
  if (delta > 0 && delta <= CHARGER_GLITCH_KWH) {
    const rawPower = entityNumOrUndef(ha, c.powerId)
    const powerKw = rawPower === undefined ? undefined : c.powerUnit === 'W' ? rawPower / 1000 : rawPower
    addChargerKwh(db, todayStr(), delta, delta * pvShareNow(ha, powerKw))
  } else if (delta > CHARGER_GLITCH_KWH) {
    console.warn(`[helios] cargador: salto anómalo ${delta} kWh, ignorado (¿glitch o sustitución del contador?)`)
  }
  if (delta !== 0) kvSet(db, 'charger_counter', JSON.stringify({ date: todayStr(), total }))
}

/** Histórico diario del cargador (kWh por día + fracción solar) desde daily.
 *  Los días acumulados antes de la atribución (o con pv NULL) se estiman por
 *  balance diario: solar = min(kwh, max(0, producción − (consumo − kwh))). */
export function chargerHistory(db, from, to) {
  return db
    .prepare(
      `SELECT date, ext_charger_kwh, ext_charger_pv_kwh, production_kwh, consumption_kwh
       FROM daily WHERE date >= ? AND date <= ? ORDER BY date ASC`
    )
    .all(from, to)
    .map((r) => {
      const kwh = r.ext_charger_kwh ?? null
      let pvKwh = r.ext_charger_pv_kwh ?? null
      if (kwh !== null && kwh > 0 && pvKwh === null) {
        const other = Math.max(0, (r.consumption_kwh ?? 0) - kwh)
        pvKwh = Math.max(0, Math.min(kwh, (r.production_kwh ?? 0) - other))
      }
      return { date: r.date, kwh, pvKwh }
    })
}

// ── Cargador: backfill y curva desde el recorder de HAOS ─────────────────────

// Deltas diarios de una serie de estados de un contador creciente.
// `rows`: [{ state, last_changed }] (historial REST de HAOS, en orden).
// `divisor`: unidades del contador → kWh (DPS Tuya suele ir en centésimas).
// Suma los INCREMENTOS POSITIVOS dentro de cada día: un reset del contador
// (bajada) no resta ni suma, y los saltos imposibles (> CHARGER_GLITCH_KWH
// en kWh) se ignoran como glitch. Devuelve Map<dateKey, kWh>.
export function dailyDeltasFromHistory(rows, divisor = 1) {
  const perDay = new Map()
  let prevKey = null
  let prevV = 0
  for (const r of rows) {
    const v = parseFloat(r.state)
    if (!Number.isFinite(v)) continue // unavailable/unknown
    const key = dateKey(new Date(r.last_changed))
    if (key !== prevKey) {
      prevKey = key
      prevV = v
      if (!perDay.has(key)) perDay.set(key, 0)
      continue // el valor inicial del día es la base, no un delta
    }
    const diffKwh = (v - prevV) / (divisor || 1)
    if (diffKwh > 0) {
      if (diffKwh <= CHARGER_GLITCH_KWH) perDay.set(key, round3((perDay.get(key) || 0) + diffKwh))
      else console.warn(`[helios] cargador: salto anómalo ${diffKwh} kWh ignorado en backfill`)
    }
    prevV = v
  }
  return perDay
}

/** Rellena (solo días a NULL, HOY incluida) desde el recorder de HAOS.
 *  Ventana ~9 días (retención típica del recorder). Incluir hoy es seguro:
 *  el acumulador en vivo siembra su base con el contador ACTUAL tras el
 *  backfill, así no hay doble conteo del tramo ya cubierto.
 *  Devuelve días escritos. */
export async function backfillChargerHistory(ha, db, ext = getExtensions()) {
  if (!chargerActive(ext)) return 0
  const c = ext.carCharger
  if (!c.energyTotalId) return 0

  const today = todayStr()
  const from = new Date()
  from.setDate(from.getDate() - 9)
  const fromKey = dateKey(from)

  const missing = db
    .prepare('SELECT date FROM daily WHERE date >= ? AND date <= ? AND ext_charger_kwh IS NULL ORDER BY date ASC')
    .all(fromKey, today)
    .map((r) => r.date)
  if (!missing.length) return 0

  const startTime = new Date(fromKey + 'T00:00:00').toISOString()
  const endTime = new Date().toISOString()
  const rows = await ha.historyDuringPeriod({ startTime, endTime, entityId: c.energyTotalId })
  if (!Array.isArray(rows) || rows.length === 0) return 0

  const perDay = dailyDeltasFromHistory(rows, c.energyDivisor)
  // Primer día cubierto por el recorder: los días sin estados a partir de ahí
  // son días SIN carga (contador estático) → 0, no NULL (evita re-consultar).
  const firstRowKey = dateKey(new Date(rows.find((r) => Number.isFinite(parseFloat(r.state)))?.last_changed || 0))
  let n = 0
  for (const date of missing) {
    if (perDay.has(date)) {
      setChargerKwhIfNull(db, date, perDay.get(date))
      n++
    } else if (date >= firstRowKey) {
      // Día dentro de la ventana del recorder sin estados: contador estático
      // → 0 kWh (hoy incluida: aún sin carga; el acumulador suma encima).
      setChargerKwhIfNull(db, date, 0)
      n++
    }
  }

  // HOY sin fila en daily (la consolidación solar la crea de madrugada): no se
  // escribe directo (una fila con producción 0 ensuciaría el histórico solar).
  // En su lugar se siembra la base del acumulador con el PRIMER estado de hoy
  // y el tick de 60 s aplica el delta del día (addChargerKwh ya crea filas).
  const hasTodayRow = db.prepare('SELECT 1 AS x FROM daily WHERE date = ?').get(today)
  if (!hasTodayRow && !kvGet(db, 'charger_counter')) {
    const firstToday = rows.find(
      (r) => Number.isFinite(parseFloat(r.state)) && dateKey(new Date(r.last_changed)) === today
    )
    if (firstToday) {
      kvSet(db, 'charger_counter', JSON.stringify({ date: today, total: round3(parseFloat(firstToday.state) / (c.energyDivisor || 1)) }))
      console.log('[helios] cargador: hoy se aplicará vía acumulador (base sembrada del recorder)')
    }
  }
  return n
}

// Curva del día (kW cada STEP_MIN) por interpolación escalonada del
// historial de estados del sensor de potencia. Cacheada: hoy 60 s, pasado 6 h.
export const chargerDayCurve = cachedCollector(
  (_ha, dateStr) => `charger:${dateStr || todayStr()}`,
  (_ha, dateStr) => ((dateStr || todayStr()) === todayStr() ? 60_000 : 6 * 3600_000),
  async (ha, dateStr, ext = getExtensions()) => {
    if (!chargerActive(ext)) return { points: [] }
    const c = ext.carCharger
    if (!c.powerId) return { points: [] }

    const key = dateStr || todayStr()
    const start = new Date(key + 'T00:00:00')
    const end = new Date(start)
    end.setDate(end.getDate() + 1)
    const rows = await ha.historyDuringPeriod({
      startTime: start.toISOString(),
      endTime: end.toISOString(),
      entityId: c.powerId,
    })
    if (!Array.isArray(rows) || rows.length === 0) return { points: [] }

    // Estados numéricos con su timestamp (ms desde medianoche local).
    const steps = []
    for (const r of rows) {
      const v = parseFloat(r.state)
      if (!Number.isFinite(v)) continue
      const ts = new Date(r.last_changed).getTime()
      if (Number.isNaN(ts)) continue
      const min = Math.max(0, Math.round((ts - start.getTime()) / 60000))
      steps.push({ min, kw: c.powerUnit === 'W' ? v / 1000 : v })
    }
    if (!steps.length) return { points: [] }

    const points = []
    let idx = 0
    for (let min = 0; min < 1440; min += STEP_MIN) {
      while (idx < steps.length - 1 && steps[idx + 1].min <= min) idx++
      if (steps[idx].min > min) break // aún no hay dato en este tramo
      const hh = String(Math.floor(min / 60)).padStart(2, '0')
      const mm = String(min % 60).padStart(2, '0')
      points.push({ t: min, label: `${hh}:${mm}`, kw: round3(Math.max(0, steps[idx].kw)) })
    }
    return { points }
  }
)

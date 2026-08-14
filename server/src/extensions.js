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
    powerUnit: 'kW',
    energyTotalId: 'sensor.cargador_coche_energia_total',
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
    sessionKwh: entityNumOrUndef(ha, c.energySessionId),
    totalKwh: entityNumOrUndef(ha, c.energyTotalId),
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

/** Suma `kwh` al día (crea la fila con ceros si no existe). */
export function addChargerKwh(db, date, kwh) {
  db.prepare(
    `INSERT INTO daily (date, production_kwh, consumption_kwh, grid_import_kwh, grid_export_kwh, battery_charged_kwh, battery_discharged_kwh, ext_charger_kwh)
     VALUES (?, 0, 0, 0, 0, 0, 0, ?)
     ON CONFLICT(date) DO UPDATE SET
       ext_charger_kwh = COALESCE(ext_charger_kwh, 0) + excluded.ext_charger_kwh`
  ).run(date, round3(kwh))
}

/** Fija (sin sumar) el kWh de un día SOLO si estaba a NULL (backfill). */
export function setChargerKwhIfNull(db, date, kwh) {
  db.prepare('UPDATE daily SET ext_charger_kwh = ? WHERE date = ? AND ext_charger_kwh IS NULL').run(round3(kwh), date)
}

/** Tick de acumulación (llamar cada ~60 s con HAOS conectado). */
export function accumulateChargerDaily(ha, db, ext = getExtensions()) {
  if (!chargerActive(ext)) return
  const c = ext.carCharger
  if (!c.energyTotalId) return
  const total = entityNumOrUndef(ha, c.energyTotalId)
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
    addChargerKwh(db, todayStr(), delta)
  } else if (delta > CHARGER_GLITCH_KWH) {
    console.warn(`[helios] cargador: salto anómalo ${delta} kWh, ignorado (¿glitch o sustitución del contador?)`)
  }
  if (delta !== 0) kvSet(db, 'charger_counter', JSON.stringify({ date: todayStr(), total }))
}

/** Histórico diario del cargador (kWh por día) desde la tabla daily. */
export function chargerHistory(db, from, to) {
  return db
    .prepare('SELECT date, ext_charger_kwh FROM daily WHERE date >= ? AND date <= ? ORDER BY date ASC')
    .all(from, to)
    .map((r) => ({ date: r.date, kwh: r.ext_charger_kwh ?? null }))
}

// ── Cargador: backfill y curva desde el recorder de HAOS ─────────────────────

// Deltas diarios de una serie de estados de un contador creciente.
// `rows`: [{ state, last_changed }] (historial REST de HAOS, en orden).
// Devuelve Map<dateKey, kWh> con las guardas de reset y glitch.
export function dailyDeltasFromHistory(rows) {
  const perDay = new Map()
  const first = new Map()
  for (const r of rows) {
    const v = parseFloat(r.state)
    if (!Number.isFinite(v)) continue // unavailable/unknown
    const key = dateKey(new Date(r.last_changed))
    if (!first.has(key)) first.set(key, v)
    const delta = v - first.get(key)
    if (delta > 0 && delta <= CHARGER_GLITCH_KWH) perDay.set(key, round3(delta))
  }
  return perDay
}

/** Rellena (solo días a NULL, anteriores a hoy) desde el recorder de HAOS.
 *  Ventana ~9 días (retención típica del recorder). Devuelve días escritos. */
export async function backfillChargerHistory(ha, db, ext = getExtensions()) {
  if (!chargerActive(ext)) return 0
  const c = ext.carCharger
  if (!c.energyTotalId) return 0

  const today = todayStr()
  const from = new Date()
  from.setDate(from.getDate() - 9)
  const fromKey = dateKey(from)

  const missing = db
    .prepare('SELECT date FROM daily WHERE date >= ? AND date < ? AND ext_charger_kwh IS NULL ORDER BY date ASC')
    .all(fromKey, today)
    .map((r) => r.date)
  if (!missing.length) return 0

  const startTime = new Date(fromKey + 'T00:00:00').toISOString()
  const endTime = new Date(today + 'T00:00:00').toISOString()
  const rows = await ha.historyDuringPeriod({ startTime, endTime, entityId: c.energyTotalId })
  if (!Array.isArray(rows) || rows.length === 0) return 0

  const perDay = dailyDeltasFromHistory(rows)
  let n = 0
  for (const date of missing) {
    const kwh = perDay.get(date)
    if (kwh === undefined) continue
    setChargerKwhIfNull(db, date, kwh)
    n++
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

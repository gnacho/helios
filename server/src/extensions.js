// extensions.js - marco de extensiones (issue 94) + varios cargadores (146).
//
// Las extensiones son modulos opcionales que amplian la app con vistas y
// sensores propios. La primera: el cargador del coche electrico.
//
// Resolucion (misma filosofia que install.js):
//   1. `extensions_config` en kv -> configuracion explicita del admin
//      (editable por API/UI desde Ajustes).
//   2. Instalacion existente (daily con filas) -> perfil LEGACY con las
//      entidades tipicas de un cargador local, extension APAGADA.
//   3. Instalacion nueva (daily vacio) -> placeholders genericos, APAGADA.
//
// Desde el issue 146 carCharger lleva una LISTA de cargadores (`chargers`),
// cada uno con sus entidades y nombre. La config antigua (campos sueltos en
// carCharger) se migra al primer cargador (key 'main') sin perder nada.
//
// El interruptor maestro (`enabled`) gobierna todo el marco: si esta apagado,
// ninguna extension corre y no aparecen menus extra. Cada extension tiene su
// propio `enabled` (se enciende con el marco activo).
//
// El estado resuelto se instala con resolveAndSet(db) al arrancar. Guardar
// cambios (PUT /api/extensions) re-resuelve en memoria, pero las suscripciones
// de entidades con HAOS se fijan al arrancar -> el cambio pleno requiere
// reinicio (mismo contrato que la topologia).

import fs from 'node:fs'
import path from 'node:path'
import { kvGet, kvSet } from './db.js'
import { getInstall } from './install.js'
import { cachedCollector } from './solar.js'

const round3 = (v) => Math.round(v * 1000) / 1000

const CHARGER_GLITCH_KWH = 50 // salto imposible entre ticks de 60 s
const STEP_MIN = 5

// ── Perfil LEGACY del módulo BYD: entidades reales de la instalación
// (BYD Atto 3, integración hass-byd-vehicle; verificadas 15-Ago-2026 con
// GET /api/states). APAGADO por defecto.
export const LEGACY_BYD = {
  enabled: false,
  name: 'BYD Atto 3',
  socId: 'sensor.byd_atto_3_nivel_de_bateria',
  rangeId: 'sensor.byd_atto_3_autonomia',
  odometerId: 'sensor.byd_atto_3_cuentakilometros',
  batteryPowerId: 'sensor.byd_atto_3_battery_power',
  chargingId: 'binary_sensor.byd_atto_3_cargando',
  pluggedId: 'binary_sensor.byd_atto_3_enchufe',
  onlineId: 'binary_sensor.byd_atto_3_en_linea',
  lockedId: 'binary_sensor.byd_atto_3_bloqueado',
  doorsId: 'binary_sensor.byd_atto_3_puertas_abiertas',
  windowsId: 'binary_sensor.byd_atto_3_ventanas_abiertas',
  sentryId: 'binary_sensor.byd_atto_3_modo_centinela',
  cabinTempId: 'sensor.byd_atto_3_temperatura_de_la_cabina',
  exteriorTempId: 'sensor.byd_atto_3_temperatura_exterior',
  tireFlId: 'sensor.byd_atto_3_presion_neumatico_delantero_izquierdo',
  tireFrId: 'sensor.byd_atto_3_presion_neumatico_delantero_derecho',
  tireRlId: 'sensor.byd_atto_3_presion_neumatico_trasero_izquierdo',
  tireRrId: 'sensor.byd_atto_3_presion_neumatico_trasero_derecho',
  locationId: 'device_tracker.byd_atto_3_ubicacion',
  gpsAgeId: 'sensor.byd_atto_3_gps_ultima_actualizacion',
  lastUpdateId: 'sensor.byd_atto_3_ultima_actualizacion',
  startChargeId: 'button.byd_atto_3_start_charging',
  stopChargeId: 'button.byd_atto_3_stop_charging',
  forcePollId: 'button.byd_atto_3_forzar_consulta',
  chargeToFullId: 'switch.byd_atto_3_charge_to_full',
  scheduleEnabledId: 'switch.byd_atto_3_schedule_enabled',
  scheduleStartId: 'time.byd_atto_3_start_time',
  scheduleEndId: 'time.byd_atto_3_end_time',
  repeatDailyId: 'switch.byd_atto_3_repeat_daily',
}
// Cargador generico (placeholders; el admin lo completa en Ajustes).
export const GENERIC_CHARGER = {
  key: '',
  name: 'Car charger',
  powerId: 'sensor.charger_power',
  powerUnit: 'kW',
  energyTotalId: 'sensor.charger_energy_total',
  energyDivisor: 1,
  energySessionId: '',
  stateId: 'sensor.charger_state',
  tempId: '',
  switchId: '',
  ampId: '',
  chargerInHouseMeters: true,
  chargingStates: ['charging', 'charger_charging'],
  connectedStates: ['charger_insert', 'connected', 'charging', 'charger_charging'],
}

// Perfil LEGACY: instalacion existente -> cargador local tipico (localtuya).
export const LEGACY_EXTENSIONS = {
  enabled: false,
  carCharger: {
    enabled: false,
    chargers: [
      {
        ...GENERIC_CHARGER,
        key: 'main',
        name: 'Cargador coche',
        powerId: 'sensor.cargador_coche_potencia',
        // Verificado contra el recorder (14-Ago-2026): el DPS reporta W aunque
        // la entidad localtuya diga kW (2605 "kW" durante carga = 2,6 kW reales).
        powerUnit: 'W',
        energyTotalId: 'sensor.cargador_coche_energia_total',
        // El contador va en centesimas de kWh (DPS 1): 469 = 4,69 kWh.
        energyDivisor: 100,
        energySessionId: 'sensor.cargador_coche_energia_sesion',
        stateId: 'sensor.cargador_coche_estado',
        tempId: 'sensor.cargador_coche_temperatura',
        switchId: 'switch.cargador_coche',
        // Corriente de carga configurada (A): number del propio cargador.
        ampId: 'number.cargador_coche_corriente',
        // Verificado 14-Ago-2026 contra el recorder: durante la carga, los
        // medidores de consumo de la casa leen 3-4 W -> el cargador va en
        // circuito APARTE, no esta incluido en ellos.
        chargerInHouseMeters: false,
        chargingStates: ['charger_charging', 'charging'],
        connectedStates: ['charger_insert', 'charger_charging', 'charging'],
      },
    ],
  },
  byd: LEGACY_BYD,
}

// Perfil GENERICO: instalacion nueva. El admin lo completa en Ajustes.
export const GENERIC_EXTENSIONS = {
  enabled: false,
  carCharger: {
    enabled: false,
    chargers: [{ ...GENERIC_CHARGER, key: 'main' }],
  },
  byd: {
    enabled: false,
    name: 'BYD',
    socId: 'sensor.byd_battery',
    rangeId: 'sensor.byd_range',
    odometerId: 'sensor.byd_odometer',
    batteryPowerId: 'sensor.byd_battery_power',
    chargingId: 'binary_sensor.byd_charging',
    pluggedId: 'binary_sensor.byd_plug',
    onlineId: 'binary_sensor.byd_online',
    lockedId: 'binary_sensor.byd_locked',
    doorsId: 'binary_sensor.byd_doors_open',
    windowsId: 'binary_sensor.byd_windows_open',
    sentryId: 'binary_sensor.byd_sentry',
    cabinTempId: 'sensor.byd_cabin_temp',
    exteriorTempId: 'sensor.byd_exterior_temp',
    tireFlId: 'sensor.byd_tire_fl',
    tireFrId: 'sensor.byd_tire_fr',
    tireRlId: 'sensor.byd_tire_rl',
    tireRrId: 'sensor.byd_tire_rr',
    locationId: 'device_tracker.byd_location',
    gpsAgeId: 'sensor.byd_gps_last_updated',
    lastUpdateId: 'sensor.byd_last_updated',
    startChargeId: 'button.byd_start_charging',
    stopChargeId: 'button.byd_stop_charging',
    forcePollId: 'button.byd_force_poll',
    chargeToFullId: 'switch.byd_charge_to_full',
    scheduleEnabledId: 'switch.byd_schedule_enabled',
    scheduleStartId: 'time.byd_start_time',
    scheduleEndId: 'time.byd_end_time',
    repeatDailyId: 'switch.byd_repeat_daily',
  },
}

// Estado resuelto (modulo)
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
      /* config corrupta -> fallback al perfil base */
    }
  }
  const hasDaily = db.prepare('SELECT COUNT(*) AS n FROM daily').get().n > 0
  return hasDaily ? LEGACY_EXTENSIONS : GENERIC_EXTENSIONS
}

// Normaliza UN cargador sobre su base (placeholder del perfil). No lanza:
// tipos inesperados revierten a valores seguros.
function normalizeCharger(raw, baseC, index) {
  const c = raw && typeof raw === 'object' ? raw : {}
  const b = baseC && typeof baseC === 'object' ? baseC : GENERIC_CHARGER
  const out = {}
  // La key NUNCA hereda del base (dos cargadores colisionarian en 'main');
  // un campo PRESENTE pero invalido va a '' (no hereda el placeholder).
  for (const k of ['key', 'name', 'powerId', 'energyTotalId', 'energySessionId', 'stateId', 'tempId', 'switchId', 'ampId', 'imageUrl']) {
    if (typeof c[k] === 'string') out[k] = c[k]
    else if (k === 'key') out[k] = ''
    else if (c[k] === undefined) out[k] = b[k] ?? ''
    else out[k] = ''
  }
  // key estable para URL (?tab=), kv y daily_charger: minusculas/alnum/guion.
  const rawKey = typeof out.key === 'string' ? out.key.trim().toLowerCase() : ''
  out.key = /^[a-z0-9-]{1,20}$/.test(rawKey) ? rawKey : index === 0 ? 'main' : `c${index + 1}`
  out.name =
    typeof c.name === 'string' && c.name.trim()
      ? c.name.trim()
      : (typeof b.name === 'string' && b.name.trim() ? b.name : `Cargador ${index + 1}`)
  out.powerUnit = c.powerUnit === 'W' || c.powerUnit === 'kW' ? c.powerUnit : (b.powerUnit === 'W' ? 'W' : 'kW')
  out.energyDivisor = Number.isInteger(c.energyDivisor) && c.energyDivisor >= 1 ? c.energyDivisor : (Number.isInteger(b.energyDivisor) && b.energyDivisor >= 1 ? b.energyDivisor : 1)
  const ihm = c.chargerInHouseMeters
  out.chargerInHouseMeters = ihm === undefined || ihm === null ? b.chargerInHouseMeters !== false : ihm !== false
  for (const k of ['chargingStates', 'connectedStates']) {
    if (Array.isArray(c[k])) out[k] = c[k].filter((s) => typeof s === 'string')
    else if (c[k] === undefined) out[k] = Array.isArray(b[k]) ? b[k].filter((s) => typeof s === 'string') : []
    else out[k] = []
  }
  return out
}

// Normaliza una config escrita por el admin sobre el perfil generico. No
// lanza: tipos inesperados revierten a valores seguros. Acepta la forma
// antigua (campos sueltos en carCharger) y la migra a chargers[0].
export function normalizeExtensions(cfg, base = GENERIC_EXTENSIONS) {
  const out = {
    enabled: typeof cfg.enabled === 'boolean' ? cfg.enabled : base.enabled,
    carCharger: {},
    byd: { ...base.byd, ...(cfg.byd || {}) },
  }
  const inC = cfg.carCharger && typeof cfg.carCharger === 'object' ? cfg.carCharger : {}
  const baseC = base.carCharger && typeof base.carCharger === 'object' ? base.carCharger : GENERIC_EXTENSIONS.carCharger
  out.carCharger.enabled = typeof inC.enabled === 'boolean' ? inC.enabled : !!baseC.enabled
  let list = Array.isArray(inC.chargers) ? inC.chargers : null
  if (!list) {
    // Forma antigua: los campos del cargador cuelgan de carCharger.
    const flat = { ...inC }
    delete flat.enabled
    list = Object.keys(flat).length > 0 ? [flat] : []
  }
  const baseList = Array.isArray(baseC.chargers) && baseC.chargers.length > 0 ? baseC.chargers : [GENERIC_CHARGER]
  out.carCharger.chargers = list.map((raw, i) => normalizeCharger(raw, baseList[Math.min(i, baseList.length - 1)], i))
  const b = out.byd
  b.enabled = typeof b.enabled === 'boolean' ? b.enabled : false
  b.name = typeof b.name === 'string' && b.name.trim() ? b.name.trim() : (base.byd?.name ?? 'BYD')
  for (const k of Object.keys(base.byd ?? {})) {
    if (k === 'enabled' || k === 'name') continue
    if (typeof b[k] !== 'string') b[k] = ''
  }
  return out
}

// Cargador: estado y suscripciones

/** Esta el marco activo, la extension del cargador encendida Y hay cargadores? */
export function chargerActive(ext = getExtensions()) {
  return !!(ext.enabled && ext.carCharger && ext.carCharger.enabled && ext.carCharger.chargers && ext.carCharger.chargers.length > 0)
}

/** Busca un cargador por key (null si no existe). */
export function getChargerByKey(ext = getExtensions(), key) {
  const list = ext?.carCharger?.chargers ?? []
  return list.find((c) => c.key === key) ?? null
}

/** IDs para subscribe_entities cuando el cargador esta activo (todos). */
export function chargerEntities(ext = getExtensions()) {
  if (!chargerActive(ext)) return []
  const ids = []
  for (const c of ext.carCharger.chargers) {
    ids.push(c.powerId, c.energyTotalId, c.energySessionId, c.stateId, c.tempId, c.switchId, c.ampId)
  }
  return [...new Set(ids.filter(Boolean))]
}

// Lectura numerica de un estado HA: undefined si no hay entidad o esta
// unavailable/unknown (asi la UI distingue "sin dato" de "0").
function entityNumOrUndef(ha, id) {
  if (!id) return undefined
  const e = ha.getState(id)
  if (!e || e.state === 'unavailable' || e.state === 'unknown') return undefined
  const n = parseFloat(e.state)
  return Number.isFinite(n) ? n : undefined
}

/** Divide un valor leido por el divisor del contador (undefined lo respeta). */
function divOpt(v, divisor) {
  return v === undefined ? undefined : round3(v / (divisor || 1))
}

function entityStateOrUndef(ha, id) {
  if (!id) return undefined
  const e = ha.getState(id)
  if (!e || e.state === 'unavailable' || e.state === 'unknown') return undefined
  return e.state
}

/** Snapshot en vivo de UN cargador (viaja en el SSE `live` dentro de chargers[]). */
export function computeOneChargerLive(ha, c) {
  const rawPower = entityNumOrUndef(ha, c.powerId)
  const powerKw =
    rawPower === undefined ? undefined : c.powerUnit === 'W' ? round3(rawPower / 1000) : round3(rawPower)
  const state = entityStateOrUndef(ha, c.stateId)
  // "Cargando": estado reconocido O potencia significativa (fallback robusto
  // ante enums distintos por instalacion; mismo umbral que la bateria).
  const charging = (state !== undefined && c.chargingStates.includes(state)) || (powerKw !== undefined && powerKw > 0.05)
  const connected = charging || (state !== undefined && c.connectedStates.includes(state))
  const sw = entityStateOrUndef(ha, c.switchId)
  // Opciones validas de la entidad de corriente (input_select/select): la UI
  // muestra solo esas; con number u otra entidad cae a los presets fijos.
  let ampOptions
  if (c.ampId) {
    const e = ha.getState(c.ampId)
    const opts = e?.attributes?.options
    if (Array.isArray(opts)) {
      ampOptions = opts.map((o) => Number(o)).filter((n) => Number.isFinite(n)).sort((a, b) => a - b)
    }
  }
  return {
    key: c.key,
    name: c.name,
    charging,
    connected,
    state,
    powerKw: powerKw ?? 0,
    sessionKwh: divOpt(entityNumOrUndef(ha, c.energySessionId), c.energyDivisor),
    totalKwh: divOpt(entityNumOrUndef(ha, c.energyTotalId), c.energyDivisor),
    tempC: entityNumOrUndef(ha, c.tempId),
    switchOn: sw === undefined ? undefined : sw === 'on',
    amps: entityNumOrUndef(ha, c.ampId),
    ampOptions,
  }
}

/** Snapshots en vivo de todos los cargadores (SSE: LivePower.chargers[]). */
export function computeChargerLive(ha, ext = getExtensions()) {
  if (!chargerActive(ext)) return []
  return ext.carCharger.chargers.map((c) => computeOneChargerLive(ha, c))
}

// ── BYD: estado, suscripciones y acciones (#100) ────────────────────────────

/** ¿El marco está activo Y la extensión BYD encendida? */
export function bydActive(ext = getExtensions()) {
  return !!(ext.enabled && ext.byd && ext.byd.enabled)
}

/** IDs para subscribe_entities cuando el módulo BYD está activo. */
export function bydEntities(ext = getExtensions()) {
  if (!bydActive(ext)) return []
  const b = ext.byd
  return [
    ...new Set(
      [
        b.socId, b.rangeId, b.odometerId, b.batteryPowerId,
        b.chargingId, b.pluggedId, b.onlineId, b.lockedId, b.doorsId, b.windowsId, b.sentryId,
        b.cabinTempId, b.exteriorTempId,
        b.tireFlId, b.tireFrId, b.tireRlId, b.tireRrId,
        b.locationId, b.gpsAgeId, b.lastUpdateId,
        b.startChargeId, b.stopChargeId, b.forcePollId, b.chargeToFullId,
        b.scheduleEnabledId, b.scheduleStartId, b.scheduleEndId, b.repeatDailyId,
        b.lockId, b.acSwitchId, b.flashLightsId, b.findCarId, b.closeWindowsId,
        b.remainingHoursId, b.remainingMinutesId,
        b.consumptionRecentId, b.consumption50Id, b.consumptionLifetimeId, b.consumptionTodayId,
      ].filter(Boolean),
    ),
  ]
}

function entityBoolOrUndef(ha, id) {
  const s = entityStateOrUndef(ha, id)
  return s === undefined ? undefined : s === 'on'
}

/** Antigüedad (minutos) de un sensor timestamp ISO; undefined si no hay dato. */
function entityAgeMinOrUndef(ha, id) {
  const s = entityStateOrUndef(ha, id)
  if (!s) return undefined
  const ts = new Date(s).getTime()
  if (Number.isNaN(ts)) return undefined
  return Math.max(0, Math.round((Date.now() - ts) / 60000))
}

/** Snapshot en vivo del BYD (viaja en el SSE `live` como `byd`). */
export function computeBydLive(ha, ext = getExtensions()) {
  if (!bydActive(ext)) return undefined
  const b = ext.byd
  const powerW = entityNumOrUndef(ha, b.batteryPowerId)
  const tires = {
    fl: entityNumOrUndef(ha, b.tireFlId),
    fr: entityNumOrUndef(ha, b.tireFrId),
    rl: entityNumOrUndef(ha, b.tireRlId),
    rr: entityNumOrUndef(ha, b.tireRrId),
  }
  // Coordenadas del device_tracker (attributes latitude/longitude)
  let lat
  let lon
  if (b.locationId) {
    const attrs = ha.getState(b.locationId)?.attributes
    const la = attrs ? parseFloat(attrs.latitude) : NaN
    const lo = attrs ? parseFloat(attrs.longitude) : NaN
    if (Number.isFinite(la) && Number.isFinite(lo)) {
      lat = Math.round(la * 1000) / 1000
      lon = Math.round(lo * 1000) / 1000
    }
  }
  return {
    name: b.name,
    online: entityBoolOrUndef(ha, b.onlineId) ?? false,
    soc: entityNumOrUndef(ha, b.socId),
    rangeKm: entityNumOrUndef(ha, b.rangeId),
    odometerKm: entityNumOrUndef(ha, b.odometerId),
    charging: entityBoolOrUndef(ha, b.chargingId) ?? false,
    plugged: entityBoolOrUndef(ha, b.pluggedId) ?? false,
    powerKw: powerW === undefined ? undefined : round3(powerW / 1000),
    locked: entityBoolOrUndef(ha, b.lockedId),
    doorsOpen: entityBoolOrUndef(ha, b.doorsId) ?? false,
    windowsOpen: entityBoolOrUndef(ha, b.windowsId) ?? false,
    sentry: entityBoolOrUndef(ha, b.sentryId),
    cabinTempC: entityNumOrUndef(ha, b.cabinTempId),
    exteriorTempC: entityNumOrUndef(ha, b.exteriorTempId),
    tires,
    location: entityStateOrUndef(ha, b.locationId),
    lat,
    lon,
    gpsAgeMin: entityAgeMinOrUndef(ha, b.gpsAgeId),
    lastUpdateMin: entityAgeMinOrUndef(ha, b.lastUpdateId),
    // Estado de acciones (para pintar switches coherentes)
    chargeToFullOn: entityBoolOrUndef(ha, b.chargeToFullId),
    scheduleEnabledOn: entityBoolOrUndef(ha, b.scheduleEnabledId),
    scheduleStart: entityStateOrUndef(ha, b.scheduleStartId),
    scheduleEnd: entityStateOrUndef(ha, b.scheduleEndId),
    repeatDailyOn: entityBoolOrUndef(ha, b.repeatDailyId),
    // v2: control + consumo + tiempo restante
    lockUnlocked: (() => {
      const s = entityStateOrUndef(ha, b.lockId)
      return s === undefined ? undefined : s === 'unlocked'
    })(),
    acOn: (() => {
      const s = entityStateOrUndef(ha, b.acSwitchId)
      return s === undefined ? undefined : s === 'on'
    })(),
    remainingMin: (() => {
      const rh = entityNumOrUndef(ha, b.remainingHoursId)
      const rm = entityNumOrUndef(ha, b.remainingMinutesId)
      return rh === undefined && rm === undefined ? undefined : Math.round((rh ?? 0) * 60 + (rm ?? 0))
    })(),
    consumptionRecent: entityNumOrUndef(ha, b.consumptionRecentId),
    consumption50: entityNumOrUndef(ha, b.consumption50Id),
    consumptionLifetime: entityNumOrUndef(ha, b.consumptionLifetimeId),
    consumptionToday: entityNumOrUndef(ha, b.consumptionTodayId),
  }
}

/**
 * Ejecuta una acción del módulo BYD contra HAOS con whitelist estricta de
 * servicios. `ha` es el cliente websocket; `action` viene del frontend.
 * Devuelve true si el servicio se despachó (sin esperar resultado).
 */
export async function bydAction(ha, ext, action, value) {
  if (!bydActive(ext)) throw new Error('byd_disabled')
  const b = ext.byd
  const press = (id) =>
    ha.call({ type: 'call_service', domain: 'button', service: 'press', target: { entity_id: id } })
  const setSwitch = (id, on) =>
    ha.call({ type: 'call_service', domain: 'switch', service: on ? 'turn_on' : 'turn_off', target: { entity_id: id } })

  switch (action) {
    case 'start_charging':
      if (!b.startChargeId) throw new Error('unknown_action')
      return press(b.startChargeId)
    case 'stop_charging':
      if (!b.stopChargeId) throw new Error('unknown_action')
      return press(b.stopChargeId)
    case 'force_poll':
      if (!b.forcePollId) throw new Error('unknown_action')
      return press(b.forcePollId)
    case 'charge_to_full':
      if (!b.chargeToFullId || typeof value !== 'boolean') throw new Error('unknown_action')
      return setSwitch(b.chargeToFullId, value)
    case 'repeat_daily':
      if (!b.repeatDailyId || typeof value !== 'boolean') throw new Error('unknown_action')
      return setSwitch(b.repeatDailyId, value)
    case 'schedule_enabled':
      if (!b.scheduleEnabledId || typeof value !== 'boolean') throw new Error('unknown_action')
      return setSwitch(b.scheduleEnabledId, value)
    case 'lock':
    case 'unlock': {
      if (!b.lockId) throw new Error('unknown_action')
      return ha.call({
        type: 'call_service',
        domain: 'lock',
        service: action === 'lock' ? 'lock' : 'unlock',
        target: { entity_id: b.lockId },
      })
    }
    case 'ac': {
      if (!b.acSwitchId || typeof value !== 'boolean') throw new Error('unknown_action')
      return setSwitch(b.acSwitchId, value)
    }
    case 'flash_lights':
      if (!b.flashLightsId) throw new Error('unknown_action')
      return press(b.flashLightsId)
    case 'find_car':
      if (!b.findCarId) throw new Error('unknown_action')
      return press(b.findCarId)
    case 'close_windows':
      if (!b.closeWindowsId) throw new Error('unknown_action')
      return press(b.closeWindowsId)
    case 'schedule_start':
    case 'schedule_end': {
      const id = action === 'schedule_start' ? b.scheduleStartId : b.scheduleEndId
      // HH:MM (o HH:MM:SS, lo acepta HA) — validación estricta antes del servicio
      if (!id || typeof value !== 'string' || !/^([01]\d|2[0-3]):[0-5]\d(:[0-5]\d)?$/.test(value)) throw new Error('unknown_action')
      return ha.call({ type: 'call_service', domain: 'time', service: 'set_value', target: { entity_id: id }, time: value })
    }
    default:
      throw new Error('unknown_action')
  }
}

// Cargador: energia diaria propia
// Los contadores de energia de cargadores locales suelen exponerse SIN
// state_class -> sin statistics en HAOS. Helios acumula sus propios deltas por
// dia y POR CARGADOR (tabla daily_charger, v7) a partir del contador total:
//   - La base del contador persiste en kv (`charger_counter:<key>`) -> los
//     reinicios del servidor no pierden el hilo. La base legacy
//     (`charger_counter`) es heredada por el cargador 'main'.
//   - Bajada del contador (reset del equipo) -> nueva base, sin sumar.
//   - Salto imposible (> CHARGER_GLITCH_KWH entre ticks) -> se ignora y se
//     re-basa (los huecos los cubre el backfill desde el recorder de HAOS).
//
// Ademas atribuye el ORIGEN de cada delta (v6): la fraccion que en ese
// instante pudo venir de excedente FV. Balance por slot: solar al cargador =
// min(potencia cargador, max(0, produccion - consumoSinCargador)). Lo que no
// es FV directo se etiqueta "red y bateria" (honesto y simple).

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

/** Suma `kwh` al dia del cargador `key` (crea la fila con ceros si no existe).
 *  `pvKwh` (0..kwh) es la fraccion de origen solar, clampeada al total. */
export function addChargerKwh(db, key, date, kwh, pvKwh = 0) {
  const pv = round3(Math.max(0, Math.min(pvKwh ?? 0, kwh)))
  db.prepare(
    `INSERT INTO daily_charger (date, charger, kwh, pv_kwh)
     VALUES (?, ?, ?, ?)
     ON CONFLICT(date, charger) DO UPDATE SET
       kwh = COALESCE(kwh, 0) + excluded.kwh,
       pv_kwh = MIN(COALESCE(pv_kwh, 0) + excluded.pv_kwh, COALESCE(kwh, 0) + excluded.kwh)`
  ).run(date, key, round3(kwh), pv)
}

/** Fija (sin sumar) el kWh (y su fraccion solar) de un dia/cargador SOLO si
 *  estaba a NULL, CREANDO la fila si no existe (backfill: daily_charger no
 *  pre-crea filas por dia; los huecos se buscan contra daily). pv clampeado. */
export function setChargerKwhIfNull(db, key, date, kwh, pvKwh) {
  const pv = pvKwh === undefined || pvKwh === null ? null : round3(Math.max(0, Math.min(pvKwh, kwh)))
  db.prepare('INSERT OR IGNORE INTO daily_charger (date, charger, kwh, pv_kwh) VALUES (?, ?, NULL, NULL)').run(date, key)
  db.prepare('UPDATE daily_charger SET kwh = ?, pv_kwh = ? WHERE date = ? AND charger = ? AND kwh IS NULL').run(
    round3(kwh),
    pv,
    date,
    key
  )
}

/** Fraccion [0..1] del delta que pudo venir de excedente FV en este instante.
 *  Balance por slot para UN cargador. Si los medidores de la casa incluyen el
 *  circuito del cargador (chargerInHouseMeters), el "resto de la casa" es
 *  consumo - potCargador; si va en circuito aparte, es el consumo tal cual.
 *  Sin lectura de potencia del cargador -> reparto proporcion prod/consumo. */
export function pvShareNow(ha, chargerPowerKw, ext = getExtensions(), charger) {
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
    const other = Math.max(0, consumption - (charger.chargerInHouseMeters ? chargerPowerKw : 0))
    const solarToCharger = Math.min(chargerPowerKw, Math.max(0, production - other))
    return Math.max(0, Math.min(1, solarToCharger / chargerPowerKw))
  }
  if (consumption > 0.01) return Math.max(0, Math.min(1, production / consumption))
  return production > 0 ? 1 : 0
}

// Base persistida del contador de un cargador. El key legacy plano
// (`charger_counter`) pasa a ser heredado por 'main' (primera lectura).
function counterKey(key) {
  return `charger_counter:${key}`
}

function readCounterBase(db, key) {
  const raw = kvGet(db, counterKey(key))
  if (raw) {
    try {
      const saved = JSON.parse(raw)
      if (saved && typeof saved.total === 'number') return saved
    } catch {
      /* kv corrupto -> re-basar */
    }
  }
  if (key === 'main') {
    const legacy = kvGet(db, 'charger_counter')
    if (legacy) {
      try {
        const saved = JSON.parse(legacy)
        if (saved && typeof saved.total === 'number') return saved
      } catch {
        /* kv corrupto -> re-basar */
      }
    }
  }
  return null
}

function writeCounterBase(db, key, total) {
  kvSet(db, counterKey(key), JSON.stringify({ date: todayStr(), total }))
}

/** Tick de acumulacion (llamar cada ~60 s con HAOS conectado): todos. */
export function accumulateChargerDaily(ha, db, ext = getExtensions()) {
  if (!chargerActive(ext)) return
  for (const c of ext.carCharger.chargers) accumulateOneCharger(ha, db, c)
}

function accumulateOneCharger(ha, db, c) {
  if (!c.energyTotalId) return
  const total = divOpt(entityNumOrUndef(ha, c.energyTotalId), c.energyDivisor)
  if (total === undefined) return

  const saved = readCounterBase(db, c.key)
  if (!saved || saved.total > total) {
    // Primera vez, kv perdido o reset del contador: nueva base, sin sumar.
    writeCounterBase(db, c.key, total)
    return
  }

  const delta = total - saved.total
  if (delta > 0 && delta <= CHARGER_GLITCH_KWH) {
    const rawPower = entityNumOrUndef(ha, c.powerId)
    const powerKw = rawPower === undefined ? undefined : c.powerUnit === 'W' ? rawPower / 1000 : rawPower
    addChargerKwh(db, c.key, todayStr(), delta, delta * pvShareNow(ha, powerKw, getExtensions(), c))
  } else if (delta > CHARGER_GLITCH_KWH) {
    console.warn(`[helios] cargador ${c.key}: salto anomalo ${delta} kWh, ignorado (glitch o sustitucion del contador?)`)
  }
  if (delta !== 0) writeCounterBase(db, c.key, total)
}

/** Historico diario de UN cargador (kwh/dia + fraccion solar) desde
 *  daily_charger. Dias a NULL (o sin pv) se estiman por balance diario:
 *  solar = min(kwh, max(0, produccion - (consumo - kwh si va en casa))). */
export function chargerHistory(db, from, to, key = 'main') {
  const rows = db
    .prepare(
      `SELECT date, kwh, pv_kwh FROM daily_charger
       WHERE date >= ? AND date <= ? AND charger = ? ORDER BY date ASC`
    )
    .all(from, to, key)
  if (rows.length === 0) return []
  const consRows = db
    .prepare('SELECT date, production_kwh, consumption_kwh FROM daily WHERE date >= ? AND date <= ?')
    .all(from, to)
  const consByDay = new Map(consRows.map((r) => [r.date, r]))
  const inHouse = getChargerByKey(getExtensions(), key)?.chargerInHouseMeters ?? true
  return rows.map((r) => {
    const kwh = r.kwh ?? null
    let pvKwh = r.pv_kwh ?? null
    if (kwh !== null && kwh > 0 && pvKwh === null) {
      const d = consByDay.get(r.date)
      const other = Math.max(0, (d?.consumption_kwh ?? 0) - (inHouse ? kwh : 0))
      pvKwh = Math.max(0, Math.min(kwh, (d?.production_kwh ?? 0) - other))
    }
    return { date: r.date, kwh, pvKwh }
  })
}

/** Atribucion solar por CURVAS para los dias del recorder: integra
 *  min(cargador, max(0, produccion - resto_casa)) sobre buckets de 5 min.
 *  Devuelve Map<dateKey, kWh>; vacio si no hay statistics disponibles. */
export async function chargerPvFromCurves(ha, startTime, endTime, charger) {
  const t = getInstall()
  const c = charger
  const statIds = [...new Set([...t.inverters.map((i) => i.powerId).filter(Boolean), ...t.consumption.powerIds.filter(Boolean)])]
  if (!statIds.length) return new Map()
  let stats = {}
  try {
    stats = await ha.statisticsDuringPeriod({ startTime, endTime, statisticIds: statIds, period: '5minute', types: ['mean'] })
  } catch {
    return new Map()
  }
  // bucketStartMs -> mean, por sensor
  const bySensor = new Map()
  for (const [id, rows] of Object.entries(stats)) {
    const m = new Map()
    for (const r of rows || []) if (r.mean !== null && r.mean !== undefined) m.set(new Date(r.start).getTime(), r.mean)
    bySensor.set(id, m)
  }
  if (!bySensor.size) return new Map()

  // Escalon de potencia del cargador (sin statistics -> historial REST).
  // Unavailable/unknown se trata como 0 kW, NUNCA como "mantener el ultimo
  // valor": el sensor cae entre sesiones (dropout localtuya) y un hold-last
  // fabricaria cargas fantasma de dias (bug cazado 14-Ago: 93% vs 74% real).
  const steps = []
  if (c.powerId) {
    try {
      const rows = await ha.historyDuringPeriod({ startTime, endTime, entityId: c.powerId })
      for (const r of rows) {
        const v = parseFloat(r.state)
        const kw = Number.isFinite(v) ? (c.powerUnit === 'W' ? v / 1000 : v) : 0
        steps.push({ t: new Date(r.last_changed).getTime(), kw })
      }
      steps.sort((a, b) => a.t - b.t)
    } catch {
      /* sin curva del cargador -> sin atribucion */
    }
  }
  if (!steps.length) return new Map()
  const chgAt = (ms) => {
    let v = 0
    for (const st of steps) {
      if (st.t > ms) break
      v = st.kw
    }
    return v
  }

  const consDiv = t.consumption.powerUnit === 'W' ? 1000 : 1
  const pvByDay = new Map()
  const BUCKET = 5 * 60 * 1000
  const startMs = Math.ceil(new Date(startTime).getTime() / BUCKET) * BUCKET
  const endMs = Math.min(new Date(endTime).getTime(), Date.now())
  for (let ms = startMs; ms < endMs; ms += BUCKET) {
    const k = chgAt(ms)
    if (!(k > 0.05)) continue
    let prod = 0
    for (const inv of t.inverters) {
      const v = bySensor.get(inv.powerId)?.get(ms)
      if (v !== undefined) prod += inv.powerUnit === 'W' ? v / 1000 : v
    }
    let cons = 0
    let haveCons = true
    for (const id of t.consumption.powerIds) {
      const v = bySensor.get(id)?.get(ms)
      if (v === undefined) {
        haveCons = false
        break
      }
      cons += v / consDiv
    }
    if (!haveCons) continue
    const other = Math.max(0, cons - (c.chargerInHouseMeters ? k : 0))
    const solar = Math.min(k, Math.max(0, prod - other)) * (BUCKET / 3600000)
    if (solar <= 0) continue
    const key = dateKey(new Date(ms))
    pvByDay.set(key, round3((pvByDay.get(key) || 0) + solar))
  }
  return pvByDay
}

// Cargador: backfill y curva desde el recorder de HAOS

// Deltas diarios de una serie de estados de un contador creciente.
// `rows`: [{ state, last_changed }] (historial REST de HAOS, en orden).
// `divisor`: unidades del contador -> kWh (DPS Tuya suele ir en centesimas).
// Suma los INCREMENTOS POSITIVOS dentro de cada dia: un reset del contador
// (bajada) no resta ni suma, y los saltos imposibles (> CHARGER_GLITCH_KWH)
// se ignoran como glitch. Devuelve Map<dateKey, kWh>.
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
      continue // el valor inicial del dia es la base, no un delta
    }
    const diffKwh = (v - prevV) / (divisor || 1)
    if (diffKwh > 0) {
      if (diffKwh <= CHARGER_GLITCH_KWH) perDay.set(key, round3((perDay.get(key) || 0) + diffKwh))
      else console.warn(`[helios] cargador: salto anomalo ${diffKwh} kWh ignorado en backfill`)
    }
    prevV = v
  }
  return perDay
}

/** Backfill de UN cargador: rellena dias a NULL (hoy incluida) desde el
 *  recorder de HAOS (ventana ~9 dias). Devuelve dias escritos. */
export async function backfillOneCharger(ha, db, c) {
  if (!c.energyTotalId) return 0
  const today = todayStr()
  const from = new Date()
  from.setDate(from.getDate() - 9)
  const fromKey = dateKey(from)

  // Huecos = dias con fila en daily (creadas por la consolidacion solar)
  // SIN fila de este cargador en daily_charger.
  const missing = db
    .prepare(
      `SELECT d.date FROM daily d
       WHERE d.date >= ? AND d.date <= ?
       AND NOT EXISTS (SELECT 1 FROM daily_charger dc WHERE dc.date = d.date AND dc.charger = ?)
       ORDER BY d.date`
    )
    .all(fromKey, today, c.key)
    .map((r) => r.date)
  // Sin huecos el backfill no escribe dias, PERO sigue hasta el final:
  // la semilla de HOY del acumulador (primer arranque sin filas daily)
  // necesita las rows del recorder.

  const startTime = new Date(fromKey + 'T00:00:00').toISOString()
  const endTime = new Date().toISOString()
  let rows
  try {
    rows = await ha.historyDuringPeriod({ startTime, endTime, entityId: c.energyTotalId })
  } catch {
    return 0
  }
  if (!Array.isArray(rows) || rows.length === 0) return 0

  const perDay = dailyDeltasFromHistory(rows, c.energyDivisor)
  // Atribucion solar por curvas (statistics 5 min de prod/cons + potencia del
  // cargador): solo para los dias que el recorder cubre.
  const needCurves = missing.some((date) => perDay.has(date) && perDay.get(date) > 0)
  const pvByDay = needCurves ? await chargerPvFromCurves(ha, startTime, endTime, c).catch(() => new Map()) : new Map()
  // Primer dia cubierto por el recorder: los dias sin estados a partir de ahi
  // son dias SIN carga (contador estatico) -> 0, no NULL (evita re-consultar).
  const firstRowKey = dateKey(new Date(rows.find((r) => Number.isFinite(parseFloat(r.state)))?.last_changed || 0))
  let n = 0
  for (const date of missing) {
    if (perDay.has(date)) {
      setChargerKwhIfNull(db, c.key, date, perDay.get(date), pvByDay.get(date))
      n++
    } else if (date >= firstRowKey) {
      // Dia dentro de la ventana del recorder sin estados: contador estatico
      // -> 0 kWh (hoy incluida: aun sin carga; el acumulador suma encima).
      setChargerKwhIfNull(db, c.key, date, 0)
      n++
    }
  }

  // HOY sin fila (la consolidacion solar crea la fila daily de madrugada): no
  // se escribe directo (una fila con produccion 0 ensuciaria el historico
  // solar). En su lugar se siembra la base del acumulador con el PRIMER estado
  // de hoy y el tick de 60 s aplica el delta del dia (addChargerKwh crea la
  // fila de daily_charger).
  const legacyBase = c.key === 'main' ? kvGet(db, 'charger_counter') : null
  const hasTodayRow = !!db.prepare('SELECT 1 AS x FROM daily WHERE date = ?').get(today)
  if (!hasTodayRow && !kvGet(db, counterKey(c.key)) && !legacyBase) {
    const firstToday = rows.find(
      (r) => Number.isFinite(parseFloat(r.state)) && dateKey(new Date(r.last_changed)) === today
    )
    if (firstToday) {
      writeCounterBase(db, c.key, round3(parseFloat(firstToday.state) / (c.energyDivisor || 1)))
      console.log(`[helios] cargador ${c.key}: hoy se aplicara via acumulador (base sembrada del recorder)`)
    }
  }
  return n
}

/** Backfill de todos los cargadores activos. Devuelve dias escritos (total). */
export async function backfillChargerHistory(ha, db, ext = getExtensions()) {
  if (!chargerActive(ext)) return 0
  let n = 0
  for (const c of ext.carCharger.chargers) {
    try {
      n += await backfillOneCharger(ha, db, c)
    } catch (err) {
      console.warn(`[helios] cargador ${c.key}: backfill fallo: ${err?.message || err}`)
    }
  }
  return n
}


// ── Cargador: imagen del coche (subida editable; NUNCA en el repo) ─────────
const CHARGER_IMAGE_EXTS = { 'image/png': 'png', 'image/jpeg': 'jpg', 'image/webp': 'webp' }
const CHARGER_IMAGE_MAX = 4 * 1024 * 1024

export function chargerImageDir(dataDir) {
  const dir = path.join(dataDir, 'charger-images')
  fs.mkdirSync(dir, { recursive: true })
  return dir
}

function setChargerImageUrl(db, key, url) {
  const raw = kvGet(db, 'extensions_config')
  if (!raw) return
  try {
    const cfg = JSON.parse(raw)
    const list = cfg?.carCharger?.chargers
    if (!Array.isArray(list)) return
    const ch = list.find((c) => c.key === key)
    if (!ch) return
    ch.imageUrl = url
    kvSet(db, 'extensions_config', JSON.stringify(cfg))
    resolveAndSet(db)
  } catch {
    /* kv corrupto: no tocar */
  }
}

/** Guarda la imagen (data URL) del cargador `key` y apunta imageUrl al endpoint. */
export function saveChargerImage(db, dataDir, key, dataUrl) {
  const m = typeof dataUrl === 'string' ? dataUrl.match(/^data:image\/(png|jpeg|webp);base64,(.+)$/) : null
  if (!m) return { ok: false, error: 'formato de imagen invalido' }
  const buf = Buffer.from(m[2], 'base64')
  if (!buf.length) return { ok: false, error: 'imagen vacia' }
  if (buf.length > CHARGER_IMAGE_MAX) return { ok: false, error: 'imagen demasiado grande (max 4 MB)' }
  const dir = chargerImageDir(dataDir)
  for (const ext of Object.values(CHARGER_IMAGE_EXTS)) {
    const f = path.join(dir, `${key}.${ext}`)
    if (fs.existsSync(f)) fs.unlinkSync(f)
  }
  fs.writeFileSync(path.join(dir, `${key}.${CHARGER_IMAGE_EXTS['image/' + m[1]]}`), buf)
  setChargerImageUrl(db, key, `/api/charger/image/${key}`)
  return { ok: true }
}

/** Quita la imagen del cargador `key` (ficheros + imageUrl en kv). */
export function clearChargerImage(db, dataDir, key) {
  const dir = chargerImageDir(dataDir)
  for (const ext of Object.values(CHARGER_IMAGE_EXTS)) {
    const f = path.join(dir, `${key}.${ext}`)
    if (fs.existsSync(f)) fs.unlinkSync(f)
  }
  setChargerImageUrl(db, key, '')
  return { ok: true }
}

/** Fichero+mime de la imagen del cargador, o null. */
export function findChargerImage(dataDir, key) {
  const dir = chargerImageDir(dataDir)
  for (const [mime, ext] of Object.entries(CHARGER_IMAGE_EXTS)) {
    const f = path.join(dir, `${key}.${ext}`)
    if (fs.existsSync(f)) return { path: f, mime }
  }
  return null
}

// Curva del dia (kW cada STEP_MIN) por interpolacion escalonada del historial
// de estados del sensor de potencia, POR CARGADOR. Cacheada: hoy 60 s, pasado 6 h.
export const chargerDayCurve = cachedCollector(
  (_ha, dateStr, _ext, charger) => `charger:${charger?.key || 'main'}:${dateStr || todayStr()}`,
  (_ha, dateStr) => ((dateStr || todayStr()) === todayStr() ? 60_000 : 6 * 3600_000),
  async (ha, dateStr, ext = getExtensions(), charger) => {
    if (!charger) return { points: [] }
    if (!charger.powerId) return { points: [] }

    const key = dateStr || todayStr()
    const start = new Date(key + 'T00:00:00')
    const end = new Date(start)
    end.setDate(end.getDate() + 1)
    const rows = await ha.historyDuringPeriod({
      startTime: start.toISOString(),
      endTime: end.toISOString(),
      entityId: charger.powerId,
    })
    if (!Array.isArray(rows) || rows.length === 0) return { points: [] }

    // Estados con su timestamp (ms desde medianoche local). unavailable/
    // unknown cuenta como 0 kW (ver nota en chargerPvFromCurves: el hold-last
    // fabrica cargas fantasma en los dropouts del sensor).
    const steps = []
    for (const r of rows) {
      const v = parseFloat(r.state)
      const ts = new Date(r.last_changed).getTime()
      if (Number.isNaN(ts)) continue
      const min = Math.max(0, Math.round((ts - start.getTime()) / 60000))
      steps.push({ min, kw: Number.isFinite(v) ? (charger.powerUnit === 'W' ? v / 1000 : v) : 0 })
    }
    if (!steps.length) return { points: [] }

    // HOY: la curva termina en el minuto actual (los buckets posteriores solo
    // tendrian el ultimo estado repetido: pintaria "encendido" hasta
    // medianoche, al contrario que la generacion solar en el DayChart, que se
    // dibuja hasta ahora). Dias pasados: dia completo.
    const isToday = key === todayStr()
    const limitMin = isToday
      ? Math.max(0, Math.floor((Date.now() - start.getTime()) / 60000))
      : 1440

    const points = []
    let idx = 0
    for (let min = 0; min < limitMin; min += STEP_MIN) {
      while (idx < steps.length - 1 && steps[idx + 1].min <= min) idx++
      if (steps[idx].min > min) break // aun no hay dato en este tramo
      const hh = String(Math.floor(min / 60)).padStart(2, '0')
      const mm = String(min % 60).padStart(2, '0')
      points.push({ t: min, label: `${hh}:${mm}`, kw: round3(Math.max(0, steps[idx].kw)) })
    }
    return { points }
  }
)

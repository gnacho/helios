// install.js — topología de la instalación (Fase 3 del roadmap, issue #37).
//
// Fuente de verdad de qué sensores de HAOS alimentan a Helios y con qué forma:
// cuántos inversores hay, si hay batería, cómo se mide la red (scraper Solis o
// sensores planos), y qué entidades de energía alimentan el histórico.
//
// Resolución (en orden de prioridad):
//   1. `install_config` en kv con sección `topology` → configuración explícita
//      del admin (editable por API/UI).
//   2. Instalación existente (daily con filas) → TOPOLOGY_LEGACY = la de la
//      casa (scraper Solis + 2 inversores + batería en español). Da exactamente
//      el comportamiento actual sin tocar la BD.
//   3. Instalación nueva (daily vacío) → TOPOLOGY_GENERIC: sin scraper, grid
//      por sensor plano, un inversor placeholder y batería opcional. El admin
//      la completa en Ajustes.
//
// El estado resuelto se instala con resolveAndSet() al arrancar (index.js) y se
// lee con getInstall(). Las funciones getEntities()/liveEntities()/
// getEnergyEntities()/deepSources() exponen vistas compatibles con el shape que
// consumían config.js y solar.js antes del refactor.

import { kvGet, dailyEmpty } from './db.js'

// ── Perfil LEGACY: la instalación actual (scraper Solis + Solis/Fox + batería)
const FOX_GLITCH_OFFSETS = {
  '2025-06-15': 5.46,
  '2025-07-26': 13.88,
  '2025-08-20': 4.25,
  '2025-11-02': 206.58,
  '2026-03-13': 4.71,
}

export const LEGACY_TOPOLOGY = {
  inverters: [
    {
      key: 'solis',
      name: 'Solis',
      model: 'Solis S5-EH1P5K-L',
      kwp: 4.4,
      panels: '10 × 440 W',
      tempC: 46,
      hasBattery: true,
      batteryKwh: 5,
      powerId: 'sensor.solis_potencia_actual',
      powerUnit: 'kW',
      energyId: 'sensor.solis_energia_hoy',
      energyAcc: 'sum',
      energyCap: 35,
      deepIds: ['sensor.energia_solis_diaria_um'],
      glitchOffsets: {},
    },
    {
      key: 'fox',
      name: 'Fox',
      model: 'Fox H1-3.0-E',
      kwp: 2.7,
      panels: '6 × 450 W',
      tempC: 41,
      hasBattery: false,
      powerId: 'sensor.almacen_pinza_power_b',
      powerUnit: 'W',
      energyId: 'sensor.energia_fox_diaria',
      energyAcc: 'state',
      energyCap: 20,
      deepIds: ['sensor.almacen_pinza_energy_produced_b'],
      glitchOffsets: FOX_GLITCH_OFFSETS,
    },
  ],
  battery: {
    enabled: true,
    powerId: 'sensor.solis_bateria_potencia',
    stateId: 'sensor.solis_bateria_estado',
    socId: 'sensor.solis_bateria_soc',
    capacityKwh: 5.12,
    chargingStates: ['Cargando'],
    dischargingStates: ['Descargando'],
  },
  // Todo viene de HAOS. grid.mode decide CÓMO se lee la red:
  //  - 'attrs': potencia/dirección en los ATRIBUTOS de un sensor (attrsId).
  //    Es el caso del sensor expuesto por un scraper Solis, pero es un sensor
  //    de HAOS cualquiera: si no existe, se usa otro sensor sin tocar la app.
  //  - 'sensor': de estados (sensorId con signo, o importId/exportId).
  // statusAttrsId (opcional) = sensor cuyos atributos dan el estado del
  // inversor (inverterOnline/stationName/lastUpdate). Desacoplado del grid.
  grid: {
    mode: 'attrs', // 'attrs' | 'sensor'
    attrsId: 'sensor.solis_scraper',
    sensorId: null,
    importId: null,
    exportId: null,
  },
  statusAttrsId: 'sensor.solis_scraper',
  consumption: {
    powerIds: [
      'sensor.medidor_respaldo_power',
      'sensor.vivienda_medidor_power',
      'sensor.almacen_pinza_power_a',
    ],
    powerUnit: 'W',
    energyIds: [
      'sensor.medidor_respaldo_energy',
      'sensor.vivienda_medidor_energy',
      'sensor.almacen_pinza_energy_a',
    ],
    respaldoId: 'sensor.medidor_respaldo_power',
    noRespaldadaId: 'sensor.vivienda_medidor_power',
  },
  energy: {
    gridImportId: 'sensor.energia_red_importada_solis',
    gridExportId: 'sensor.energia_red_exportada_solis',
    batChargeId: 'sensor.energia_bateria_carga_diaria',
    batDischargeId: 'sensor.energia_bateria_descarga_diaria',
    consumptionId: 'sensor.consumo_total_diario',
  },
  sun: 'sun.sun',
  weather: 'weather.forecast_casa',
  weatherTemp: 'sensor.sensor_temp_ext_temperature',
}

// ── Perfil GENÉRICO: instalación nueva sin scraper. El admin lo completa.
export const GENERIC_TOPOLOGY = {
  inverters: [
    {
      key: 'inv1',
      name: 'Inverter',
      model: '',
      kwp: 0,
      panels: '',
      tempC: 0,
      hasBattery: false,
      batteryKwh: 0,
      powerId: 'sensor.inverter_power',
      powerUnit: 'kW',
      energyId: 'sensor.inverter_energy_today',
      energyAcc: 'state',
      energyCap: 100,
      deepIds: ['sensor.inverter_energy_total'],
      glitchOffsets: {},
    },
  ],
  battery: {
    enabled: false,
    powerId: 'sensor.battery_power',
    stateId: 'sensor.battery_state',
    socId: 'sensor.battery_soc',
    capacityKwh: 0,
    chargingStates: ['charging', 'Cargando'],
    dischargingStates: ['discharging', 'Descargando'],
  },
  grid: {
    mode: 'sensor',
    attrsId: null,
    sensorId: 'sensor.grid_power',
    importId: 'sensor.grid_import_power',
    exportId: 'sensor.grid_export_power',
  },
  statusAttrsId: null,
  consumption: {
    powerIds: ['sensor.house_power'],
    powerUnit: 'W',
    energyIds: ['sensor.house_energy'],
    respaldoId: null,
    noRespaldadaId: null,
  },
  energy: {
    gridImportId: 'sensor.grid_import_energy',
    gridExportId: 'sensor.grid_export_energy',
    batChargeId: 'sensor.battery_charge_energy',
    batDischargeId: 'sensor.battery_discharge_energy',
    consumptionId: 'sensor.house_energy_today',
  },
  sun: 'sun.sun',
  weather: 'weather.forecast',
  weatherTemp: 'sensor.outdoor_temperature',
}

// ── Estado resuelto (módulo) ────────────────────────────────────────────────
let current = LEGACY_TOPOLOGY

// Instala la topología resuelta y devuelve una copia congelada.
export function resolveAndSet(db) {
  current = resolveInstall(db)
  return current
}

export function getInstall() {
  return current
}

// --- Tests: inyectar una topología sin BD -----------------------------------
export function _setForTests(topology) {
  current = topology
  return current
}

export function resolveInstall(db) {
  const base = dailyEmpty(db) ? GENERIC_TOPOLOGY : LEGACY_TOPOLOGY
  const raw = kvGet(db, 'install_config')
  if (raw) {
    try {
      const cfg = JSON.parse(raw)
      if (cfg.topology) return normalizeTopology(cfg.topology, base)
    } catch {
      /* config corrupta → fallback */
    }
  }
  return base
}

// Normaliza una topología escrita por el admin: completa los huecos con el
// perfil base (legacy/generic según la instalación). No lanza: una topología
// inválida revierte al base.
export function normalizeTopology(cfg, base = GENERIC_TOPOLOGY) {
  const out = {
    inverters: Array.isArray(cfg.inverters) && cfg.inverters.length > 0 ? cfg.inverters : base.inverters,
    battery: { ...base.battery, ...(cfg.battery || {}) },
    grid: { ...base.grid, ...(cfg.grid || {}) },
    consumption: { ...base.consumption, ...(cfg.consumption || {}) },
    energy: { ...base.energy, ...(cfg.energy || {}) },
    sun: cfg.sun || base.sun,
    weather: cfg.weather || base.weather,
    weatherTemp: cfg.weatherTemp || base.weatherTemp,
    statusAttrsId: cfg.statusAttrsId !== undefined ? cfg.statusAttrsId : base.statusAttrsId,
  }
  out.inverters = out.inverters.map((inv, i) => ({
    ...(base.inverters[i] || {}),
    ...inv,
    key: inv.key || `inv${i + 1}`,
    name: inv.name || `Inverter ${i + 1}`,
    powerId: inv.powerId || '',
    powerUnit: inv.powerUnit || 'kW',
    energyId: inv.energyId || '',
    energyAcc: inv.energyAcc || 'state',
    energyCap: typeof inv.energyCap === 'number' ? inv.energyCap : 100,
    deepIds: Array.isArray(inv.deepIds) ? inv.deepIds : [inv.energyId || ''].filter(Boolean),
    glitchOffsets: inv.glitchOffsets || {},
  }))
  return out
}

// ── Vistas compatibles con el shape anterior (config.js/solar.js) ───────────

export function getEntities() {
  const t = getInstall()
  const inv0 = t.inverters[0] || {}
  const inv1 = t.inverters[1] || {}
  const c = t.consumption
  const b = t.battery
  return {
    pvSolis: inv0.powerId || '',
    pvFox: inv1.powerId || '',
    consRespaldo: c.powerIds[0] || '',
    consNoRespaldada: c.powerIds[1] || '',
    consAlmacen: c.powerIds[2] || '',
    consRespaldoEnergy: c.energyIds[0] || '',
    consNoRespaldadaEnergy: c.energyIds[1] || '',
    consAlmacenEnergy: c.energyIds[2] || '',
    batteryPower: b.powerId,
    batteryState: b.stateId,
    batterySoc: b.socId,
    scraper: t.grid.attrsId,
    statusAttrsId: t.statusAttrsId,
    sun: t.sun,
    weather: t.weather,
    weatherTemp: t.weatherTemp,
    eSolis: inv0.energyId || '',
    eFox: inv1.energyId || '',
    eConsumption: t.energy.consumptionId,
    eGridImport: t.energy.gridImportId,
    eGridExport: t.energy.gridExportId,
    eBatCharge: t.energy.batChargeId,
    eBatDischarge: t.energy.batDischargeId,
  }
}

// IDs para subscribe_entities: todos los sensores de la topología activa.
export function liveEntities() {
  const t = getInstall()
  const e = getEntities()
  const ids = new Set()
  for (const inv of t.inverters) {
    if (inv.powerId) ids.add(inv.powerId)
    if (inv.energyId) ids.add(inv.energyId)
    for (const d of inv.deepIds || []) if (d) ids.add(d)
  }
  for (const id of t.consumption.powerIds) if (id) ids.add(id)
  for (const id of t.consumption.energyIds) if (id) ids.add(id)
  if (t.battery.enabled) {
    if (t.battery.powerId) ids.add(t.battery.powerId)
    if (t.battery.stateId) ids.add(t.battery.stateId)
    if (t.battery.socId) ids.add(t.battery.socId)
  }
  if (t.grid.mode === 'attrs' && t.grid.attrsId) ids.add(t.grid.attrsId)
  else {
    if (t.grid.sensorId) ids.add(t.grid.sensorId)
    if (t.grid.importId) ids.add(t.grid.importId)
    if (t.grid.exportId) ids.add(t.grid.exportId)
  }
  if (t.statusAttrsId && t.statusAttrsId !== t.grid.attrsId) ids.add(t.statusAttrsId)
  if (t.energy.gridImportId) ids.add(t.energy.gridImportId)
  if (t.energy.gridExportId) ids.add(t.energy.gridExportId)
  if (t.battery.enabled) {
    if (t.energy.batChargeId) ids.add(t.energy.batChargeId)
    if (t.energy.batDischargeId) ids.add(t.energy.batDischargeId)
  }
  if (t.energy.consumptionId) ids.add(t.energy.consumptionId)
  if (t.sun) ids.add(t.sun)
  if (t.weather) ids.add(t.weather)
  if (t.weatherTemp) ids.add(t.weatherTemp)
  return [...ids]
}

export function getEnergyEntities() {
  const t = getInstall()
  const inv0 = t.inverters[0] || {}
  const inv1 = t.inverters[1] || {}
  return {
    solis: inv0.energyId || '',
    fox: inv1.energyId || '',
    consumption: t.energy.consumptionId,
    gridImport: t.energy.gridImportId,
    gridExport: t.energy.gridExportId,
    batCharge: t.energy.batChargeId,
    batDischarge: t.energy.batDischargeId,
  }
}

// DEEP_SOURCES para backfillHistory, derivado de la topología (N inversores).
export function deepSources() {
  const t = getInstall()
  const srcs = {}
  t.inverters.forEach((inv, i) => {
    srcs[`inv${i}`] = {
      ids: (inv.deepIds && inv.deepIds.length ? inv.deepIds : [inv.energyId]).filter(Boolean),
      acc: inv.energyAcc || 'state',
      cap: inv.energyCap || 100,
    }
  })
  if (t.consumption.energyIds.length) {
    srcs.consumption = { ids: t.consumption.energyIds, acc: 'state', cap: 100, requireAll: true }
  }
  if (t.energy.gridImportId) srcs.gridImport = { ids: [t.energy.gridImportId], acc: 'sum', cap: 100 }
  if (t.energy.gridExportId) srcs.gridExport = { ids: [t.energy.gridExportId], acc: 'sum', cap: 25 }
  if (t.battery.enabled) {
    if (t.energy.batChargeId) srcs.batCharge = { ids: [t.energy.batChargeId], acc: 'sum', cap: 12 }
    if (t.energy.batDischargeId) srcs.batDischarge = { ids: [t.energy.batDischargeId], acc: 'sum', cap: 12 }
  }
  return srcs
}

// Empareja deepSources() con la columna per-inversor de la tabla daily. Para
// mantener compatibilidad con las columnas solis_kwh/fox_kwh existentes, los dos
// primeros inversores se mapean a solis/fox y el resto a invN.
export function deepSourceDailyKeys() {
  const srcs = deepSources()
  const map = {}
  const keys = Object.keys(srcs).filter((k) => k.startsWith('inv'))
  keys.forEach((k, i) => {
    map[k] = i === 0 ? 'solis' : i === 1 ? 'fox' : `inv${i + 1}`
  })
  return { srcs, map }
}

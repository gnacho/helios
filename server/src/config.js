import path from 'node:path'
import { fileURLToPath } from 'node:url'

const dirname = path.dirname(fileURLToPath(import.meta.url))

export const config = {
  port: parseInt(process.env.PORT || '80', 10),
  host: process.env.HOST || '0.0.0.0',
  haosUrl: (process.env.HAOS_URL || 'http://192.168.10.244:8123').replace(/\/+$/, ''),
  haosToken: process.env.HAOS_TOKEN || '',
  authUser: process.env.AUTH_USER || 'admin',
  authPass: process.env.AUTH_PASS || '',
  sessionSecret: process.env.SESSION_SECRET || '',
  sessionTtlMs: 30 * 24 * 3600 * 1000,
  dataDir: process.env.DATA_DIR || path.join(dirname, '..', 'data'),
  staticDir: process.env.STATIC_DIR || path.join(dirname, '..', 'public'),
  priceImport: parseFloat(process.env.PRICE_IMPORT_EUR || '0.15'),
  priceExport: parseFloat(process.env.PRICE_EXPORT_EUR || '0.08'),
  co2PerKwh: parseFloat(process.env.CO2_KG_PER_KWH || '0.25'),
}

export const ENTITIES = {
  pvSolis: process.env.E_PV_SOLIS || 'sensor.solis_potencia_actual',
  pvFox: process.env.E_PV_FOX || 'sensor.almacen_pinza_power_b',
  consRespaldo: process.env.E_CONS_RESPALDO || 'sensor.medidor_respaldo_power',
  consNoRespaldada: process.env.E_CONS_NO_RESPALDADA || 'sensor.vivienda_medidor_power',
  consAlmacen: process.env.E_CONS_ALMACEN || 'sensor.almacen_pinza_power_a',
  consRespaldoEnergy: process.env.E_CONS_RESPALDO_EN || 'sensor.medidor_respaldo_energy',
  consNoRespaldadaEnergy: process.env.E_CONS_NO_RESPALDADA_EN || 'sensor.vivienda_medidor_energy',
  consAlmacenEnergy: process.env.E_CONS_ALMACEN_EN || 'sensor.almacen_pinza_energy_a',
  batteryPower: process.env.E_BAT_POWER || 'sensor.solis_bateria_potencia',
  batteryState: process.env.E_BAT_STATE || 'sensor.solis_bateria_estado',
  batterySoc: process.env.E_BAT_SOC || 'sensor.solis_bateria_soc',
  scraper: process.env.E_SCRAPER || 'sensor.solis_scraper',
  sun: process.env.E_SUN || 'sun.sun',
  weather: process.env.E_WEATHER || 'weather.forecast_casa',
  weatherTemp: process.env.E_WEATHER_TEMP || 'sensor.sensor_temp_ext_temperature',
  eSolis: process.env.E_EN_SOLIS || 'sensor.solis_energia_hoy',
  eFox: process.env.E_EN_FOX || 'sensor.energia_fox_diaria',
  eConsumption: process.env.E_EN_CONSUMPTION || 'sensor.consumo_total_diario',
  eGridImport: process.env.E_EN_GRID_IMPORT || 'sensor.energia_red_importada_solis',
  eGridExport: process.env.E_EN_GRID_EXPORT || 'sensor.energia_red_exportada_solis',
  eBatCharge: process.env.E_EN_BAT_CHARGE || 'sensor.solis_bateria_carga_hoy',
  eBatDischarge: process.env.E_EN_BAT_DISCHARGE || 'sensor.solis_bateria_descarga_hoy',
}

export const LIVE_ENTITIES = [
  ENTITIES.pvSolis,
  ENTITIES.pvFox,
  ENTITIES.consRespaldo,
  ENTITIES.consNoRespaldada,
  ENTITIES.consAlmacen,
  ENTITIES.consRespaldoEnergy,
  ENTITIES.consNoRespaldadaEnergy,
  ENTITIES.consAlmacenEnergy,
  ENTITIES.batteryPower,
  ENTITIES.batteryState,
  ENTITIES.batterySoc,
  ENTITIES.scraper,
  ENTITIES.sun,
  ENTITIES.weather,
  ENTITIES.weatherTemp,
  ENTITIES.eSolis,
  ENTITIES.eFox,
  ENTITIES.eConsumption,
  ENTITIES.eGridImport,
  ENTITIES.eGridExport,
  ENTITIES.eBatCharge,
  ENTITIES.eBatDischarge,
]

export const ENERGY_ENTITIES = {
  solis: ENTITIES.eSolis,
  fox: ENTITIES.eFox,
  consumption: ENTITIES.eConsumption,
  gridImport: ENTITIES.eGridImport,
  gridExport: ENTITIES.eGridExport,
  batCharge: ENTITIES.eBatCharge,
  batDischarge: ENTITIES.eBatDischarge,
}

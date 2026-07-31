import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { z } from 'zod'

const dirname = path.dirname(fileURLToPath(import.meta.url))

const envSchema = z.object({
  PORT: z.coerce.number().int().min(1).max(65535).default(80),
  HOST: z.string().default('0.0.0.0'),
  HAOS_URL: z.url().default('http://192.168.10.244:8123'),
  HAOS_TOKEN: z.string().min(1, 'FALTA HAOS_TOKEN en .env'),
  AUTH_USER: z.string().min(1).default('admin'),
  AUTH_PASS: z.string().min(1, 'FALTA AUTH_PASS en .env'),
  SESSION_SECRET: z.string().default(''),
  DATA_DIR: z.string().optional(),
  STATIC_DIR: z.string().optional(),
  PRICE_IMPORT_EUR: z.coerce.number().default(0.15),
  PRICE_EXPORT_EUR: z.coerce.number().default(0.08),
  CO2_KG_PER_KWH: z.coerce.number().default(0.25),
})

const parsed = envSchema.safeParse(process.env)
if (!parsed.success) {
  console.error(
    '[helios] config inválida:',
    parsed.error.issues.map((i) => `${i.path.join('.')}: ${i.message}`).join('; ')
  )
  process.exit(1)
}
const env = parsed.data

export const config = {
  port: env.PORT,
  host: env.HOST,
  haosUrl: env.HAOS_URL.replace(/\/+$/, ''),
  haosToken: env.HAOS_TOKEN,
  authUser: env.AUTH_USER,
  authPass: env.AUTH_PASS,
  sessionSecret: env.SESSION_SECRET,
  sessionTtlMs: 30 * 24 * 3600 * 1000,
  dataDir: env.DATA_DIR || path.join(dirname, '..', 'data'),
  staticDir: env.STATIC_DIR || path.join(dirname, '..', 'public'),
  priceImport: env.PRICE_IMPORT_EUR,
  priceExport: env.PRICE_EXPORT_EUR,
  co2PerKwh: env.CO2_KG_PER_KWH,
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

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
  VAPID_PUBLIC_KEY: z.string().optional(),
  VAPID_PRIVATE_KEY: z.string().optional(),
  VAPID_SUBJECT: z.string().optional(),
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

// Contrato API compartido (única fuente de verdad de los payloads del servidor).
// El frontend tipa contra estas mismas formas en app/src/data/types.ts.
import { z } from 'zod'

export const dateSchema = z.string().regex(/^\d{4}-\d{2}-\d{2}$/).optional()

export const loginSchema = z.object({
  username: z.string().min(1),
  password: z.string().min(1),
})

export const registerSchema = z.object({
  username: z.string().min(3).max(50),
  password: z.string().min(6),
  language: z.string().regex(/^(es|en|zh-CN)$/).optional(),
  role: z.enum(['user', 'admin']).optional(),
})

export const profileSchema = z.object({
  email: z.string().email().optional().nullable(),
  phone: z.string().optional().nullable(),
  language: z.string().regex(/^(es|en|zh-CN)$/).optional(),
})

export const passwordSchema = z.object({
  current: z.string().min(1),
  password: z.string().min(6),
})

export const historyQuerySchema = z.object({
  limit: z.coerce.number().int().min(1).max(1000).default(1000),
  offset: z.coerce.number().int().min(0).default(0),
})

export const auditQuerySchema = z.object({
  limit: z.coerce.number().int().min(1).max(200).default(50),
  offset: z.coerce.number().int().min(0).default(0),
})

export const adminPasswordSchema = z.object({ password: z.string().min(6) })
export const adminLanguageSchema = z.object({ language: z.string().regex(/^(es|en|zh-CN)$/) })
export const adminRoleSchema = z.object({ role: z.enum(['user', 'admin']) })

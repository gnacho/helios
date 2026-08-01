// Contrato API compartido (única fuente de verdad de los payloads del servidor).
// El frontend tipa contra estas mismas formas en app/src/data/types.ts.
// Factory: recibe zod del consumidor (shared/ no tiene node_modules propio).
export function createSchemas(z) {
  return {
    dateSchema: z.string().regex(/^\d{4}-\d{2}-\d{2}$/).optional(),

    loginSchema: z.object({
      username: z.string().min(1),
      password: z.string().min(1),
    }),

    registerSchema: z.object({
      username: z.string().min(3).max(50),
      password: z.string().min(6),
      language: z.string().regex(/^(es|en|zh-CN)$/).optional(),
      role: z.enum(['user', 'admin']).optional(),
    }),

    profileSchema: z.object({
      email: z.string().email().optional().nullable(),
      phone: z.string().optional().nullable(),
      language: z.string().regex(/^(es|en|zh-CN)$/).optional(),
    }),

    passwordSchema: z.object({
      current: z.string().min(1),
      password: z.string().min(6),
    }),

    historyQuerySchema: z.object({
      limit: z.coerce.number().int().min(1).max(1000).default(1000),
      offset: z.coerce.number().int().min(0).default(0),
    }),

    auditQuerySchema: z.object({
      limit: z.coerce.number().int().min(1).max(200).default(50),
      offset: z.coerce.number().int().min(0).default(0),
    }),

    adminPasswordSchema: z.object({ password: z.string().min(6) }),
    adminLanguageSchema: z.object({ language: z.string().regex(/^(es|en|zh-CN)$/) }),
    adminRoleSchema: z.object({ role: z.enum(['user', 'admin']) }),
  }
}

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
      display_name: z.string().min(1).max(50).optional().nullable(),
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

    // Topología de la instalación (issue #41): lo que el admin edita desde la
    // UI de Ajustes y se guarda como `install_config.topology`.
    topologySchema: z.object({
      inverters: z.array(
        z.object({
          key: z.string().min(1),
          name: z.string().min(1),
          model: z.string().optional().default(''),
          kwp: z.number().min(0).optional().default(0),
          panels: z.string().optional().default(''),
          tempC: z.number().optional().default(0),
          hasBattery: z.boolean().optional().default(false),
          batteryKwh: z.number().min(0).optional().default(0),
          powerId: z.string().optional().default(''),
          powerUnit: z.enum(['kW', 'W']).optional().default('kW'),
          energyId: z.string().optional().default(''),
          energyAcc: z.enum(['sum', 'state']).optional().default('state'),
          energyCap: z.number().min(0).optional().default(100),
          deepIds: z.array(z.string()).optional().default([]),
          glitchOffsets: z.record(z.string(), z.number()).optional().default({}),
        })
      ).min(1),
      battery: z.object({
        enabled: z.boolean().optional().default(false),
        powerId: z.string().optional().default(''),
        stateId: z.string().optional().default(''),
        socId: z.string().optional().default(''),
        capacityKwh: z.number().min(0).optional().default(0),
        chargingStates: z.array(z.string()).optional().default([]),
        dischargingStates: z.array(z.string()).optional().default([]),
      }).optional().default({}),
      grid: z.object({
        mode: z.enum(['attrs', 'sensor']).optional().default('sensor'),
        attrsId: z.string().nullable().optional().default(null),
        sensorId: z.string().nullable().optional().default(null),
        importId: z.string().nullable().optional().default(null),
        exportId: z.string().nullable().optional().default(null),
      }).optional().default({}),
      statusAttrsId: z.string().nullable().optional().default(null),
      consumption: z.object({
        powerIds: z.array(z.string()).optional().default([]),
        powerUnit: z.enum(['W', 'kW']).optional().default('W'),
        energyIds: z.array(z.string()).optional().default([]),
        respaldoId: z.string().nullable().optional().default(null),
        noRespaldadaId: z.string().nullable().optional().default(null),
      }).optional().default({}),
      energy: z.object({
        gridImportId: z.string().optional().default(''),
        gridExportId: z.string().optional().default(''),
        batChargeId: z.string().optional().default(''),
        batDischargeId: z.string().optional().default(''),
        consumptionId: z.string().optional().default(''),
      }).optional().default({}),
      sun: z.string().optional().default('sun.sun'),
      weather: z.string().optional().default(''),
      weatherTemp: z.string().optional().default(''),
    }),

    // Extensiones (issue #94): marco opcional de módulos. Interrupor maestro
    // + primer módulo (cargador de coche) con sus entidades HAOS editables.
    extensionsSchema: z.object({
      enabled: z.boolean().optional().default(false),
      carCharger: z.object({
        enabled: z.boolean().optional().default(false),
        name: z.string().min(1).max(60).optional().default(''),
        powerId: z.string().optional().default(''),
        powerUnit: z.enum(['kW', 'W']).optional().default('kW'),
        energyTotalId: z.string().optional().default(''),
        energySessionId: z.string().optional().default(''),
        // Divisor del contador de energía (unidades → kWh): muchos cargadores
        // Tuya reportan en centésimas de kWh (DPS 1) aunque la integración los
        // etiquete como kWh. 1 = ya está en kWh.
        energyDivisor: z.number().int().min(1).max(100000).optional().default(1),
        // ¿Los medidores de consumo de la topología ya incluyen el circuito
        // del cargador? Si true, la atribución solar descuenta la potencia del
        // cargador del consumo medido; si false (circuito dedicado aparte), no.
        chargerInHouseMeters: z.boolean().optional().default(true),
        stateId: z.string().optional().default(''),
        tempId: z.string().optional().default(''),
        switchId: z.string().optional().default(''),
        chargingStates: z.array(z.string()).optional().default([]),
        connectedStates: z.array(z.string()).optional().default([]),
      }).optional().default({}),
      // Módulo BYD (#100): panel del vehículo eléctrico (integración
      // hass-byd-vehicle en HAOS). Todos los IDs son editables; los del
      // perfil legacy son los reales de la instalación (BYD Atto 3).
      byd: z.object({
        enabled: z.boolean().optional().default(false),
        name: z.string().min(1).max(60).optional().default(''),
        socId: z.string().optional().default(''),
        rangeId: z.string().optional().default(''),
        odometerId: z.string().optional().default(''),
        batteryPowerId: z.string().optional().default(''),
        chargingId: z.string().optional().default(''),
        pluggedId: z.string().optional().default(''),
        onlineId: z.string().optional().default(''),
        lockedId: z.string().optional().default(''),
        doorsId: z.string().optional().default(''),
        windowsId: z.string().optional().default(''),
        sentryId: z.string().optional().default(''),
        cabinTempId: z.string().optional().default(''),
        exteriorTempId: z.string().optional().default(''),
        tireFlId: z.string().optional().default(''),
        tireFrId: z.string().optional().default(''),
        tireRlId: z.string().optional().default(''),
        tireRrId: z.string().optional().default(''),
        locationId: z.string().optional().default(''),
        gpsAgeId: z.string().optional().default(''),
        lastUpdateId: z.string().optional().default(''),
        // Acciones (servicios HAOS; vacío = botón oculto)
        startChargeId: z.string().optional().default(''),
        stopChargeId: z.string().optional().default(''),
        forcePollId: z.string().optional().default(''),
        chargeToFullId: z.string().optional().default(''),
        scheduleEnabledId: z.string().optional().default(''),
        scheduleStartId: z.string().optional().default(''),
        scheduleEndId: z.string().optional().default(''),
        repeatDailyId: z.string().optional().default(''),
      }).optional().default({}),
    }),
  }
}

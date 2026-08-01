/**
 * Umbrales de la app: SIEMPRE aquí, nunca literales dispersos en componentes.
 * (Skill webapp-stack § umbrales configurables — la fuente remota, si existe, manda sobre estos defaults.)
 */

/** Reserva mínima de batería (%): por debajo se considera "en reserva". */
export const BATTERY_RESERVE_PCT = 20;

/** SoC máximo mostrado como pleno en gauges (%). */
export const BATTERY_FULL_PCT = 100;

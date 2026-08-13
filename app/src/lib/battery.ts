import { BatteryCharging, BatteryFull, BatteryLow, BatteryMedium, BatteryWarning, type LucideIcon } from 'lucide-react';

/**
 * Icono de estado de la batería según SOC y flujo de carga.
 *
 * Orden de evaluación:
 *  1. cargando → BatteryCharging (el flujo de carga manda, aunque esté llena:
 *     el inversor rara vez reporta "cargando" al 100%, y el rayo comunica flujo activo).
 *  2. soc >= 80 → BatteryFull
 *  3. soc > 30 → BatteryMedium
 *  4. soc > 20 → BatteryLow
 *  5. resto (soc <= 20) → BatteryWarning (reserva)
 */
export function batteryIcon(soc: number, charging: boolean): LucideIcon {
  if (charging) return BatteryCharging;
  if (soc >= 80) return BatteryFull;
  if (soc > 30) return BatteryMedium;
  if (soc > 20) return BatteryLow;
  return BatteryWarning;
}

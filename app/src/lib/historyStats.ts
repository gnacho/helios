import { format } from 'date-fns';
import { es } from 'date-fns/locale';
import type { HistoryDay } from '@/data/types';
import type { EnergySettings } from '@/hooks/useEnergySettings';

/** Periodos del histórico. */
export type Period = 'dia' | 'semana' | 'mes' | 'ano';

export const PERIOD_LABELS: Record<Period, string> = {
  dia: 'Día',
  semana: 'Semana',
  mes: 'Mes',
  ano: 'Año',
};

/** Texto del comparativo "vs … anterior" por periodo. */
export const PREVIOUS_LABELS: Record<Period, string> = {
  dia: 'vs día anterior',
  semana: 'vs semana anterior',
  mes: 'vs mes anterior',
  ano: 'vs año anterior',
};

function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

/**
 * Ventana del periodo [start, end] (ambos incluidos, a las 00:00/23:59).
 * - Semana: ventana móvil de 7 días que TERMINA en el anchor
 *   (estado inicial: 8 – 14 jul, la última semana completa).
 * - Mes / Año: anchor = primer día del mes / del año.
 */
export function periodWindow(period: Period, anchor: Date): { start: Date; end: Date } {
  const a = startOfDay(anchor);
  switch (period) {
    case 'dia':
      return { start: a, end: a };
    case 'semana':
      return { start: new Date(a.getFullYear(), a.getMonth(), a.getDate() - 6), end: a };
    case 'mes':
      return { start: new Date(a.getFullYear(), a.getMonth(), 1), end: new Date(a.getFullYear(), a.getMonth() + 1, 0) };
    case 'ano':
      return { start: new Date(a.getFullYear(), 0, 1), end: new Date(a.getFullYear(), 11, 31) };
  }
}

/** Anchor inicial de cada periodo (día/semana completos: terminan ayer). */
export function initialAnchor(period: Period, today: Date): Date {
  const t = startOfDay(today);
  switch (period) {
    case 'dia':
      // El día actual tiene datos parciales: por defecto se muestra ayer.
      return new Date(t.getFullYear(), t.getMonth(), t.getDate() - 1);
    case 'semana':
      // Última semana completa: termina ayer (8 – 14 jul si hoy es 15 jul).
      return new Date(t.getFullYear(), t.getMonth(), t.getDate() - 1);
    case 'mes':
      return new Date(t.getFullYear(), t.getMonth(), 1);
    case 'ano':
      return new Date(t.getFullYear(), 0, 1);
  }
}

/** Desplaza el anchor una unidad del periodo (dir = ±1). */
export function shiftAnchor(period: Period, anchor: Date, dir: 1 | -1): Date {
  const a = startOfDay(anchor);
  switch (period) {
    case 'dia':
      return new Date(a.getFullYear(), a.getMonth(), a.getDate() + dir);
    case 'semana':
      return new Date(a.getFullYear(), a.getMonth(), a.getDate() + dir * 7);
    case 'mes':
      return new Date(a.getFullYear(), a.getMonth() + dir, 1);
    case 'ano':
      return new Date(a.getFullYear() + dir, 0, 1);
  }
}

/** ¿El periodo mostrado es el "actual" (chip Hoy deshabilitado)? */
export function isCurrentPeriod(period: Period, anchor: Date, today: Date): boolean {
  return sameDay(anchor, initialAnchor(period, today));
}

/** ¿Se puede avanzar al periodo siguiente sin pasar de "hoy"? */
export function canGoNext(period: Period, anchor: Date, today: Date): boolean {
  if (isCurrentPeriod(period, anchor, today)) return false;
  const next = shiftAnchor(period, anchor, 1);
  const { start, end } = periodWindow(period, next);
  const todayEndMs = startOfDay(today).getTime() + 86_399_999;
  if (period === 'dia' || period === 'semana') {
    // La ventana completa debe quedar a lo sumo en "hoy".
    return end.getTime() <= todayEndMs;
  }
  // Mes / año: basta con que el periodo ya haya comenzado.
  return start.getTime() <= todayEndMs;
}

/** Etiqueta corta del navegador de fecha: `15 jul` · `8 – 14 jul` · `julio 2025` · `2025`. */
export function navLabel(period: Period, anchor: Date): string {
  const { start, end } = periodWindow(period, anchor);
  switch (period) {
    case 'dia':
      return format(anchor, 'd MMM', { locale: es });
    case 'semana': {
      const left =
        start.getMonth() === end.getMonth() ? `${start.getDate()}` : format(start, 'd MMM', { locale: es });
      return `${left} – ${format(end, 'd MMM', { locale: es })}`;
    }
    case 'mes':
      return format(anchor, 'MMMM yyyy', { locale: es });
    case 'ano':
      return format(anchor, 'yyyy');
  }
}

/** Subtítulo largo bajo el título: `Semana del 8 al 14 de julio`, etc. */
export function periodSubtitle(period: Period, anchor: Date): string {
  const { start, end } = periodWindow(period, anchor);
  switch (period) {
    case 'dia':
      return format(anchor, "EEEE, d 'de' MMMM", { locale: es });
    case 'semana':
      return `Semana del ${start.getDate()} al ${format(end, "d 'de' MMMM", { locale: es })}`;
    case 'mes':
      return `Mes de ${format(anchor, "MMMM 'de' yyyy", { locale: es })}`;
    case 'ano':
      return `Año ${format(anchor, 'yyyy')}`;
  }
}

/** Días del histórico sintético que caen dentro de la ventana. */
export function daysInWindow(period: Period, anchor: Date, history: HistoryDay[]): HistoryDay[] {
  const { start, end } = periodWindow(period, anchor);
  const endMs = end.getTime() + 86_399_999;
  return history.filter((d) => d.date.getTime() >= start.getTime() && d.date.getTime() <= endMs);
}

export interface PeriodTotals {
  productionKwh: number;
  consumptionKwh: number;
  gridImportKwh: number;
  gridExportKwh: number;
  /** Media ponderada por producción. */
  autoconsumoPct: number;
  ahorroEur: number;
  co2Kg: number;
  dayCount: number;
}

/** Agrega los días de la ventana en totales del periodo (precios de ajustes). */
export function totalsFor(days: HistoryDay[], settings: EnergySettings): PeriodTotals {
  let productionKwh = 0;
  let consumptionKwh = 0;
  let gridImportKwh = 0;
  let gridExportKwh = 0;
  let autoWeighted = 0;

  for (const d of days) {
    productionKwh += d.productionKwh;
    consumptionKwh += d.consumptionKwh;
    gridImportKwh += d.gridImportKwh;
    gridExportKwh += d.gridExportKwh;
    autoWeighted += d.autoconsumoPct * d.productionKwh;
  }

  const autoconsumoPct = productionKwh > 0 ? autoWeighted / productionKwh : 0;
  const ahorroEur =
    Math.max(0, consumptionKwh - gridImportKwh) * settings.priceImport + gridExportKwh * settings.priceExport;
  const co2Kg = productionKwh * settings.co2Factor;

  return { productionKwh, consumptionKwh, gridImportKwh, gridExportKwh, autoconsumoPct, ahorroEur, co2Kg, dayCount: days.length };
}

/** Variación porcentual respecto al periodo anterior (null si no hay base). */
export function deltaPct(current: number, previous: number): number | null {
  if (previous <= 0) return null;
  return ((current - previous) / previous) * 100;
}

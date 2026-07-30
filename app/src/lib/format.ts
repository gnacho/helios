/** Formatos numéricos es-ES: decimal con coma, miles con punto. */

const nf2 = new Intl.NumberFormat('es-ES', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
const nf1 = new Intl.NumberFormat('es-ES', { minimumFractionDigits: 1, maximumFractionDigits: 1 });
const nf0 = new Intl.NumberFormat('es-ES', { maximumFractionDigits: 0 });

/** Potencias < 1000 W en W, ≥ 1000 W en kW con 2 decimales. Devuelve [cifra, unidad]. */
export function fmtPower(kW: number): [string, string] {
  const abs = Math.abs(kW);
  if (abs < 1) return [nf0.format(Math.round(kW * 1000)), 'W'];
  return [nf2.format(kW), 'kW'];
}

/** Potencia siempre en kW con 2 decimales (para series del día). */
export function fmtKw(kW: number): string {
  return nf2.format(kW);
}

/** Energías en kWh con 1 decimal. */
export function fmtEnergy(kWh: number): string {
  return nf1.format(kWh);
}

export function fmtPct(v: number): string {
  return nf0.format(Math.round(v));
}

export function fmtEuro(v: number): string {
  return `${nf2.format(v)} €`;
}

/** HH:MM a partir de minutos desde medianoche. */
export function fmtTime(minutes: number): string {
  const m = ((Math.round(minutes) % 1440) + 1440) % 1440;
  const h = Math.floor(m / 60);
  const mm = m % 60;
  return `${String(h).padStart(2, '0')}:${String(mm).padStart(2, '0')}`;
}

/** Reloj HH:MM:SS. */
export function fmtClock(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

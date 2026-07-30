import { useMemo } from 'react';
import {
  Area,
  AreaChart,
  CartesianGrid,
  ReferenceArea,
  ReferenceDot,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import type { PowerPoint } from '@/data/types';
import { STEP_MIN } from '@/data/types';
import { useEnergyColors } from '@/lib/colors';
import { fmtPct, fmtTime } from '@/lib/format';

const AXIS_TICKS = [0, 180, 360, 540, 720, 900, 1080, 1260, 1440];

interface SocDayChartProps {
  data: PowerPoint[];
  /** "Ahora" simulado en minutos: separa curva real de proyección. */
  nowMin: number;
  height?: number;
}

interface SocEvent {
  t: number;
  soc: number;
  label: string;
  past: boolean;
}

function SocTooltip({ active, payload, label }: { active?: boolean; payload?: { name: string; value: number; color: string }[]; label?: number }) {
  if (!active || !payload || payload.length === 0 || label === undefined) return null;
  const row = payload.find((p) => p.value !== null && p.value !== undefined);
  if (!row) return null;
  return (
    <div className="rounded-xl border border-app bg-surface/95 px-3 py-2 shadow-lg backdrop-blur-md">
      <p className="mb-1 text-xs font-semibold text-app">{fmtTime(label)}</p>
      <p className="flex items-center gap-1.5 text-xs text-muted">
        <span className="h-2 w-2 rounded-full" style={{ backgroundColor: row.color }} />
        Estado de carga <span className="ml-auto pl-3 font-semibold text-app">{fmtPct(row.value)} %</span>
      </p>
    </div>
  );
}

/** Punto de anotación con pop (scale 0→1 spring, stagger 150ms, delay 700ms). */
function AnnotationDot({ cx, cy, color, delay, faded, label }: { cx?: number; cy?: number; color: string; delay: number; faded: boolean; label: string }) {
  if (cx === undefined || cy === undefined) return <g />;
  return (
    <g
      className="helios-soc-pop"
      style={{ animationDelay: `${delay}ms`, opacity: faded ? 0.45 : 1 }}
      aria-label={label}
    >
      <title>{label}</title>
      <circle cx={cx} cy={cy} r={6} fill={color} fillOpacity={0.25} />
      <circle cx={cx} cy={cy} r={3.5} fill={color} stroke="var(--surface)" strokeWidth={1.5} />
    </g>
  );
}

/**
 * §3 Curva SOC del día: 0–100 %, zona de reserva 0–20 % rose, parte futura
 * como proyección discontinua y anotaciones de eventos de carga/descarga.
 */
export default function SocDayChart({ data, nowMin, height = 280 }: SocDayChartProps) {
  const palette = useEnergyColors();

  const { rows, events, nowIdx } = useMemo(() => {
    const idx = Math.round(nowMin / STEP_MIN);
    const rows = data.map((p, i) => ({
      t: p.t,
      label: p.label,
      socPast: i <= idx ? p.soc : null,
      socFuture: i >= idx ? p.soc : null,
    }));
    const events: SocEvent[] = [];
    const chargeStart = data.find((p) => p.batteryPower > 0.05);
    if (chargeStart) events.push({ t: chargeStart.t, soc: chargeStart.soc, label: `Inicio de carga ${chargeStart.label}`, past: chargeStart.t <= nowMin });
    const full = data.find((p) => p.soc >= 99.5);
    if (full) events.push({ t: full.t, soc: full.soc, label: `Batería llena ${full.label}`, past: full.t <= nowMin });
    const dischargeStart = data.find((p) => p.batteryPower < -0.05);
    if (dischargeStart) events.push({ t: dischargeStart.t, soc: dischargeStart.soc, label: `Inicio descarga ${dischargeStart.label}`, past: dischargeStart.t <= nowMin });
    return { rows, events, nowIdx: idx };
  }, [data, nowMin]);

  const nowT = nowIdx * STEP_MIN;

  return (
    <section className="helios-card shadow-card dark:shadow-card-dark" aria-label="Estado de carga de la batería a lo largo del día">
      <style>{`.helios-soc-pop{transform-box:fill-box;transform-origin:center;animation:helios-soc-pop .45s cubic-bezier(.34,1.56,.64,1) both}@keyframes helios-soc-pop{from{transform:scale(0)}to{transform:scale(1)}}`}</style>
      <div className="flex flex-wrap items-center gap-2 px-4 pb-1 pt-4 sm:px-5">
        <div className="mr-auto">
          <h2 className="text-[15px] font-semibold text-app">Estado de carga · hoy</h2>
          <p className="text-xs text-faint">0–100 % · proyección nocturna discontinua</p>
        </div>
      </div>

      <div className="px-1">
        <ResponsiveContainer width="100%" height={height} className="max-lg:!h-[220px]">
          <AreaChart data={rows} margin={{ top: 10, right: 12, bottom: 0, left: 4 }}>
            <defs>
              <linearGradient id="grad-soc-day" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={palette.bateria} stopOpacity={0.25} />
                <stop offset="100%" stopColor={palette.bateria} stopOpacity={0} />
              </linearGradient>
              <linearGradient id="grad-soc-now" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="#F59E0B" />
                <stop offset="100%" stopColor="#F97316" />
              </linearGradient>
            </defs>
            <CartesianGrid vertical={false} stroke="var(--line)" strokeOpacity={0.6} strokeDasharray="3 6" />
            <XAxis
              dataKey="t"
              type="number"
              domain={[0, 1440]}
              ticks={AXIS_TICKS}
              tickFormatter={(v: number) => fmtTime(v)}
              tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
              axisLine={false}
              tickLine={false}
            />
            <YAxis
              domain={[0, 100]}
              ticks={[0, 20, 40, 60, 80, 100]}
              width={30}
              tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
              axisLine={false}
              tickLine={false}
            />
            <Tooltip content={<SocTooltip />} cursor={{ stroke: 'var(--text-faint)', strokeDasharray: '3 3', strokeOpacity: 0.5 }} />

            {/* Zona de reserva 0–20 % */}
            <ReferenceArea
              y1={0}
              y2={20}
              fill={palette.redCompra}
              fillOpacity={0.06}
              label={{ value: 'Reserva', position: 'insideBottomRight', fill: palette.redCompra, fontSize: 10, fontWeight: 600 }}
            />

            <Area
              name="SOC"
              type="monotone"
              dataKey="socPast"
              stroke={palette.bateria}
              strokeWidth={2.5}
              fill="url(#grad-soc-day)"
              animationDuration={1200}
              connectNulls={false}
              dot={false}
              activeDot={{ r: 4 }}
            />
            <Area
              name="SOC (proyección)"
              type="monotone"
              dataKey="socFuture"
              stroke={palette.bateria}
              strokeWidth={2}
              strokeDasharray="6 6"
              strokeOpacity={0.55}
              fill="none"
              animationDuration={500}
              animationBegin={250}
              connectNulls={false}
              dot={false}
              activeDot={false}
              legendType="none"
              tooltipType="none"
            />

            {/* Anotaciones de eventos */}
            {events.map((e, i) => (
              <ReferenceDot
                key={e.label}
                x={e.t}
                y={e.soc}
                shape={<AnnotationDot color={palette.bateria} delay={700 + i * 150} faded={!e.past} label={e.label} />}
              />
            ))}

            <ReferenceLine
              x={nowT}
              stroke="url(#grad-soc-now)"
              strokeWidth={2}
              label={{ value: 'AHORA', position: 'insideTopLeft', fill: '#F59E0B', fontSize: 10, fontWeight: 700 }}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>

      {/* Leyenda de eventos */}
      <div className="flex flex-wrap gap-x-4 gap-y-1 px-4 pb-4 sm:px-5">
        {events.map((e) => (
          <span key={e.label} className={`inline-flex items-center gap-1.5 text-xs ${e.past ? 'text-muted' : 'text-faint'}`}>
            <span className="h-2 w-2 rounded-full" style={{ backgroundColor: palette.bateria, opacity: e.past ? 1 : 0.45 }} />
            {e.label}
          </span>
        ))}
      </div>
    </section>
  );
}

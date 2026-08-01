import { SOLAR_GRADIENT } from '@/lib/colors';
import { useMemo, useState } from 'react';
import {
  Area,
  AreaChart,
  CartesianGrid,
  Line,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { useTranslation } from 'react-i18next';
import { fmtWeekdayDate } from '@/i18n';
import type { PowerPoint } from '@/data/types';
import { STEP_MIN } from '@/data/types';
import { fmtKw, fmtTime } from '@/lib/format';
import { cn } from '@/lib/utils';

const AXIS_TICKS = [0, 180, 360, 540, 720, 900, 1080, 1260, 1440];

interface InverterDayChartProps {
  data: PowerPoint[];
  dataKey: 'solis' | 'fox';
  color: string;
  /** "Ahora" simulado (minutos): separa curva real de previsión. */
  nowMin: number;
  /** Fecha del día para el subtítulo. */
  today: Date;
  height?: number;
}

function InvTooltip({ active, payload, label }: { active?: boolean; payload?: { name: string; value: number; color?: string; stroke?: string }[]; label?: number }) {
  if (!active || !payload || payload.length === 0 || label === undefined) return null;
  const seen = new Set<string>();
  const rows = payload.filter((p) => {
    if (p.value === null || p.value === undefined || seen.has(p.name)) return false;
    seen.add(p.name);
    return true;
  });
  return (
    <div className="rounded-xl border border-app bg-surface/95 px-3 py-2 shadow-lg backdrop-blur-md">
      <p className="mb-1 text-xs font-semibold text-app">{fmtTime(label)}</p>
      {rows.map((p) => (
        <p key={p.name} className="flex items-center gap-1.5 text-xs text-muted">
          <span className="h-2 w-2 rounded-full" style={{ backgroundColor: p.color ?? p.stroke }} />
          {p.name} <span className="ml-auto pl-3 font-semibold text-app">{fmtKw(p.value)} kW</span>
        </p>
      ))}
    </div>
  );
}

/** §4 Curva del día del inversor con toggle "Comparar con ayer". */
export default function InverterDayChart({ data, dataKey, color, nowMin, today, height = 300 }: InverterDayChartProps) {
  const [showYesterday, setShowYesterday] = useState(false);
  const { t } = useTranslation();

  const rows = useMemo(() => {
    const nowIdx = Math.round(nowMin / STEP_MIN);
    return data.map((p, i) => ({
      t: p.t,
      past: i <= nowIdx ? p[dataKey] : null,
      future: i >= nowIdx ? p[dataKey] : null,
      // Curva de ayer (mock −8 %)
      yesterday: p[dataKey] * 0.92,
    }));
  }, [data, dataKey, nowMin]);

  const nowT = Math.round(nowMin / STEP_MIN) * STEP_MIN;
  const gradId = `grad-inv-${dataKey}`;

  return (
    <section className="helios-card h-full shadow-card dark:shadow-card-dark" aria-label={t('inversores.powerTodayAria')}>
      <div className="flex flex-wrap items-center gap-2 px-4 pb-1 pt-4 sm:px-5">
        <div className="mr-auto">
          <h2 className="text-[15px] font-semibold text-app">{t('inversores.powerToday')}</h2>
          <p className="text-xs capitalize text-faint">{fmtWeekdayDate(today)}</p>
        </div>
        <button
          onClick={() => setShowYesterday((v) => !v)}
          aria-pressed={showYesterday}
          className={cn(
            'inline-flex h-7 items-center gap-1.5 rounded-full border px-2.5 text-xs font-medium transition-colors',
            showYesterday ? 'border-app bg-surface-2 text-app' : 'border-app text-faint hover:text-muted',
          )}
        >
          <span className={cn('h-2 w-2 rounded-full bg-slate-400', !showYesterday && 'opacity-35')} />
          {t('inversores.compareYesterday')}
        </button>
      </div>

      <div className="px-1 pb-2">
        <ResponsiveContainer width="100%" height={height} className="max-lg:!h-[230px]">
          <AreaChart data={rows} margin={{ top: 10, right: 12, bottom: 0, left: 4 }}>
            <defs>
              <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={color} stopOpacity={0.25} />
                <stop offset="100%" stopColor={color} stopOpacity={0} />
              </linearGradient>
              <linearGradient id={`${gradId}-now`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={SOLAR_GRADIENT.from} />
                <stop offset="100%" stopColor={SOLAR_GRADIENT.to} />
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
              domain={[0, 'dataMax + 0.4']}
              width={30}
              tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
              axisLine={false}
              tickLine={false}
            />
            <Tooltip content={<InvTooltip />} cursor={{ stroke: 'var(--text-faint)', strokeDasharray: '3 3', strokeOpacity: 0.5 }} />

            <Area
              name={t('inversores.today')}
              type="monotone"
              dataKey="past"
              stroke={color}
              strokeWidth={2.5}
              fill={`url(#${gradId})`}
              animationDuration={1100}
              connectNulls={false}
              dot={false}
              activeDot={{ r: 4 }}
            />
            <Area
              name={t('inversores.forecastToday')}
              type="monotone"
              dataKey="future"
              stroke={color}
              strokeWidth={2}
              strokeDasharray="6 6"
              strokeOpacity={0.5}
              fill="none"
              animationDuration={500}
              animationBegin={200}
              connectNulls={false}
              dot={false}
              activeDot={false}
              legendType="none"
              tooltipType="none"
            />
            {showYesterday && (
              <Line
                name={t('inversores.yesterday')}
                type="monotone"
                dataKey="yesterday"
                stroke="var(--text-faint)"
                strokeWidth={1.5}
                strokeDasharray="4 4"
                dot={false}
                activeDot={{ r: 3 }}
                animationDuration={400}
              />
            )}

            <ReferenceLine
              x={nowT}
              stroke={`url(#${gradId}-now)`}
              strokeWidth={2}
              label={{ value: 'AHORA', position: 'insideTopLeft', fill: SOLAR_GRADIENT.from, fontSize: 10, fontWeight: 700 }}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </section>
  );
}

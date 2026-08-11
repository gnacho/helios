import { SOLAR_GRADIENT } from '@/lib/colors';
import { useMemo, useState } from 'react';
import {
  Area,
  AreaChart,
  CartesianGrid,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { AnimatePresence, motion } from 'framer-motion';
import { TrendingUp } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { PowerPoint } from '@/data/types';
import { STEP_MIN, seriesInvValue } from '@/data/types';
import { fmtKw, fmtTime } from '@/lib/format';
import { cn } from '@/lib/utils';

const AXIS_TICKS = [0, 180, 360, 540, 720, 900, 1080, 1260, 1440];

export interface CompareInverter {
  key: string;
  name: string;
  color: string;
  kwp: number;
}

interface CompareChartProps {
  data: PowerPoint[];
  inverters: CompareInverter[];
  nowMin: number;
  height?: number;
}

function CompareTooltip({
  active,
  payload,
  label,
  normalized,
}: {
  active?: boolean;
  payload?: { name: string; value: number; color?: string; stroke?: string }[];
  label?: number;
  normalized?: boolean;
}) {
  if (!active || !payload || payload.length === 0 || label === undefined) return null;
  const unit = normalized ? 'kW/kWp' : 'kW';
  return (
    <div className="rounded-xl border border-app bg-surface/95 px-3 py-2 shadow-lg backdrop-blur-md">
      <p className="mb-1 text-xs font-semibold text-app">{fmtTime(label)}</p>
      {payload
        .filter((p) => p.value !== null && p.value !== undefined)
        .map((p) => (
          <p key={p.name} className="flex items-center gap-1.5 text-xs text-muted">
            <span className="h-2 w-2 rounded-full" style={{ backgroundColor: p.color ?? p.stroke }} />
            {p.name}{' '}
            <span className="ml-auto pl-3 font-semibold text-app">
              {fmtKw(p.value)} {unit}
            </span>
          </p>
        ))}
    </div>
  );
}

const seriesValue = (p: PowerPoint, key: string): number => seriesInvValue(p, key);

/**
 * §7 Gráfica superpuesta de los inversores con toggle "Normalizar por kWp"
 * (divide cada curva por su potencia pico instalada).
 */
export default function CompareChart({ data, inverters, nowMin, height = 340 }: CompareChartProps) {
  const { t } = useTranslation();
  const [normalized, setNormalized] = useState(false);

  const rows = useMemo(
    () =>
      data.map((p) => {
        const row: Record<string, number> = { t: p.t };
        for (const inv of inverters) {
          row[inv.key] = normalized && inv.kwp > 0 ? seriesValue(p, inv.key) / inv.kwp : seriesValue(p, inv.key);
        }
        return row;
      }),
    [data, inverters, normalized],
  );

  const nowT = Math.round(nowMin / STEP_MIN) * STEP_MIN;
  // En topologías de 2 inversores se puede comparar rendimiento normalizado;
  // con más, el aviso es menos informativo → solo con exactamente 2.
  const twoInv = inverters.length === 2;

  return (
    <section className="helios-card shadow-card dark:shadow-card-dark" aria-label={t('inversores.curvesTitle')}>
      <div className="flex flex-wrap items-center gap-2 px-4 pb-1 pt-4 sm:px-5">
        <div className="mr-auto">
          <h2 className="text-[15px] font-semibold text-app">{t('inversores.curvesTitle')}</h2>
          <p className="text-xs text-faint">{normalized ? t('inversores.specificYield') : t('inversores.instantPower')}</p>
        </div>
        {inverters.map((inv) => (
          <span key={inv.key} className="inline-flex items-center gap-1.5 text-xs text-muted">
            <span className="h-2 w-2 rounded-full" style={{ backgroundColor: inv.color }} /> {inv.name}
          </span>
        ))}
        <button
          onClick={() => setNormalized((v) => !v)}
          aria-pressed={normalized}
          className={cn(
            'inline-flex h-7 items-center gap-1.5 rounded-full border px-2.5 text-xs font-medium transition-colors',
            normalized ? 'border-app bg-surface-2 font-semibold text-app' : 'border-app text-faint hover:text-muted',
          )}
        >
          {t('inversores.normalize')}
        </button>
      </div>

      <AnimatePresence>
        {normalized && twoInv && (
          <motion.p
            initial={{ opacity: 0, y: -6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -6 }}
            transition={{ duration: 0.3 }}
            className="mx-4 mb-1 mt-1 inline-flex w-fit items-center gap-1.5 rounded-full bg-emerald-500/12 px-3 py-1 text-xs font-semibold text-emerald-600 dark:text-emerald-400 sm:mx-5"
          >
            <TrendingUp size={13} /> {t('inversores.foxBetter')}
          </motion.p>
        )}
      </AnimatePresence>

      <div className="px-1 pb-2">
        <ResponsiveContainer width="100%" height={height} className="max-lg:!h-[240px]">
          <AreaChart data={rows} margin={{ top: 10, right: 12, bottom: 0, left: 4 }}>
            <defs>
              {inverters.map((inv) => (
                <linearGradient key={`grad-cmp-${inv.key}`} id={`grad-cmp-${inv.key}`} x1="0" y1="0" x2="0" y2="1">
                  <stop offset="0%" stopColor={inv.color} stopOpacity={0.2} />
                  <stop offset="100%" stopColor={inv.color} stopOpacity={0} />
                </linearGradient>
              ))}
              <linearGradient id="grad-cmp-now" x1="0" y1="0" x2="0" y2="1">
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
              domain={normalized ? [0, 1] : [0, 'dataMax + 0.4']}
              width={30}
              tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
              tickFormatter={(v: number) => (normalized ? v.toFixed(1) : `${v}`)}
              axisLine={false}
              tickLine={false}
            />
            <Tooltip
              content={<CompareTooltip normalized={normalized} />}
              cursor={{ stroke: 'var(--text-faint)', strokeDasharray: '3 3', strokeOpacity: 0.5 }}
            />

            {inverters.map((inv, i) => (
              <Area
                key={inv.key}
                name={inv.name}
                type="monotone"
                dataKey={inv.key}
                stroke={inv.color}
                strokeWidth={2}
                fill={`url(#grad-cmp-${inv.key})`}
                fillOpacity={0.9}
                animationDuration={1000}
                animationBegin={i * 300}
                dot={false}
                activeDot={{ r: 4 }}
              />
            ))}

            <ReferenceLine
              x={nowT}
              stroke="url(#grad-cmp-now)"
              strokeWidth={2}
              label={{ value: 'AHORA', position: 'insideTopLeft', fill: SOLAR_GRADIENT.from, fontSize: 10, fontWeight: 700 }}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </section>
  );
}

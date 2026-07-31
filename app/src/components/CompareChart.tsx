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
import { FOX_KWP, SOLIS_KWP, STEP_MIN } from '@/data/types';
import { useEnergyColors } from '@/lib/colors';
import { fmtKw, fmtTime } from '@/lib/format';
import { cn } from '@/lib/utils';

const AXIS_TICKS = [0, 180, 360, 540, 720, 900, 1080, 1260, 1440];

interface CompareChartProps {
  data: PowerPoint[];
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

/**
 * §7 Gráfica superpuesta Solis vs Fox con toggle "Normalizar por kWp"
 * (divide cada curva por su potencia pico instalada: Solis/4,4 · Fox/2,7).
 */
export default function CompareChart({ data, nowMin, height = 340 }: CompareChartProps) {
  const palette = useEnergyColors();
  const { t } = useTranslation();
  const [normalized, setNormalized] = useState(false);

  // Mismas keys de datos en ambos modos → Recharts hace tween de valores al normalizar.
  const rows = useMemo(
    () =>
      data.map((p) => ({
        t: p.t,
        solis: normalized ? p.solis / SOLIS_KWP : p.solis,
        fox: normalized ? p.fox / FOX_KWP : p.fox,
      })),
    [data, normalized],
  );

  const nowT = Math.round(nowMin / STEP_MIN) * STEP_MIN;

  return (
    <section className="helios-card shadow-card dark:shadow-card-dark" aria-label={t('inversores.curvesTitle')}>
      <div className="flex flex-wrap items-center gap-2 px-4 pb-1 pt-4 sm:px-5">
        <div className="mr-auto">
          <h2 className="text-[15px] font-semibold text-app">{t('inversores.curvesTitle')}</h2>
          <p className="text-xs text-faint">{normalized ? t('inversores.specificYield') : t('inversores.instantPower')}</p>
        </div>
        <span className="inline-flex items-center gap-1.5 text-xs text-muted">
          <span className="h-2 w-2 rounded-full" style={{ backgroundColor: palette.solis }} /> Solis
        </span>
        <span className="inline-flex items-center gap-1.5 text-xs text-muted">
          <span className="h-2 w-2 rounded-full" style={{ backgroundColor: palette.fox }} /> Fox
        </span>
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
        {normalized && (
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
              <linearGradient id="grad-cmp-solis" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={palette.solis} stopOpacity={0.2} />
                <stop offset="100%" stopColor={palette.solis} stopOpacity={0} />
              </linearGradient>
              <linearGradient id="grad-cmp-fox" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={palette.fox} stopOpacity={0.2} />
                <stop offset="100%" stopColor={palette.fox} stopOpacity={0} />
              </linearGradient>
              <linearGradient id="grad-cmp-now" x1="0" y1="0" x2="0" y2="1">
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

            {/* fillOpacity 0.18: mezcla legible donde se solapan */}
            <Area
              name="Solis"
              type="monotone"
              dataKey="solis"
              stroke={palette.solis}
              strokeWidth={2}
              fill="url(#grad-cmp-solis)"
              fillOpacity={0.9}
              animationDuration={1000}
              dot={false}
              activeDot={{ r: 4 }}
            />
            <Area
              name="Fox"
              type="monotone"
              dataKey="fox"
              stroke={palette.fox}
              strokeWidth={2}
              fill="url(#grad-cmp-fox)"
              fillOpacity={0.9}
              animationDuration={1000}
              animationBegin={300}
              dot={false}
              activeDot={{ r: 4 }}
            />

            <ReferenceLine
              x={nowT}
              stroke="url(#grad-cmp-now)"
              strokeWidth={2}
              label={{ value: 'AHORA', position: 'insideTopLeft', fill: '#F59E0B', fontSize: 10, fontWeight: 700 }}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </section>
  );
}

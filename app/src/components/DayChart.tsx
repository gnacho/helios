import { SOLAR_GRADIENT } from '@/lib/colors';
import { useCallback, useMemo, useRef, useState } from 'react';
import {
  Area,
  ComposedChart,
  CartesianGrid,
  Line,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { AnimatePresence, motion } from 'framer-motion';
import { Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { PowerPoint } from '@/data/types';
import { STEP_MIN } from '@/data/types';
import { useEnergyColors } from '@/lib/colors';
import { fmtKw, fmtTime } from '@/lib/format';
import { cn } from '@/lib/utils';

const PLOT_LEFT = 34; // margen izq (4) + ancho eje Y (30)
const PLOT_RIGHT = 12;
const AXIS_TICKS = [0, 180, 360, 540, 720, 900, 1080, 1260, 1440];

interface ChartRow {
  t: number;
  label: string;
  production: number;
  productionPast: number | null;
  productionFuture: number | null;
  consumption: number;
  solis: number;
  fox: number;
}

type SeriesKey = 'total' | 'solis' | 'fox' | 'consumo';

interface DayChartProps {
  data: PowerPoint[];
  /** "Ahora" real simulado (minutos): separa curva real de previsión. */
  nowMin: number;
  /** Instante de replay (minutos) o null. */
  replayMin: number | null;
  onReplayChange: (min: number | null) => void;
  height?: number;
}

function ChartTooltip({ active, payload, label }: { active?: boolean; payload?: { name: string; value: number; color: string }[]; label?: number }) {
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
          <span className="h-2 w-2 rounded-full" style={{ backgroundColor: p.color }} />
          {p.name} <span className="ml-auto pl-3 font-semibold text-app">{fmtKw(p.value)} kW</span>
        </p>
      ))}
    </div>
  );
}

/** Gráfica gigante del día + scrubber "replay del día" (DayTimelineScrubber). */
export default function DayChart({ data, nowMin, replayMin, onReplayChange, height = 380, fill = false }: DayChartProps & { fill?: boolean }) {
  const palette = useEnergyColors();
  const { t } = useTranslation();
  const [visible, setVisible] = useState<Record<SeriesKey, boolean>>({ total: true, solis: false, fox: false, consumo: true });
  const [replayArmed, setReplayArmed] = useState(false);
  const plotRef = useRef<HTMLDivElement>(null);

  const rows = useMemo<ChartRow[]>(() => {
    const nowIdx = Math.round(nowMin / STEP_MIN);
    return data.map((p, i) => ({
      t: p.t,
      label: p.label,
      production: p.production,
      productionPast: i <= nowIdx ? p.production : null,
      productionFuture: i >= nowIdx ? p.production : null,
      consumption: p.consumption,
      solis: p.solis,
      fox: p.fox,
    }));
  }, [data, nowMin]);

  const yMax = useMemo(() => {
    let max = 0.5;
    for (const r of rows) {
      if (visible.total && r.production > max) max = r.production;
      if (visible.solis && r.solis > max) max = r.solis;
      if (visible.fox && r.fox > max) max = r.fox;
      if (visible.consumo && r.consumption > max) max = r.consumption;
    }
    return Math.max(1, Math.ceil(max + 0.3));
  }, [rows, visible]);

  const minFromClientX = useCallback((clientX: number): number => {
    const el = plotRef.current;
    if (!el) return 0;
    const rect = el.getBoundingClientRect();
    const frac = Math.min(1, Math.max(0, (clientX - rect.left - PLOT_LEFT) / (rect.width - PLOT_LEFT - PLOT_RIGHT)));
    return Math.round((frac * 1440) / STEP_MIN) * STEP_MIN;
  }, []);

  const scrubTo = useCallback(
    (clientX: number) => {
      onReplayChange(Math.min(1435, minFromClientX(clientX)));
    },
    [minFromClientX, onReplayChange],
  );

  const exitReplay = useCallback(() => {
    setReplayArmed(false);
    onReplayChange(null);
  }, [onReplayChange]);

  const replaying = replayMin !== null;

  const seriesChips: { key: SeriesKey; label: string; color: string }[] = [
    { key: 'total', label: t('chart.total'), color: palette.solar },
    { key: 'solis', label: 'Solis', color: palette.solis },
    { key: 'fox', label: 'Fox', color: palette.fox },
    { key: 'consumo', label: t('chart.consumption'), color: palette.consumo },
  ];

  return (
    <section
      className={cn('helios-card relative overflow-hidden shadow-card dark:shadow-card-dark', fill && 'flex h-full flex-col')}
      aria-label={t('chart.aria')}
    >
      {/* Header */}
      <div className="flex flex-wrap items-center gap-2 px-4 pb-1 pt-4 sm:px-5">
        <div className="mr-auto">
          <h2 className="text-[15px] font-semibold text-app">{t('chart.title')}</h2>
          <p className="text-xs text-faint">{t('chart.subtitle')}</p>
        </div>
        {seriesChips.map((chip) => (
          <button
            key={chip.key}
            onClick={() => setVisible((v) => ({ ...v, [chip.key]: !v[chip.key] }))}
            aria-pressed={visible[chip.key]}
            className={cn(
              'inline-flex h-7 items-center gap-1.5 rounded-full border px-2.5 text-xs font-medium transition-colors',
              visible[chip.key] ? 'border-app bg-surface-2 text-app' : 'border-app text-faint hover:text-muted',
            )}
          >
            <span className="h-2 w-2 rounded-full" style={{ backgroundColor: chip.color, opacity: visible[chip.key] ? 1 : 0.35 }} />
            {chip.label}
          </button>
        ))}
        <button
          onClick={() => {
            if (replaying) exitReplay();
            else {
              setReplayArmed(true);
              onReplayChange(Math.round(nowMin / STEP_MIN) * STEP_MIN);
            }
          }}
          aria-pressed={replaying}
          className={cn(
            'inline-flex h-7 items-center gap-1.5 rounded-full px-3 text-xs font-semibold transition-all',
            replaying ? 'bg-brand-gradient text-white shadow-md' : 'border border-app text-amber-500 hover:bg-surface-2',
          )}
        >
          <Zap size={13} strokeWidth={2.4} /> {t('chart.replay')}
        </button>
      </div>

      {/* Leyenda compacta */}
      <div className="flex flex-wrap gap-x-4 gap-y-1 px-4 pb-1 sm:px-5">
        {seriesChips
          .filter((c) => visible[c.key])
          .map((c) => (
            <span key={c.key} className="inline-flex items-center gap-1.5 text-xs text-muted">
              <span className="h-2 w-2 rounded-full" style={{ backgroundColor: c.color }} />
              {c.key === 'total' ? t('chart.productionTotal') : c.label}
            </span>
          ))}
        {replaying && (
          <span className="inline-flex items-center gap-1.5 text-xs font-semibold text-amber-500">
            {t('chart.replayAt', { time: fmtTime(replayMin) })}
          </span>
        )}
      </div>

      {/* Gráfica + scrubber */}
      <div
        ref={plotRef}
        className={cn('relative px-1', fill && 'min-h-[380px] flex-1 max-lg:min-h-0', replayArmed && !replaying && 'cursor-crosshair')}
        onDoubleClick={exitReplay}
        onPointerDown={(e) => {
          if (replaying || replayArmed) {
            setReplayArmed(true);
            scrubTo(e.clientX);
          }
        }}
        onPointerMove={(e) => {
          if (replaying && (e.buttons === 1 || e.pointerType === 'touch')) scrubTo(e.clientX);
        }}
        onKeyDown={(e) => {
          if (!replaying) return;
          if (e.key === 'Escape') exitReplay();
          else if (e.key === 'ArrowLeft' && replayMin !== null) onReplayChange(Math.max(0, replayMin - STEP_MIN));
          else if (e.key === 'ArrowRight' && replayMin !== null) onReplayChange(Math.min(1435, replayMin + STEP_MIN));
        }}
        role={replaying ? 'slider' : undefined}
        tabIndex={replaying ? 0 : undefined}
        aria-valuemin={replaying ? 0 : undefined}
        aria-valuemax={replaying ? 1435 : undefined}
        aria-valuenow={replaying ? replayMin : undefined}
        aria-valuetext={replaying ? fmtTime(replayMin) : undefined}
        aria-label={replaying ? t('chart.replayAria') : undefined}
      >
        <ResponsiveContainer width="100%" height={fill ? '100%' : height} className="max-lg:!h-[260px]">
          <ComposedChart data={rows} margin={{ top: 10, right: PLOT_RIGHT, bottom: 0, left: 4 }}>
            <defs>
              <linearGradient id="grad-prod" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={palette.solar} stopOpacity={0.25} />
                <stop offset="100%" stopColor={palette.solar} stopOpacity={0} />
              </linearGradient>
              <linearGradient id="grad-cons" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={palette.consumo} stopOpacity={0.18} />
                <stop offset="100%" stopColor={palette.consumo} stopOpacity={0} />
              </linearGradient>
              <linearGradient id="grad-now" x1="0" y1="0" x2="0" y2="1">
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
              domain={[0, yMax]}
              width={30}
              ticks={Array.from({ length: yMax + 1 }, (_, i) => i)}
              tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
              tickFormatter={(v: number) => `${v}`}
              axisLine={false}
              tickLine={false}
              unit=""
            />
            <Tooltip content={<ChartTooltip />} cursor={{ stroke: 'var(--text-faint)', strokeDasharray: '3 3', strokeOpacity: 0.5 }} />

            {visible.total && (
              <Area
                name={t('chart.production')}
                type="monotone"
                dataKey="productionPast"
                stroke={palette.solar}
                strokeWidth={2.5}
                fill="url(#grad-prod)"
                animationDuration={1200}
                connectNulls={false}
                dot={false}
                activeDot={{ r: 4 }}
              />
            )}
            {visible.total && (
              <Area
                name={t('chart.productionForecast')}
                type="monotone"
                dataKey="productionFuture"
                stroke={palette.solar}
                strokeWidth={2}
                strokeDasharray="6 6"
                strokeOpacity={0.5}
                fill="none"
                animationDuration={600}
                connectNulls={false}
                dot={false}
                activeDot={false}
                legendType="none"
                tooltipType="none"
              />
            )}
            {visible.consumo && (
              <Area
                name={t('chart.consumption')}
                type="monotone"
                dataKey="consumption"
                stroke={palette.consumo}
                strokeWidth={2}
                fill="url(#grad-cons)"
                animationDuration={1200}
                animationBegin={300}
                dot={false}
                activeDot={{ r: 4 }}
              />
            )}
            {visible.solis && (
              <Line name="Solis" type="monotone" dataKey="solis" stroke={palette.solis} strokeWidth={1.5} dot={false} animationDuration={900} />
            )}
            {visible.fox && (
              <Line name="Fox" type="monotone" dataKey="fox" stroke={palette.fox} strokeWidth={1.5} dot={false} animationDuration={900} />
            )}

            {/* Marcador AHORA */}
            <ReferenceLine
              x={Math.round(nowMin / STEP_MIN) * STEP_MIN}
              stroke="url(#grad-now)"
              strokeWidth={2}
              label={{
                value: t('chart.now'),
                position: 'insideTopLeft',
                fill: SOLAR_GRADIENT.from,
                fontSize: 10,
                fontWeight: 700,
              }}
            />
            {/* Marcador de replay */}
            {replaying && (
              <ReferenceLine
                x={replayMin}
                stroke={palette.solar}
                strokeWidth={2}
                label={{ value: fmtTime(replayMin), position: 'top', fill: palette.solar, fontSize: 11, fontWeight: 700 }}
              />
            )}
          </ComposedChart>
        </ResponsiveContainer>
      </div>

      {/* Chip flotante "Volver a ahora" */}
      <AnimatePresence>
        {replaying && (
          <motion.button
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: 16 }}
            transition={{ type: 'spring', stiffness: 400, damping: 28 }}
            onClick={exitReplay}
            className="bg-brand-gradient absolute bottom-12 left-1/2 z-10 -translate-x-1/2 rounded-full px-4 py-2 text-xs font-semibold text-white shadow-lg"
          >
            {t('chart.backToNow')}
          </motion.button>
        )}
      </AnimatePresence>

      {/* Tabla accesible de la curva principal */}
      <table className="sr-only">
        <caption>{t('chart.caption')}</caption>
        <thead>
          <tr>
            <th>{t('chart.hour')}</th>
            <th>{t('chart.production')} (kW)</th>
            <th>{t('chart.consumption')} (kW)</th>
          </tr>
        </thead>
        <tbody>
          {rows
            .filter((r) => r.t % 60 === 0)
            .map((r) => (
              <tr key={r.t}>
                <td>{r.label}</td>
                <td>{fmtKw(r.production)}</td>
                <td>{fmtKw(r.consumption)}</td>
              </tr>
            ))}
        </tbody>
      </table>
    </section>
  );
}

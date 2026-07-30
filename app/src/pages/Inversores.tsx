import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { AnimatePresence, motion } from 'framer-motion';
import {
  Activity,
  ArrowLeftRight,
  Bell,
  CalendarRange,
  Gauge,
  History,
  RefreshCw,
  Sun,
  Triangle,
} from 'lucide-react';
import { format } from 'date-fns';
import { es } from 'date-fns/locale';
import {
  Bar,
  BarChart,
  Cell,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { useEnergyData } from '@/data/EnergyDataProvider';
import { FOX_KWP, SOLIS_KWP, STEP_MIN } from '@/data/types';
import { useEnergyColors } from '@/lib/colors';
import { fmtClock, fmtEnergy, fmtKw, fmtPct, fmtTime } from '@/lib/format';
import { cn } from '@/lib/utils';
import KpiCard from '@/components/KpiCard';
import InverterHeroCard from '@/components/InverterHeroCard';
import type { InverterMeta } from '@/components/InverterHeroCard';
import InverterDayChart from '@/components/InverterDayChart';
import CompareChart from '@/components/CompareChart';
import ThemeToggle from '@/components/ThemeToggle';
import ConnectionStatus from '@/components/ConnectionStatus';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Tooltip as UiTooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';

const EASE_OUT: [number, number, number, number] = [0.25, 1, 0.5, 1];

const INVERTERS: Record<'solis' | 'fox', InverterMeta> = {
  solis: {
    key: 'solis',
    name: 'Solis',
    model: 'Solis S5-EH1P5K-L (híbrido)',
    kwp: SOLIS_KWP,
    panels: '10 × 440 W',
    monthKwh: 412,
    totalMwh: 8.42,
    tempC: 46,
    hasBattery: true,
  },
  fox: {
    key: 'fox',
    name: 'Fox',
    model: 'Fox H1-3.0-E',
    kwp: FOX_KWP,
    panels: '6 × 450 W',
    monthKwh: 251,
    totalMwh: 3.15,
    tempC: 41,
    hasBattery: false,
  },
};

type TabKey = 'solis' | 'fox' | 'compare';

// ── §5 Producción últimos 14 días ───────────────────────────────────────────

interface Last14Props {
  share: number;
  color: string;
}

function Last14Chart({ share, color }: Last14Props) {
  const { getHistory } = useEnergyData();
  const rows = useMemo(
    () =>
      getHistory()
        .slice(-14)
        .map((d) => ({
          dayNum: d.date.getDate(),
          label: format(d.date, 'd MMM', { locale: es }),
          kwh: d.productionKwh * share,
        })),
    [getHistory, share],
  );

  return (
    <section className="helios-card h-full shadow-card dark:shadow-card-dark" aria-label="Producción de los últimos 14 días">
      <div className="px-4 pb-1 pt-4 sm:px-5">
        <h2 className="text-[15px] font-semibold text-app">Últimos 14 días</h2>
        <p className="text-xs text-faint">Energía diaria · kWh</p>
      </div>
      <div className="px-1 pb-2">
        <ResponsiveContainer width="100%" height={238} className="max-lg:!h-[220px]">
          <BarChart data={rows} margin={{ top: 10, right: 12, bottom: 0, left: 12 }}>
            <XAxis
              dataKey="dayNum"
              tick={{ fontSize: 11, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
              axisLine={false}
              tickLine={false}
              interval={1}
            />
            <YAxis hide domain={[0, 'dataMax + 4']} />
            <Tooltip
              cursor={{ fill: 'var(--surface-2)', opacity: 0.5 }}
              content={({ active, payload }) => {
                if (!active || !payload || payload.length === 0) return null;
                const d = payload[0]?.payload as { label: string; kwh: number } | undefined;
                if (!d) return null;
                return (
                  <div className="rounded-xl border border-app bg-surface/95 px-3 py-2 text-xs shadow-lg backdrop-blur-md">
                    <p className="font-semibold text-app">
                      {d.label} · {fmtEnergy(d.kwh)} kWh
                    </p>
                  </div>
                );
              }}
            />
            <Bar dataKey="kwh" name="Producción" fill={color} radius={[4, 4, 0, 0]} animationDuration={500}>
              {rows.map((r, i) => (
                <Cell
                  key={r.dayNum}
                  opacity={i === rows.length - 1 ? 1 : 0.55}
                  stroke={i === rows.length - 1 ? color : 'none'}
                  strokeWidth={i === rows.length - 1 ? 1.5 : 0}
                />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>
    </section>
  );
}

// ── Vista por inversor (§2–§5) ──────────────────────────────────────────────

interface InverterViewProps {
  meta: InverterMeta;
  nowKw: number;
  dayKwh: number;
  share: number;
  color: string;
}

function InverterView({ meta, nowKw, dayKwh, share, color }: InverterViewProps) {
  const { nowMin, today, getDaySeries } = useEnergyData();
  const series = useMemo(() => getDaySeries(today), [getDaySeries, today]);

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-12 lg:gap-5">
      <div className="lg:col-span-4">
        <InverterHeroCard meta={meta} nowKw={nowKw} color={color} />
      </div>

      {/* §3 KPIs del inversor */}
      <div className="grid grid-cols-2 gap-4 lg:col-span-8 xl:grid-cols-4" aria-label={`Indicadores del ${meta.name}`}>
        <KpiCard
          icon={Sun}
          color={meta.key}
          label="Hoy"
          value={dayKwh}
          unit="kWh"
          delta={{ direction: 'up', text: '8% vs ayer', good: true }}
          index={0}
        />
        <KpiCard icon={CalendarRange} color={meta.key} label="Este mes" value={meta.monthKwh} unit="kWh" decimals={0} index={1} />
        <UiTooltip>
          <TooltipTrigger asChild>
            <div className="h-full">
              <KpiCard icon={History} color={meta.key} label="Total" value={meta.totalMwh} unit="MWh" decimals={2} index={2} />
            </div>
          </TooltipTrigger>
          <TooltipContent>Desde puesta en marcha · 14 marzo 2023</TooltipContent>
        </UiTooltip>
        <KpiCard icon={Gauge} color={meta.key} label="Rendimiento" value={dayKwh / meta.kwp} unit="kWh/kWp" index={3} />
      </div>

      {/* §4 Curva del día */}
      <motion.div
        initial={{ opacity: 0, y: 24 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.5, delay: 0.2, ease: EASE_OUT }}
        className="lg:col-span-8"
      >
        <InverterDayChart data={series} dataKey={meta.key} color={color} nowMin={nowMin} today={today} height={300} />
      </motion.div>

      {/* §5 Barras 14 días */}
      <motion.div
        initial={{ opacity: 0, y: 24 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, amount: 0.2 }}
        transition={{ duration: 0.5, delay: 0.1, ease: EASE_OUT }}
        className="lg:col-span-4"
      >
        <Last14Chart share={share} color={color} />
      </motion.div>
    </div>
  );
}

// ── Vista comparativa (§6–§9) ───────────────────────────────────────────────

interface CompareViewProps {
  nowSolis: number;
  nowFox: number;
  solisKwh: number;
  foxKwh: number;
  peakSolis: { v: number; t: number };
  peakFox: { v: number; t: number };
}

function CompareView({ nowSolis, nowFox, solisKwh, foxKwh, peakSolis, peakFox }: CompareViewProps) {
  const { nowMin, today, getDaySeries } = useEnergyData();
  const palette = useEnergyColors();
  const series = useMemo(() => getDaySeries(today), [getDaySeries, today]);
  const [repartoMode, setRepartoMode] = useState<'barra' | 'donut'>('barra');

  const total = solisKwh + foxKwh;
  const shareSolis = total > 0 ? (solisKwh / total) * 100 : 50;
  const shareFox = 100 - shareSolis;

  const nf2 = new Intl.NumberFormat('es-ES', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

  const bestChip = (
    <span className="ml-1.5 inline-flex items-center rounded-full bg-emerald-500/12 p-0.5 text-emerald-600 dark:text-emerald-400" aria-label="Mejor métrica">
      <Triangle size={8} fill="currentColor" strokeWidth={0} />
    </span>
  );

  const metricRows: { label: string; solis: React.ReactNode; fox: React.ReactNode; best: 'solis' | 'fox' | null }[] = [
    {
      label: 'Potencia ahora',
      solis: `${fmtKw(nowSolis)} kW`,
      fox: `${fmtKw(nowFox)} kW`,
      best: nowSolis >= nowFox ? 'solis' : 'fox',
    },
    {
      label: 'Energía hoy',
      solis: `${fmtEnergy(solisKwh)} kWh`,
      fox: `${fmtEnergy(foxKwh)} kWh`,
      best: solisKwh >= foxKwh ? 'solis' : 'fox',
    },
    {
      label: 'Pico del día (hora)',
      solis: `${fmtKw(peakSolis.v)} kW · ${fmtTime(peakSolis.t)}`,
      fox: `${fmtKw(peakFox.v)} kW · ${fmtTime(peakFox.t)}`,
      best: peakSolis.v >= peakFox.v ? 'solis' : 'fox',
    },
    {
      label: 'kWh/kWp',
      solis: nf2.format(solisKwh / SOLIS_KWP),
      fox: nf2.format(foxKwh / FOX_KWP),
      best: solisKwh / SOLIS_KWP >= foxKwh / FOX_KWP ? 'solis' : 'fox',
    },
    {
      label: 'Temperatura',
      solis: `${INVERTERS.solis.tempC} °C`,
      fox: `${INVERTERS.fox.tempC} °C`,
      best: 'fox', // más frío = mejor
    },
    {
      label: 'Aportación al total',
      solis: `${fmtPct(shareSolis)} %`,
      fox: `${fmtPct(shareFox)} %`,
      best: shareSolis >= shareFox ? 'solis' : 'fox',
    },
    {
      label: 'Estado',
      solis: (
        <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/12 px-2 py-0.5 text-[11px] font-semibold text-emerald-600 dark:text-emerald-400">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" /> Online
        </span>
      ),
      fox: (
        <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/12 px-2 py-0.5 text-[11px] font-semibold text-emerald-600 dark:text-emerald-400">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" /> Online
        </span>
      ),
      best: null,
    },
  ];

  return (
    <div className="flex flex-col gap-4 lg:gap-5">
      {/* §6 Hero comparativo */}
      <section className="helios-card relative overflow-hidden shadow-card dark:shadow-card-dark" aria-label="Comparativa de inversores">
        <div className="grid lg:grid-cols-2">
          <motion.div
            initial={{ opacity: 0, x: -32 }}
            animate={{ opacity: 1, x: 0 }}
            transition={{ duration: 0.5, ease: 'easeOut' }}
            className="flex flex-col gap-1 p-5 sm:p-6"
          >
            <p className="flex items-center gap-2 text-sm font-semibold text-app">
              <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: palette.solis }} />
              Solis <span className="text-xs font-normal text-faint">4,4 kWp</span>
            </p>
            <p className="font-display text-[32px] font-semibold leading-tight tracking-[-0.01em]" style={{ color: palette.solis }} aria-live="off">
              {fmtKw(nowSolis)} <span className="text-[0.55em] font-medium text-faint">kW</span>
            </p>
            <p className="text-sm text-muted">
              Hoy <span className="font-semibold text-app">{fmtEnergy(solisKwh)} kWh</span>
            </p>
          </motion.div>
          <motion.div
            initial={{ opacity: 0, x: 32 }}
            animate={{ opacity: 1, x: 0 }}
            transition={{ duration: 0.5, ease: 'easeOut' }}
            className="flex flex-col gap-1 border-t border-app p-5 sm:p-6 lg:border-l lg:border-t-0 lg:text-right"
          >
            <p className="flex items-center gap-2 text-sm font-semibold text-app lg:justify-end">
              <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: palette.fox }} />
              Fox <span className="text-xs font-normal text-faint">2,7 kWp</span>
            </p>
            <p className="font-display text-[32px] font-semibold leading-tight tracking-[-0.01em]" style={{ color: palette.fox }} aria-live="off">
              {fmtKw(nowFox)} <span className="text-[0.55em] font-medium text-faint">kW</span>
            </p>
            <p className="text-sm text-muted">
              Hoy <span className="font-semibold text-app">{fmtEnergy(foxKwh)} kWh</span>
            </p>
          </motion.div>
        </div>
        {/* Chip VS (solo desktop) */}
        <motion.span
          initial={{ scale: 0 }}
          animate={{ scale: 1 }}
          transition={{ type: 'spring', stiffness: 400, damping: 24, delay: 0.3 }}
          className="absolute left-1/2 top-1/2 hidden h-10 w-10 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full border border-app bg-surface font-display text-xs font-bold text-muted shadow-md lg:flex"
        >
          VS
        </motion.span>
      </section>

      {/* §7 Gráfica superpuesta */}
      <motion.div initial={{ opacity: 0, y: 24 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.5, delay: 0.15, ease: EASE_OUT }}>
        <CompareChart data={series} nowMin={nowMin} height={340} />
      </motion.div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12 lg:gap-5">
        {/* §8 Tabla de métricas */}
        <motion.section
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.2 }}
          transition={{ duration: 0.5, ease: EASE_OUT }}
          className="helios-card p-4 shadow-card dark:shadow-card-dark sm:p-5 lg:col-span-7"
          aria-label="Métricas de hoy lado a lado"
        >
          <h2 className="px-1 text-[15px] font-semibold text-app">Métricas de hoy</h2>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-faint">Métrica</TableHead>
                <TableHead style={{ color: palette.solis }}>Solis</TableHead>
                <TableHead style={{ color: palette.fox }}>Fox</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {metricRows.map((row, i) => (
                <motion.tr
                  key={row.label}
                  initial={{ opacity: 0, y: 12 }}
                  whileInView={{ opacity: 1, y: 0 }}
                  viewport={{ once: true }}
                  transition={{ duration: 0.4, delay: i * 0.06 }}
                  className="border-b border-app"
                >
                  <TableCell className="text-sm text-muted">{row.label}</TableCell>
                  <TableCell className="text-sm font-medium tabular-nums" style={{ color: palette.solis }}>
                    {row.solis}
                    {row.best === 'solis' && bestChip}
                  </TableCell>
                  <TableCell className="text-sm font-medium tabular-nums" style={{ color: palette.fox }}>
                    {row.fox}
                    {row.best === 'fox' && bestChip}
                  </TableCell>
                </motion.tr>
              ))}
            </TableBody>
          </Table>
        </motion.section>

        {/* §9 Reparto de la producción */}
        <motion.section
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.2 }}
          transition={{ duration: 0.5, delay: 0.1, ease: EASE_OUT }}
          className="helios-card flex flex-col p-4 shadow-card dark:shadow-card-dark sm:p-5 lg:col-span-5"
          aria-label="Reparto de la producción de hoy"
        >
          <div className="flex items-center gap-2">
            <h2 className="mr-auto text-[15px] font-semibold text-app">Reparto de hoy</h2>
            {(['barra', 'donut'] as const).map((m) => (
              <button
                key={m}
                onClick={() => setRepartoMode(m)}
                aria-pressed={repartoMode === m}
                className={cn(
                  'inline-flex h-7 items-center rounded-full border px-2.5 text-xs font-medium capitalize transition-colors',
                  repartoMode === m ? 'border-app bg-surface-2 text-app' : 'border-app text-faint hover:text-muted',
                )}
              >
                {m === 'barra' ? 'Barras' : 'Donut'}
              </button>
            ))}
          </div>

          <div className="mt-4 flex min-h-[200px] flex-1 flex-col justify-center">
            <AnimatePresence mode="wait">
              {repartoMode === 'barra' ? (
                <motion.div
                  key="barra"
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  exit={{ opacity: 0 }}
                  transition={{ duration: 0.3 }}
                  className="flex h-7 w-full overflow-hidden rounded-full bg-surface-2"
                  role="img"
                  aria-label={`Solis ${fmtPct(shareSolis)} %, Fox ${fmtPct(shareFox)} %`}
                >
                  <motion.div
                    className="flex h-full items-center overflow-hidden"
                    style={{ backgroundColor: palette.solis }}
                    initial={{ width: 0 }}
                    whileInView={{ width: `${shareSolis}%` }}
                    viewport={{ once: true }}
                    transition={{ duration: 0.7, ease: 'easeOut' }}
                  >
                    <span className="whitespace-nowrap px-3 text-xs font-semibold text-white">Solis {fmtPct(shareSolis)} %</span>
                  </motion.div>
                  <motion.div
                    className="flex h-full items-center overflow-hidden"
                    style={{ backgroundColor: palette.fox }}
                    initial={{ width: 0 }}
                    whileInView={{ width: `${shareFox}%` }}
                    viewport={{ once: true }}
                    transition={{ duration: 0.7, ease: 'easeOut', delay: 0.15 }}
                  >
                    <span className="whitespace-nowrap px-3 text-xs font-semibold text-white">Fox {fmtPct(shareFox)} %</span>
                  </motion.div>
                </motion.div>
              ) : (
                <motion.div
                  key="donut"
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  exit={{ opacity: 0 }}
                  transition={{ duration: 0.3 }}
                  className="relative"
                >
                  <ResponsiveContainer width="100%" height={200}>
                    <PieChart>
                      <Pie
                        data={[
                          { name: 'Solis', value: solisKwh },
                          { name: 'Fox', value: foxKwh },
                        ]}
                        dataKey="value"
                        nameKey="name"
                        innerRadius="62%"
                        outerRadius="88%"
                        startAngle={90}
                        endAngle={-270}
                        paddingAngle={3}
                        cornerRadius={6}
                        strokeWidth={0}
                        animationDuration={800}
                      >
                        <Cell fill={palette.solis} />
                        <Cell fill={palette.fox} />
                      </Pie>
                    </PieChart>
                  </ResponsiveContainer>
                  <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
                    <p className="font-display text-xl font-semibold text-app">{fmtEnergy(total)}</p>
                    <p className="text-[11px] font-medium text-faint">kWh hoy</p>
                  </div>
                  <div className="mt-1 flex justify-center gap-4">
                    <span className="inline-flex items-center gap-1.5 text-xs text-muted">
                      <span className="h-2 w-2 rounded-full" style={{ backgroundColor: palette.solis }} /> Solis {fmtPct(shareSolis)} %
                    </span>
                    <span className="inline-flex items-center gap-1.5 text-xs text-muted">
                      <span className="h-2 w-2 rounded-full" style={{ backgroundColor: palette.fox }} /> Fox {fmtPct(shareFox)} %
                    </span>
                  </div>
                </motion.div>
              )}
            </AnimatePresence>
          </div>

          <p className="mt-4 text-sm text-muted">El Solis aporta casi dos tercios de tu energía hoy.</p>
        </motion.section>
      </div>
    </div>
  );
}

// ── Página ──────────────────────────────────────────────────────────────────

const TABS: { key: TabKey; label: string }[] = [
  { key: 'compare', label: 'Comparativa' },
  { key: 'solis', label: 'Solis' },
  { key: 'fox', label: 'Fox' },
];

export default function Inversores() {
  const { now, nowMin, liveTick, today, getLivePower, getDaySeries } = useEnergyData();
  const palette = useEnergyColors();
  const [searchParams, setSearchParams] = useSearchParams();
  const [spinning, setSpinning] = useState(false);

  const tabParam = searchParams.get('tab');
  const tab: TabKey = tabParam === 'fox' || tabParam === 'solis' ? tabParam : 'compare';
  const setTab = (t: TabKey) => setSearchParams({ tab: t }, { replace: true });

  const live = getLivePower(nowMin, liveTick);
  const series = useMemo(() => getDaySeries(today), [getDaySeries, today]);

  // Energía del día completo por inversor + picos
  const { solisKwh, foxKwh, peakSolis, peakFox } = useMemo(() => {
    let s = 0;
    let f = 0;
    let ps = { v: 0, t: 840 };
    let pf = { v: 0, t: 840 };
    for (const p of series) {
      s += p.solis * (STEP_MIN / 60);
      f += p.fox * (STEP_MIN / 60);
      if (p.solis > ps.v) ps = { v: p.solis, t: p.t };
      if (p.fox > pf.v) pf = { v: p.fox, t: p.t };
    }
    return { solisKwh: s, foxKwh: f, peakSolis: ps, peakFox: pf };
  }, [series]);

  const total = solisKwh + foxKwh;
  const shareSolis = total > 0 ? solisKwh / total : 0.62;
  const shareFox = total > 0 ? foxKwh / total : 0.38;

  const tabColor = (t: TabKey) => (t === 'solis' ? palette.solis : t === 'fox' ? palette.fox : undefined);

  return (
    <TooltipProvider delayDuration={200}>
      <div className="flex flex-col gap-4 lg:gap-5">
        {/* ── §1 Encabezado ─────────────────────────────────────── */}
        <motion.header
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.4, ease: 'easeOut' }}
          className="flex flex-wrap items-center gap-3"
        >
          <div className="mr-auto">
            <h1 className="font-display text-2xl font-semibold tracking-[-0.01em] text-app">Inversores</h1>
            <p className="text-sm text-muted">7,1 kWp en total · 2 sistemas</p>
          </div>
          <motion.div
            initial={{ opacity: 0, y: 16 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.4, delay: 0.08 }}
            className="hidden items-center gap-3 lg:flex"
          >
            <span className="font-mono text-sm tabular-nums text-faint" aria-live="off">
              {fmtClock(now)}
            </span>
            <button
              aria-label="Refrescar datos"
              onClick={() => {
                setSpinning(true);
                window.setTimeout(() => setSpinning(false), 600);
              }}
              className="rounded-full border border-app bg-surface p-2 text-muted transition-colors hover:text-app"
            >
              <motion.span animate={spinning ? { rotate: 360 } : { rotate: 0 }} transition={{ duration: 0.6 }} className="flex">
                <RefreshCw size={16} />
              </motion.span>
            </button>
            <button aria-label="Notificaciones" className="relative rounded-full border border-app bg-surface p-2 text-muted transition-colors hover:text-app">
              <Bell size={16} />
              <span className="absolute right-1.5 top-1.5 h-1.5 w-1.5 rounded-full bg-rose-500" />
            </button>
            <ThemeToggle />
            <ConnectionStatus />
          </motion.div>
        </motion.header>

        {/* ── §1 Segmented control (sticky en móvil) ────────────── */}
        <motion.div
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.4, delay: 0.1 }}
          className="sticky top-14 z-30 -mx-4 bg-[color-mix(in_srgb,var(--bg)_85%,transparent)] px-4 py-2 backdrop-blur-[16px] lg:static lg:mx-0 lg:bg-transparent lg:p-0 lg:backdrop-blur-none"
        >
          <div className="flex h-10 w-full items-center gap-1 rounded-full border border-app bg-surface p-1 lg:w-fit" role="tablist" aria-label="Selector de inversor">
            {TABS.map((t) => {
              const active = tab === t.key;
              const color = tabColor(t.key);
              return (
                <button
                  key={t.key}
                  role="tab"
                  aria-selected={active}
                  onClick={() => setTab(t.key)}
                  className={cn(
                    'relative flex h-full flex-1 items-center justify-center gap-1.5 rounded-full px-4 text-sm font-medium transition-colors lg:flex-none',
                    active ? '' : 'text-faint hover:text-muted',
                  )}
                  style={active ? { color: color ?? 'var(--text)' } : undefined}
                >
                  {active && (
                    <motion.span
                      layoutId="inv-tab-pill"
                      className="absolute inset-0 rounded-full bg-surface-2 shadow-inner"
                      transition={{ type: 'spring', stiffness: 400, damping: 32 }}
                    />
                  )}
                  <span className="relative z-10 flex items-center gap-1.5">
                    {t.key === 'compare' ? (
                      <ArrowLeftRight size={14} />
                    ) : (
                      <span className="h-2 w-2 rounded-full" style={{ backgroundColor: color, opacity: active ? 1 : 0.4 }} />
                    )}
                    {t.label}
                  </span>
                </button>
              );
            })}
          </div>
        </motion.div>

        {/* ── Contenido por pestaña (crossfade + y 8px) ─────────── */}
        <AnimatePresence mode="wait">
          <motion.div
            key={tab}
            initial={{ opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -8 }}
            transition={{ duration: 0.25, ease: 'easeOut' }}
          >
            {tab === 'compare' ? (
              <CompareView
                nowSolis={live.solis}
                nowFox={live.fox}
                solisKwh={solisKwh}
                foxKwh={foxKwh}
                peakSolis={peakSolis}
                peakFox={peakFox}
              />
            ) : (
              <InverterView
                meta={INVERTERS[tab]}
                nowKw={tab === 'solis' ? live.solis : live.fox}
                dayKwh={tab === 'solis' ? solisKwh : foxKwh}
                share={tab === 'solis' ? shareSolis : shareFox}
                color={tabColor(tab) ?? palette.solis}
              />
            )}
          </motion.div>
        </AnimatePresence>

        {/* Nota de actividad (mock) */}
        <p className="flex items-center gap-1.5 text-xs text-faint">
          <Activity size={12} /> Datos locales vía Home Assistant · actualización cada 5 s
        </p>
      </div>
    </TooltipProvider>
  );
}

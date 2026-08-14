import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router';
import { AnimatePresence, motion } from 'framer-motion';
import {
  Activity,
  ArrowLeftRight,
  CalendarRange,
  Gauge,
  History,
  Sun,
  Triangle,
} from 'lucide-react';
import { format } from 'date-fns';
import { useTranslation } from 'react-i18next';
import { dateLocale, numLocale } from '@/i18n';
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
import { useInstall } from '@/hooks/useInstall';
import { STEP_MIN, seriesInvValue } from '@/data/types';
import { useEnergyColors, inverterColor } from '@/lib/colors';
import { useTheme } from '@/theme/ThemeProvider';
import { fmtEnergy, fmtKw, fmtPct, fmtTime } from '@/lib/format';
import { cn } from '@/lib/utils';
import KpiCard from '@/components/KpiCard';
import InverterHeroCard from '@/components/InverterHeroCard';
import InverterDayChart from '@/components/InverterDayChart';
import CompareChart, { type CompareInverter } from '@/components/CompareChart';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Tooltip as UiTooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';

const EASE_OUT: [number, number, number, number] = [0.25, 1, 0.5, 1];

interface InvMeta {
  key: string;
  name: string;
  model: string;
  kwp: number;
  panels: string;
  tempC: number;
  hasBattery: boolean;
  batteryKwh?: number;
  monthKwh: number;
  totalMwh: number;
}

// ── §5 Producción últimos 14 días ───────────────────────────────────────────

interface Last14Props {
  share: number;
  color: string;
}

function Last14Chart({ share, color }: Last14Props) {
  const { getHistory } = useEnergyData();
  const { t, i18n } = useTranslation();
  const rows = useMemo(
    () =>
      getHistory()
        .slice(-14)
        .map((d) => ({
          dayNum: d.date.getDate(),
          label: format(d.date, 'd MMM', { locale: dateLocale() }),
          kwh: d.productionKwh * share,
        })),
    [getHistory, share, i18n.language],
  );

  return (
    <section className="helios-card h-full shadow-card dark:shadow-card-dark" aria-label={t('inversores.last14aria')}>
      <div className="px-4 pb-1 pt-4 sm:px-5">
        <h2 className="text-[15px] font-semibold text-app">{t('inversores.last14')}</h2>
        <p className="text-xs text-faint">{t('inversores.last14sub')}</p>
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
            <Bar dataKey="kwh" name={t('common.production')} fill={color} radius={[4, 4, 0, 0]} animationDuration={500}>
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
  meta: InvMeta;
  nowKw: number;
  dayKwh: number;
  share: number;
  color: string;
}

function InverterView({ meta, nowKw, dayKwh, share, color }: InverterViewProps) {
  const { nowMin, today, getDaySeries } = useEnergyData();
  const { t } = useTranslation();
  const series = useMemo(() => getDaySeries(today), [getDaySeries, today]);

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-12 lg:gap-5">
      <div className="lg:col-span-4">
        <InverterHeroCard meta={meta} nowKw={nowKw} color={color} />
      </div>

      {/* §3 KPIs del inversor */}
      <div className="grid grid-cols-2 gap-4 lg:col-span-8 xl:grid-cols-4" aria-label={t('inversores.kpisAria', { name: meta.name })}>
        <KpiCard
          icon={Sun}
          color={meta.key === 'solis' || meta.key === 'fox' ? meta.key : 'solis'}
          colorHex={meta.key === 'solis' || meta.key === 'fox' ? undefined : color}
          label={t('inversores.today')}
          value={dayKwh}
          unit="kWh"
          delta={{ direction: 'up', text: t('inversores.vsYesterday'), good: true }}
          index={0}
        />
        <KpiCard icon={CalendarRange} color={meta.key === 'solis' || meta.key === 'fox' ? meta.key : 'solis'} colorHex={meta.key === 'solis' || meta.key === 'fox' ? undefined : color} label={t('inversores.thisMonth')} value={meta.monthKwh} unit="kWh" decimals={0} index={1} />
        <UiTooltip>
          <TooltipTrigger asChild>
            <div className="h-full">
              <KpiCard icon={History} color={meta.key === 'solis' || meta.key === 'fox' ? meta.key : 'solis'} colorHex={meta.key === 'solis' || meta.key === 'fox' ? undefined : color} label={t('inversores.total')} value={meta.totalMwh} unit="MWh" decimals={2} index={2} />
            </div>
          </TooltipTrigger>
          <TooltipContent>{t('inversores.totalSince')}</TooltipContent>
        </UiTooltip>
        <KpiCard icon={Gauge} color={meta.key === 'solis' || meta.key === 'fox' ? meta.key : 'solis'} colorHex={meta.key === 'solis' || meta.key === 'fox' ? undefined : color} label={t('inversores.performance')} value={meta.kwp > 0 ? dayKwh / meta.kwp : 0} unit="kWh/kWp" index={3} />
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

// ── Vista comparativa (§6–§9) — N inversores ───────────────────────────────

interface CompareViewProps {
  inverters: CompareInverter[];
  metas: InvMeta[];
  liveKws: number[];
  dayKwhs: number[];
  peakKws: { v: number; t: number }[];
}

function CompareView({ inverters, metas, liveKws, dayKwhs, peakKws }: CompareViewProps) {
  const { nowMin, today, getDaySeries } = useEnergyData();
  const { t } = useTranslation();
  const series = useMemo(() => getDaySeries(today), [getDaySeries, today]);
  const [repartoMode, setRepartoMode] = useState<'barra' | 'donut'>('donut');

  const total = dayKwhs.reduce((a, b) => a + b, 0);
  const shares = dayKwhs.map((k) => (total > 0 ? (k / total) * 100 : 100 / Math.max(1, dayKwhs.length)));

  const nf2 = new Intl.NumberFormat(numLocale(), { minimumFractionDigits: 2, maximumFractionDigits: 2 });

  const bestChip = (
    <span className="ml-1.5 inline-flex items-center rounded-full bg-emerald-500/12 p-0.5 text-emerald-600 dark:text-emerald-400" aria-label={t('inversores.bestMetric')}>
      <Triangle size={8} fill="currentColor" strokeWidth={0} />
    </span>
  );

  const best = (values: number[]): number | null => {
    if (values.length < 2) return null;
    const max = Math.max(...values);
    return values.every((v) => v === max) ? null : values.indexOf(max);
  };

  const metricRows: { label: string; values: React.ReactNode[]; bestIdx: number | null }[] = [
    {
      label: t('inversores.powerNow'),
      values: liveKws.map((v) => `${fmtKw(v)} kW`),
      bestIdx: best(liveKws),
    },
    {
      label: t('inversores.energyToday'),
      values: dayKwhs.map((v) => `${fmtEnergy(v)} kWh`),
      bestIdx: best(dayKwhs),
    },
    {
      label: t('inversores.peakDay'),
      values: peakKws.map((p) => `${fmtKw(p.v)} kW · ${fmtTime(p.t)}`),
      bestIdx: best(peakKws.map((p) => p.v)),
    },
    {
      label: 'kWh/kWp',
      values: dayKwhs.map((v, i) => (metas[i].kwp > 0 ? nf2.format(v / metas[i].kwp) : '—')),
      bestIdx: best(dayKwhs.map((v, i) => (metas[i].kwp > 0 ? v / metas[i].kwp : -1))),
    },
    {
      label: t('inverter.temperature'),
      values: metas.map((m) => `${m.tempC} °C`),
      bestIdx: metas.length === 2 ? (metas[0].tempC <= metas[1].tempC ? 0 : 1) : null,
    },
    {
      label: t('inversores.contribution'),
      values: shares.map((s) => `${fmtPct(s)} %`),
      bestIdx: best(shares),
    },
    {
      label: t('inversores.status'),
      values: inverters.map(() => (
        <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/12 px-2 py-0.5 text-[11px] font-semibold text-emerald-600 dark:text-emerald-400">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" /> {t('common.online')}
        </span>
      )),
      bestIdx: null,
    },
  ];

  return (
    <div className="flex flex-col gap-4 lg:gap-5">
      {/* §6 Hero comparativo */}
      <section
        className="helios-card relative overflow-hidden shadow-card dark:shadow-card-dark"
        aria-label={t('inversores.compareAria')}
      >
        <div className={`grid ${inverters.length === 2 ? 'lg:grid-cols-2' : `sm:grid-cols-2 lg:grid-cols-${Math.min(inverters.length, 4)}`}`}>
          {inverters.map((inv, i) => (
            <motion.div
              key={inv.key}
              initial={{ opacity: 0, x: i % 2 === 0 ? -32 : 32 }}
              animate={{ opacity: 1, x: 0 }}
              transition={{ duration: 0.5, ease: 'easeOut' }}
              className={`flex flex-col gap-1 p-5 sm:p-6 ${i > 0 ? 'border-t border-app lg:border-t-0' : ''} ${i > 0 ? 'lg:border-l' : ''}`}
            >
              <p className="flex items-center gap-2 text-sm font-semibold text-app">
                <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: inv.color }} />
                {inv.name} <span className="text-xs font-normal text-faint">{inv.kwp > 0 ? `${nf2.format(inv.kwp)} kWp` : ''}</span>
              </p>
              <p className="font-display text-[32px] font-semibold leading-tight tracking-[-0.01em]" style={{ color: inv.color }} aria-live="off">
                {fmtKw(liveKws[i])} <span className="text-[0.55em] font-medium text-faint">kW</span>
              </p>
              <p className="text-sm text-muted">
                {t('inversores.today')} <span className="font-semibold text-app">{fmtEnergy(dayKwhs[i])} kWh</span>
              </p>
            </motion.div>
          ))}
        </div>
        {inverters.length === 2 && (
          <motion.span
            initial={{ scale: 0 }}
            animate={{ scale: 1 }}
            transition={{ type: 'spring', stiffness: 400, damping: 24, delay: 0.3 }}
            className="absolute left-1/2 top-1/2 hidden h-10 w-10 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full border border-app bg-surface font-display text-xs font-bold text-muted shadow-md lg:flex"
          >
            VS
          </motion.span>
        )}
      </section>

      {/* §7 Gráfica superpuesta */}
      <motion.div initial={{ opacity: 0, y: 24 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.5, delay: 0.15, ease: EASE_OUT }}>
        <CompareChart data={series} inverters={inverters} nowMin={nowMin} height={340} />
      </motion.div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12 lg:gap-5">
        {/* §8 Tabla de métricas */}
        <motion.section
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.2 }}
          transition={{ duration: 0.5, ease: EASE_OUT }}
          className="helios-card p-4 shadow-card dark:shadow-card-dark sm:p-5 lg:col-span-7"
          aria-label={t('inversores.metricsAria')}
        >
          <h2 className="px-1 text-[15px] font-semibold text-app">{t('inversores.metricsTitle')}</h2>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-faint">{t('inversores.metric')}</TableHead>
                {inverters.map((inv) => (
                  <TableHead key={inv.key} style={{ color: inv.color }}>
                    {inv.name}
                  </TableHead>
                ))}
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
                  {row.values.map((v, j) => (
                    <TableCell key={j} className="text-sm font-medium tabular-nums" style={{ color: inverters[j].color }}>
                      {v}
                      {row.bestIdx === j && bestChip}
                    </TableCell>
                  ))}
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
          aria-label={t('inversores.shareAria')}
        >
          <div className="flex items-center gap-2">
            <h2 className="mr-auto text-[15px] font-semibold text-app">{t('inversores.shareTitle')}</h2>
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
                {m === 'barra' ? t('inversores.bars') : 'Donut'}
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
                  aria-label={inverters.map((inv, i) => `${inv.name} ${fmtPct(shares[i])} %`).join(', ')}
                >
                  {inverters.map((inv, i) => (
                    <motion.div
                      key={inv.key}
                      className="flex h-full items-center overflow-hidden"
                      style={{ backgroundColor: inv.color }}
                      initial={{ width: 0 }}
                      whileInView={{ width: `${shares[i]}%` }}
                      viewport={{ once: true }}
                      transition={{ duration: 0.7, ease: 'easeOut', delay: i * 0.15 }}
                    >
                      <span className="whitespace-nowrap px-3 text-xs font-semibold text-white">
                        {inv.name} {fmtPct(shares[i])} %
                      </span>
                    </motion.div>
                  ))}
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
                        data={inverters.map((inv, i) => ({ name: inv.name, value: dayKwhs[i] }))}
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
                        {inverters.map((inv) => (
                          <Cell key={inv.key} fill={inv.color} />
                        ))}
                      </Pie>
                    </PieChart>
                  </ResponsiveContainer>
                  <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
                    <p className="font-display text-xl font-semibold text-app">{fmtEnergy(total)}</p>
                    <p className="text-[11px] font-medium text-faint">{t('inversores.kwhToday')}</p>
                  </div>
                  <div className="mt-1 flex justify-center gap-4">
                    {inverters.map((inv, i) => (
                      <span key={inv.key} className="inline-flex items-center gap-1.5 text-xs text-muted">
                        <span className="h-2 w-2 rounded-full" style={{ backgroundColor: inv.color }} /> {inv.name} {fmtPct(shares[i])} %
                      </span>
                    ))}
                  </div>
                </motion.div>
              )}
            </AnimatePresence>
          </div>

          <p className="mt-4 text-sm text-muted">{t('inversores.shareInsight')}</p>
        </motion.section>
      </div>
    </div>
  );
}

// ── Página ──────────────────────────────────────────────────────────────────

export default function Inversores() {
  const { nowMin, liveTick, today, getLivePower, getDaySeries } = useEnergyData();
  const install = useInstall();
  const palette = useEnergyColors();
  const { isDark } = useTheme();
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();

  // Metadatos de inversores: de la topología (install.inverters) o fallback
  // a los 2 clásicos (Solis/Fox) si la API aún no responde.
  const invMetas: InvMeta[] = useMemo(() => {
    const fromInstall = (install?.inverters ?? []).map((inv) => ({
      key: inv.key,
      name: inv.name,
      model: inv.model || '',
      kwp: inv.kwp || 0,
      panels: inv.panels || '',
      tempC: 41,
      hasBattery: inv.hasBattery,
      batteryKwh: inv.batteryKwh || 0,
      monthKwh: 300,
      totalMwh: 5,
    }));
    if (fromInstall.length > 0) return fromInstall;
    return [
      {
        key: 'solis',
        name: 'Solis',
        model: 'Solis S5-EH1P5K-L',
        kwp: 4.4,
        panels: '10 × 440 W',
        tempC: 46,
        hasBattery: true,
        batteryKwh: 5,
        monthKwh: 412,
        totalMwh: 8.42,
      },
      {
        key: 'fox',
        name: 'Fox',
        model: 'Fox H1-3.0-E',
        kwp: 2.7,
        panels: '6 × 450 W',
        tempC: 41,
        hasBattery: false,
        monthKwh: 251,
        totalMwh: 3.15,
      },
    ];
  }, [install]);

  const colorOf = (key: string, index: number) =>
    key === 'solis' ? palette.solis : key === 'fox' ? palette.fox : inverterColor(key, isDark, index);

  const compareInverters: CompareInverter[] = invMetas.map((m, i) => ({
    key: m.key,
    name: m.name,
    color: colorOf(m.key, i),
    kwp: m.kwp,
  }));

  // Con varios inversores: comparativa + una vista por inversor (nombres de la
  // topología). Con uno solo: vista individual directa, sin barra de tabs.
  const tabKeys = useMemo(() => {
    const keys = invMetas.map((m) => m.key);
    return invMetas.length >= 2 ? ['compare', ...keys] : keys;
  }, [invMetas]);

  const tabParam = searchParams.get('tab');
  const tab: string = tabKeys.includes(tabParam ?? '') ? (tabParam as string) : tabKeys[0] ?? 'compare';
  const setTab = (t: string) => setSearchParams({ tab: t }, { replace: true });

  const live = getLivePower(nowMin, liveTick);
  const series = useMemo(() => getDaySeries(today), [getDaySeries, today]);

  // kW live por inversor: live.inverters[] (topología) o fallback solis/fox.
  const liveKws = useMemo(() => {
    if (live.inverters && live.inverters.length > 0) {
      return invMetas.map((m) => live.inverters?.find((i) => i.key === m.key)?.kw ?? 0);
    }
    return invMetas.map((m) => (m.key === 'solis' ? live.solis : m.key === 'fox' ? live.fox : 0));
  }, [live, invMetas]);

  // Energía y picos del día por inversor
  const { dayKwhs, peakKws } = useMemo(() => {
    const kWhs: number[] = [];
    const peaks: { v: number; t: number }[] = [];
    for (const m of invMetas) {
      let kwh = 0;
      let peak = { v: 0, t: 840 };
      for (const p of series) {
        const v = seriesInvValue(p, m.key);
        kwh += v * (STEP_MIN / 60);
        if (v > peak.v) peak = { v, t: p.t };
      }
      kWhs.push(kwh);
      peaks.push(peak);
    }
    return { dayKwhs: kWhs, peakKws: peaks };
  }, [series, invMetas]);

  const isCompare = tab === 'compare';

  return (
    <TooltipProvider delayDuration={200}>
      <div className="flex flex-col gap-4 lg:gap-5">
        {/* ── §1 Segmented control (sticky en móvil) — solo con varias vistas ── */}
        {tabKeys.length > 1 && (
        <motion.div
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.4, delay: 0.1 }}
          className="sticky top-14 z-30 -mx-4 bg-[color-mix(in_srgb,var(--bg)_85%,transparent)] px-4 py-2 backdrop-blur-[16px] lg:static lg:mx-0 lg:bg-transparent lg:p-0 lg:backdrop-blur-none"
        >
          <div className="flex h-10 w-full items-center gap-1 overflow-x-auto rounded-full border border-app bg-surface p-1 no-scrollbar lg:w-fit" role="tablist" aria-label={t('inversores.tabAria')}>
            {tabKeys.map((key) => {
              const active = tab === key;
              const isCompareKey = key === 'compare';
              const idx = isCompareKey ? -1 : invMetas.findIndex((m) => m.key === key);
              const color = isCompareKey ? undefined : colorOf(key, idx);
              const label = isCompareKey ? t('inversores.compare') : invMetas[idx]?.name || key;
              return (
                <button
                  key={key}
                  role="tab"
                  aria-selected={active}
                  onClick={() => setTab(key)}
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
                    {isCompareKey ? (
                      <ArrowLeftRight size={14} />
                    ) : (
                      <span className="h-2 w-2 rounded-full" style={{ backgroundColor: color, opacity: active ? 1 : 0.4 }} />
                    )}
                    {label}
                  </span>
                </button>
              );
            })}
          </div>
        </motion.div>
        )}

        {/* ── Contenido por pestaña (crossfade + y 8px) ─────────── */}
        <AnimatePresence mode="wait">
          <motion.div
            key={tab}
            initial={{ opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -8 }}
            transition={{ duration: 0.25, ease: 'easeOut' }}
          >
            {isCompare ? (
              <CompareView inverters={compareInverters} metas={invMetas} liveKws={liveKws} dayKwhs={dayKwhs} peakKws={peakKws} />
            ) : (
              <InverterView
                meta={invMetas[invMetas.findIndex((m) => m.key === tab)]}
                nowKw={liveKws[invMetas.findIndex((m) => m.key === tab)] ?? 0}
                dayKwh={dayKwhs[invMetas.findIndex((m) => m.key === tab)] ?? 0}
                share={totalShare(dayKwhs, invMetas.findIndex((m) => m.key === tab))}
                color={colorOf(tab, invMetas.findIndex((m) => m.key === tab))}
              />
            )}
          </motion.div>
        </AnimatePresence>

        {/* Nota de actividad (mock) */}
        <p className="flex items-center gap-1.5 text-xs text-faint">
          <Activity size={12} /> {t('inversores.localData')}
        </p>
      </div>
    </TooltipProvider>
  );
}

function totalShare(dayKwhs: number[], index: number): number {
  const total = dayKwhs.reduce((a, b) => a + b, 0);
  if (total <= 0) return index === 0 ? 100 : 0;
  return (dayKwhs[index] / total) * 100;
}

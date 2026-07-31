import { useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import {
  CalendarCheck,
  ChevronDown,
  Download,
  Euro,
  Flame,
  House,
  Leaf,
  Moon,
  Sun,
  Trophy,
  Zap,
  ArrowUpFromLine,
} from 'lucide-react';
import { format } from 'date-fns';
import { useTranslation } from 'react-i18next';
import { dateLocale, fmtDayMonthLong, fmtWeekdayDate } from '@/i18n';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useEnergyData } from '@/data/EnergyDataProvider';
import type { DayKpis } from '@/data/types';
import { useEnergySettings } from '@/hooks/useEnergySettings';
import {
  canGoNext,
  daysInWindow,
  deltaPct,
  initialAnchor,
  isCurrentPeriod,
  navLabel,
  periodSubtitle,
  shiftAnchor,
  totalsFor,
} from '@/lib/historyStats';
import type { Period, PeriodTotals } from '@/lib/historyStats';
import { useEnergyColors } from '@/lib/colors';
import { fmtEnergy, fmtEuro, fmtKw, fmtPct, fmtTime } from '@/lib/format';
import { cn } from '@/lib/utils';
import KpiCard from '@/components/KpiCard';
import PeriodSelector from '@/components/PeriodSelector';
import HistoryChart from '@/components/HistoryChart';
import EnergySourceDonut from '@/components/EnergySourceDonut';
import type { SourceSplit } from '@/components/EnergySourceDonut';
import YearHeatmap from '@/components/YearHeatmap';
import HeliosToaster from '@/components/HeliosToaster';
import { heliosToast } from '@/lib/toast';

const easeOutQuart = [0.25, 1, 0.5, 1] as [number, number, number, number];

function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

/** Totales del periodo Día a partir de los KPIs detallados del día simulado. */
function totalsFromDayKpis(kpis: DayKpis, priceImport: number, priceExport: number, co2Factor: number): PeriodTotals {
  return {
    productionKwh: kpis.productionKwh,
    consumptionKwh: kpis.consumptionKwh,
    gridImportKwh: kpis.gridImportKwh,
    gridExportKwh: kpis.gridExportKwh,
    autoconsumoPct: kpis.autoconsumoPct,
    ahorroEur:
      Math.max(0, kpis.consumptionKwh - kpis.gridImportKwh) * priceImport + kpis.gridExportKwh * priceExport,
    co2Kg: kpis.productionKwh * co2Factor,
    dayCount: 1,
  };
}

/** Contenedor de gráfica con cabecera (ChartCard, 7.4 de design.md). */
function ChartCard({
  title,
  subtitle,
  right,
  children,
  className,
  ariaLabel,
}: {
  title: string;
  subtitle?: string;
  right?: ReactNode;
  children: ReactNode;
  className?: string;
  ariaLabel?: string;
}) {
  return (
    <section className={cn('helios-card overflow-hidden shadow-card dark:shadow-card-dark', className)} aria-label={ariaLabel ?? title}>
      <div className="flex flex-wrap items-center gap-2 px-4 pb-1 pt-4 sm:px-5">
        <div className="mr-auto">
          <h2 className="text-[15px] font-semibold text-app">{title}</h2>
          {subtitle && <p className="text-xs capitalize text-faint">{subtitle}</p>}
        </div>
        {right}
      </div>
      {children}
    </section>
  );
}

interface RecordCard {
  key: string;
  icon: typeof Trophy;
  color: string;
  title: string;
  value: string;
  text: string;
}

export default function Historico() {
  const { today, nowMin, getDaySeries, getKpis, getHistory, isDayEstimated } = useEnergyData();
  const [settings] = useEnergySettings();
  const palette = useEnergyColors();
  const { t, i18n } = useTranslation();

  const [period, setPeriod] = useState<Period>('semana');
  const [anchor, setAnchor] = useState<Date>(() => initialAnchor('semana', today));

  const history = useMemo(() => getHistory(), [getHistory]);
  const days = useMemo(() => daysInWindow(period, anchor, history), [period, anchor, history]);
  const empty = days.length === 0;

  const dayKpis = useMemo(() => getKpis(period === 'dia' ? anchor : today), [getKpis, anchor, today, period]);
  const daySeries = useMemo(() => getDaySeries(anchor), [getDaySeries, anchor]);
  const isToday = period === 'dia' && sameDay(anchor, today);

  // Totales del periodo visible.
  const totals = useMemo<PeriodTotals>(() => {
    if (period === 'dia') {
      return totalsFromDayKpis(dayKpis, settings.priceImport, settings.priceExport, settings.co2Factor);
    }
    return totalsFor(days, settings);
  }, [period, dayKpis, days, settings]);

  // Totales del periodo anterior (para los chips delta).
  const prevTotals = useMemo<PeriodTotals | null>(() => {
    const prevAnchor = shiftAnchor(period, anchor, -1);
    if (period === 'dia') {
      const prevDay = daysInWindow('dia', prevAnchor, history)[0];
      if (!prevDay) return null;
      return totalsFor([prevDay], settings);
    }
    const prevDays = daysInWindow(period, prevAnchor, history);
    if (prevDays.length === 0) return null;
    return totalsFor(prevDays, settings);
  }, [period, anchor, history, settings]);

  // Donut: origen de la energía consumida.
  const split = useMemo<SourceSplit>(() => {
    if (period === 'dia') {
      const red = dayKpis.gridImportKwh;
      const bateria = Math.min(dayKpis.batteryDischargedKwh, Math.max(0, dayKpis.consumptionKwh - red));
      const solar = Math.max(0, dayKpis.consumptionKwh - red - bateria);
      return { solar, bateria, red };
    }
    const red = totals.gridImportKwh;
    const nonRed = Math.max(0, totals.consumptionKwh - red);
    const base = Math.max(0.001, dayKpis.consumptionKwh - dayKpis.gridImportKwh);
    const batteryRatio = Math.min(0.5, dayKpis.batteryDischargedKwh / base);
    const bateria = nonRed * batteryRatio;
    return { solar: nonRed - bateria, bateria, red };
  }, [period, dayKpis, totals]);

  // Récords del periodo visible (usando days, no todo el histórico).
  const records = useMemo<RecordCard[]>(() => {
    if (days.length === 0) return [];
    const list: RecordCard[] = [];

    if (period === 'dia') {
      list.push({
        key: 'pico',
        icon: Zap,
        color: palette.solar,
        title: t('historico.records.peak'),
        value: `${fmtKw(dayKpis.peakProductionKw)} kW`,
        text: t('historico.records.peakText', { time: fmtTime(dayKpis.peakAt), kwh: fmtEnergy(dayKpis.productionKwh) }),
      });
      if (dayKpis.gridExportKwh > 0) {
        list.push({
          key: 'exportacion',
          icon: ArrowUpFromLine,
          color: palette.redVertido,
          title: t('historico.records.export'),
          value: `${fmtEnergy(dayKpis.gridExportKwh)} kWh`,
          text: t('historico.records.exportText', { price: fmtEuro(settings.priceExport) }),
        });
      }
      list.push({
        key: 'autoconsumo',
        icon: Leaf,
        color: palette.bateria,
        title: t('common.autoconsumo'),
        value: `${fmtPct(dayKpis.autoconsumoPct)} %`,
        text: t('historico.records.autoText', { kwh: fmtEnergy(dayKpis.productionKwh - dayKpis.gridExportKwh) }),
      });
      return list.slice(0, 3);
    }

    const best = days.reduce((a, b) => (b.productionKwh > a.productionKwh ? b : a));
    list.push({
      key: 'mejor-dia',
      icon: Trophy,
      color: palette.solar,
      title: period === 'semana' ? t('historico.records.bestWeek') : period === 'mes' ? t('historico.records.bestMonth') : t('historico.records.bestYear'),
      value: `${fmtEnergy(best.productionKwh)} kWh`,
      text: fmtWeekdayDate(best.date),
    });

    const daysWithConsumption = days.filter((d) => d.consumptionKwh > 1);
    const bestNight = daysWithConsumption.length
      ? daysWithConsumption.reduce((a, b) => (b.autoconsumoPct > a.autoconsumoPct ? b : a))
      : null;
    if (bestNight) {
      list.push({
        key: 'noche',
        icon: Moon,
        color: palette.fox,
        title: t('historico.records.autonomous'),
        value: `${fmtPct(Math.min(97, bestNight.autoconsumoPct))} %`,
        text: fmtDayMonthLong(bestNight.date),
      });
    }

    const threshold = period === 'ano' ? 35 : 25;
    let bestStreak = 0;
    let cur = 0;
    for (const d of days) {
      if (d.productionKwh >= threshold) {
        cur++;
        bestStreak = Math.max(bestStreak, cur);
      } else {
        cur = 0;
      }
    }
    if (bestStreak > 0) {
      list.push({
        key: 'racha',
        icon: Flame,
        color: palette.redCompra,
        title: t('historico.records.streak'),
        value: t('historico.records.streakDays', { count: bestStreak }),
        text: t('historico.records.streakText', { threshold }),
      });
    }

    if (period === 'ano' && days.length > 0) {
      const byMonth = new Map<number, { date: Date; kwh: number }>();
      for (const d of days) {
        const m = d.date.getMonth();
        const cur = byMonth.get(m) ?? { date: new Date(d.date.getFullYear(), m, 1), kwh: 0 };
        cur.kwh += d.productionKwh;
        byMonth.set(m, cur);
      }
      const bestMonth = Array.from(byMonth.values()).reduce((a, b) => (b.kwh > a.kwh ? b : a));
      list.push({
        key: 'mejor-mes',
        icon: CalendarCheck,
        color: palette.bateria,
        title: t('historico.records.bestMonthTitle'),
        value: `${format(bestMonth.date, 'MMMM', { locale: dateLocale() })} · ${Math.round(bestMonth.kwh)} kWh`,
        text: t('historico.records.bestMonthText'),
      });
    }

    return list.slice(0, 4);
  }, [days, period, dayKpis, palette, settings, t, i18n.language]);

  const label = navLabel(period, anchor);
  const subtitle = periodSubtitle(period, anchor);
  const isCurrent = isCurrentPeriod(period, anchor, today);
  const canNext = canGoNext(period, anchor, today);

  const changePeriod = (p: Period) => {
    setPeriod(p);
    setAnchor(initialAnchor(p, today));
  };
  const goToday = () => setAnchor(initialAnchor(period, today));
  const drillDay = (date: Date) => {
    setPeriod('dia');
    setAnchor(date);
  };
  const drillMonth = (date: Date) => {
    setPeriod('mes');
    setAnchor(new Date(date.getFullYear(), date.getMonth(), 1));
  };

  const exportToast = () => heliosToast(t('historico.exportToast'), { tone: 'warning' });

  const chartTitle = useMemo(() => {
    switch (period) {
      case 'dia':
        return t('historico.chartTitle', { period: isToday ? t('historico.todayLower') : label });
      case 'semana':
        return t('historico.chartTitle', { period: isCurrent ? t('historico.thisWeek') : label });
      case 'mes':
        return t('historico.chartTitle', { period: format(anchor, 'MMMM', { locale: dateLocale() }) });
      case 'ano':
        return t('historico.chartTitle', { period: format(anchor, 'yyyy') });
    }
  }, [period, isToday, isCurrent, label, anchor, t, i18n.language]);

  const prodDelta = prevTotals ? deltaPct(totals.productionKwh, prevTotals.productionKwh) : null;
  const consDelta = prevTotals ? deltaPct(totals.consumptionKwh, prevTotals.consumptionKwh) : null;
  const trees = totals.co2Kg > 0 ? Math.max(1, Math.round(totals.co2Kg / 21)) : 0;

  const contentKey = `${period}-${anchor.toISOString()}`;

  return (
    <div className="flex flex-col gap-4 lg:gap-5">
      {/* ── §1 Encabezado + controles ─────────────────────────────── */}
      <motion.header
        initial={{ opacity: 0, y: 16 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.4, ease: 'easeOut' }}
        className="flex flex-wrap items-center gap-3"
      >
        <div className="mr-auto">
          <h1 className="font-display text-2xl font-semibold tracking-[-0.01em] text-app">{t('historico.title')}</h1>
          <p className="text-sm capitalize text-muted" aria-live="polite">
            {subtitle}
          </p>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button className="inline-flex h-9 items-center gap-1.5 rounded-full border border-app bg-surface px-4 text-[13px] font-semibold text-muted transition-colors hover:text-app">
              <Download size={15} />
              {t('historico.export')}
              <ChevronDown size={13} className="text-faint" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-[160px]">
            <DropdownMenuItem onClick={exportToast}>CSV</DropdownMenuItem>
            <DropdownMenuItem onClick={exportToast}>{t('historico.exportPng')}</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </motion.header>

      {/* Controles de periodo (sticky en móvil bajo el header) */}
      <motion.div
        initial={{ opacity: 0, y: 16 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.4, delay: 0.08, ease: 'easeOut' }}
        className="sticky top-14 z-30 -mx-4 bg-app/85 px-4 py-2 backdrop-blur-md lg:static lg:mx-0 lg:bg-transparent lg:p-0 lg:backdrop-blur-none"
      >
        <PeriodSelector
          period={period}
          onPeriodChange={changePeriod}
          label={label}
          canNext={canNext}
          isCurrent={isCurrent}
          onPrev={() => setAnchor((a) => shiftAnchor(period, a, -1))}
          onNext={() => setAnchor((a) => shiftAnchor(period, a, 1))}
          onToday={goToday}
        />
      </motion.div>

      {/* ── Contenido del periodo (crossfade al cambiar) ─────────── */}
      <AnimatePresence mode="wait" initial={false}>
        <motion.div
          key={contentKey}
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, y: -8 }}
          transition={{ duration: 0.3, ease: 'easeOut' }}
          className="flex flex-col gap-4 lg:gap-5"
        >
          {empty ? (
            /* Estado vacío */
            <div className="helios-card flex flex-col items-center gap-4 p-10 text-center shadow-card dark:shadow-card-dark">
              <img src="/empty-solar.svg" alt="" className="w-full max-w-[300px]" />
              <p className="text-sm font-medium text-muted">{t('historico.empty')}</p>
              <p className="max-w-xs text-xs text-faint">
                {t('historico.emptyDetail')}
              </p>
              <button
                onClick={goToday}
                className="bg-brand-gradient rounded-full px-5 py-2 text-[13px] font-semibold text-white shadow-md transition-transform hover:scale-[1.03] active:scale-95"
              >
                {t('historico.backToToday')}
              </button>
            </div>
          ) : (
            <>
              {/* ── §2 KPIs del periodo ─────────────────────────── */}
              <div
                className="flex snap-x snap-mandatory gap-4 overflow-x-auto no-scrollbar lg:grid lg:snap-none lg:grid-cols-3 lg:overflow-visible xl:grid-cols-5"
                aria-label={t('historico.kpisAria')}
              >
                <div className="w-[42%] shrink-0 snap-center lg:w-auto">
                  <KpiCard
                    icon={Sun}
                    color="solar"
                    label={t('common.production')}
                    value={totals.productionKwh}
                    unit="kWh"
                    decimals={totals.productionKwh >= 100 ? 0 : 1}
                    index={0}
                    delta={
                      prodDelta !== null
                        ? {
                            direction: prodDelta >= 0 ? 'up' : 'down',
                            text: `${fmtPct(Math.abs(prodDelta))} % ${t(`historico.vs.${period}`)}`,
                            good: prodDelta >= 0,
                          }
                        : undefined
                    }
                  />
                </div>
                <div className="w-[42%] shrink-0 snap-center lg:w-auto">
                  <KpiCard
                    icon={House}
                    color="consumo"
                    label={t('common.consumption')}
                    value={totals.consumptionKwh}
                    unit="kWh"
                    decimals={totals.consumptionKwh >= 100 ? 0 : 1}
                    index={1}
                    delta={
                      consDelta !== null
                        ? {
                            direction: consDelta >= 0 ? 'up' : 'down',
                            text: `${fmtPct(Math.abs(consDelta))} % ${t(`historico.vs.${period}`)}`,
                            good: consDelta < 0,
                          }
                        : undefined
                    }
                  />
                </div>
                <div className="w-[42%] shrink-0 snap-center lg:w-auto">
                  <KpiCard
                    icon={Leaf}
                    color="bateria"
                    label={t('common.autoconsumo')}
                    value={totals.autoconsumoPct}
                    unit="%"
                    decimals={0}
                    progress={totals.autoconsumoPct}
                    index={2}
                  />
                </div>
                <div className="w-[42%] shrink-0 snap-center lg:w-auto">
                  <KpiCard icon={Euro} color="bateria" label={t('historico.savings')} value={totals.ahorroEur} unit="€" decimals={2} index={3}>
                    <p className="text-[11px] leading-snug text-faint">
                      {t('historico.savingsDetail', { importPrice: fmtEuro(settings.priceImport), exportPrice: fmtEuro(settings.priceExport) })}
                    </p>
                  </KpiCard>
                </div>
                <div className="w-[42%] shrink-0 snap-center lg:w-auto">
                  <KpiCard icon={Leaf} color="bateria" label={t('historico.co2')} value={totals.co2Kg} unit="kg" decimals={0} index={4}>
                    <p className="text-[11px] text-faint">{t('historico.trees', { count: trees })}</p>
                  </KpiCard>
                </div>
              </div>

              {/* ── §3 + §4: barras + donut ─────────────────────── */}
              <div className="grid gap-4 lg:grid-cols-12 lg:gap-5">
                <motion.div
                  initial={{ opacity: 0, y: 24 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ duration: 0.5, delay: 0.1, ease: easeOutQuart }}
                  className="lg:col-span-8"
                >
                  <ChartCard
                    title={chartTitle}
                    subtitle={period === 'dia' ? t('historico.powerCurves') : subtitle}
                    ariaLabel={t('historico.chartAria')}
                    right={
                      period !== 'dia' ? (
                        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                          <span className="inline-flex items-center gap-1.5 text-xs text-muted">
                            <span className="h-2 w-2 rounded-full" style={{ backgroundColor: palette.solar }} />
                            {t('common.production')}
                          </span>
                          <span className="inline-flex items-center gap-1.5 text-xs text-muted">
                            <span className="h-2 w-2 rounded-full" style={{ backgroundColor: palette.consumo }} />
                            {t('common.consumption')}
                          </span>
                          <span className="inline-flex items-center gap-1.5 text-xs text-muted">
                            <span className="inline-block h-0 w-4 border-t-2 border-dashed" style={{ borderColor: palette.bateria }} />
                            {t('common.autoconsumo')}
                          </span>
                        </div>
                      ) : undefined
                    }
                  >
                    <div className="px-1 pb-2">
                      <HistoryChart
                        period={period}
                        days={days}
                        daySeries={daySeries}
                        nowMin={nowMin}
                        isToday={isToday}
                        today={today}
                        onDrillDay={drillDay}
                        onDrillMonth={drillMonth}
                      />
                    </div>
                    {period === 'dia' && isDayEstimated(anchor) && (
                      <p className="px-5 pb-3 text-[11px] font-medium text-amber-600 dark:text-amber-400">
                        {t('historico.estimatedNote')}
                      </p>
                    )}
                    {period !== 'dia' && (
                      <p className="px-5 pb-3 text-[11px] text-faint">
                        {period === 'ano' ? t('historico.drillMonth') : t('historico.drillDay')}
                      </p>
                    )}
                  </ChartCard>
                </motion.div>

                <motion.div
                  initial={{ opacity: 0, y: 24 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ duration: 0.5, delay: 0.18, ease: easeOutQuart }}
                  className="lg:col-span-4"
                >
                  <ChartCard title={t('historico.donutTitle')} subtitle={subtitle} className="h-full" ariaLabel={t('historico.donutAria')}>
                    <div className="flex h-[calc(100%-64px)] flex-col px-4 pb-3 sm:px-5">
                      <EnergySourceDonut split={split} />
                    </div>
                  </ChartCard>
                </motion.div>
              </div>

              {/* ── §5 Heatmap anual (solo Año) ─────────────────── */}
              {period === 'ano' && (
                <motion.div
                  initial={{ opacity: 0, y: 24 }}
                  whileInView={{ opacity: 1, y: 0 }}
                  viewport={{ once: true, amount: 0.2 }}
                  transition={{ duration: 0.5, ease: easeOutQuart }}
                >
                  <ChartCard
                    title={t('historico.heatmapTitle')}
                    subtitle={t('historico.heatmapSubtitle')}
                    ariaLabel={t('historico.heatmapAria')}
                  >
                    <div className="px-4 pb-3 sm:px-5">
                      <YearHeatmap days={history} today={today} onSelectDay={drillDay} />
                    </div>
                  </ChartCard>
                </motion.div>
              )}

              {/* ── §6 Récords y curiosidades ───────────────────── */}
              <div className={cn('grid gap-4 sm:grid-cols-2', records.length > 3 ? 'lg:grid-cols-4' : 'md:grid-cols-3')}>
                {records.map((r, i) => (
                  <motion.div
                    key={r.key}
                    initial={{ opacity: 0, y: 24 }}
                    whileInView={{ opacity: 1, y: 0 }}
                    viewport={{ once: true, amount: 0.2 }}
                    transition={{ duration: 0.5, delay: i * 0.1, ease: easeOutQuart }}
                    className="group helios-card flex items-start gap-3.5 p-5 shadow-card transition-all hover:-translate-y-0.5 hover:shadow-lg dark:shadow-card-dark"
                  >
                    <motion.span
                      initial={{ rotate: -6, scale: 0.9 }}
                      whileInView={{ rotate: 0, scale: 1 }}
                      viewport={{ once: true }}
                      transition={{ type: 'spring', stiffness: 300, damping: 18, delay: i * 0.1 + 0.1 }}
                      className="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl transition-transform group-hover:scale-110"
                      style={{ backgroundColor: `${r.color}1F`, color: r.color }}
                    >
                      <r.icon size={21} strokeWidth={2.1} />
                    </motion.span>
                    <div className="min-w-0">
                      <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{r.title}</p>
                      <p className="mt-0.5 font-display text-lg font-semibold leading-tight text-app">{r.value}</p>
                      <p className="mt-1 text-xs leading-snug text-muted">{r.text}</p>
                    </div>
                  </motion.div>
                ))}
              </div>
            </>
          )}
        </motion.div>
      </AnimatePresence>

      <HeliosToaster />
    </div>
  );
}

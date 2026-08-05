import { useMemo } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import {
  ArrowDownToLine,
  ArrowUpFromLine,
  BatteryCharging,
  Clock,
  HeartPulse,
  RefreshCw,
  Thermometer,
} from 'lucide-react';
import { format } from 'date-fns';
import { useTranslation } from 'react-i18next';
import { dateLocale } from '@/i18n';
import { Bar, BarChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts';
import { useEnergyData } from '@/data/EnergyDataProvider';
import { useEnergyColors } from '@/lib/colors';
import { fmtEnergy, fmtKw } from '@/lib/format';
import { cn } from '@/lib/utils';
import SocHeroGauge from '@/components/SocHeroGauge';
import SocDayChart from '@/components/SocDayChart';
import BatteryPowerChart from '@/components/BatteryPowerChart';

const EASE_OUT: [number, number, number, number] = [0.25, 1, 0.5, 1];

type BatState = 'charging' | 'discharging' | 'idle';

function batteryState(bp: number): BatState {
  if (bp > 0.05) return 'charging';
  if (bp < -0.05) return 'discharging';
  return 'idle';
}

// ── §5 Balance semanal (últimos 7 días reales del histórico) ────────────────

interface WeekDay {
  day: string;
  date: string;
  carga: number;
  descarga: number;
}

function WeekTooltip({ active, payload }: { active?: boolean; payload?: { payload?: WeekDay }[] }) {
  const { t } = useTranslation();
  if (!active || !payload || payload.length === 0) return null;
  const d = payload[0]?.payload;
  if (!d) return null;
  return (
    <div className="rounded-xl border border-app bg-surface/95 px-3 py-2 text-xs shadow-lg backdrop-blur-md">
      <p className="font-semibold text-app">
        {t('bateria.weekTooltip', { date: d.date, charge: fmtEnergy(d.carga), discharge: fmtEnergy(d.descarga) })}
      </p>
    </div>
  );
}

export default function Bateria() {
  const { nowMin, liveTick, today, getLivePower, getDaySeries, getKpis, getHistory } = useEnergyData();
  const palette = useEnergyColors();
  const { t, i18n } = useTranslation();

  const live = getLivePower(nowMin, liveTick);
  const kpis = useMemo(() => getKpis(today), [getKpis, today]);
  const series = useMemo(() => getDaySeries(today), [getDaySeries, today]);

  const weekData = useMemo<WeekDay[]>(() => {
    return getHistory()
      .slice(-7)
      .map((d) => ({
        day: format(d.date, 'EEEEE', { locale: dateLocale() }).toUpperCase(),
        date: format(d.date, 'EEE d', { locale: dateLocale() }),
        carga: Math.round((d.batteryChargedKwh ?? 0) * 100) / 100,
        descarga: Math.round((d.batteryDischargedKwh ?? 0) * 100) / 100,
      }));
  }, [getHistory, i18n.language]);

  const autoconsumoSemana = useMemo(() => {
    const days = getHistory().slice(-7);
    if (days.length === 0) return 0;
    return Math.round(days.reduce((acc, d) => acc + d.autoconsumoPct, 0) / days.length);
  }, [getHistory]);

  const state = batteryState(live.batteryPower);

  const stateChip = {
    charging: { text: `${t('common.charging')} · ${fmtKw(live.batteryPower)} kW`, cls: 'bg-emerald-500/12 text-emerald-600 dark:text-emerald-400' },
    discharging: { text: `${t('common.discharging')} · ${fmtKw(Math.abs(live.batteryPower))} kW`, cls: 'bg-amber-500/12 text-amber-600 dark:text-amber-400' },
    idle: { text: t('common.idle'), cls: 'bg-surface-2 text-muted' },
  }[state];

  return (
    <div className="flex flex-col gap-4 lg:gap-5">
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12 lg:gap-5">
        {/* ── Soluna + Estado (tarjeta única compacta) ──────────────── */}
        <motion.section
          initial={{ opacity: 0, y: 24 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, ease: EASE_OUT }}
          className="helios-card col-span-12 p-5 shadow-card dark:shadow-card-dark sm:p-6"
        >
          <div className="flex flex-col items-center gap-6 lg:flex-row lg:items-stretch">
            {/* Gauge ~2/5 */}
            <div className="flex shrink-0 flex-col items-center justify-center gap-3 lg:w-[38%]">
              <SocHeroGauge soc={live.soc} size={230} />
              <AnimatePresence mode="wait">
                <motion.span
                  key={state}
                  initial={{ opacity: 0, y: 6 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -6 }}
                  transition={{ duration: 0.3 }}
                  className={cn('inline-flex items-center gap-1.5 rounded-full px-4 py-1.5 text-sm font-semibold', stateChip.cls)}
                >
                  {stateChip.text}
                </motion.span>
              </AnimatePresence>
            </div>

            {/* Stats compactos ~3/5 */}
            <div className="flex min-w-0 flex-1 items-center lg:border-l lg:border-app lg:pl-6">
              <div className="grid w-full grid-cols-1 gap-x-4 gap-y-3 sm:grid-cols-2 lg:grid-cols-3">
                {[
                  { icon: ArrowDownToLine, label: t('bateria.chargedToday'), value: `${fmtEnergy(kpis.batteryChargedKwh)} kWh` },
                  { icon: ArrowUpFromLine, label: t('bateria.dischargedToday'), value: `${fmtEnergy(kpis.batteryDischargedKwh)} kWh` },
                  { icon: RefreshCw, label: t('bateria.cycles'), value: '312' },
                  { icon: HeartPulse, label: t('bateria.health'), value: '98 %' },
                  { icon: Thermometer, label: t('bateria.temperature'), value: '28 °C' },
                  { icon: Clock, label: t('bateria.autonomy'), value: '~3 h' },
                ].map((d, i) => (
                  <motion.div
                    key={d.label}
                    initial={{ opacity: 0, y: 12 }}
                    whileInView={{ opacity: 1, y: 0 }}
                    viewport={{ once: true }}
                    transition={{ duration: 0.35, delay: i * 0.05 }}
                    className="flex items-center gap-2 rounded-lg px-2 py-2 hover:bg-surface-2"
                  >
                    <d.icon size={15} className="shrink-0 text-faint" />
                    <span className="text-[13px] text-muted">{d.label}</span>
                    <span className="ml-auto text-[15px] font-semibold tabular-nums text-app">{d.value}</span>
                  </motion.div>
                ))}
              </div>
            </div>
          </div>
        </motion.section>

        {/* ── Potencia hoy + Estado de carga hoy (misma fila = misma altura) */}
        <motion.div
          initial={{ opacity: 0, y: 24 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.15, ease: EASE_OUT }}
          className="col-span-12 lg:col-span-6"
        >
          <BatteryPowerChart data={series} height={260} />
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.2 }}
          transition={{ duration: 0.5, ease: EASE_OUT }}
          className="col-span-12 lg:col-span-6"
        >
          <SocDayChart data={series} nowMin={nowMin} height={260} />
        </motion.div>

        {/* ── Esta semana (2/3) + Modo (1/3) ───────────────────────── */}
        <motion.section
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.2 }}
          transition={{ duration: 0.5, delay: 0.1, ease: EASE_OUT }}
          className="helios-card col-span-12 shadow-card dark:shadow-card-dark lg:col-span-8"
          aria-label={t('bateria.weekAria')}
        >
          <div className="flex flex-wrap items-center gap-2 px-4 pb-1 pt-4 sm:px-5">
            <div className="mr-auto">
              <h2 className="text-[15px] font-semibold text-app">{t('bateria.weekTitle')}</h2>
              <p className="text-xs text-faint">{t('bateria.weekSubtitle')}</p>
            </div>
            <span className="inline-flex items-center gap-1.5 text-xs text-muted">
              <span className="h-2 w-2 rounded-full" style={{ backgroundColor: palette.bateria }} /> {t('bateria.charge')}
            </span>
            <span className="inline-flex items-center gap-1.5 text-xs text-muted">
              <span className="h-2 w-2 rounded-full" style={{ backgroundColor: palette.solar }} /> {t('bateria.discharge')}
            </span>
          </div>
          <div className="px-1">
            <ResponsiveContainer width="100%" height={240}>
              <BarChart data={weekData} margin={{ top: 10, right: 12, bottom: 0, left: 4 }} barGap={2}>
                <CartesianGrid vertical={false} stroke="var(--line)" strokeOpacity={0.6} strokeDasharray="3 6" />
                <XAxis
                  dataKey="day"
                  tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
                  axisLine={false}
                  tickLine={false}
                />
                <YAxis
                  domain={[0, 8]}
                  ticks={[0, 2, 4, 6, 8]}
                  width={26}
                  tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
                  axisLine={false}
                  tickLine={false}
                />
                <Tooltip content={<WeekTooltip />} cursor={{ fill: 'var(--surface-2)', opacity: 0.5 }} />
                <Bar dataKey="carga" name={t('bateria.charge')} fill={palette.bateria} radius={[3, 3, 0, 0]} animationDuration={500} />
                <Bar dataKey="descarga" name={t('bateria.discharge')} fill={palette.solar} radius={[3, 3, 0, 0]} animationDuration={500} animationBegin={60} />
              </BarChart>
            </ResponsiveContainer>
          </div>
          <div className="px-4 pb-4 sm:px-5">
            <motion.span
              initial={{ opacity: 0, y: 12 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true }}
              transition={{ duration: 0.4, delay: 0.5 }}
              className="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/12 px-3 py-1 text-xs font-semibold text-emerald-600 dark:text-emerald-400"
            >
              {t('bateria.avgAuto', { pct: autoconsumoSemana })}
            </motion.span>
          </div>
        </motion.section>

        <motion.section
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.2 }}
          transition={{ duration: 0.5, delay: 0.2, ease: EASE_OUT }}
          className="helios-card col-span-12 flex flex-col gap-5 p-5 shadow-card dark:shadow-card-dark sm:p-6 lg:col-span-4"
        >
          <div>
            <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('bateria.workMode')}</p>
            <span className="mt-1.5 inline-flex items-center gap-1.5 rounded-full bg-emerald-500/12 px-3 py-1 text-xs font-semibold text-emerald-600 dark:text-emerald-400">
              <BatteryCharging size={13} /> {t('common.autoconsumo')}
            </span>
            <p className="mt-2 text-sm text-muted">
              {t('bateria.workModeDesc')}
            </p>
          </div>

          <div className="border-t border-app pt-5">
            <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('bateria.minReserve')}</p>
            <p className="mt-1 font-display text-[28px] font-semibold leading-none tracking-[-0.01em] text-app">
              20 <span className="text-[0.6em] font-medium text-faint">%</span>
            </p>
            <div className="mt-3 h-2 w-full overflow-hidden rounded-full bg-surface-2" role="progressbar" aria-valuenow={20} aria-valuemin={0} aria-valuemax={100} aria-label={t('bateria.minReserveAria', { pct: 20 })}>
              <motion.div
                className="h-full rounded-full"
                style={{ backgroundColor: palette.redCompra }}
                initial={{ width: 0 }}
                whileInView={{ width: '20%' }}
                viewport={{ once: true }}
                transition={{ duration: 0.8, ease: 'easeOut', delay: 0.4 }}
              />
            </div>
            <p className="mt-1.5 text-sm text-muted">{t('bateria.minReserveDesc', { kwh: '1,0' })}</p>
          </div>

          <div className="border-t border-app pt-5">
            <p className="text-xs text-faint">{t('bateria.readOnly')}</p>
          </div>
        </motion.section>
      </div>
    </div>
  );
}

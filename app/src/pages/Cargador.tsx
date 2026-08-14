import { useEffect, useMemo, useState } from 'react';
import { motion } from 'framer-motion';
import {
  BatteryCharging,
  Cable,
  CarFront,
  Clock,
  Gauge,
  PlugZap,
  Thermometer,
  Zap,
} from 'lucide-react';
import { format } from 'date-fns';
import { useTranslation } from 'react-i18next';
import { Navigate } from 'react-router';
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { useEnergyData } from '@/data/EnergyDataProvider';
import { useExtensions, } from '@/hooks/useExtensions';
import { chargerEnabled } from '@/data/types';
import type { ChargerCurvePoint } from '@/data/types';
import { useEnergyColors } from '@/lib/colors';
import { fmtEnergy, fmtKw } from '@/lib/format';
import { apiFetch } from '@/data/api-client';
import { dateLocale } from '@/i18n';
import { cn } from '@/lib/utils';

const EASE_OUT: [number, number, number, number] = [0.25, 1, 0.5, 1];

interface HistoryDay {
  date: string;
  kwh: number | null;
}

function CurveTooltip({ active, payload, label }: { active?: boolean; payload?: { value?: number }[]; label?: string }) {
  if (!active || !payload || payload.length === 0) return null;
  const v = payload[0]?.value;
  if (v === undefined || v === null) return null;
  return (
    <div className="rounded-xl border border-app bg-surface/95 px-3 py-2 text-xs shadow-lg backdrop-blur-md">
      <p className="font-semibold text-app">{label}</p>
      <p className="text-muted">
        <span className="font-semibold text-app">{fmtKw(v)} kW</span>
      </p>
    </div>
  );
}

function DayTooltip({ active, payload }: { active?: boolean; payload?: { payload?: HistoryDay }[] }) {
  const { t } = useTranslation();
  if (!active || !payload || payload.length === 0) return null;
  const d = payload[0]?.payload;
  if (!d || d.kwh === null) return null;
  return (
    <div className="rounded-xl border border-app bg-surface/95 px-3 py-2 text-xs shadow-lg backdrop-blur-md">
      <p className="font-semibold text-app">
        {format(new Date(d.date + 'T00:00:00'), 'EEE d MMM', { locale: dateLocale() })}
      </p>
      <p className="text-muted">
        {t('cargador.charged')}: <span className="font-semibold text-app">{fmtEnergy(d.kwh)} kWh</span>
      </p>
    </div>
  );
}

/** Página del cargador de coche (extensión #94): estado en vivo, curva del día
 *  e histórico de energía cargada. Sólo se llega con la extensión activa. */
export default function Cargador() {
  const { t } = useTranslation();
  const ext = useExtensions();
  const palette = useEnergyColors();
  const { getLivePower, nowMin, liveTick } = useEnergyData();

  const [curve, setCurve] = useState<ChargerCurvePoint[] | null>(null);
  const [history, setHistory] = useState<HistoryDay[] | null>(null);

  const live = getLivePower(nowMin, liveTick).charger;

  const todayKey = useMemo(() => {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  }, []);

  useEffect(() => {
    if (!chargerEnabled(ext)) return;
    let alive = true;
    apiFetch<{ points: ChargerCurvePoint[] }>(`/api/charger/day?date=${todayKey}`)
      .then((res) => {
        if (alive) setCurve(res.points ?? []);
      })
      .catch(() => {
        if (alive) setCurve([]);
      });
    const to = new Date();
    const from = new Date();
    from.setDate(from.getDate() - 29);
    const fmt = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
    apiFetch<{ days: HistoryDay[] }>(`/api/charger/history?from=${fmt(from)}&to=${fmt(to)}`)
      .then((res) => {
        if (alive) setHistory(res.days ?? []);
      })
      .catch(() => {
        if (alive) setHistory([]);
      });
    return () => {
      alive = false;
    };
  }, [ext, todayKey]);

  const histData = useMemo(() => {
    if (!history) return [];
    return history.slice(-14).map((d) => ({
      ...d,
      day: format(new Date(d.date + 'T00:00:00'), 'EEEEE', { locale: dateLocale() }).toUpperCase(),
    }));
  }, [history]);

  const histStats = useMemo(() => {
    const withData = (history ?? []).filter((d) => d.kwh !== null && d.kwh > 0);
    const total = withData.reduce((acc, d) => acc + (d.kwh ?? 0), 0);
    const max = withData.reduce((acc, d) => Math.max(acc, d.kwh ?? 0), 0);
    return {
      days: withData.length,
      total,
      avg: withData.length ? total / withData.length : 0,
      max,
    };
  }, [history]);

  // Deep-link / extensión desactivada a mitad de sesión → vuelta al inicio.
  if (!chargerEnabled(ext)) return <Navigate to="/" replace />;

  const stateChip = live?.charging
    ? { text: `${t('common.charging')} · ${fmtKw(live.powerKw)} kW`, cls: 'bg-emerald-500/12 text-emerald-600 dark:text-emerald-400', Icon: BatteryCharging }
    : live?.connected
      ? { text: t('cargador.connected'), cls: 'bg-amber-500/12 text-amber-600 dark:text-amber-400', Icon: Cable }
      : { text: t('common.idle'), cls: 'bg-surface-2 text-muted', Icon: PlugZap };

  const stats = [
    { Icon: Zap, label: t('cargador.currentPower'), value: live ? `${fmtKw(live.powerKw)} kW` : '…' },
    { Icon: Clock, label: t('cargador.session'), value: live?.sessionKwh !== undefined ? `${fmtEnergy(live.sessionKwh)} kWh` : '—' },
    { Icon: Gauge, label: t('cargador.lifetime'), value: live?.totalKwh !== undefined ? `${fmtEnergy(live.totalKwh)} kWh` : '—' },
    { Icon: Thermometer, label: t('cargador.temperature'), value: live?.tempC !== undefined ? `${Math.round(live.tempC)} °C` : '—' },
  ];

  const yMax = Math.max(1, Math.ceil(Math.max(...histData.map((d) => d.kwh ?? 0), 0)));

  return (
    <div className="flex flex-col gap-4 lg:gap-5">
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12 lg:gap-5">
        {/* ── Estado del cargador ─────────────────────────────────────── */}
        <motion.section
          initial={{ opacity: 0, y: 24 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, ease: EASE_OUT }}
          className="helios-card col-span-12 flex flex-col items-center gap-6 p-5 shadow-card dark:shadow-card-dark sm:p-6 lg:flex-row"
        >
          <div className="flex shrink-0 flex-col items-center gap-4 lg:w-[38%]">
            <motion.div
              animate={live?.charging ? { scale: [1, 1.04, 1] } : { scale: 1 }}
              transition={live?.charging ? { duration: 2, repeat: Infinity, ease: 'easeInOut' } : undefined}
              className={cn(
                'flex h-32 w-32 items-center justify-center rounded-3xl border',
                live?.charging ? 'border-emerald-500/30 bg-emerald-500/10' : 'border-app bg-surface-2',
              )}
            >
              <CarFront
                size={64}
                strokeWidth={1.5}
                className={live?.charging ? 'text-emerald-500' : 'text-faint'}
              />
            </motion.div>
            <span className={cn('inline-flex items-center gap-1.5 rounded-full px-4 py-1.5 text-sm font-semibold', stateChip.cls)}>
              <stateChip.Icon size={14} />
              {stateChip.text}
            </span>
          </div>

          <div className="flex min-w-0 flex-1 items-center lg:border-l lg:border-app lg:pl-6">
            <div className="grid w-full grid-cols-1 gap-x-4 gap-y-3 sm:grid-cols-2">
              {stats.map((d, i) => (
                <motion.div
                  key={d.label}
                  initial={{ opacity: 0, y: 12 }}
                  whileInView={{ opacity: 1, y: 0 }}
                  viewport={{ once: true }}
                  transition={{ duration: 0.35, delay: i * 0.05 }}
                  className="flex items-center gap-2 rounded-lg px-2 py-2 hover:bg-surface-2"
                >
                  <d.Icon size={15} className="shrink-0 text-faint" />
                  <span className="text-[13px] text-muted">{d.label}</span>
                  <span className="ml-auto text-[15px] font-semibold tabular-nums text-app">{d.value}</span>
                </motion.div>
              ))}
            </div>
          </div>
        </motion.section>

        {/* ── Curva de hoy ─────────────────────────────────────────────── */}
        <motion.section
          initial={{ opacity: 0, y: 24 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.1, ease: EASE_OUT }}
          className="helios-card col-span-12 shadow-card dark:shadow-card-dark"
          aria-label={t('cargador.todayAria')}
        >
          <div className="px-4 pb-1 pt-4 sm:px-5">
            <h2 className="text-[15px] font-semibold text-app">{t('cargador.todayTitle')}</h2>
            <p className="text-xs text-faint">{t('cargador.todaySubtitle')}</p>
          </div>
          <div className="px-1">
            {curve === null ? (
              <div className="flex h-[220px] items-center justify-center text-sm text-faint">…</div>
            ) : curve.length === 0 ? (
              <div className="flex h-[220px] items-center justify-center px-6 text-center text-sm text-faint">
                {t('cargador.noDataToday')}
              </div>
            ) : (
              <ResponsiveContainer width="100%" height={220}>
                <AreaChart data={curve} margin={{ top: 10, right: 12, bottom: 0, left: 4 }}>
                  <defs>
                    <linearGradient id="chgCurve" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor={palette.consumo} stopOpacity={0.35} />
                      <stop offset="100%" stopColor={palette.consumo} stopOpacity={0.02} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid vertical={false} stroke="var(--line)" strokeOpacity={0.6} strokeDasharray="3 6" />
                  <XAxis
                    dataKey="label"
                    tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
                    axisLine={false}
                    tickLine={false}
                    minTickGap={48}
                  />
                  <YAxis
                    width={30}
                    tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
                    axisLine={false}
                    tickLine={false}
                  />
                  <Tooltip content={<CurveTooltip />} cursor={{ stroke: 'var(--line)' }} />
                  <Area
                    type="monotone"
                    dataKey="kw"
                    name={t('cargador.currentPower')}
                    stroke={palette.consumo}
                    strokeWidth={2}
                    fill="url(#chgCurve)"
                    animationDuration={500}
                  />
                </AreaChart>
              </ResponsiveContainer>
            )}
          </div>
        </motion.section>

        {/* ── Histórico (14 días) + totales ────────────────────────────── */}
        <motion.section
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.2 }}
          transition={{ duration: 0.5, delay: 0.15, ease: EASE_OUT }}
          className="helios-card col-span-12 shadow-card dark:shadow-card-dark lg:col-span-8"
          aria-label={t('cargador.historyAria')}
        >
          <div className="flex flex-wrap items-center gap-2 px-4 pb-1 pt-4 sm:px-5">
            <div className="mr-auto">
              <h2 className="text-[15px] font-semibold text-app">{t('cargador.historyTitle')}</h2>
              <p className="text-xs text-faint">{t('cargador.historySubtitle')}</p>
            </div>
            <span className="inline-flex items-center gap-1.5 text-xs text-muted">
              <span className="h-2 w-2 rounded-full" style={{ backgroundColor: palette.consumo }} /> {t('cargador.charged')}
            </span>
          </div>
          <div className="px-1">
            {histData.length === 0 ? (
              <div className="flex h-[220px] items-center justify-center px-6 text-center text-sm text-faint">
                {t('cargador.noHistory')}
              </div>
            ) : (
              <ResponsiveContainer width="100%" height={220}>
                <BarChart data={histData} margin={{ top: 10, right: 12, bottom: 0, left: 4 }}>
                  <CartesianGrid vertical={false} stroke="var(--line)" strokeOpacity={0.6} strokeDasharray="3 6" />
                  <XAxis
                    dataKey="day"
                    tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
                    axisLine={false}
                    tickLine={false}
                  />
                  <YAxis
                    domain={[0, yMax]}
                    width={30}
                    tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
                    axisLine={false}
                    tickLine={false}
                  />
                  <Tooltip content={<DayTooltip />} cursor={{ fill: 'var(--surface-2)', opacity: 0.5 }} />
                  <Bar dataKey="kwh" name={t('cargador.charged')} fill={palette.consumo} radius={[3, 3, 0, 0]} animationDuration={500} />
                </BarChart>
              </ResponsiveContainer>
            )}
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
            <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('cargador.last30')}</p>
            <p className="mt-1 font-display text-[28px] font-semibold leading-none tracking-[-0.01em] text-app">
              {fmtEnergy(histStats.total)} <span className="text-[0.6em] font-medium text-faint">kWh</span>
            </p>
            <p className="mt-1.5 text-sm text-muted">{t('cargador.daysWithData', { n: histStats.days })}</p>
          </div>
          <div className="border-t border-app pt-5">
            <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('cargador.avgDay')}</p>
            <p className="mt-1 font-display text-[22px] font-semibold leading-none text-app">
              {fmtEnergy(histStats.avg)} <span className="text-[0.6em] font-medium text-faint">kWh</span>
            </p>
          </div>
          <div className="border-t border-app pt-5">
            <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('cargador.bestDay')}</p>
            <p className="mt-1 font-display text-[22px] font-semibold leading-none text-app">
              {fmtEnergy(histStats.max)} <span className="text-[0.6em] font-medium text-faint">kWh</span>
            </p>
          </div>
        </motion.section>
      </div>
    </div>
  );
}

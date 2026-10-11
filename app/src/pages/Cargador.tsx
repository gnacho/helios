import { useEffect, useMemo, useState } from 'react';
import { motion } from 'framer-motion';
import {
  BatteryCharging,
  Cable,
  CarFront,
  Clock,
  Gauge,
  PlugZap,
  Power,
  Thermometer,
  Zap,
} from 'lucide-react';
import { format } from 'date-fns';
import { useTranslation } from 'react-i18next';
import { Navigate, useSearchParams } from 'react-router';
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
import { useExtensions } from '@/hooks/useExtensions';
import { chargerEnabled } from '@/data/types';
import type { ChargerCfg, ChargerCurvePoint, ChargerLive } from '@/data/types';
import { useEnergyColors } from '@/lib/colors';
import { fmtEnergy, fmtKw } from '@/lib/format';
import { apiFetch } from '@/data/api-client';
import { heliosToast } from '@/lib/toast';
import { dateLocale } from '@/i18n';
import { cn } from '@/lib/utils';
import PeriodSelector from '@/components/PeriodSelector';
import ChargerImageButtons from '@/components/ChargerImageButtons';
import type { Period } from '@/lib/historyStats';
import {
  periodWindow,
  initialAnchor,
  shiftAnchor,
  canGoNext,
  navLabel,
  periodSubtitle,
} from '@/lib/historyStats';

const dateKeyOf = (d: Date) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;

const EASE_OUT: [number, number, number, number] = [0.25, 1, 0.5, 1];

/** Presets de corriente de carga (A). IEC 61851: min 6 A. */
const AMP_STEPS = [6, 8, 10, 13, 16, 20, 25, 32];

interface HistoryDay {
  date: string;
  kwh: number | null;
  pvKwh: number | null;
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

interface HistRow {
  label: string;
  full: string;
  kwh: number;
  pv: number;
  rest: number;
  prod: number;
}

function HistTooltip({ active, payload }: { active?: boolean; payload?: { payload?: HistRow }[] }) {
  const { t } = useTranslation();
  if (!active || !payload || payload.length === 0) return null;
  const d = payload[0]?.payload;
  if (!d) return null;
  return (
    <div className="rounded-xl border border-app bg-surface/95 px-3 py-2 text-xs shadow-lg backdrop-blur-md">
      <p className="mb-1 font-semibold capitalize text-app">{d.full}</p>
      <p className="text-muted">
        {t('common.production')}: <span className="font-semibold text-app">{fmtEnergy(d.prod)} kWh</span>
      </p>
      <p className="text-muted">
        {t('cargador.charged')}: <span className="font-semibold text-app">{fmtEnergy(d.kwh)} kWh</span>
      </p>
      {d.kwh > 0 && (
        <p className="text-muted">
          {t('cargador.fromSolar')}: <span className="font-semibold text-app">{fmtEnergy(d.pv)} kWh</span>
        </p>
      )}
    </div>
  );
}

/** Accion admin sobre un cargador (switch on/off o corriente). */
async function chargerAction(key: string, action: 'on' | 'off' | 'amp', amps?: number) {
  return apiFetch<{ ok: boolean; amps?: number }>('/api/charger/action', {
    method: 'POST',
    body: JSON.stringify({ charger: key, action, amps }),
    headers: { 'Content-Type': 'application/json' },
  });
}

/** Pestaña de UN cargador: estado en vivo, curva del día e histórico propio
 *  (energía diaria por cargador, issue 146). */
function ChargerView({ charger, live }: { charger: ChargerCfg; live?: ChargerLive }) {
  const { t, i18n } = useTranslation();
  const palette = useEnergyColors();
  const { getHistory } = useEnergyData();

  const [curve, setCurve] = useState<ChargerCurvePoint[] | null>(null);
  const [history, setHistory] = useState<HistoryDay[] | null>(null);
  const [pwrBusy, setPwrBusy] = useState(false);
  const [ampBusy, setAmpBusy] = useState(false);

  const key = charger.key;

  const todayKey = useMemo(() => {
    const d = new Date();
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  }, []);

  useEffect(() => {
    let alive = true;
    apiFetch<{ points: ChargerCurvePoint[] }>(`/api/charger/day?date=${todayKey}&charger=${encodeURIComponent(key)}`)
      .then((res) => {
        if (alive) setCurve(res.points ?? []);
      })
      .catch(() => {
        if (alive) setCurve([]);
      });
    const to = new Date();
    const from = new Date();
    from.setDate(from.getDate() - 400); // cubre la vista por año del año anterior
    const fmt = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
    apiFetch<{ days: HistoryDay[] }>(`/api/charger/history?from=${fmt(from)}&to=${fmt(to)}&charger=${encodeURIComponent(key)}`)
      .then((res) => {
        if (alive) setHistory(res.days ?? []);
      })
      .catch(() => {
        if (alive) setHistory([]);
      });
    return () => {
      alive = false;
    };
  }, [key, todayKey]);

  const togglePower = async () => {
    if (!live || live.switchOn === undefined || pwrBusy) return;
    setPwrBusy(true);
    try {
      await chargerAction(key, live.switchOn ? 'off' : 'on');
      heliosToast(t(live.switchOn ? 'cargador.powerToastOff' : 'cargador.powerToastOn', { name: live.name }));
    } catch (err) {
      heliosToast(err instanceof Error ? err.message : t('common.error'), { tone: 'warning' });
    } finally {
      setPwrBusy(false);
    }
  };

  const setAmps = async (amps: number) => {
    if (ampBusy || live?.amps === amps) return;
    setAmpBusy(true);
    try {
      await chargerAction(key, 'amp', amps);
      heliosToast(t('cargador.ampsToast', { amps, name: live?.name ?? charger.name }));
    } catch (err) {
      heliosToast(err instanceof Error ? err.message : t('common.error'), { tone: 'warning' });
    } finally {
      setAmpBusy(false);
    }
  };

  // Histórico tipo Histórico: Semana / Mes / Año con navegador
  const [period, setPeriod] = useState<Period>('semana');
  // Semana anclada a HOY (no a ayer como el Histórico): la carga de hoy es lo
  // primero que uno quiere ver. Ventana = últimos 7 días incluyendo hoy.
  const [anchor, setAnchor] = useState<Date>(() => {
    const n = new Date();
    return new Date(n.getFullYear(), n.getMonth(), n.getDate());
  });
  const today = useMemo(() => {
    const n = new Date();
    return new Date(n.getFullYear(), n.getMonth(), n.getDate());
  }, []);
  const solarHistory = getHistory(); // producción diaria para la barra de contexto

  /** Ancla "actual" para el cargador: semana = HOY (incluye el día en curso);
   *  mes/año igual que el Histórico. */
  const homeAnchor = (p: Period, tt: Date): Date =>
    p === 'semana' ? new Date(tt.getFullYear(), tt.getMonth(), tt.getDate()) : initialAnchor(p, tt);

  const changePeriod = (p: Period) => {
    setPeriod(p);
    setAnchor(homeAnchor(p, today));
  };

  const isCurrent = (p: Period, a: Date): boolean =>
    periodWindow(p, a).start.getTime() === periodWindow(p, homeAnchor(p, today)).start.getTime();

  /** Filas del periodo: rango completo (con ceros) para semana/mes/año. */
  const rows = useMemo(() => {
    const chargerByDay = new Map((history ?? []).map((d) => [d.date, d]));
    const prodByDay = new Map<string, number>();
    for (const d of solarHistory) prodByDay.set(dateKeyOf(d.date), d.productionKwh);

    const out: HistRow[] = [];

    const dayRow = (date: Date) => {
      const k = dateKeyOf(date);
      const c = chargerByDay.get(k);
      const kwh = c?.kwh ?? 0;
      const pv = c?.pvKwh ?? 0;
      out.push({
        label: format(date, period === 'mes' ? 'd' : 'EEEEE', { locale: dateLocale() }).toUpperCase(),
        full: format(date, 'EEE d MMM', { locale: dateLocale() }),
        kwh,
        pv,
        rest: Math.max(0, kwh - pv),
        prod: prodByDay.get(k) ?? 0,
      });
    };

    if (period === 'ano') {
      for (let m = 0; m < 12; m++) {
        const monthDate = new Date(anchor.getFullYear(), m, 1);
        let kwh = 0;
        let pv = 0;
        let prod = 0;
        const daysInMonth = new Date(anchor.getFullYear(), m + 1, 0).getDate();
        for (let dd = 1; dd <= daysInMonth; dd++) {
          const dayDate = new Date(anchor.getFullYear(), m, dd);
          if (dayDate > today) break;
          const k = dateKeyOf(dayDate);
          const c = chargerByDay.get(k);
          kwh += c?.kwh ?? 0;
          pv += c?.pvKwh ?? 0;
          prod += prodByDay.get(k) ?? 0;
        }
        out.push({
          label: format(monthDate, 'MMM', { locale: dateLocale() }),
          full: format(monthDate, 'MMMM yyyy', { locale: dateLocale() }),
          kwh,
          pv,
          rest: Math.max(0, kwh - pv),
          prod,
        });
      }
    } else {
      const { start, end } = periodWindow(period, anchor);
      const last = end > today ? today : end;
      for (const d = new Date(start); d <= last; d.setDate(d.getDate() + 1)) {
        dayRow(new Date(d));
      }
    }
    return out;
  }, [history, solarHistory, period, anchor, today, i18n.language]);

  const stats = useMemo(() => {
    const kwh = rows.reduce((acc, r) => acc + r.kwh, 0);
    const pv = rows.reduce((acc, r) => acc + r.pv, 0);
    const prod = rows.reduce((acc, r) => acc + r.prod, 0);
    const activeDays = rows.filter((r) => r.kwh > 0).length;
    return {
      kwh,
      pv,
      prod,
      pctSolar: kwh > 0 ? (pv / kwh) * 100 : 0,
      pctOfProduction: prod > 0 ? (kwh / prod) * 100 : 0,
      avgDay: activeDays > 0 ? kwh / activeDays : 0,
    };
  }, [rows]);

  const hasData = rows.some((r) => r.kwh > 0 || r.prod > 0);
  const yMax = Math.max(1, Math.ceil(Math.max(...rows.map((r) => r.kwh), 0)));
  // Ancho de barra adaptado al nº de categorías (mes = ~30 días).
  const barW = period === 'mes' ? 10 : 22;

  const stateChip = live?.charging
    ? { text: `${t('common.charging')} · ${fmtKw(live.powerKw)} kW`, cls: 'bg-emerald-500/12 text-emerald-600 dark:text-emerald-400', Icon: BatteryCharging }
    : live?.connected
      ? { text: t('cargador.connected'), cls: 'bg-amber-500/12 text-amber-600 dark:text-amber-400', Icon: Cable }
      : { text: t('common.idle'), cls: 'bg-surface-2 text-muted', Icon: PlugZap };

  const liveStats = [
    { Icon: Zap, label: t('cargador.currentPower'), value: live ? `${fmtKw(live.powerKw)} kW` : '…' },
    { Icon: Clock, label: t('cargador.session'), value: live?.sessionKwh !== undefined ? `${fmtEnergy(live.sessionKwh)} kWh` : '—' },
    { Icon: Gauge, label: t('cargador.lifetime'), value: live?.totalKwh !== undefined ? `${fmtEnergy(live.totalKwh)} kWh` : '—' },
  ];

  return (
    <div className="flex flex-col gap-4 lg:gap-5">
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12 lg:gap-5">
        {/* Cargador: titulo, foto y acciones */}
        <motion.section
          initial={{ opacity: 0, y: 24 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, ease: EASE_OUT }}
          className="helios-card col-span-12 flex flex-col gap-4 p-5 shadow-card dark:shadow-card-dark sm:p-6"
        >
          <div className="flex flex-wrap items-center gap-3">
            <h2 className="text-[17px] font-semibold text-app">{charger.name}</h2>
            <span className={cn('inline-flex items-center gap-1.5 rounded-full px-4 py-1.5 text-sm font-semibold', stateChip.cls)}>
              <stateChip.Icon size={14} />
              {stateChip.text}
            </span>
              <ChargerImageButtons charger={charger} />
          </div>

          <div className="flex flex-col gap-5 lg:flex-row lg:items-stretch">
            {/* Foto del coche: ocupa el alto del bloque, fondo blanco/oscuro sin acento */}
            <motion.div
              animate={live?.charging ? { scale: [1, 1.02, 1] } : { scale: 1 }}
              transition={live?.charging ? { duration: 2, repeat: Infinity, ease: 'easeInOut' } : undefined}
              className="flex min-h-44 shrink-0 items-center justify-center self-stretch rounded-3xl bg-white p-3 lg:w-[42%] dark:bg-transparent"
            >
              {charger.imageUrl ? (
                <img
                  src={charger.imageUrl}
                  alt={charger.name}
                  draggable={false}
                  className="h-full max-h-full w-full object-contain select-none"
                />
              ) : (
                <CarFront
                  size={72}
                  strokeWidth={1.5}
                  className={live?.charging ? 'text-emerald-500' : 'text-faint'}
                />
              )}
            </motion.div>

            {/* Acciones: encender/apagar grande, amperaje y temperatura */}
            <div className="flex min-w-0 flex-1 flex-col justify-center gap-5">
              <button
                type="button"
                onClick={togglePower}
                disabled={live?.switchOn === undefined || pwrBusy}
                aria-pressed={live?.switchOn === true}
                title={t(live?.switchOn ? 'cargador.powerOff' : 'cargador.powerOn')}
                className={cn(
                  'inline-flex h-14 w-full items-center justify-center gap-2.5 rounded-2xl text-base font-semibold transition-colors disabled:opacity-50',
                  live?.switchOn
                    ? 'bg-emerald-500/12 text-emerald-600 hover:bg-emerald-500/20 dark:text-emerald-400'
                    : 'bg-brand text-white hover:opacity-90',
                )}
              >
                <Power size={20} strokeWidth={2} className={pwrBusy ? 'animate-pulse' : undefined} />
                {t(live?.switchOn ? 'cargador.powerOff' : 'cargador.powerOn')}
              </button>
              {/* Amperaje (solo con entidad de corriente configurada) */}
              {charger.ampId && (
                <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label={t('cargador.ampsAria')}>
                  <span className="mr-1 text-xs text-faint">{t('cargador.amps')}</span>
                  {(live?.ampOptions?.length ? live.ampOptions : AMP_STEPS).map((a) => (
                    <button
                      key={a}
                      type="button"
                      onClick={() => void setAmps(a)}
                      disabled={ampBusy || live?.amps === a}
                      aria-pressed={live?.amps === a}
                      className={cn(
                        'inline-flex h-7 min-w-9 items-center justify-center rounded-full border px-2 text-xs font-semibold tabular-nums transition-colors disabled:opacity-50',
                        live?.amps === a
                          ? 'border-brand bg-brand/10 text-brand'
                          : 'border-app bg-surface text-muted hover:bg-surface-2 hover:text-app',
                      )}
                    >
                      {a}
                    </button>
                  ))}
                </div>
              )}
              <div className="flex items-center gap-2 px-1">
                <Thermometer size={15} className="shrink-0 text-faint" />
                <span className="text-[13px] text-muted">{t('cargador.temperature')}</span>
                <span className="text-[15px] font-semibold tabular-nums text-app">
                  {live?.tempC !== undefined ? `${Math.round(live.tempC)} °C` : '—'}
                </span>
              </div>
            </div>
          </div>
        </motion.section>

        {/* Curva de hoy */}
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
          <div className="flex flex-wrap items-center gap-x-5 gap-y-1.5 px-4 pb-2 sm:px-5">
            {liveStats.map((d) => (
              <span key={d.label} className="inline-flex items-center gap-1.5 text-[13px] text-muted">
                <d.Icon size={13} className="shrink-0 text-faint" />
                {d.label}
                <span className="font-semibold tabular-nums text-app">{d.value}</span>
              </span>
            ))}
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

        {/* Histórico + totales del periodo */}
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
              <p className="text-xs capitalize text-faint">{periodSubtitle(period, anchor)}</p>
              <p className="mt-0.5 text-xs text-faint">{t('cargador.historyHint')}</p>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2 px-4 pb-2 sm:px-5">
            <PeriodSelector
              period={period}
              periods={['semana', 'mes', 'ano']}
              onPeriodChange={changePeriod}
              label={navLabel(period, anchor)}
              canNext={canGoNext(period, anchor, today)}
              isCurrent={isCurrent(period, anchor)}
              onPrev={() => setAnchor((a) => shiftAnchor(period, a, -1))}
              onNext={() => setAnchor((a) => shiftAnchor(period, a, 1))}
              onToday={() => setAnchor(homeAnchor(period, today))}
            />
          </div>
          <div className="flex flex-wrap items-center gap-3 px-4 pb-1 sm:px-5">
            <span className="inline-flex items-center gap-1.5 text-xs text-muted">
              <span className="h-2 w-2 rounded-full" style={{ backgroundColor: palette.bateria }} /> {t('cargador.fromSolar')}
            </span>
            <span className="inline-flex items-center gap-1.5 text-xs text-muted">
              <span className="h-2 w-2 rounded-full" style={{ backgroundColor: palette.consumo }} /> {t('cargador.fromGrid')}
            </span>
          </div>
          <div className="px-1">
            {!hasData ? (
              <div className="flex h-[220px] items-center justify-center px-6 text-center text-sm text-faint">
                {t('cargador.noHistory')}
              </div>
            ) : (
              <ResponsiveContainer width="100%" height={220}>
                <BarChart data={rows} margin={{ top: 10, right: 12, bottom: 0, left: 4 }} barGap={2}>
                  <CartesianGrid vertical={false} stroke="var(--line)" strokeOpacity={0.6} strokeDasharray="3 6" />
                  <XAxis
                    dataKey="label"
                    tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
                    axisLine={false}
                    tickLine={false}
                    minTickGap={period === 'semana' ? 0 : 12}
                    interval="preserveStartEnd"
                  />
                  {/* Un único eje: kWh cargados. La producción va en el tooltip y
                   *  en la tarjeta lateral (un segundo eje con escala distinta
                   *  resultaba confuso). */}
                  <YAxis
                    domain={[0, yMax]}
                    width={34}
                    tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
                    axisLine={false}
                    tickLine={false}
                  />
                  <Tooltip content={<HistTooltip />} cursor={{ fill: 'var(--surface-2)', opacity: 0.5 }} />
                  {/* Carga del coche, partida en solar vs resto */}
                  <Bar dataKey="pv" name={t('cargador.fromSolar')} stackId="chg" fill={palette.bateria} barSize={barW} animationDuration={500} />
                  <Bar
                    dataKey="rest"
                    name={t('cargador.fromGrid')}
                    stackId="chg"
                    fill={palette.consumo}
                    barSize={barW}
                    radius={[3, 3, 0, 0]}
                    animationDuration={500}
                    animationBegin={60}
                  />
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
            <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('cargador.periodTotal')}</p>
            <p className="mt-1 font-display text-[28px] font-semibold leading-none tracking-[-0.01em] text-app">
              {fmtEnergy(stats.kwh)} <span className="text-[0.6em] font-medium text-faint">kWh</span>
            </p>
          </div>
          <div className="border-t border-app pt-5">
            <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('cargador.fromSolar')}</p>
            <p className="mt-1 font-display text-[22px] font-semibold leading-none text-app">
              {Math.round(stats.pctSolar)} <span className="text-[0.6em] font-medium text-faint">%</span>
            </p>
            <div className="mt-2.5 h-2 w-full overflow-hidden rounded-full bg-surface-2" role="progressbar" aria-valuenow={Math.round(stats.pctSolar)} aria-valuemin={0} aria-valuemax={100} aria-label={t('cargador.pctSolarAria')}>
              <motion.div
                className="h-full rounded-full"
                style={{ backgroundColor: palette.bateria }}
                initial={{ width: 0 }}
                whileInView={{ width: `${Math.min(100, stats.pctSolar)}%` }}
                viewport={{ once: true }}
                transition={{ duration: 0.8, ease: 'easeOut', delay: 0.3 }}
              />
            </div>
            <p className="mt-1.5 text-sm text-muted">
              {fmtEnergy(stats.pv)} kWh · {t('cargador.fromGrid')}: {fmtEnergy(stats.kwh - stats.pv)} kWh
            </p>
          </div>
          <div className="border-t border-app pt-5">
            <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('common.production')}</p>
            <p className="mt-1 font-display text-[22px] font-semibold leading-none text-app">
              {fmtEnergy(stats.prod)} <span className="text-[0.6em] font-medium text-faint">kWh</span>
            </p>
            <p className="mt-1.5 text-sm text-muted">{t('cargador.ofProduction', { pct: Math.round(stats.pctOfProduction) })}</p>
          </div>
          <div className="border-t border-app pt-5">
            <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{t('cargador.avgDay')}</p>
            <p className="mt-1 font-display text-[22px] font-semibold leading-none text-app">
              {fmtEnergy(stats.avgDay)} <span className="text-[0.6em] font-medium text-faint">kWh</span>
            </p>
          </div>
        </motion.section>
      </div>
    </div>
  );
}

/** Página del cargador (extensión #94): pestañas por cargador (146). Solo se
 *  llega con la extensión activa. */
export default function Cargador() {
  const { t } = useTranslation();
  const ext = useExtensions();
  const { getLivePower, nowMin, liveTick } = useEnergyData();
  const [searchParams, setSearchParams] = useSearchParams();

  const live = getLivePower(nowMin, liveTick).chargers;
  const cfg = ext?.carCharger;
  const tabKeys = useMemo(() => (cfg?.chargers ?? []).map((c) => c.key), [cfg]);

  const tabParam = searchParams.get('tab');
  const tab = tabKeys.includes(tabParam ?? '') ? (tabParam as string) : (tabKeys[0] ?? 'main');
  const setTab = (k: string) => setSearchParams({ tab: k }, { replace: true });

  const charger = cfg?.chargers.find((c) => c.key === tab) ?? cfg?.chargers[0];
  const liveOfTab = live?.find((c) => c.key === tab) ?? live?.[0];

  // Mientras la config de extensiones carga (null) NO redirigimos (race de
  // deep-link: un F5 en /cargador debe caer aquí, no en el dashboard).
  // Extensión desactivada a mitad de sesión → vuelta al inicio.
  if (ext !== null && !chargerEnabled(ext)) return <Navigate to="/" replace />;
  if (ext === null || !charger || !cfg) {
    return (
      <div className="flex h-[50vh] items-center justify-center text-sm text-faint">…</div>
    );
  }

  return (
    <div className="flex flex-col gap-4 lg:gap-5">
      {/* Segmented control por cargador (mismo patrón que Inversores), solo
       *  con más de un cargador configurado. */}
      {tabKeys.length > 1 && (
        <motion.div
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.4, delay: 0.05 }}
          className="sticky top-14 z-30 -mx-4 bg-[color-mix(in_srgb,var(--bg)_85%,transparent)] px-4 py-2 backdrop-blur-[16px] lg:static lg:mx-0 lg:bg-transparent lg:p-0 lg:backdrop-blur-none"
        >
          <div className="flex h-10 w-full items-center gap-1 overflow-x-auto rounded-full border border-app bg-surface p-1 no-scrollbar lg:w-fit" role="tablist" aria-label={t('cargador.tabsAria')}>
            {cfg.chargers.map((c) => {
              const active = c.key === tab;
              return (
                <button
                  key={c.key}
                  role="tab"
                  aria-selected={active}
                  onClick={() => setTab(c.key)}
                  className={cn(
                    'relative flex h-full flex-1 items-center justify-center gap-1.5 rounded-full px-4 text-sm font-medium transition-colors lg:flex-none',
                    active ? 'text-app' : 'text-faint hover:text-muted',
                  )}
                >
                  {active && (
                    <motion.span
                      layoutId="chg-tab-pill"
                      className="absolute inset-0 rounded-full bg-surface-2 shadow-inner"
                      transition={{ type: 'spring', stiffness: 400, damping: 32 }}
                    />
                  )}
                  <span className="relative z-10 flex items-center gap-1.5">
                    <span
                      className={cn('h-2 w-2 rounded-full', live?.find((l) => l.key === c.key)?.charging ? 'bg-emerald-500' : 'bg-faint opacity-40')}
                    />
                    {c.name || c.key}
                  </span>
                </button>
              );
            })}
          </div>
        </motion.div>
      )}

      <ChargerView charger={charger} live={liveOfTab} />
    </div>
  );
}

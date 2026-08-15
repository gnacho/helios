import { lazy, Suspense, useCallback, useEffect, useMemo, useState } from 'react';
import {
  BatteryCharging,
  Car,
  CarFront,
  Fan,
  Flashlight,
  Gauge,
  Lock,
  LockOpen,
  MapPin,
  PlugZap,
  RefreshCw,
  Snowflake,
  Thermometer,
  DoorOpen,
  Eye,
  Volume2,
  AppWindow,
  Zap,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Navigate } from 'react-router';
import { useEnergyData } from '@/data/EnergyDataProvider';
import { useExtensions } from '@/hooks/useExtensions';
import { bydEnabled } from '@/data/types';
import type { BydLive } from '@/data/types';
import { apiFetch } from '@/data/api-client';
import { heliosToast } from '@/lib/toast';
import { cn } from '@/lib/utils';

// Mapa Leaflet en chunk aparte (solo se carga al abrir la página del coche).
const CarMap = lazy(() => import('@/components/CarMap'));

/**
 * Página del vehículo BYD (extensión, issue 100, personal): estado en vivo del
 * coche, acciones de carga, estado del vehículo y presiones de neumáticos.
 * Sólo se llega con la extensión activa; las acciones requieren admin.
 */

/** Color semántico del SoC (verde >50, ámbar 20-50, rojo <20). */
function socTone(soc?: number): string {
  if (soc === undefined) return 'bg-muted'
  if (soc > 50) return 'bg-emerald-500'
  if (soc > 20) return 'bg-amber-500'
  return 'bg-rose-500'
}

function Chip({
  on,
  onLabel,
  offLabel,
  icon: Icon,
  warn,
}: {
  on?: boolean;
  onLabel: string;
  offLabel: string;
  icon: typeof Lock;
  warn?: boolean;
}) {
  const active = warn ? !!on : on !== undefined && on;
  return (
    <div
      className={cn(
        'flex items-center gap-2 rounded-xl border px-3 py-2 text-[13px] font-medium',
        active
          ? warn
            ? 'border-amber-500/40 bg-amber-500/10 text-amber-600 dark:text-amber-400'
            : 'border-emerald-500/40 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
          : 'border-app bg-surface text-muted',
      )}
    >
      <Icon size={16} aria-hidden="true" />
      {active ? onLabel : offLabel}
    </div>
  );
}

function TireCell({ label, value, mean }: { label: string; value?: number; mean?: number }) {
  const dev = value !== undefined && mean !== undefined ? value - mean : undefined;
  // Desviación respecto a la media de los 4 (hecho observable, no umbral
  // oficial): ±10 kPa marca la rueda desviada.
  const off = dev !== undefined && Math.abs(dev) > 10;
  return (
    <div
      className={cn(
        'flex flex-col items-center rounded-xl border px-3 py-2.5',
        off ? 'border-amber-500/40 bg-amber-500/10' : 'border-app bg-surface',
      )}
    >
      <span className="text-[10px] font-semibold uppercase tracking-wide text-faint">{label}</span>
      <span className={cn('font-display text-lg font-semibold tnum', off ? 'text-amber-600 dark:text-amber-400' : 'text-app')}>
        {value !== undefined ? (value / 100).toFixed(1) : '—'}
        <span className="ml-0.5 text-[11px] font-normal text-muted">bar</span>
      </span>
    </div>
  );
}

export default function Coche() {
  const { t } = useTranslation();
  const ext = useExtensions();
  const { getLivePower, nowMin, liveTick } = useEnergyData();
  const live: BydLive | undefined = getLivePower(nowMin, liveTick).byd;

  const [userRole, setUserRole] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  useEffect(() => {
    apiFetch<{ authenticated?: boolean; user?: { role?: string } }>('/api/auth/me')
      .then((data) => {
        if (data.authenticated && data.user) setUserRole(data.user.role ?? null);
      })
      .catch(() => setUserRole(null));
  }, []);

  const isAdmin = userRole === 'admin';

  const act = useCallback(
    async (action: string, value?: string | boolean) => {
      setBusy(action + String(value ?? ''));
      try {
        await apiFetch('/api/byd/action', {
          method: 'POST',
          body: JSON.stringify({ action, value }),
        });
        heliosToast(t('coche.actionSent'), { tone: 'success' });
      } catch (e) {
        const msg = e instanceof Error ? e.message : 'error';
        heliosToast(msg === 'solo administradores pueden controlar el vehículo' ? t('coche.adminOnly') : t('coche.actionFailed'), {
          tone: 'warning',
        });
      } finally {
        setBusy(null);
      }
    },
    [t],
  );

  // Media de presiones (para detectar la rueda desviada)
  const tireMean = useMemo(() => {
    const v = [live?.tires.fl, live?.tires.fr, live?.tires.rl, live?.tires.rr].filter(
      (x): x is number => x !== undefined,
    );
    return v.length ? v.reduce((a, b) => a + b, 0) / v.length : undefined;
  }, [live]);

  // Estado del coche: offline > cargando > enchufado > en marcha > reposo
  const state = !live?.online
    ? { key: 'offline', cls: 'bg-muted text-muted' }
    : live.charging
      ? { key: 'charging', cls: 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400' }
      : live.plugged
        ? { key: 'plugged', cls: 'bg-amber-500/15 text-amber-600 dark:text-amber-400' }
        : { key: 'idle', cls: 'bg-surface-2 text-app' };

  if (ext !== null && !bydEnabled(ext)) return <Navigate to="/" replace />;
  if (ext === null || !live) {
    return (
      <div className="grid min-h-[40vh] place-items-center text-sm text-muted" role="status">
        {t('common.loading')}
      </div>
    );
  }

  const soc = live.soc;
  const busyKey = (a: string, v?: string | boolean) => a + String(v ?? '');

  return (
    <div className="space-y-4 pb-8">
      {/* ── Hero: batería + estado ─────────────────────────────────────── */}
      <section className="overflow-hidden rounded-2xl border border-app bg-surface">
        <div className="flex flex-col gap-6 p-5 sm:flex-row sm:items-center sm:gap-8 sm:p-6">
          {/* SoC */}
          <div className="flex items-center gap-4">
            <span
              className={cn(
                'grid h-16 w-16 shrink-0 place-items-center rounded-2xl',
                live.charging ? 'bg-emerald-500/12 text-emerald-500' : 'bg-brand/12 text-brand',
              )}
            >
              {live.charging ? <BatteryCharging size={34} /> : <CarFront size={34} />}
            </span>
            <div>
              <p className="font-display text-5xl font-semibold leading-none tracking-tight text-app tnum">
                {soc !== undefined ? Math.round(soc) : '—'}
                <span className="ml-1 text-xl font-medium text-muted">%</span>
              </p>
              <p className="mt-1 text-sm text-muted tnum">
                {live.rangeKm !== undefined ? `≈ ${Math.round(live.rangeKm)} km` : '—'}
              </p>
            </div>
          </div>

          {/* Estado + barra + meta */}
          <div className="min-w-0 flex-1 space-y-3">
            <div className="flex flex-wrap items-center gap-2">
              <span className={cn('rounded-full px-3 py-1 text-[12px] font-semibold', state.cls)}>
                {t(`coche.state.${state.key}`)}
                {live.charging && live.powerKw !== undefined && live.powerKw > 0
                  ? ` · ${(live.powerKw).toFixed(1)} kW`
                  : ''}
                {live.charging && live.remainingMin !== undefined && live.remainingMin > 0
                  ? ` · ${Math.floor(live.remainingMin / 60)}h ${String(live.remainingMin % 60).padStart(2, '0')}min`
                  : ''}
              </span>
              {live.odometerKm !== undefined && (
                <span className="rounded-full bg-surface-2 px-3 py-1 text-[12px] text-muted tnum">
                  {t('coche.odometer')}: {Math.round(live.odometerKm).toLocaleString()} km
                </span>
              )}
              {live.lastUpdateMin !== undefined && (
                <span className="text-[11px] text-faint">
                  {t('coche.updated')}: {t('coche.minAgo', { n: live.lastUpdateMin })}
                </span>
              )}
            </div>
            <div
              className="h-3 overflow-hidden rounded-full bg-surface-2"
              role="progressbar"
              aria-valuenow={soc !== undefined ? Math.round(soc) : undefined}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-label={t('coche.soc')}
            >
              <div
                className={cn('h-full rounded-full transition-all duration-700', socTone(soc))}
                style={{ width: soc !== undefined ? `${Math.min(100, Math.max(0, soc))}%` : '0%' }}
              />
            </div>
          </div>
        </div>
      </section>

      {/* ── Acciones de carga (admin) ──────────────────────────────────── */}
      {isAdmin && (
        <section className="rounded-2xl border border-app bg-surface p-5">
          <h2 className="mb-3 flex items-center gap-2 font-display text-base font-semibold text-app">
            <Zap size={17} aria-hidden="true" />
            {t('coche.chargeTitle')}
          </h2>
          <div className="flex flex-wrap items-center gap-2.5">
            <button
              type="button"
              onClick={() => act('start_charging')}
              disabled={live.charging || busyKey('start_charging') === busy}
              className="flex items-center gap-2 rounded-xl bg-emerald-600 px-4 py-2 text-sm font-semibold text-white transition-opacity hover:opacity-90 disabled:opacity-40"
            >
              <PlugZap size={16} aria-hidden="true" />
              {t('coche.startCharge')}
            </button>
            <button
              type="button"
              onClick={() => act('stop_charging')}
              disabled={!live.charging || busyKey('stop_charging') === busy}
              className="flex items-center gap-2 rounded-xl border border-app bg-surface-2 px-4 py-2 text-sm font-semibold text-app transition-colors hover:bg-surface-2/60 disabled:opacity-40"
            >
              {t('coche.stopCharge')}
            </button>
            <button
              type="button"
              onClick={() => act('force_poll')}
              disabled={busyKey('force_poll') === busy}
              className="flex items-center gap-2 rounded-xl border border-app bg-surface-2 px-4 py-2 text-sm font-semibold text-app transition-colors hover:bg-surface-2/60 disabled:opacity-40"
              title={t('coche.forcePollHint')}
            >
              <RefreshCw size={15} className={busyKey('force_poll') === busy ? 'animate-spin' : ''} aria-hidden="true" />
              {t('coche.forcePoll')}
            </button>
            <label className="flex cursor-pointer items-center gap-2 rounded-xl border border-app bg-surface-2 px-4 py-2 text-sm font-medium text-app">
              <input
                type="checkbox"
                className="h-4 w-4 accent-[rgb(var(--accent-rgb))]"
                checked={live.chargeToFullOn ?? false}
                onChange={(e) => act('charge_to_full', e.target.checked)}
              />
              {t('coche.chargeToFull')}
            </label>
          </div>
          <p className="mt-2.5 text-[11px] text-faint">{t('coche.chargeHint')}</p>
        </section>
      )}

      {/* ── Control del vehículo (admin) ───────────────────────────────── */}
      {isAdmin && (
        <section className="rounded-2xl border border-app bg-surface p-5">
          <h2 className="mb-3 flex items-center gap-2 font-display text-base font-semibold text-app">
            <Fan size={17} aria-hidden="true" />
            {t('coche.controlTitle')}
          </h2>
          <div className="flex flex-wrap items-center gap-2.5">
            <button
              type="button"
              onClick={() => act('ac', live.acOn !== true)}
              disabled={busyKey('ac', live.acOn !== true) === busy}
              className={cn(
                'flex items-center gap-2 rounded-xl px-4 py-2 text-sm font-semibold transition-colors disabled:opacity-40',
                live.acOn
                  ? 'bg-sky-600 text-white hover:opacity-90'
                  : 'border border-app bg-surface-2 text-app hover:bg-surface-2/60',
              )}
            >
              {live.acOn ? <Snowflake size={16} aria-hidden="true" /> : <Fan size={16} aria-hidden="true" />}
              {live.acOn ? t('coche.acOn') : t('coche.acOff')}
            </button>
            <button
              type="button"
              onClick={() => act(live.lockUnlocked ? 'lock' : 'unlock')}
              disabled={busyKey(live.lockUnlocked ? 'lock' : 'unlock') === busy}
              className="flex items-center gap-2 rounded-xl border border-app bg-surface-2 px-4 py-2 text-sm font-semibold text-app transition-colors hover:bg-surface-2/60 disabled:opacity-40"
            >
              {live.lockUnlocked ? <LockOpen size={16} aria-hidden="true" /> : <Lock size={16} aria-hidden="true" />}
              {live.lockUnlocked ? t('coche.lockAction') : t('coche.unlockAction')}
            </button>
            <button
              type="button"
              onClick={() => act('flash_lights')}
              disabled={busyKey('flash_lights') === busy}
              className="flex items-center gap-2 rounded-xl border border-app bg-surface-2 px-4 py-2 text-sm font-semibold text-app transition-colors hover:bg-surface-2/60 disabled:opacity-40"
              title={t('coche.flashHint')}
            >
              <Flashlight size={16} aria-hidden="true" />
              {t('coche.flashLights')}
            </button>
            <button
              type="button"
              onClick={() => act('find_car')}
              disabled={busyKey('find_car') === busy}
              className="flex items-center gap-2 rounded-xl border border-app bg-surface-2 px-4 py-2 text-sm font-semibold text-app transition-colors hover:bg-surface-2/60 disabled:opacity-40"
              title={t('coche.findCarHint')}
            >
              <Volume2 size={16} aria-hidden="true" />
              {t('coche.findCar')}
            </button>
            <button
              type="button"
              onClick={() => act('close_windows')}
              disabled={busyKey('close_windows') === busy}
              className="flex items-center gap-2 rounded-xl border border-app bg-surface-2 px-4 py-2 text-sm font-semibold text-app transition-colors hover:bg-surface-2/60 disabled:opacity-40"
            >
              <AppWindow size={16} aria-hidden="true" />
              {t('coche.closeWindows')}
            </button>
          </div>
          <p className="mt-2.5 text-[11px] text-faint">{t('coche.controlHint')}</p>
        </section>
      )}

      {/* ── Ubicación ──────────────────────────────────────────────────── */}
      <section className="rounded-2xl border border-app bg-surface p-5">
        <h2 className="mb-3 flex items-center gap-2 font-display text-base font-semibold text-app">
          <MapPin size={17} aria-hidden="true" />
          {t('coche.locationTitle')}
          {live.gpsAgeMin !== undefined && (
            <span className="ml-auto text-[11px] font-normal text-faint">
              {t('coche.updated')} {t('coche.minAgo', { n: live.gpsAgeMin })}
            </span>
          )}
        </h2>
        {live.lat !== undefined && live.lon !== undefined ? (
          <Suspense fallback={<div className="h-64 w-full animate-pulse rounded-xl bg-surface-2" />}>
            <CarMap lat={live.lat} lon={live.lon} label={live.name} />
          </Suspense>
        ) : (
          <p className="text-sm text-muted">{t('coche.locationUnknown')}</p>
        )}
      </section>

      {/* ── Consumo ────────────────────────────────────────────────────── */}
      {(live.consumptionRecent !== undefined ||
        live.consumption50 !== undefined ||
        live.consumptionLifetime !== undefined ||
        live.consumptionToday !== undefined) && (
        <section className="rounded-2xl border border-app bg-surface p-5">
          <h2 className="mb-3 flex items-center gap-2 font-display text-base font-semibold text-app">
            <Zap size={17} aria-hidden="true" />
            {t('coche.consumptionTitle')}
          </h2>
          <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
            {[
              { label: t('coche.consumptionRecent'), v: live.consumptionRecent },
              { label: t('coche.consumption50'), v: live.consumption50 },
              { label: t('coche.consumptionToday'), v: live.consumptionToday },
              { label: t('coche.consumptionLifetime'), v: live.consumptionLifetime },
            ]
              .filter((s) => s.v !== undefined)
              .map((s) => (
                <div key={s.label} className="flex flex-col items-center rounded-xl border border-app bg-surface px-3 py-2.5">
                  <span className="text-[10px] font-semibold uppercase tracking-wide text-faint">{s.label}</span>
                  <span className="font-display text-lg font-semibold text-app tnum">
                    {(s.v as number).toFixed(1)}
                    <span className="ml-0.5 text-[11px] font-normal text-muted">kWh/100km</span>
                  </span>
                </div>
              ))}
          </div>
        </section>
      )}

      {/* ── Estado del vehículo ────────────────────────────────────────── */}
      <section className="rounded-2xl border border-app bg-surface p-5">
        <h2 className="mb-3 flex items-center gap-2 font-display text-base font-semibold text-app">
          <Car size={17} aria-hidden="true" />
          {t('coche.vehicleTitle')}
        </h2>
        <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-3 lg:grid-cols-4">
          <Chip
            on={live.locked}
            onLabel={t('coche.locked')}
            offLabel={t('coche.unlocked')}
            icon={live.locked ? Lock : LockOpen}
          />
          <Chip on={live.doorsOpen} warn onLabel={t('coche.doorsOpen')} offLabel={t('coche.doorsClosed')} icon={DoorOpen} />
          <Chip on={live.windowsOpen} warn onLabel={t('coche.windowsOpen')} offLabel={t('coche.windowsClosed')} icon={DoorOpen} />
          <Chip on={live.sentry} onLabel={t('coche.sentryOn')} offLabel={t('coche.sentryOff')} icon={Eye} />
        </div>
        <div className="mt-2.5 flex flex-wrap items-center gap-2.5">
          <div className="flex items-center gap-2 rounded-xl border border-app bg-surface px-3 py-2 text-[13px] font-medium text-app">
            <MapPin size={15} aria-hidden="true" />
            {live.location === 'home'
              ? t('coche.atHome')
              : live.location === 'not_home'
                ? t('coche.away')
                : (live.location ?? '—')}
          </div>
          {live.cabinTempC !== undefined && live.exteriorTempC !== undefined && (
            <div className="flex items-center gap-2 rounded-xl border border-app bg-surface px-3 py-2 text-[13px] font-medium text-app tnum">
              <Thermometer size={15} aria-hidden="true" />
              {t('coche.cabin')}: {Math.round(live.cabinTempC)}° · {t('coche.outside')}: {Math.round(live.exteriorTempC)}°
              <span className="text-[11px] text-faint">
                ({live.cabinTempC > live.exteriorTempC ? '+' : ''}
                {Math.round(live.cabinTempC - live.exteriorTempC)}°)
              </span>
            </div>
          )}
        </div>
      </section>

      {/* ── Neumáticos ─────────────────────────────────────────────────── */}
      <section className="rounded-2xl border border-app bg-surface p-5">
        <h2 className="mb-3 flex items-center gap-2 font-display text-base font-semibold text-app">
          <Gauge size={17} aria-hidden="true" />
          {t('coche.tiresTitle')}
        </h2>
        <div className="mx-auto grid max-w-sm grid-cols-2 gap-3">
          {/* Vista coche: delante arriba, detrás abajo */}
          <TireCell label={t('coche.tireFL')} value={live.tires.fl} mean={tireMean} />
          <TireCell label={t('coche.tireFR')} value={live.tires.fr} mean={tireMean} />
          <TireCell label={t('coche.tireRL')} value={live.tires.rl} mean={tireMean} />
          <TireCell label={t('coche.tireRR')} value={live.tires.rr} mean={tireMean} />
        </div>
        <p className="mt-2.5 text-center text-[11px] text-faint">{t('coche.tiresHint')}</p>
      </section>

      {/* ── Programación de carga (admin) ──────────────────────────────── */}
      {isAdmin && (
        <section className="rounded-2xl border border-app bg-surface p-5">
          <h2 className="mb-3 flex items-center gap-2 font-display text-base font-semibold text-app">
            <BatteryCharging size={17} aria-hidden="true" />
            {t('coche.scheduleTitle')}
          </h2>
          <div className="flex flex-wrap items-center gap-3">
            <label className="flex cursor-pointer items-center gap-2 text-sm font-medium text-app">
              <input
                type="checkbox"
                className="h-4 w-4 accent-[rgb(var(--accent-rgb))]"
                checked={live.scheduleEnabledOn ?? false}
                onChange={(e) => act('schedule_enabled', e.target.checked)}
              />
              {t('coche.scheduleEnabled')}
            </label>
            <div className="flex items-center gap-2 text-sm text-app">
              <span className="text-[13px] text-muted">{t('coche.scheduleFrom')}</span>
              <input
                type="time"
                value={live.scheduleStart?.slice(0, 5) ?? ''}
                onChange={(e) => e.target.value && act('schedule_start', e.target.value)}
                className="rounded-lg border border-app bg-surface-2 px-2.5 py-1.5 text-sm tnum"
                aria-label={t('coche.scheduleFrom')}
              />
              <span className="text-[13px] text-muted">{t('coche.scheduleTo')}</span>
              <input
                type="time"
                value={live.scheduleEnd?.slice(0, 5) ?? ''}
                onChange={(e) => e.target.value && act('schedule_end', e.target.value)}
                className="rounded-lg border border-app bg-surface-2 px-2.5 py-1.5 text-sm tnum"
                aria-label={t('coche.scheduleTo')}
              />
            </div>
            <label className="flex cursor-pointer items-center gap-2 text-sm font-medium text-app">
              <input
                type="checkbox"
                className="h-4 w-4 accent-[rgb(var(--accent-rgb))]"
                checked={live.repeatDailyOn ?? false}
                onChange={(e) => act('repeat_daily', e.target.checked)}
              />
              {t('coche.repeatDaily')}
            </label>
          </div>
        </section>
      )}
    </div>
  );
}

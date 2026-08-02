import { createContext, useContext, useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { getTimes } from 'suncalc';
import type { ConnectionStatus, DayKpis, HistoryDay, LivePower, PowerPoint } from '@/data/types';
import { CO2_KG_PER_KWH, LIVE_STALE_MS, PRICE_EXPORT_EUR, PRICE_IMPORT_EUR, STEP_MIN, SUNRISE_MIN, SUNSET_MIN } from '@/data/types';
import { useEnergySettings } from '@/hooks/useEnergySettings';
import { apiFetch } from '@/data/api-client';

export interface EnergyDataApi {
  connectionStatus: ConnectionStatus;
  today: Date;
  nowMin: number;
  now: Date;
  liveTick: number;
  liveUpdatedAt: number | null;
  sunriseMin: number;
  sunsetMin: number;
  refresh: () => void;
  getLivePower: (atMin?: number, jitterSeed?: number) => LivePower;
  getDaySeries: (date?: Date) => PowerPoint[];
  isDayEstimated: (date?: Date) => boolean;
  getKpis: (date?: Date, untilMin?: number) => DayKpis;
  getHistory: (range?: { from?: Date; to?: Date }) => HistoryDay[];
}

const EnergyDataContext = createContext<EnergyDataApi | null>(null);

const EMPTY_LIVE: LivePower = {
  production: 0,
  consumption: 0,
  batteryPower: 0,
  soc: 0,
  grid: 0,
  solis: 0,
  fox: 0,
  at: 0,
};

const EMPTY_KPIS: DayKpis = {
  productionKwh: 0,
  consumptionKwh: 0,
  gridImportKwh: 0,
  gridExportKwh: 0,
  batteryChargedKwh: 0,
  batteryDischargedKwh: 0,
  soc: 0,
  autoconsumoPct: 0,
  autosuficienciaPct: 0,
  ahorroEur: 0,
  co2EvitadoKg: 0,
  peakProductionKw: 0,
  peakAt: 14 * 60,
};

function dateKey(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}-${m}-${day}`;
}

function integrateSeries(series: PowerPoint[], untilMin: number, socFallback: number): DayKpis {
  const dtH = STEP_MIN / 60;
  let productionKwh = 0;
  let consumptionKwh = 0;
  let gridImportKwh = 0;
  let gridExportKwh = 0;
  let batteryChargedKwh = 0;
  let batteryDischargedKwh = 0;
  let directFvKwh = 0;
  let peakProductionKw = 0;
  let peakAt = 14 * 60;
  let soc = socFallback;

  for (const p of series) {
    if (p.t >= untilMin) break;
    productionKwh += p.production * dtH;
    consumptionKwh += p.consumption * dtH;
    if (p.grid > 0) gridImportKwh += p.grid * dtH;
    else gridExportKwh += -p.grid * dtH;
    if (p.batteryPower > 0) batteryChargedKwh += p.batteryPower * dtH;
    if (p.batteryPower < 0) batteryDischargedKwh += -p.batteryPower * dtH;
    directFvKwh += Math.min(p.production, p.consumption) * dtH;
    if (p.production > peakProductionKw) {
      peakProductionKw = p.production;
      peakAt = p.t;
    }
    if (p.soc > 0) soc = p.soc;
  }

  const autoconsumoPct = productionKwh > 0 ? Math.min(100, ((productionKwh - gridExportKwh) / productionKwh) * 100) : 0;
  const autosuficienciaPct =
    consumptionKwh > 0 ? Math.min(100, ((consumptionKwh - gridImportKwh) / consumptionKwh) * 100) : 0;
  return {
    productionKwh,
    consumptionKwh,
    gridImportKwh,
    gridExportKwh,
    batteryChargedKwh,
    batteryDischargedKwh,
    soc,
    autoconsumoPct,
    autosuficienciaPct,
    ahorroEur: (directFvKwh + batteryDischargedKwh) * PRICE_IMPORT_EUR + gridExportKwh * PRICE_EXPORT_EUR,
    co2EvitadoKg: productionKwh * CO2_KG_PER_KWH,
    peakProductionKw,
    peakAt,
  };
}

export function EnergyDataProvider({ children }: { children: ReactNode }) {
  const [version, setVersion] = useState(0);
  const [now, setNow] = useState(() => new Date());
  const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>('reconnecting');
  const [settings] = useEnergySettings();

  const [liveData, setLiveData] = useState<LivePower | null>(null);
  const [liveUpdatedAt, setLiveUpdatedAt] = useState<number | null>(null);
  const lastMsgRef = useRef(0);
  const dayCache = useRef(new Map<string, PowerPoint[]>());
  const dayEstimated = useRef(new Map<string, boolean>());
  const kpisCache = useRef(new Map<string, DayKpis>());
  const historyRef = useRef<HistoryDay[]>([]);
  const pending = useRef(new Set<string>());
  const refreshRef = useRef<() => void>(() => {});

  const bump = () => setVersion((v) => v + 1);

  useEffect(() => {
    const clock = window.setInterval(() => {
      if (document.hidden) return; // pestaña oculta: no re-renderizamos en vivo
      setNow(new Date());
    }, 1000);
    return () => window.clearInterval(clock);
  }, []);

  useEffect(() => {
    let es: EventSource | null = null;
    let stopped = false;

    const connect = () => {
      if (stopped) return;
      es?.close();
      lastMsgRef.current = Date.now(); // ventana fresca para el watchdog
      try {
        es = new EventSource('/api/solar/stream');
        es.onopen = () => setConnectionStatus('connected');
        es.onmessage = (ev) => {
          try {
            const msg = JSON.parse(ev.data) as { type: string; data: LivePower & { connected?: boolean } };
            const ts = Date.now();
            lastMsgRef.current = ts;
            setLiveUpdatedAt(ts);
            if (msg.type === 'live') {
              setLiveData(msg.data);
              setConnectionStatus('connected');
              bump();
            }
          } catch {
            /* mensaje no parseable */
          }
        };
        es.onerror = () => setConnectionStatus('reconnecting');
      } catch {
        setConnectionStatus('reconnecting');
      }
    };

    connect();

    // Watchdog: el navegador puede matar el SSE en silencio (móvil suspendido,
    // proxy) sin disparar onerror — si no llega nada en LIVE_STALE_MS, recreamos.
    const watchdog = window.setInterval(() => {
      if (document.hidden) return;
      if (Date.now() - lastMsgRef.current > LIVE_STALE_MS) connect();
    }, 5000);

    const onVisible = () => {
      if (!document.hidden && Date.now() - lastMsgRef.current > LIVE_STALE_MS) connect();
    };
    document.addEventListener('visibilitychange', onVisible);

    return () => {
      stopped = true;
      window.clearInterval(watchdog);
      document.removeEventListener('visibilitychange', onVisible);
      es?.close();
    };
  }, []);

  useEffect(() => {
    const load = async (key: string, url: string, apply: (data: never) => void) => {
      if (pending.current.has(key)) return;
      pending.current.add(key);
      try {
        const data = await apiFetch(url);
        apply(data as never);
        bump();
      } catch {
        /* reintento en el próximo ciclo */
      } finally {
        pending.current.delete(key);
      }
    };

    const refresh = () => {
      const today = dateKey(new Date());
      void load(`day:${today}`, `/api/solar/day?date=${today}`, (res: { points: PowerPoint[]; estimated?: boolean }) => {
        dayCache.current.set(today, Array.isArray(res?.points) ? res.points : []);
        dayEstimated.current.set(today, !!res?.estimated);
      });
      void load(`kpis:${today}`, `/api/solar/kpis?date=${today}`, (k: DayKpis) => {
        kpisCache.current.set(today, k);
      });
      void load('history', '/api/solar/history', (res: { days: (Omit<HistoryDay, 'date'> & { date: string })[] }) => {
        historyRef.current = res.days.map((d) => ({ ...d, date: new Date(d.date + 'T00:00:00') }));
      });
    };

    refreshRef.current = refresh;
    refresh();
    const timer = window.setInterval(refresh, 5 * 60 * 1000);
    const onVisible = () => {
      if (!document.hidden) {
        setNow(new Date());
        refresh();
      }
    };
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener('visibilitychange', onVisible);
    };
  }, []);

  const value = useMemo<EnergyDataApi>(() => {
    void version;
    const todayStart = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const todayKey = dateKey(now);
    const nowMin = now.getHours() * 60 + now.getMinutes() + now.getSeconds() / 60;

    const ensureDay = (key: string) => {
      if (dayCache.current.has(key) || pending.current.has(`day:${key}`)) return;
      pending.current.add(`day:${key}`);
      apiFetch<{ points: PowerPoint[]; estimated?: boolean }>(`/api/solar/day?date=${key}`)
        .then((res) => {
          dayCache.current.set(key, res.points);
          dayEstimated.current.set(key, !!res.estimated);
          bump();
        })
        .catch(() => {})
        .finally(() => pending.current.delete(`day:${key}`));
    };

    const ensureKpis = (key: string) => {
      if (kpisCache.current.has(key) || pending.current.has(`kpis:${key}`)) return;
      pending.current.add(`kpis:${key}`);
      apiFetch<DayKpis>(`/api/solar/kpis?date=${key}`)
        .then((k) => {
          kpisCache.current.set(key, k);
          bump();
        })
        .catch(() => {})
        .finally(() => pending.current.delete(`kpis:${key}`));
    };

    const live = liveData ?? EMPTY_LIVE;
    const sunTimes =
      settings.locationLat !== null && settings.locationLon !== null
        ? getTimes(now, settings.locationLat, settings.locationLon)
        : null;
    const sunriseMin = (() => {
      if (sunTimes?.sunrise && !Number.isNaN(sunTimes.sunrise.getTime())) {
        return sunTimes.sunrise.getHours() * 60 + sunTimes.sunrise.getMinutes();
      }
      const nr = live.sun?.nextRising ? new Date(live.sun.nextRising) : null;
      if (nr && dateKey(nr) === todayKey) return nr.getHours() * 60 + nr.getMinutes();
      return SUNRISE_MIN;
    })();
    const sunsetMin = (() => {
      if (sunTimes?.sunset && !Number.isNaN(sunTimes.sunset.getTime())) {
        return sunTimes.sunset.getHours() * 60 + sunTimes.sunset.getMinutes();
      }
      const ns = live.sun?.nextSetting ? new Date(live.sun.nextSetting) : null;
      if (ns && dateKey(ns) === todayKey) return ns.getHours() * 60 + ns.getMinutes();
      return SUNSET_MIN;
    })();

    return {
      connectionStatus,
      today: todayStart,
      nowMin,
      now,
      liveTick: version,
      liveUpdatedAt,
      refresh: () => {
        dayCache.current.clear();
        kpisCache.current.clear();
        historyRef.current = [];
        refreshRef.current();
      },
      getLivePower: (atMin?: number) => {
        if (atMin !== undefined && atMin < nowMin - 2) {
          const series = dayCache.current.get(todayKey);
          if (series && series.length > 0) {
            const idx = Math.min(series.length - 1, Math.max(0, Math.round(atMin / STEP_MIN)));
            const p = series[Math.min(idx, series.length - 1)];
            return {
              production: p.production,
              consumption: p.consumption,
              batteryPower: p.batteryPower,
              soc: p.soc,
              grid: p.grid,
              solis: p.solis,
              fox: p.fox,
              at: p.t,
              sun: live.sun,
              weather: live.weather,
            };
          }
        }
        return { ...live, at: live.at || nowMin };
      },
      getDaySeries: (date?: Date) => {
        const key = date ? dateKey(date) : todayKey;
        ensureDay(key);
        const v = dayCache.current.get(key);
        return Array.isArray(v) ? v : [];
      },
      isDayEstimated: (date?: Date) => {
        const key = date ? dateKey(date) : todayKey;
        return dayEstimated.current.get(key) === true;
      },
      getKpis: (date?: Date, untilMin?: number) => {
        const key = date ? dateKey(date) : todayKey;
        if (untilMin !== undefined && key === todayKey && untilMin < nowMin - 2) {
          const series = dayCache.current.get(key);
          if (series && series.length > 0) return integrateSeries(series, untilMin, live.soc || 0);
        }
        ensureKpis(key);
        return kpisCache.current.get(key) ?? EMPTY_KPIS;
      },
      getHistory: (range?: { from?: Date; to?: Date }) => {
        const days = historyRef.current;
        if (!range?.from && !range?.to) return days;
        return days.filter((d) => (!range.from || d.date >= range.from) && (!range.to || d.date <= range.to));
      },
      sunriseMin,
      sunsetMin,
    };
  }, [version, now, connectionStatus, liveData, liveUpdatedAt, settings.locationLat, settings.locationLon]);

  return <EnergyDataContext.Provider value={value}>{children}</EnergyDataContext.Provider>;
}

export function useEnergyData(): EnergyDataApi {
  const ctx = useContext(EnergyDataContext);
  if (!ctx) throw new Error('useEnergyData debe usarse dentro de <EnergyDataProvider>');
  return ctx;
}

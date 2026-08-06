import { useMemo, useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { motion } from 'framer-motion';
import {
  Sun,
  House,
  BatteryCharging,
  Leaf,
  UtilityPole,
  ArrowUpFromLine,
  ArrowDownToLine,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useEnergyData } from '@/data/EnergyDataProvider';
import { STEP_MIN } from '@/data/types';
import { useEnergyColors } from '@/lib/colors';
import { fmtEnergy } from '@/lib/format';
import KpiCard from '@/components/KpiCard';
import LivePowerStrip from '@/components/LivePowerStrip';
import DayChart from '@/components/DayChart';
import EnergyFlowDiagram from '@/components/EnergyFlowDiagram';
import SolarArc from '@/components/SolarArc';
import InverterCard from '@/components/InverterCard';

function batteryState(bp: number): 'charging' | 'discharging' | 'idle' {
  if (bp > 0.05) return 'charging';
  if (bp < -0.05) return 'discharging';
  return 'idle';
}

export default function Dashboard() {
  const { nowMin, liveTick, today, sunriseMin, sunsetMin, getLivePower, getDaySeries, getKpis } =
    useEnergyData();
  const palette = useEnergyColors();
  const navigate = useNavigate();
  const { t } = useTranslation();
  const [replayMin, setReplayMin] = useState<number | null>(null);

  const effectiveMin = replayMin ?? nowMin;
  const live = getLivePower(effectiveMin, replayMin === null ? liveTick : 0);
  const kpis = getKpis(today, effectiveMin);
  const dayKpis = useMemo(() => getKpis(today), [getKpis, today]);
  const series = useMemo(() => getDaySeries(today), [getDaySeries, today]);

  const { solisKwh, foxKwh } = useMemo(() => {
    let s = 0;
    let f = 0;
    for (const p of series) {
      if (p.t >= effectiveMin) break;
      s += p.solis * (STEP_MIN / 60);
      f += p.fox * (STEP_MIN / 60);
    }
    return { solisKwh: s, foxKwh: f };
  }, [series, effectiveMin]);

  const totalInv = solisKwh + foxKwh;
  const solisSeries = useMemo(() => series.map((p) => p.solis), [series]);
  const foxSeries = useMemo(() => series.map((p) => p.fox), [series]);

  const socKwh = (live.soc / 100) * 5;

  return (
    <div className="flex flex-col gap-4 lg:gap-5">
      {/* Ahora mismo + Flujo + Prod/Consumo */}
      <div className="grid grid-cols-12 gap-4 lg:gap-5">
        <div className="col-span-12 order-1">
          <LivePowerStrip live={live} atMin={effectiveMin} />
        </div>

        <motion.section
          initial={{ opacity: 0, y: 24 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.2 }}
          transition={{ duration: 0.5, ease: [0.25, 1, 0.5, 1] }}
          className="helios-card col-span-12 order-2 p-5 shadow-card dark:shadow-card-dark lg:order-4 lg:col-span-4"
        >
          <div className="mb-2 flex items-center justify-between">
            <h2 className="text-[15px] font-semibold text-app">{t('dashboard.flowTitle')}</h2>
            <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/12 px-2 py-0.5 text-[10px] font-bold tracking-wide text-emerald-600 dark:text-emerald-400">
              <span className="relative flex h-1.5 w-1.5">
                <span className="absolute inline-flex h-full w-full animate-ping-soft rounded-full bg-emerald-500 opacity-60" />
                <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-emerald-500" />
              </span>
              {t('dashboard.live')}
            </span>
          </div>
          <EnergyFlowDiagram live={live} />
        </motion.section>

        <motion.div
          initial={{ opacity: 0, y: 24 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.5, delay: 0.25, ease: [0.25, 1, 0.5, 1] }}
          className="col-span-12 order-3 lg:order-2 lg:col-span-8"
          style={{ boxShadow: `0 8px 32px -8px ${palette.solar}2E`, borderRadius: 16 }}
        >
          <DayChart data={series} nowMin={nowMin} replayMin={replayMin} onReplayChange={setReplayMin} fill />
        </motion.div>
      </div>

      {/* ── KPIs del día (debajo del chart) ───────────────────────── */}
      <div
        className="flex snap-x snap-mandatory gap-4 overflow-x-auto no-scrollbar lg:grid lg:snap-none lg:grid-cols-3 lg:overflow-visible xl:grid-cols-5"
        aria-label={t('dashboard.kpisLabel')}
      >
        <div className="w-[42%] shrink-0 snap-center lg:w-auto">
          <KpiCard
            icon={Sun}
            color="solar"
            label={t('dashboard.productionToday')}
            value={kpis.productionKwh}
            unit="kWh"
            live
            index={0}
          />
        </div>
        <div className="w-[42%] shrink-0 snap-center lg:w-auto">
          <KpiCard
            icon={House}
            color="consumo"
            label={t('dashboard.consumptionToday')}
            value={kpis.consumptionKwh}
            unit="kWh"
            index={1}
          />
        </div>
        <div className="w-[42%] shrink-0 snap-center lg:w-auto">
          <KpiCard
            icon={BatteryCharging}
            color="bateria"
            label={t('common.battery')}
            value={socKwh}
            unit="kWh"
            decimals={1}
            onClick={() => navigate('/bateria')}
            index={2}
          >
            <p className="text-xs font-medium text-muted">
              {Math.round(live.soc)}% · {t(`common.${batteryState(live.batteryPower)}`)}
            </p>
          </KpiCard>
        </div>
        <div className="w-[42%] shrink-0 snap-center lg:w-auto">
          <KpiCard
            icon={Leaf}
            color="bateria"
            label={t('common.autoconsumo')}
            value={kpis.autoconsumoPct}
            unit="%"
            decimals={0}
            progress={kpis.autoconsumoPct}
            index={3}
          />
        </div>
        <div className="w-[42%] shrink-0 snap-center lg:w-auto">
          <KpiCard
            icon={UtilityPole}
            color="redVertido"
            label={t('dashboard.gridBalance')}
            value={kpis.gridExportKwh}
            unit="kWh"
            onClick={() => navigate('/historico')}
            index={4}
          >
            <div className="flex flex-col gap-0.5 text-[11px] font-medium">
              <span className="inline-flex items-center gap-1" style={{ color: palette.redVertido }}>
                <ArrowUpFromLine size={11} /> {fmtEnergy(kpis.gridExportKwh)} kWh {t('dashboard.exported')}
              </span>
              <span className="inline-flex items-center gap-1" style={{ color: palette.redCompra }}>
                <ArrowDownToLine size={11} /> {fmtEnergy(kpis.gridImportKwh)} kWh {t('dashboard.imported')}
              </span>
            </div>
          </KpiCard>
        </div>
      </div>

      {/* ── Sol de hoy + Inversores ────────────────────────────────── */}
      <section>
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-display text-lg font-semibold tracking-[-0.01em] text-app">{t('dashboard.yourInverters')}</h2>
          <Link to="/inversores" className="text-xs font-semibold text-amber-500 hover:underline">
            {t('dashboard.viewCompare')}
          </Link>
        </div>
        <div className="grid gap-4 lg:grid-cols-3 lg:gap-5">
          <motion.section
            initial={{ opacity: 0, y: 24 }}
            whileInView={{ opacity: 1, y: 0 }}
            viewport={{ once: true, amount: 0.2 }}
            transition={{ duration: 0.5, ease: [0.25, 1, 0.5, 1] }}
            className="helios-card p-5 shadow-card dark:shadow-card-dark"
          >
            <h2 className="mb-2 text-[15px] font-semibold text-app">{t('dashboard.sunToday')}</h2>
            <SolarArc
              atMin={effectiveMin}
              peakKw={dayKpis.peakProductionKw}
              peakAtMin={dayKpis.peakAt}
              sunriseMin={sunriseMin}
              sunsetMin={sunsetMin}
              weather={live.weather}
              weatherTemp={live.weatherTemp}
            />
          </motion.section>
          <InverterCard
            name={`Solis · ${t('common.hybrid')}`}
            model="4,4 kWp · 10 × 440 W"
            kwp={4.4}
            nowKw={live.solis}
            todayKwh={solisKwh}
            series={solisSeries}
            sharePct={totalInv > 0 ? (solisKwh / totalInv) * 100 : 0}
            color={palette.solis}
            tab="solis"
            index={1}
          />
          <InverterCard
            name="Fox · FoxESS"
            model="2,7 kWp · 6 × 450 W"
            kwp={2.7}
            nowKw={live.fox}
            todayKwh={foxKwh}
            series={foxSeries}
            sharePct={totalInv > 0 ? (foxKwh / totalInv) * 100 : 0}
            color={palette.fox}
            tab="fox"
            index={2}
          />
        </div>
      </section>
    </div>
  );
}

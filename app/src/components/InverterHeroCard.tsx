import { motion } from 'framer-motion';
import { BatteryCharging, Sun, ThermometerSun, Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { fmtKw } from '@/lib/format';
import { useAnimatedNumber } from '@/lib/useAnimatedNumber';
import { cn } from '@/lib/utils';

export interface InverterMeta {
  key: 'solis' | 'fox';
  name: string;
  model: string;
  kwp: number;
  panels: string;
  monthKwh: number;
  totalMwh: number;
  tempC: number;
  hasBattery: boolean;
  batteryKwh?: number;
}

interface InverterHeroCardProps {
  meta: InverterMeta;
  /** kW actuales del inversor. */
  nowKw: number;
  color: string;
}

/** §2 HeroCard del inversor: acento de color, cifra héroe y mini-lista. */
export default function InverterHeroCard({ meta, nowKw, color }: InverterHeroCardProps) {
  const animatedNow = useAnimatedNumber(nowKw, 0.9);
  const { t } = useTranslation();

  return (
    <motion.section
      initial={{ opacity: 0, x: -24 }}
      animate={{ opacity: 1, x: 0 }}
      transition={{ duration: 0.5, ease: [0.25, 1, 0.5, 1] }}
      className="helios-card h-full border-t-[3px] p-5 shadow-card hover:-translate-y-0.5 hover:shadow-lg dark:shadow-card-dark"
      style={{ borderTopColor: color }}
      aria-label={t('inverter.aria', { name: meta.name })}
    >
      <div className="flex items-start gap-3">
        <span className="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl" style={{ backgroundColor: `${color}1F`, color }}>
          <Zap size={24} strokeWidth={2.2} />
        </span>
        <div className="min-w-0 flex-1">
          <p className="font-display text-[22px] font-semibold leading-tight tracking-[-0.01em] text-app">{meta.name}</p>
          <p className="truncate text-[13px] text-muted">{meta.model}</p>
        </div>
        <div className="flex flex-col items-end gap-1.5">
          <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/12 px-2.5 py-1 text-[11px] font-semibold text-emerald-600 dark:text-emerald-400">
            <span className="relative flex h-1.5 w-1.5">
              <span className="absolute inline-flex h-full w-full animate-ping-soft rounded-full bg-emerald-500 opacity-60" />
              <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-emerald-500" />
            </span>
            {t('common.online')}
          </span>
          {meta.hasBattery && meta.batteryKwh != null && (
            <span className="inline-flex items-center gap-1.5 text-[12px] font-medium text-muted">
              <BatteryCharging size={13} className="shrink-0 text-faint" />
              {meta.batteryKwh} <span className="text-faint">kWh</span>
            </span>
          )}
          <span
            className={cn(
              'inline-flex items-center gap-1.5 text-[12px] font-medium',
              meta.tempC > 60 ? 'text-amber-600 dark:text-amber-400' : 'text-muted',
            )}
          >
            <ThermometerSun size={13} className="shrink-0 text-faint" />
            {meta.tempC} <span className="text-faint">°C</span>
          </span>
        </div>
      </div>

      <div className="mt-5">
        <p className="font-display text-[48px] font-semibold leading-none tracking-[-0.02em]" style={{ color }} aria-live="off">
          {fmtKw(animatedNow)}
          <span className="ml-1.5 text-[0.42em] font-medium text-faint">kW</span>
        </p>
        <p className="mt-1 text-sm text-muted">{t('inverter.producingNow')}</p>
      </div>

      <ul className="mt-5 flex flex-col gap-2.5 border-t border-app pt-4 text-sm">
        <li className="flex items-center gap-2.5 text-muted">
          <Sun size={15} className="shrink-0 text-faint" />
          {t('inverter.solarField')} <span className="ml-auto font-medium text-app">{meta.panels}</span>
        </li>
        {meta.hasBattery && (
          <li className="flex items-center gap-2.5 text-muted">
            <BatteryCharging size={15} className="shrink-0 text-faint" />
            {t('common.battery')} <span className="ml-auto font-medium text-app">{t('inverter.batteryCoupled')}</span>
          </li>
        )}
      </ul>
    </motion.section>
  );
}

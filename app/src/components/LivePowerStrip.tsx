import { motion } from 'framer-motion';
import { SolarPanel, House, ArrowUpFromLine, ArrowDownToLine } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { LucideIcon } from 'lucide-react';
import type { LivePower } from '@/data/types';
import { useEnergyColors } from '@/lib/colors';
import { batteryIcon } from '@/lib/battery';
import { fmtKw, fmtTime } from '@/lib/format';
import { useAnimatedNumber } from '@/lib/useAnimatedNumber';
import { cn } from '@/lib/utils';

interface StripValueProps {
  icon: LucideIcon;
  label: string;
  kw: number;
  status?: string;
  color: string;
  /** Color del icono; si se omite, usa `color` (icono y celda iguales). */
  iconColor?: string;
  className?: string;
  /** Sustituye la cifra grande por este valor y unidad (p.ej. batería: 20% en vez de kW). */
  altValue?: string;
  altUnit?: string;
}

function StripValue({ icon: Icon, label, kw, status, color, iconColor, className, altValue, altUnit }: StripValueProps) {
  const animated = useAnimatedNumber(altValue === undefined ? kw : NaN);
  return (
    <div className={cn('flex items-center gap-3 px-4 py-3 sm:px-5', className)}>
      <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl" style={{ backgroundColor: `${color}1F`, color: iconColor ?? color }}>
        <Icon size={18} strokeWidth={2.2} />
      </span>
      <div className="min-w-0">
        <p className="text-[11px] font-medium uppercase tracking-[0.06em] text-faint">{label}</p>
        <p className="font-display text-[22px] font-semibold leading-tight text-app" aria-live="off">
          {altValue !== undefined ? altValue : fmtKw(animated)}
          {altUnit !== undefined ? (
            <span className="ml-1 text-[0.6em] font-medium text-faint">{altUnit}</span>
          ) : (
            <span className="ml-1 text-[0.6em] font-medium text-faint">kW</span>
          )}
        </p>
        {status && (
          <p className="text-[11px] font-medium" style={{ color }}>
            {status}
          </p>
        )}
      </div>
    </div>
  );
}

interface LivePowerStripProps {
  live: LivePower;
  /** Hora de la lectura mostrada (minutos). */
  atMin: number;
}

/** Banda "Ahora mismo" con los 4 valores instantáneos. */
export default function LivePowerStrip({ live, atMin }: LivePowerStripProps) {
  const palette = useEnergyColors();
  const { t } = useTranslation();
  const exporting = live.grid < -0.05;

  const batteryLabel = (bp: number): string => {
    if (bp > 0.05) return t('common.charging');
    if (bp < -0.05) return t('common.discharging');
    return t('common.idle');
  };
  const gridLabel = (grid: number): string => {
    if (grid > 0.05) return t('live.buying');
    if (grid < -0.05) return t('live.exporting');
    return t('live.balanced');
  };

  return (
    <motion.section
      initial={{ opacity: 0, y: 24 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.5, delay: 0.15, ease: [0.25, 1, 0.5, 1] }}
      className="helios-card shadow-card dark:shadow-card-dark"
      aria-label={t('live.aria')}
    >
      <div className="flex items-center gap-2 border-b border-app px-4 py-3 sm:px-5">
        <span className="relative flex h-2 w-2">
          <span className="absolute inline-flex h-full w-full animate-ping-soft rounded-full bg-emerald-500 opacity-60" />
          <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
        </span>
        <h2 className="text-[15px] font-semibold text-app">{t('live.title')}</h2>
        <span className="text-xs text-faint">· {t('live.lastReading', { time: fmtTime(atMin) })}</span>
      </div>

      <div className="grid grid-cols-2 lg:grid-cols-4">
        <StripValue icon={SolarPanel} label={t('common.production')} kw={live.production} color={palette.solar} iconColor={live.production > 0.1 ? palette.solar : 'var(--text-faint)'} />
        <StripValue icon={House} label={t('common.consumption')} kw={live.consumption} color={palette.consumo} className="border-l border-app" />
        <StripValue
          icon={batteryIcon(live.soc, live.batteryPower > 0.05)}
          label={t('common.battery')}
          kw={Math.abs(live.batteryPower)}
          status={batteryLabel(live.batteryPower)}
          color={palette.bateria}
          altValue={`${Math.round(live.soc)}`}
          altUnit="%"
          className="border-t border-app lg:border-l lg:border-t-0"
        />
        <StripValue
          icon={exporting ? ArrowUpFromLine : ArrowDownToLine}
          label={t('common.grid')}
          kw={Math.abs(live.grid)}
          status={gridLabel(live.grid)}
          color={exporting ? palette.redVertido : palette.redCompra}
          className="border-l border-t border-app lg:border-t-0"
        />
      </div>
    </motion.section>
  );
}

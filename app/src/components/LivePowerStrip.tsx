import { motion } from 'framer-motion';
import { Sun, House, BatteryCharging, ArrowUpFromLine, ArrowDownToLine } from 'lucide-react';
import type { LivePower } from '@/data/types';
import { useEnergyColors } from '@/lib/colors';
import { fmtKw, fmtTime } from '@/lib/format';
import { useAnimatedNumber } from '@/lib/useAnimatedNumber';
import { cn } from '@/lib/utils';

function batteryLabel(bp: number): string {
  if (bp > 0.05) return 'Cargando';
  if (bp < -0.05) return 'Descargando';
  return 'En reposo';
}

function gridLabel(grid: number): string {
  if (grid > 0.05) return 'Comprando';
  if (grid < -0.05) return 'Vertiendo';
  return 'En equilibrio';
}

interface StripValueProps {
  icon: typeof Sun;
  label: string;
  kw: number;
  status?: string;
  color: string;
  className?: string;
  /** Sustituye la cifra grande por este valor y unidad (p.ej. batería: 20% en vez de kW). */
  altValue?: string;
  altUnit?: string;
}

function StripValue({ icon: Icon, label, kw, status, color, className, altValue, altUnit }: StripValueProps) {
  const animated = useAnimatedNumber(altValue === undefined ? kw : NaN);
  return (
    <div className={cn('flex items-center gap-3 px-4 py-3 sm:px-5', className)}>
      <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl" style={{ backgroundColor: `${color}1F`, color }}>
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
  const exporting = live.grid < -0.05;

  return (
    <motion.section
      initial={{ opacity: 0, y: 24 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.5, delay: 0.15, ease: [0.25, 1, 0.5, 1] }}
      className="helios-card shadow-card dark:shadow-card-dark"
      aria-label="Potencia en tiempo real"
    >
      <div className="flex items-center gap-2 border-b border-app px-4 py-3 sm:px-5">
        <span className="relative flex h-2 w-2">
          <span className="absolute inline-flex h-full w-full animate-ping-soft rounded-full bg-emerald-500 opacity-60" />
          <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
        </span>
        <h2 className="text-[15px] font-semibold text-app">Ahora mismo</h2>
        <span className="text-xs text-faint">· última lectura {fmtTime(atMin)}</span>
      </div>

      <div className="grid grid-cols-2 lg:grid-cols-4">
        <StripValue icon={Sun} label="Producción" kw={live.production} color={palette.solar} />
        <StripValue icon={House} label="Consumo" kw={live.consumption} color={palette.consumo} className="border-l border-app" />
        <StripValue
          icon={BatteryCharging}
          label="Batería"
          kw={Math.abs(live.batteryPower)}
          status={batteryLabel(live.batteryPower)}
          color={palette.bateria}
          altValue={`${Math.round(live.soc)}`}
          altUnit="%"
          className="border-t border-app lg:border-l lg:border-t-0"
        />
        <StripValue
          icon={exporting ? ArrowUpFromLine : ArrowDownToLine}
          label="Red"
          kw={Math.abs(live.grid)}
          status={gridLabel(live.grid)}
          color={exporting ? palette.redVertido : palette.redCompra}
          className="border-l border-t border-app lg:border-t-0"
        />
      </div>
    </motion.section>
  );
}

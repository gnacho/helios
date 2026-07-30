import type { ReactNode } from 'react';
import { motion } from 'framer-motion';
import type { LucideIcon } from 'lucide-react';
import { ArrowDown, ArrowUp } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useEnergyColors } from '@/lib/colors';
import type { EnergyColorKey } from '@/lib/colors';
import { useAnimatedNumber } from '@/lib/useAnimatedNumber';

export interface KpiDelta {
  direction: 'up' | 'down';
  text: string;
  /** Verde si la variación es favorable semánticamente. */
  good: boolean;
}

interface KpiCardProps {
  icon: LucideIcon;
  color: EnergyColorKey;
  label: string;
  value: number;
  unit?: string;
  decimals?: number;
  live?: boolean;
  delta?: KpiDelta;
  /** Barra de progreso fina (3px) bajo la cifra, 0–100. */
  progress?: number;
  /** Líneas extra bajo la cifra (p. ej. balance de red). */
  children?: ReactNode;
  onClick?: () => void;
  /** Índice para stagger de entrada (70ms). */
  index?: number;
}

export default function KpiCard({
  icon: Icon,
  color,
  label,
  value,
  unit,
  decimals = 1,
  live = false,
  delta,
  progress,
  children,
  onClick,
  index = 0,
}: KpiCardProps) {
  const palette = useEnergyColors();
  const accent = palette[color];
  const animated = useAnimatedNumber(value);
  const formatted = new Intl.NumberFormat('es-ES', {
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals,
  }).format(animated);

  const Tag = onClick ? 'button' : 'div';

  return (
    <motion.div
      initial={{ opacity: 0, y: 24 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.5, delay: index * 0.07, ease: [0.25, 1, 0.5, 1] }}
      className="h-full"
    >
      <Tag
        onClick={onClick}
        className={cn(
          'helios-card flex h-full w-full flex-col gap-2 p-3 text-left shadow-card dark:shadow-card-dark sm:p-4',
          'hover:-translate-y-0.5 hover:shadow-lg active:scale-[0.98]',
          onClick && 'cursor-pointer',
        )}
      >
        <div className="flex items-center justify-between">
            <span
              className="flex h-8 w-8 items-center justify-center rounded-xl"
              style={{ backgroundColor: `${accent}1F`, color: accent }}
            >
              <Icon size={17} strokeWidth={2.2} />
            </span>
          {live && (
            <span className="relative flex h-2 w-2" aria-label="En vivo">
              <span className="absolute inline-flex h-full w-full animate-ping-soft rounded-full bg-emerald-500 opacity-60" />
              <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
            </span>
          )}
        </div>

        <div>
          <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{label}</p>
          <p className="mt-0.5 font-display text-[22px] font-semibold leading-none tracking-[-0.01em] text-app" aria-live="off">
            {formatted}
            {unit && <span className="ml-1 text-[0.6em] font-medium text-faint">{unit}</span>}
          </p>
        </div>

        {progress !== undefined && (
          <div className="h-[3px] w-full overflow-hidden rounded-full bg-surface-2" aria-hidden>
            <motion.div
              className="h-full rounded-full"
              style={{ backgroundColor: accent }}
              initial={{ width: 0 }}
              animate={{ width: `${Math.min(100, Math.max(0, progress))}%` }}
              transition={{ duration: 0.8, ease: 'easeOut', delay: 0.3 }}
            />
          </div>
        )}

        {delta && (
          <span
            className={cn(
              'inline-flex w-fit items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-semibold',
              delta.good ? 'bg-emerald-500/12 text-emerald-600 dark:text-emerald-400' : 'bg-rose-500/12 text-rose-600 dark:text-rose-400',
            )}
          >
            {delta.direction === 'up' ? <ArrowUp size={12} /> : <ArrowDown size={12} />}
            {delta.text}
          </span>
        )}

        {children}
      </Tag>
    </motion.div>
  );
}

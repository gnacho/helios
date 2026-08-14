import { AnimatePresence, motion } from 'framer-motion';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { Period } from '@/lib/historyStats';
import { cn } from '@/lib/utils';

const DEFAULT_PERIODS: Period[] = ['dia', 'semana', 'mes', 'ano'];

interface PeriodSelectorProps {
  period: Period;
  onPeriodChange: (p: Period) => void;
  /** Subconjunto de periodos a ofrecer (por defecto, los cuatro). */
  periods?: Period[];
  /** Etiqueta corta del periodo visible: `15 jul` · `8 – 14 jul` · `julio 2025` · `2025`. */
  label: string;
  canNext: boolean;
  isCurrent: boolean;
  onPrev: () => void;
  onNext: () => void;
  onToday: () => void;
}

/**
 * PeriodSelector: segmented `Día / Semana / Mes / Año` (36px) + navegador de
 * fecha con chevrons y chip `Hoy` (7.11 de design.md).
 */
export default function PeriodSelector({
  period,
  onPeriodChange,
  periods = DEFAULT_PERIODS,
  label,
  canNext,
  isCurrent,
  onPrev,
  onNext,
  onToday,
}: PeriodSelectorProps) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-2 sm:gap-3">
      {/* Segmented control */}
      <div
        role="tablist"
        aria-label={t('historico.periodAria')}
        className="flex h-9 items-center rounded-full border border-app bg-surface p-0.5"
      >
        {periods.map((p) => {
          const active = p === period;
          return (
            <button
              key={p}
              role="tab"
              aria-selected={active}
              onClick={() => onPeriodChange(p)}
              className={cn(
                'relative flex h-8 items-center rounded-full px-3 text-[13px] font-medium transition-colors sm:px-3.5',
                active ? 'text-app' : 'text-faint hover:text-muted',
              )}
            >
              {active && (
                <motion.span
                  layoutId="period-seg-pill"
                  transition={{ type: 'spring', stiffness: 400, damping: 32 }}
                  className="absolute inset-0 rounded-full bg-surface-2 shadow-inner"
                />
              )}
              <span className="relative">{t(`historico.periods.${p}`)}</span>
            </button>
          );
        })}
      </div>

      {/* Navegador de fecha */}
      <div className="flex h-9 items-center gap-0.5 rounded-full border border-app bg-surface p-0.5">
        <button
          onClick={onPrev}
          aria-label={t('historico.prevAria')}
          className="flex h-8 w-8 items-center justify-center rounded-full text-muted transition-colors hover:bg-surface-2 hover:text-app active:scale-95"
        >
          <ChevronLeft size={16} />
        </button>
        <div className="relative flex h-8 min-w-[92px] items-center justify-center overflow-hidden px-1 sm:min-w-[110px]">
          <AnimatePresence mode="wait" initial={false}>
            <motion.span
              key={label}
              initial={{ y: 12, opacity: 0 }}
              animate={{ y: 0, opacity: 1 }}
              exit={{ y: -12, opacity: 0 }}
              transition={{ duration: 0.2, ease: 'easeOut' }}
              className="text-[13px] font-semibold tabular-nums text-app"
            >
              {label}
            </motion.span>
          </AnimatePresence>
        </div>
        <button
          onClick={onNext}
          disabled={!canNext}
          aria-label={t('historico.nextAria')}
          className={cn(
            'flex h-8 w-8 items-center justify-center rounded-full transition-colors',
            canNext ? 'text-muted hover:bg-surface-2 hover:text-app active:scale-95' : 'cursor-not-allowed text-faint/50',
          )}
        >
          <ChevronRight size={16} />
        </button>
      </div>

      {/* Chip Hoy */}
      <button
        onClick={onToday}
        disabled={isCurrent}
        className={cn(
          'h-9 rounded-full border px-3.5 text-[13px] font-semibold transition-all',
          isCurrent
            ? 'cursor-not-allowed border-app text-faint/60'
            : 'border-amber-500/40 text-amber-600 hover:bg-amber-500/10 active:scale-95 dark:text-amber-400',
        )}
      >
        {t('common.today')}
      </button>
    </div>
  );
}

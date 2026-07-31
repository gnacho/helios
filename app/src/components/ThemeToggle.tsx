import { motion, AnimatePresence } from 'framer-motion';
import { Sunrise, Sun, Moon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useTheme } from '@/theme/ThemeProvider';
import type { ThemeMode } from '@/theme/ThemeProvider';
import { cn } from '@/lib/utils';

const OPTIONS: { value: ThemeMode; key: string; icon: typeof Sun }[] = [
  { value: 'auto', key: 'theme.auto', icon: Sunrise },
  { value: 'light', key: 'theme.light', icon: Sun },
  { value: 'dark', key: 'theme.dark', icon: Moon },
];

/** Segmented control Auto / Claro / Oscuro (pill de 32px). */
export default function ThemeToggle() {
  const { mode, setMode, effective } = useTheme();
  const { t } = useTranslation();

  return (
    <div
      role="radiogroup"
      aria-label={t('theme.label')}
      className="flex h-8 items-center rounded-full border border-app bg-surface p-0.5"
    >
      {OPTIONS.map(({ value, key, icon: Icon }) => {
        const active = mode === value;
        const label = t(key);
        return (
          <button
            key={value}
            role="radio"
            aria-checked={active}
            aria-label={`${t('theme.label')} ${label}`}
            title={`${t('theme.label')} ${label}`}
            onClick={() => setMode(value)}
            className={cn(
              'relative flex h-7 items-center gap-1 rounded-full px-2.5 text-xs font-medium transition-colors',
              active ? 'bg-surface-2 text-app shadow-inner' : 'text-faint hover:text-muted',
            )}
          >
            <AnimatePresence mode="wait" initial={false}>
              <motion.span
                key={active ? `${value}-on` : `${value}-off`}
                initial={{ rotate: -90, opacity: 0 }}
                animate={{ rotate: 0, opacity: 1 }}
                exit={{ rotate: 90, opacity: 0 }}
                transition={{ duration: 0.3 }}
                className="flex"
              >
                <Icon size={15} strokeWidth={2.2} />
              </motion.span>
            </AnimatePresence>
            <span className="hidden sm:inline">{label}</span>
            {value === 'auto' && active && (
              <span
                className={cn(
                  'ml-0.5 inline-block h-1.5 w-1.5 rounded-full',
                  effective === 'dark' ? 'bg-indigo-400' : 'bg-amber-500',
                )}
                aria-label={effective === 'dark' ? t('theme.darkActive') : t('theme.lightActive')}
              />
            )}
          </button>
        );
      })}
    </div>
  );
}

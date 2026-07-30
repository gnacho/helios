import { useCallback, useEffect, useRef, useState } from 'react';
import { AnimatePresence, motion } from 'framer-motion';
import { CheckCircle2, Info, TriangleAlert, X } from 'lucide-react';
import { registerToastPush } from '@/lib/toast';
import type { ToastTone } from '@/lib/toast';
import { cn } from '@/lib/utils';

/**
 * Toasts de Helios (esquina inferior derecha; centrado abajo en móvil, por
 * encima del bottom nav). Auto-cierre 3,5 s con barra de progreso.
 * Uso: `heliosToast('Preferencias guardadas', { tone: 'success' })` (de '@/lib/toast').
 */

interface ToastItem {
  id: number;
  message: string;
  tone: ToastTone;
}

const TONE_STYLES: Record<ToastTone, { icon: typeof Info; bar: string; text: string }> = {
  success: { icon: CheckCircle2, bar: 'bg-emerald-500', text: 'text-emerald-500' },
  warning: { icon: TriangleAlert, bar: 'bg-amber-500', text: 'text-amber-500' },
  info: { icon: Info, bar: 'bg-sky-500', text: 'text-sky-500' },
};

const DURATION_MS = 3500;

export default function HeliosToaster() {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const idRef = useRef(0);

  const dismiss = useCallback((id: number) => {
    setToasts((ts) => ts.filter((t) => t.id !== id));
  }, []);

  useEffect(() => {
    registerToastPush((message, opts) => {
      const id = ++idRef.current;
      setToasts((ts) => [...ts.slice(-2), { id, message, tone: opts?.tone ?? 'info' }]);
    });
    return () => registerToastPush(null);
  }, []);

  return (
    <div
      aria-live="polite"
      className={cn(
        'pointer-events-none fixed z-[90] flex w-full max-w-sm flex-col items-center gap-2 px-4',
        'bottom-[calc(5rem+env(safe-area-inset-bottom))] left-1/2 -translate-x-1/2',
        'sm:left-auto sm:right-4 sm:translate-x-0 sm:items-end sm:px-0 lg:bottom-6',
      )}
    >
      <AnimatePresence>
        {toasts.map((t) => {
          const tone = TONE_STYLES[t.tone];
          const Icon = tone.icon;
          return (
            <motion.div
              key={t.id}
              layout="position"
              initial={{ opacity: 0, y: 16, scale: 0.96 }}
              animate={{ opacity: 1, y: 0, scale: 1 }}
              exit={{ opacity: 0, y: 8, scale: 0.96 }}
              transition={{ type: 'spring', stiffness: 420, damping: 30 }}
              className="pointer-events-auto relative w-full overflow-hidden rounded-xl border border-app bg-surface shadow-lg"
              role="status"
            >
              <div className="flex items-center gap-2.5 px-3.5 py-3">
                <Icon size={17} className={cn('shrink-0', tone.text)} strokeWidth={2.2} />
                <p className="flex-1 text-[13px] font-medium leading-snug text-app">{t.message}</p>
                <button
                  onClick={() => dismiss(t.id)}
                  aria-label="Cerrar aviso"
                  className="shrink-0 rounded-full p-1 text-faint transition-colors hover:bg-surface-2 hover:text-muted"
                >
                  <X size={13} />
                </button>
              </div>
              {/* Barra de progreso de cierre */}
              <motion.div
                className={cn('h-[3px] w-full origin-left', tone.bar)}
                initial={{ scaleX: 1 }}
                animate={{ scaleX: 0 }}
                transition={{ duration: DURATION_MS / 1000, ease: 'linear' }}
                onAnimationComplete={() => dismiss(t.id)}
              />
            </motion.div>
          );
        })}
      </AnimatePresence>
    </div>
  );
}

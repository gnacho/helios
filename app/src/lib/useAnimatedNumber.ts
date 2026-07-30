import { useEffect, useRef, useState } from 'react';
import { animate } from 'framer-motion';

export function prefersReducedMotion(): boolean {
  if (typeof window === 'undefined' || !window.matchMedia) return false;
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/**
 * Count-up animado: 800ms al montar (desde 0) y tween de 400ms al actualizarse.
 * Con prefers-reduced-motion devuelve el valor directo.
 */
export function useAnimatedNumber(value: number, mountDuration = 0.8): number {
  const [display, setDisplay] = useState(0);
  const prev = useRef(0);
  const mounted = useRef(false);
  const reduced = prefersReducedMotion();

  useEffect(() => {
    if (reduced) {
      prev.current = value;
      return;
    }
    const from = mounted.current ? prev.current : 0;
    const duration = mounted.current ? 0.4 : mountDuration;
    mounted.current = true;
    const controls = animate(from, value, {
      duration,
      ease: 'easeOut',
      onUpdate: (v) => setDisplay(v),
    });
    prev.current = value;
    return () => controls.stop();
  }, [value, mountDuration, reduced]);

  return reduced ? value : display;
}

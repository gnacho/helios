import { useCallback, useEffect, useRef } from 'react';

/** Distancia mínima (px) para que un gesto cuente como deslizamiento. */
const MIN_DX = 12;
/** Fracción del ancho que hay que arrastrar para que el gesto cambie de pestaña. */
const SNAP_FRACTION = 0.22;
/** Resistencia en los extremos de la barra (0 = no se mueve, 1 = sigue al dedo). */
const EDGE_DAMPING = 0.35;

interface Options {
  /** Clave de la pestaña activa. */
  tab: string;
  /** Claves en el orden de la barra. */
  keys: string[];
  /** Cambia de pestaña (normalmente actualiza el parámetro de la URL). */
  onChange: (key: string) => void;
  /** Desactiva el gesto (p. ej. con una sola pestaña). */
  enabled?: boolean;
  /**
   * Desfase (px) que se pide al navegador para la vista vecina mientras se
   * arrastra. El padre coloca las vistas pegadas y aplica este desplazamiento
   * al track para que el vecino asome. Por defecto, un 12 % del ancho.
   */
  peek?: number;
}

/**
 * Deslizar horizontalmente para cambiar de pestaña en móvil, como un paginador
 * nativo: el contenido se desplaza con el dedo y al soltar encaja en la
 * pestaña vecina. Deslizar a la izquierda avanza, a la derecha retrocede.
 *
 * Durante el arrastre avisa por `peekRef` de cuánto hay que enseñar de la vista
 * vecina (de -1 a 1), que es lo que da la previsualización del siguiente
 * elemento sin montarlo antes de tiempo.
 *
 * En los extremos hay resistencia elástica: el contenido se mueve amortiguado
 * pero no hay cambio de pestaña.
 *
 * Los listeners van en `window` en fase de captura y con `passive: false` para
 * poder cancelar el desplazamiento del navegador cuando el gesto es
 * horizontal. Los gestos verticales se ignoran (los maneja el navegador), así
 * que el scroll y el pull-to-refresh siguen funcionando. En escritorio no hace
 * nada.
 */
export function useTabSwipe({ tab, keys, onChange, enabled = true, peek: peekPx }: Options) {
  const keysRef = useRef(keys);
  const tabRef = useRef(tab);
  const onChangeRef = useRef(onChange);
  const enabledRef = useRef(enabled);

  const contentRef = useRef<HTMLDivElement | null>(null);

  /**
   * Ref compartida con el padre: 0 cuando no hay arrastre; desplazamiento en px
   * pedido para asomar la vista vecina (negativo = vecina anterior) y su signo.
   */
  const peekRef = useRef({ x: 0, dir: 0 as -1 | 0 | 1 });
  const peekPxRef = useRef<number | undefined>(peekPx);

  // Las refs se sincronizan en un efecto, nunca durante el render.
  useEffect(() => {
    keysRef.current = keys;
    tabRef.current = tab;
    onChangeRef.current = onChange;
    enabledRef.current = enabled;
    peekPxRef.current = peekPx;
  }, [keys, tab, onChange, enabled, peekPx]);

  const restore = useCallback((animated: boolean) => {
    const el = contentRef.current;
    if (!el) return;
    el.style.transition = animated
      ? 'transform 220ms cubic-bezier(0.22, 1, 0.36, 1)'
      : 'none';
    el.style.transform = '';
    peekRef.current = { x: 0, dir: 0 };
  }, []);

  // Al cambiar de pestaña, por la vía que sea (swipe, toque, enlace), el
  // contenido vuelve a su sitio antes de que entre la nueva vista.
  useEffect(() => {
    restore(true);
  }, [tab, restore]);

  useEffect(() => {
    // Solo en vista móvil: en escritorio las pestañas se pulsan.
    const mq = window.matchMedia('(min-width: 1024px)');
    let active = false;
    let startX = 0;
    let startY = 0;
    let tracking = false;
    let horizontal = false;

    const onStart = (e: TouchEvent) => {
      active = !mq.matches && enabledRef.current && keysRef.current.length > 1;
      tracking = false;
      horizontal = false;
      if (!active || e.touches.length !== 1) return;
      const target = e.target as Element | null;
      // Zonas con su propio arrastre horizontal, o fuera del flujo de la página.
      if (target?.closest('nav,[role="dialog"],[data-no-swipe]')) return;
      if (!contentRef.current) return;
      startX = e.touches[0].clientX;
      startY = e.touches[0].clientY;
      tracking = true;
    };

    const onMove = (e: TouchEvent) => {
      if (!active || !tracking) return;
      const t = e.touches[0];
      if (!t) return;
      const dx = t.clientX - startX;
      const dy = t.clientY - startY;
      if (!horizontal) {
        // Se decide una sola vez: umbral mínimo y claramente más horizontal
        // que vertical. Si no, el gesto es del navegador (scroll).
        if (Math.abs(dx) < MIN_DX) return;
        if (Math.abs(dx) <= Math.abs(dy) * 1.2) {
          tracking = false;
          return;
        }
        horizontal = true;
      }
      const el = contentRef.current;
      if (!el) return;
      if (e.cancelable) e.preventDefault();
      const i = keysRef.current.indexOf(tabRef.current);
      const hasPrev = i > 0;
      const hasNext = i >= 0 && i < keysRef.current.length - 1;
      // En los extremos no hay vecino: solo resistencia elástica, sin asomar.
      const beyond = (dx > 0 && !hasPrev) || (dx < 0 && !hasNext);
      const offset = beyond ? dx * EDGE_DAMPING : dx;
      el.style.transition = 'none';
      el.style.transform = `translate3d(${offset}px, 0, 0)`;
      const dir: -1 | 0 | 1 = beyond || offset === 0 ? 0 : offset > 0 ? -1 : 1;
      peekRef.current = {
        x: dir === 0 ? 0 : Math.min(peekPxRef.current ?? Math.abs(offset) * 0.4, Math.abs(offset)) * dir,
        dir,
      };
    };

    const onEnd = () => {
      if (!active || !tracking || !horizontal) {
        tracking = false;
        horizontal = false;
        return;
      }
      tracking = false;
      horizontal = false;
      const el = contentRef.current;
      const width = el?.getBoundingClientRect().width || window.innerWidth;
      const match = el ? /translate3d\((-?[\d.]+)px/.exec(el.style.transform) : null;
      const dx = match ? parseFloat(match[1]) : 0;
      const i = keysRef.current.indexOf(tabRef.current);
      const next = keysRef.current[i + 1];
      const prev = keysRef.current[i - 1];
      if (Math.abs(dx) / width >= SNAP_FRACTION) {
        if (dx < 0 && next) {
          restore(false);
          onChangeRef.current(next);
          return;
        }
        if (dx > 0 && prev) {
          restore(false);
          onChangeRef.current(prev);
          return;
        }
      }
      restore(true);
    };

    const onCancel = () => {
      if (active && tracking && horizontal) restore(true);
      tracking = false;
      horizontal = false;
    };

    window.addEventListener('touchstart', onStart, { capture: true, passive: true });
    window.addEventListener('touchmove', onMove, { capture: true, passive: false });
    window.addEventListener('touchend', onEnd, { capture: true, passive: true });
    window.addEventListener('touchcancel', onCancel, { capture: true, passive: true });
    return () => {
      window.removeEventListener('touchstart', onStart, { capture: true });
      window.removeEventListener('touchmove', onMove, { capture: true });
      window.removeEventListener('touchend', onEnd, { capture: true });
      window.removeEventListener('touchcancel', onCancel, { capture: true });
    };
  }, [restore]);

  return { contentRef, peekRef };
}

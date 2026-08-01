import { THEME_BG } from '@/lib/colors';
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';

export type ThemeMode = 'auto' | 'light' | 'dark';
export type EffectiveTheme = 'light' | 'dark';
export type Density = 'comfortable' | 'compact';

/** Amanecer 06:50 y atardecer 21:40 (mock julio, España). Fase 2: sun.sun vía HAOS. */
export const SUNRISE_MIN = 6 * 60 + 50;
export const SUNSET_MIN = 21 * 60 + 40;

const SLUG = 'helios';
const MODE_KEY = `${SLUG}-theme-mode`;
const LEGACY_MODE_KEY = `${SLUG}-theme`; // clave antigua, se migra a MODE_KEY
const DENSITY_KEY = `${SLUG}-density`;
const REDUCE_MOTION_KEY = `${SLUG}-reduce-motion`;

interface ThemeContextValue {
  mode: ThemeMode;
  setMode: (mode: ThemeMode) => void;
  effective: EffectiveTheme;
  isDark: boolean;
  density: Density;
  setDensity: (d: Density) => void;
  reduceMotion: boolean;
  setReduceMotion: (v: boolean) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

function computeAuto(): EffectiveTheme {
  const now = new Date();
  const mins = now.getHours() * 60 + now.getMinutes();
  return mins >= SUNRISE_MIN && mins < SUNSET_MIN ? 'light' : 'dark';
}

function initialMode(): ThemeMode {
  try {
    const saved = localStorage.getItem(MODE_KEY) ?? localStorage.getItem(LEGACY_MODE_KEY);
    if (saved === 'auto' || saved === 'light' || saved === 'dark') return saved;
  } catch {
    /* sin localStorage */
  }
  return 'auto';
}

function applyTheme(effective: EffectiveTheme) {
  const root = document.documentElement;
  root.classList.toggle('dark', effective === 'dark');
  const meta = document.getElementById('meta-theme-color');
  if (meta) meta.setAttribute('content', effective === 'dark' ? THEME_BG.dark : THEME_BG.light);
  // Favicon según el tema EFECTIVO de la app (pisa los estáticos con media del SO)
  let icon = document.querySelector<HTMLLinkElement>('link[rel="icon"][data-app-theme]');
  if (!icon) {
    icon = document.createElement('link');
    icon.rel = 'icon';
    icon.setAttribute('data-app-theme', '');
    document.head.appendChild(icon);
  }
  icon.href = effective === 'dark' ? '/favicon-dark.ico' : '/favicon-light.ico';
}

function applyDensity(density: Density) {
  document.documentElement.style.fontSize = density === 'compact' ? '13.5px' : '16px';
}

/** Anti-FOUC: aplicar preferencias antes del primer render (main.tsx). */
export function applyBootPreferences() {
  try {
    applyTheme(initialMode() === 'auto' ? computeAuto() : (initialMode() as EffectiveTheme));
    const density = localStorage.getItem(DENSITY_KEY);
    if (density === 'compact' || density === 'comfortable') applyDensity(density);
    if (localStorage.getItem(REDUCE_MOTION_KEY) === '1') {
      document.documentElement.classList.add('reduce-motion');
    }
  } catch {
    /* sin localStorage */
  }
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [mode, setModeState] = useState<ThemeMode>(initialMode);
  const [autoTheme, setAutoTheme] = useState<EffectiveTheme>(computeAuto);
  const [density, setDensityState] = useState<Density>(() => {
    try {
      return localStorage.getItem(DENSITY_KEY) === 'compact' ? 'compact' : 'comfortable';
    } catch {
      return 'comfortable';
    }
  });
  const [reduceMotion, setReduceMotionState] = useState<boolean>(() => {
    try {
      return localStorage.getItem(REDUCE_MOTION_KEY) === '1';
    } catch {
      return false;
    }
  });

  const effective: EffectiveTheme = mode === 'auto' ? autoTheme : mode;

  // Recalcula el tema automático cada minuto.
  useEffect(() => {
    if (mode !== 'auto') return;
    const id = window.setInterval(() => setAutoTheme(computeAuto()), 60_000);
    return () => window.clearInterval(id);
  }, [mode]);

  useEffect(() => {
    applyTheme(effective);
  }, [effective]);

  useEffect(() => {
    applyDensity(density);
  }, [density]);

  const setMode = useCallback((next: ThemeMode) => {
    setModeState(next);
    try {
      localStorage.setItem(MODE_KEY, next);
      localStorage.removeItem(LEGACY_MODE_KEY);
    } catch {
      /* sin localStorage */
    }
  }, []);

  const setDensity = useCallback((next: Density) => {
    setDensityState(next);
    try {
      localStorage.setItem(DENSITY_KEY, next);
    } catch {
      /* sin localStorage */
    }
  }, []);

  const setReduceMotion = useCallback((next: boolean) => {
    setReduceMotionState(next);
    try {
      localStorage.setItem(REDUCE_MOTION_KEY, next ? '1' : '0');
      document.documentElement.classList.toggle('reduce-motion', next);
    } catch {
      /* sin localStorage */
    }
  }, []);

  const value = useMemo<ThemeContextValue>(
    () => ({ mode, setMode, effective, isDark: effective === 'dark', density, setDensity, reduceMotion, setReduceMotion }),
    [mode, setMode, effective, density, setDensity, reduceMotion, setReduceMotion],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme debe usarse dentro de <ThemeProvider>');
  return ctx;
}

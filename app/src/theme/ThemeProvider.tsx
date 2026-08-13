import { THEME_BG } from '@/lib/colors';
import { getLiveSunState } from '@/data/EnergyDataProvider';
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';

export type ThemeMode = 'auto' | 'light' | 'dark';
export type EffectiveTheme = 'light' | 'dark';
export type Density = 'comfortable' | 'compact';

/** Fallback por reloj cuando no hay dato de sun.sun (login, HAOS caído). */
export const SUNRISE_MIN = 6 * 60 + 50;
export const SUNSET_MIN = 21 * 60 + 40;

const SLUG = 'helios';
const MODE_KEY = `${SLUG}-theme-mode`;
const LEGACY_MODE_KEY = `${SLUG}-theme`; // clave antigua, se migra a MODE_KEY
const DENSITY_KEY = `${SLUG}-density`;
const REDUCE_MOTION_KEY = `${SLUG}-reduce-motion`;
const ACCENT_KEY = `${SLUG}-accent`;

interface ThemeContextValue {
  mode: ThemeMode;
  setMode: (mode: ThemeMode) => void;
  effective: EffectiveTheme;
  isDark: boolean;
  density: Density;
  setDensity: (d: Density) => void;
  accent: string | null;
  setAccent: (rgb: string | null) => void;
  reduceMotion: boolean;
  setReduceMotion: (v: boolean) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

function computeAuto(): EffectiveTheme {
  const sun = getLiveSunState();
  if (sun === 'above_horizon') return 'light';
  if (sun === 'below_horizon') return 'dark';
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
}

function applyDensity(density: Density) {
  document.documentElement.style.fontSize = density === 'compact' ? '13.5px' : '16px';
}

/** Acento de marca: triplete RGB para --accent-rgb (null = naranja por defecto). */
function applyAccent(rgb: string | null) {
  const root = document.documentElement;
  if (rgb) root.style.setProperty('--accent-rgb', rgb);
  else root.style.removeProperty('--accent-rgb');
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
    applyAccent(localStorage.getItem(ACCENT_KEY));
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
  const [accent, setAccentState] = useState<string | null>(() => {
    try {
      return localStorage.getItem(ACCENT_KEY);
    } catch {
      return null;
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

  useEffect(() => {
    applyAccent(accent);
  }, [accent]);

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

  const setAccent = useCallback((next: string | null) => {
    setAccentState(next);
    try {
      if (next) localStorage.setItem(ACCENT_KEY, next);
      else localStorage.removeItem(ACCENT_KEY);
    } catch {
      /* sin localStorage */
    }
  }, []);

  const value = useMemo<ThemeContextValue>(
    () => ({ mode, setMode, effective, isDark: effective === 'dark', density, setDensity, accent, setAccent, reduceMotion, setReduceMotion }),
    [mode, setMode, effective, density, setDensity, accent, setAccent, reduceMotion, setReduceMotion],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme debe usarse dentro de <ThemeProvider>');
  return ctx;
}

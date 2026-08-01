import { THEME_BG } from '@/lib/colors';
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';

export type ThemeMode = 'auto' | 'light' | 'dark';
export type EffectiveTheme = 'light' | 'dark';

/** Amanecer 06:50 y atardecer 21:40 (mock julio, España). Fase 2: sun.sun vía HAOS. */
export const SUNRISE_MIN = 6 * 60 + 50;
export const SUNSET_MIN = 21 * 60 + 40;

const STORAGE_KEY = 'helios-theme';

interface ThemeContextValue {
  mode: ThemeMode;
  setMode: (mode: ThemeMode) => void;
  effective: EffectiveTheme;
  isDark: boolean;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

function computeAuto(): EffectiveTheme {
  const now = new Date();
  const mins = now.getHours() * 60 + now.getMinutes();
  return mins >= SUNRISE_MIN && mins < SUNSET_MIN ? 'light' : 'dark';
}

function initialMode(): ThemeMode {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
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

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [mode, setModeState] = useState<ThemeMode>(initialMode);
  const [autoTheme, setAutoTheme] = useState<EffectiveTheme>(computeAuto);

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

  const setMode = useCallback((next: ThemeMode) => {
    setModeState(next);
    try {
      localStorage.setItem(STORAGE_KEY, next);
    } catch {
      /* sin localStorage */
    }
  }, []);

  const value = useMemo<ThemeContextValue>(
    () => ({ mode, setMode, effective, isDark: effective === 'dark' }),
    [mode, setMode, effective],
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme debe usarse dentro de <ThemeProvider>');
  return ctx;
}

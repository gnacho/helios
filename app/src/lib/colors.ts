import { useTheme } from '@/theme/ThemeProvider';

/**
 * Colores semánticos de energía (sección 3.2 de design.md).
 * INVARIABLES en toda la app: cada tipo de energía siempre el mismo color.
 */
export const ENERGY_COLORS = {
  solar: { light: '#F59E0B', dark: '#FBBF24' },
  consumo: { light: '#3B82F6', dark: '#60A5FA' },
  bateria: { light: '#10B981', dark: '#34D399' },
  redCompra: { light: '#F43F5E', dark: '#FB7185' },
  redVertido: { light: '#06B6D4', dark: '#22D3EE' },
  solis: { light: '#0EA5E9', dark: '#38BDF8' },
  fox: { light: '#8B5CF6', dark: '#A78BFA' },
} as const;

export type EnergyColorKey = keyof typeof ENERGY_COLORS;

export const ESTADO_COLORS = {
  exito: '#10B981',
  aviso: '#F59E0B',
  error: '#EF4444',
  online: '#22C55E',
} as const;

/** Degradado ámbar de las áreas solares en gráficas. */
export const SOLAR_GRADIENT = { from: '#F59E0B', to: '#F97316' } as const;

/** Batería por debajo de la reserva (gauge). */
export const BATTERY_LOW_COLOR = '#FB7185';

/** Icono casa del flujo (SVG inline). */
export const FLOW_HOME_COLORS = {
  strokeFrom: '#3B82F6',
  strokeTo: '#1D4ED8',
  stroke: '#60A5FA',
  sun: '#FBBF24',
  sunStroke: '#B45309',
} as const;

/** Arco solar: cielo nocturno y estrellas. */
export const SOLAR_ARC_COLORS = { nightFrom: '#101828', nightTo: '#182338', star: '#FFFFFF' } as const;

/** Fondos base por tema (theme-color meta, swatches de preview). */
export const THEME_BG = { light: '#F4F6FA', dark: '#080D1A' } as const;

/** Swatches literales para la vista previa de temas en Ajustes. */
export const THEME_SWATCHES = {
  light: { bg: '#F4F6FA', surface: '#FFFFFF', text: '#0C1425', faint: '#94A3B8', border: '#E3E8F0', accent: '#F59E0B' },
  dark: { bg: '#080D1A', surface: '#101828', text: '#E9EEF7', faint: '#5C6B85', border: '#1E2B42', accent: '#FBBF24' },
} as const;

export function energyColor(key: EnergyColorKey, isDark: boolean): string {
  return isDark ? ENERGY_COLORS[key].dark : ENERGY_COLORS[key].light;
}

export type EnergyPalette = Record<EnergyColorKey, string>;

/** Paleta semántica resuelta según el tema efectivo actual. */
export function useEnergyColors(): EnergyPalette {
  const { isDark } = useTheme();
  return {
    solar: energyColor('solar', isDark),
    consumo: energyColor('consumo', isDark),
    bateria: energyColor('bateria', isDark),
    redCompra: energyColor('redCompra', isDark),
    redVertido: energyColor('redVertido', isDark),
    solis: energyColor('solis', isDark),
    fox: energyColor('fox', isDark),
  };
}

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

/** Gradiente de marca Helios (el mismo del favicon): amarillo ámbar → naranja. */
export const BRAND_GRADIENT = { from: '#FBBF24', to: '#F97316' } as const;

/** Batería por debajo de la reserva (gauge). */
export const BATTERY_LOW_COLOR = '#FB7185';

/** Arco solar: cielo nocturno y estrellas. */
export const SOLAR_ARC_COLORS = { nightFrom: '#101828', nightTo: '#182338', star: '#FFFFFF' } as const;

/** Fondos base por tema (theme-color meta, swatches de preview). */
export const THEME_BG = { light: '#F4F6FA', dark: '#080D1A' } as const;

/** Superficies del preview de tema (mini-UI pintada con los colores reales). */
export const THEME_SURFACE = { light: '#FFFFFF', dark: '#0F1729' } as const;
/** Barras internas del preview de tema. */
export const THEME_BAR = { light: '#E2E8F0', dark: '#1E293B' } as const;

/** Paleta de acentos del selector de Ajustes (triplete RGB para --accent-rgb; null = naranja por defecto). */
export const ACCENTS = [
  { id: 'naranja', rgb: null, hex: '#F59E0B' },
  { id: 'verde', rgb: '34 197 94', hex: '#22C55E' },
  { id: 'azul', rgb: '59 130 246', hex: '#3B82F6' },
  { id: 'violeta', rgb: '139 92 246', hex: '#8B5CF6' },
] as const;

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

/**
 * Color de un inversor por clave (issue #37). Los dos primeros (solis/fox)
 * mantienen su color semántico; los adicionales usan una paleta cíclica.
 */
const EXTRA_INV_COLORS: { light: string; dark: string }[] = [
  { light: '#10B981', dark: '#34D399' },
  { light: '#F43F5E', dark: '#FB7185' },
  { light: '#06B6D4', dark: '#22D3EE' },
  { light: '#F59E0B', dark: '#FBBF24' },
  { light: '#EC4899', dark: '#F472B6' },
  { light: '#84CC16', dark: '#A3E635' },
];

export function inverterColor(key: string, isDark: boolean, index = 0): string {
  if (key === 'solis') return energyColor('solis', isDark);
  if (key === 'fox') return energyColor('fox', isDark);
  const c = EXTRA_INV_COLORS[index % EXTRA_INV_COLORS.length];
  return isDark ? c.dark : c.light;
}

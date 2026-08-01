import { SOLAR_ARC_COLORS } from '@/lib/colors';
import { useEffect, useRef } from 'react';
import { gsap } from 'gsap';
import { Sunrise, Sunset, CloudSun } from 'lucide-react';
import { motion } from 'framer-motion';
import { useTranslation } from 'react-i18next';
import { SUNRISE_MIN, SUNSET_MIN } from '@/data/types';
import { useTheme } from '@/theme/ThemeProvider';
import { useEnergyColors } from '@/lib/colors';
import { fmtTime, fmtKw } from '@/lib/format';
import { prefersReducedMotion } from '@/lib/useAnimatedNumber';

interface SolarArcProps {
  /** Minutos desde medianoche del instante a mostrar. */
  atMin: number;
  peakKw: number;
  peakAtMin: number;
  sunriseMin?: number;
  sunsetMin?: number;
  weather?: string;
  weatherTemp?: number;
}

const WEATHER_KEYS = [
  'sunny',
  'clear-night',
  'partlycloudy',
  'cloudy',
  'rainy',
  'pouring',
  'lightning',
  'lightning-rainy',
  'snowy',
  'snowy-rainy',
  'fog',
  'windy',
  'windy-variant',
  'hail',
] as const;

/** Punto sobre el arco semicircular: frac 0 (amanecer, izq) → 1 (atardecer, der). */
function arcPoint(frac: number): { x: number; y: number; angle: number } {
  const cx = 150;
  const cy = 128;
  const r = 108;
  const angle = Math.PI * (1 - Math.min(1, Math.max(0, frac))); // π → 0
  return { x: cx + r * Math.cos(angle), y: cy - r * Math.sin(angle), angle };
}

const STARS: [number, number][] = [
  [38, 34],
  [84, 18],
  [150, 30],
  [216, 16],
  [262, 40],
];

/** Arco del sol: posición según la hora, tween GSAP 1,4s al entrar. */
export default function SolarArc({ atMin, peakKw, peakAtMin, sunriseMin = SUNRISE_MIN, sunsetMin = SUNSET_MIN, weather, weatherTemp }: SolarArcProps) {
  const { isDark } = useTheme();
  const palette = useEnergyColors();
  const { t } = useTranslation();
  const sunRef = useRef<SVGGElement>(null);
  const animRef = useRef({ f: 0 });
  const frac = Math.min(1, Math.max(0, (atMin - sunriseMin) / (sunsetMin - sunriseMin)));
  const daytime = atMin >= sunriseMin && atMin <= sunsetMin;

  const weatherLabel = weather
    ? (WEATHER_KEYS as readonly string[]).includes(weather)
      ? t(`weather.${weather}`)
      : weather
    : '—';

  useEffect(() => {
    const el = sunRef.current;
    if (!el) return;
    if (prefersReducedMotion()) {
      const p = arcPoint(frac);
      gsap.set(el, { x: p.x, y: p.y });
      return;
    }
    gsap.to(animRef.current, {
      f: frac,
      duration: 2.4,
      delay: 0.3,
      ease: 'sine.inOut',
      onUpdate: () => {
        const p = arcPoint(animRef.current.f);
        gsap.set(el, { x: p.x, y: p.y });
      },
    });
  }, [frac]);

  return (
    <div>
      <svg viewBox="0 0 300 140" className="mx-auto w-full max-w-[320px]" role="img" aria-label={t('solar.aria')}>
        <defs>
          <linearGradient id="night-sky" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={SOLAR_ARC_COLORS.nightFrom} stopOpacity={isDark ? 0.9 : 0} />
            <stop offset="100%" stopColor={SOLAR_ARC_COLORS.nightTo} stopOpacity={isDark ? 0.4 : 0} />
          </linearGradient>
        </defs>
        <path d="M 42 128 A 108 108 0 0 1 258 128" fill="url(#night-sky)" stroke="none" />
        {isDark &&
          STARS.map(([sx, sy], i) => <circle key={i} cx={sx} cy={sy} r={1.4} fill={SOLAR_ARC_COLORS.star} opacity={0.3} />)}
        {/* arco */}
        <path d="M 42 128 A 108 108 0 0 1 258 128" fill="none" stroke="var(--line)" strokeWidth={2} strokeDasharray="3 5" />
        {/* horizonte */}
        <line x1={20} y1={128} x2={280} y2={128} stroke="var(--line)" strokeWidth={2} strokeLinecap="round" />
        {/* sol con glow */}
        <g ref={sunRef} style={{ transform: `translate(${arcPoint(frac).x}px, ${arcPoint(frac).y}px)` }}>
          <circle r={13} fill={palette.solar} opacity={0.25}>
            <animate attributeName="r" values="13;17;13" dur="5s" repeatCount="indefinite" />
          </circle>
          <circle r={7.5} fill={daytime ? palette.solar : 'var(--text-faint)'} />
        </g>
      </svg>

      <motion.div
        initial={{ opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.4, delay: 0.5 }}
        className="mt-1 flex items-center justify-between text-xs font-medium text-muted"
      >
        <span className="inline-flex items-center gap-1.5">
          <Sunrise size={15} className="text-faint" /> {fmtTime(sunriseMin)}
        </span>
        <span className="inline-flex items-center gap-1.5">
          {fmtTime(sunsetMin)} <Sunset size={15} className="text-faint" />
        </span>
      </motion.div>

      <motion.div
        initial={{ opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.4, delay: 0.6 }}
        className="mt-3 flex flex-wrap items-center gap-2 border-t border-app pt-3 text-xs text-muted"
      >
        <span className="inline-flex items-center gap-1.5">
          <CloudSun size={15} style={{ color: palette.solar }} /> {weatherLabel}
        </span>
        {weatherTemp !== undefined && weatherTemp > -40 && (
          <>
            <span aria-hidden>·</span>
            <span>{Math.round(weatherTemp)} °C</span>
          </>
        )}
        <span className="ml-auto rounded-full bg-surface-2 px-2.5 py-1 text-[11px] font-semibold text-app">
          {t('solar.peak', { kw: fmtKw(peakKw), time: fmtTime(peakAtMin) })}
        </span>
      </motion.div>
    </div>
  );
}

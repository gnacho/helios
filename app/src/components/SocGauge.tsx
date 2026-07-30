import { useId } from 'react';
import { motion } from 'framer-motion';
import { useEnergyColors } from '@/lib/colors';
import { useAnimatedNumber, prefersReducedMotion } from '@/lib/useAnimatedNumber';

interface SocGaugeProps {
  /** Estado de carga 0–100. */
  soc: number;
  /** Texto de estado bajo la cifra, p. ej. "En reposo · reserva 20 %". */
  status?: string;
  size?: number;
}

function polar(cx: number, cy: number, r: number, angleDeg: number): [number, number] {
  const a = ((angleDeg - 90) * Math.PI) / 180;
  return [cx + r * Math.cos(a), cy + r * Math.sin(a)];
}

/** Arco de 240° (hueco abajo), de 150° a 390°. */
function arcPath(cx: number, cy: number, r: number, startDeg: number, endDeg: number): string {
  const [sx, sy] = polar(cx, cy, r, startDeg);
  const [ex, ey] = polar(cx, cy, r, endDeg);
  const large = endDeg - startDeg > 180 ? 1 : 0;
  return `M ${sx} ${sy} A ${r} ${r} 0 ${large} 1 ${ex} ${ey}`;
}

/** Gauge radial de SOC: arco 240°, emerald (ámbar <25 %, rose <15 %). */
export default function SocGauge({ soc, status, size = 180 }: SocGaugeProps) {
  const palette = useEnergyColors();
  const id = useId().replace(/[:]/g, '');
  const animated = useAnimatedNumber(soc, 1);

  const clamped = Math.min(100, Math.max(0, soc));
  const arcColor = clamped < 15 ? '#FB7185' : clamped < 25 ? palette.solar : palette.bateria;

  const cx = 100;
  const cy = 100;
  const r = 78;
  const START = 150;
  const SWEEP = 240;

  return (
    <div className="flex flex-col items-center" role="img" aria-label={`Batería al ${Math.round(clamped)} %`}>
      <svg width={size} height={size * 0.78} viewBox="0 0 200 156" fill="none">
        <defs>
          <linearGradient id={`soc-${id}`} x1="0" y1="0" x2="1" y2="1">
            <stop offset="0%" stopColor={arcColor} />
            <stop offset="100%" stopColor={palette.solar} stopOpacity={clamped < 25 ? 1 : 0.55} />
          </linearGradient>
        </defs>
        {/* track */}
        <path d={arcPath(cx, cy, r, START, START + SWEEP)} stroke="var(--surface-2)" strokeWidth={13} strokeLinecap="round" />
        {/* valor con barrido de entrada 1s ease-out */}
        <motion.path
          d={arcPath(cx, cy, r, START, START + SWEEP)}
          stroke={`url(#soc-${id})`}
          strokeWidth={13}
          strokeLinecap="round"
          initial={prefersReducedMotion() ? false : { pathLength: 0 }}
          animate={{ pathLength: clamped / 100 }}
          transition={{ duration: 1, ease: 'easeOut', delay: 0.3 }}
        />
        <text
          x={cx}
          y={cy + 6}
          textAnchor="middle"
          className="font-display"
          style={{ fill: 'var(--text)', fontSize: 40, fontWeight: 600, letterSpacing: '-0.02em' }}
        >
          {Math.round(animated)}
          <tspan style={{ fontSize: 20, fill: 'var(--text-faint)', fontWeight: 500 }}> %</tspan>
        </text>
      </svg>
      {status && <p className="-mt-3 text-xs font-medium text-muted">{status}</p>}
    </div>
  );
}

import { useRef } from 'react';
import gsap from 'gsap';
import { useGSAP } from '@gsap/react';
import { useTranslation } from 'react-i18next';
import { useEnergyColors } from '@/lib/colors';
import { prefersReducedMotion } from '@/lib/useAnimatedNumber';
import { fmtEnergy, fmtPct } from '@/lib/format';
import { BATTERY_RESERVE_PCT as RESERVE_PCT } from '@/lib/thresholds';

gsap.registerPlugin(useGSAP);

interface SocHeroGaugeProps {
  /** Estado de carga 0–100. */
  soc: number;
  /** Diámetro aproximado del gauge. */
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

const CX = 130;
const CY = 132;
const R = 100;
const START = 150;
const SWEEP = 240;

/**
 * Gauge SOC héroe (página /bateria) — §1 de bateria.md.
 * Componente GSAP aislado (sin Framer Motion en este árbol):
 * barrido del arco 0→SOC 1,4s easeOut, cifra count-up 1,2s (vía textContent,
 * sin re-renders) y tick "Reserva 20 %" con fade-in delay 800ms.
 */
export default function SocHeroGauge({ soc, size = 260 }: SocHeroGaugeProps) {
  const palette = useEnergyColors();
  const { t } = useTranslation();
  const rootRef = useRef<HTMLDivElement>(null);
  const arcRef = useRef<SVGPathElement>(null);
  const numRef = useRef<SVGTextElement>(null);
  const reserveRef = useRef<SVGGElement>(null);
  const firstRun = useRef(true);

  const clamped = Math.min(100, Math.max(0, soc));
  const arcColor = clamped < 15 ? palette.redCompra : clamped < 25 ? palette.solar : palette.bateria;
  const reduced = prefersReducedMotion();

  useGSAP(
    () => {
      const path = arcRef.current;
      if (!path) return;
      const len = path.getTotalLength();
      const target = len * (1 - clamped / 100);

      if (firstRun.current && !reduced) {
        firstRun.current = false;
        // Barrido de entrada 0 → SOC
        gsap.set(path, { strokeDasharray: len, strokeDashoffset: len });
        gsap.to(path, { strokeDashoffset: target, duration: 1.4, ease: 'power2.out' });
        // Cifra count-up 1,2s actualizando el nodo de texto directamente
        const counter = { v: 0 };
        gsap.to(counter, {
          v: clamped,
          duration: 1.2,
          ease: 'power2.out',
          onUpdate: () => {
            if (numRef.current) numRef.current.textContent = String(Math.round(counter.v));
          },
        });
        // Tick de reserva con fade-in delay 800ms
        if (reserveRef.current) {
          gsap.fromTo(reserveRef.current, { opacity: 0 }, { opacity: 1, duration: 0.5, delay: 0.8 });
        }
      } else {
        firstRun.current = false;
        gsap.set(path, { strokeDasharray: len });
        gsap.to(path, { strokeDashoffset: target, duration: reduced ? 0 : 0.6, ease: 'power2.out' });
        if (numRef.current) numRef.current.textContent = String(Math.round(clamped));
        if (reserveRef.current) gsap.set(reserveRef.current, { opacity: 1 });
      }
    },
    { scope: rootRef, dependencies: [clamped, arcColor, reduced] },
  );

  // Tick de reserva al 20 % del arco (ángulo 150° + 240°·0,2 = 198°)
  const reserveAngle = START + (SWEEP * RESERVE_PCT) / 100;
  const [tx1, ty1] = polar(CX, CY, R - 13, reserveAngle);
  const [tx2, ty2] = polar(CX, CY, R + 9, reserveAngle);
  const [lx, ly] = polar(CX, CY, R - 30, reserveAngle);

  return (
    <div ref={rootRef} className="flex justify-center" role="img" aria-label={t('bateria.gaugeAria', { pct: Math.round(clamped) })}>
      <svg width={size} height={size * 0.92} viewBox="0 0 260 240" fill="none">
        {/* track */}
        <path d={arcPath(CX, CY, R, START, START + SWEEP)} stroke="var(--surface-2)" strokeWidth={16} strokeLinecap="round" />
        {/* valor (barrido GSAP) */}
        <path
          ref={arcRef}
          d={arcPath(CX, CY, R, START, START + SWEEP)}
          stroke={arcColor}
          strokeWidth={16}
          strokeLinecap="round"
        />
        {/* marca de reserva 20 % */}
        <g ref={reserveRef} opacity={0}>
          <line x1={tx1} y1={ty1} x2={tx2} y2={ty2} stroke={palette.redCompra} strokeWidth={2} strokeDasharray="3 3" strokeLinecap="round" />
          <text x={lx} y={ly + 3} textAnchor="middle" style={{ fill: palette.redCompra, fontSize: 10, fontWeight: 600, fontFamily: 'Inter' }}>
            {t('bateria.reservePct', { pct: fmtPct(RESERVE_PCT) })}
          </text>
        </g>
        {/* cifra central */}
        <text x={CX} y={CY + 4} textAnchor="middle" className="font-display" style={{ fill: arcColor, fontSize: 56, fontWeight: 600, letterSpacing: '-0.02em' }}>
          <tspan ref={numRef}>0</tspan>
          <tspan style={{ fontSize: 26, fill: 'var(--text-faint)', fontWeight: 500 }}> %</tspan>
        </text>
        <text x={CX} y={CY + 32} textAnchor="middle" style={{ fill: 'var(--text-muted)', fontSize: 14, fontWeight: 500, fontFamily: 'Inter' }}>
          {t('bateria.ofTotalKwh', { kwh: fmtEnergy((clamped / 100) * 5), total: fmtEnergy(5) })}
        </text>
      </svg>
    </div>
  );
}

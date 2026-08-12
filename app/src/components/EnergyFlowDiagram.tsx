import { memo, useId } from 'react';
import { useNavigate } from 'react-router';
import { motion } from 'framer-motion';
import { SolarPanel, House, BatteryCharging, UtilityPole } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { LivePower } from '@/data/types';
import { useEnergyColors } from '@/lib/colors';
import { fmtKw } from '@/lib/format';
import { cn } from '@/lib/utils';

interface FlowEdge {
  id: string;
  d: string;
  value: number; // kW
  color: string;
}

/** Duración del recorrido de los puntos: más potencia = más rápido (0,8–2,6 s). */
function flowDuration(kw: number): number {
  return Math.max(0.8, Math.min(2.6, 2.6 - kw * 0.28));
}

const NODES = {
  fv: { x: 170, y: 46 },
  home: { x: 170, y: 152 },
  battery: { x: 62, y: 246 },
  grid: { x: 278, y: 246 },
} as const;

const PATHS = {
  fvHome: `M ${NODES.fv.x} ${NODES.fv.y + 30} L ${NODES.home.x} ${NODES.home.y - 34}`,
  fvBattery: `M ${NODES.fv.x - 18} ${NODES.fv.y + 16} C 96 74, ${NODES.battery.x + 6} 150, ${NODES.battery.x + 4} ${NODES.battery.y - 34}`,
  fvGrid: `M ${NODES.fv.x + 18} ${NODES.fv.y + 16} C 244 74, ${NODES.grid.x - 6} 150, ${NODES.grid.x - 4} ${NODES.grid.y - 34}`,
  batteryHome: `M ${NODES.battery.x + 28} ${NODES.battery.y - 22} C 118 206, 138 196, ${NODES.home.x - 20} ${NODES.home.y + 20}`,
  gridHome: `M ${NODES.grid.x - 28} ${NODES.grid.y - 22} C 222 206, 202 196, ${NODES.home.x + 20} ${NODES.home.y + 20}`,
  gridBattery: `M ${NODES.grid.x - 28} ${NODES.grid.y + 22} C 218 296, 122 296, ${NODES.battery.x + 28} ${NODES.battery.y + 22}`,
} as const;

interface FlowNodeProps {
  x: number;
  y: number;
  label: string;
  valueText: string;
  active: boolean;
  glowColor: string;
  onClick?: () => void;
  children: React.ReactNode;
}

const FlowNode = memo(function FlowNode({ x, y, label, valueText, active, glowColor, onClick, children }: FlowNodeProps) {
  const r = 33
  return (
    <g
      onClick={onClick}
      className={cn(onClick && 'cursor-pointer')}
      role={onClick ? 'link' : undefined}
      aria-label={`${label}: ${valueText}`}
    >
      {active && (
        <circle cx={x} cy={y} r={r + 4} fill="none" stroke={glowColor} strokeOpacity={0.2} strokeWidth={3} />
      )}
      <circle
        cx={x}
        cy={y}
        r={r}
        fill="var(--surface)"
        stroke={active ? glowColor : 'var(--line)'}
        strokeWidth={active ? 2 : 1.5}
      />
      <foreignObject x={x - 28} y={y - 28} width={56} height={56}>
        <div className="flex h-[56px] w-[56px] items-center justify-center">{children}</div>
      </foreignObject>
      <text
        x={x}
        y={y + 50}
        textAnchor="middle"
        style={{ fill: 'var(--text-faint)', fontSize: 10, fontWeight: 500, textTransform: 'uppercase', letterSpacing: '0.06em' }}
      >
        {label}
      </text>
      <text
        x={x}
        y={y + 64}
        textAnchor="middle"
        className="font-display"
        style={{ fill: 'var(--text)', fontSize: 12.5, fontWeight: 600 }}
      >
        {valueText}
      </text>
    </g>
  )
})

/** Arista con puntos fluyendo (SMIL animateMotion), aislada y memoizada. */
const FlowEdgeLayer = memo(function FlowEdgeLayer({ edge, uid }: { edge: FlowEdge; uid: string }) {
  const active = edge.value > 0.05;
  const dur = flowDuration(edge.value);
  const pathId = `${uid}-${edge.id}`;
  return (
    <g style={{ transition: 'opacity 0.4s ease' }} opacity={active ? 1 : 0.4}>
      <path id={pathId} d={edge.d} fill="none" stroke={active ? edge.color : 'var(--line)'} strokeWidth={active ? 2 : 1.5} opacity={active ? 0.35 : 1} />
      {active &&
        [0, 1, 2, 3].map((k) => (
          <circle key={k} r={3} fill={edge.color}>
            <animateMotion dur={`${dur}s`} begin={`${(-k * dur) / 4}s`} repeatCount="indefinite">
              <mpath href={`#${pathId}`} />
            </animateMotion>
          </circle>
        ))}
    </g>
  );
});

interface EnergyFlowDiagramProps {
  live: LivePower;
  className?: string;
}

/**
 * Diagrama de flujo de energía (SVG ~340×280, escalable).
 * Nodos clicables: FV → /inversores, Batería → /bateria, Red → /historico.
 */
export default function EnergyFlowDiagram({ live, className }: EnergyFlowDiagramProps) {
  const palette = useEnergyColors();
  const navigate = useNavigate();
  const { t } = useTranslation();
  const uid = useId().replace(/[:]/g, '');

  const fvToHome = Math.min(live.production, live.consumption);
  const charging = live.batteryPower > 0 ? live.batteryPower : 0;
  const discharging = live.batteryPower < 0 ? -live.batteryPower : 0;
  const exporting = live.grid < 0 ? -live.grid : 0;
  const importing = live.grid > 0 ? live.grid : 0;
  const solarSurplus = Math.max(0, live.production - live.consumption);
  const gridToBattery = Math.max(0, Math.min(charging - solarSurplus, importing));
  const fvToBattery = Math.max(0, charging - gridToBattery);

  const edges: FlowEdge[] = [
    { id: 'fv-home', d: PATHS.fvHome, value: fvToHome, color: palette.solar },
    { id: 'fv-battery', d: PATHS.fvBattery, value: fvToBattery, color: palette.bateria },
    { id: 'fv-grid', d: PATHS.fvGrid, value: exporting, color: palette.redVertido },
    { id: 'battery-home', d: PATHS.batteryHome, value: discharging, color: palette.bateria },
    { id: 'grid-home', d: PATHS.gridHome, value: importing, color: palette.redCompra },
    { id: 'grid-battery', d: PATHS.gridBattery, value: gridToBattery, color: palette.redCompra },
  ];

  const batteryText =
    live.batteryPower > 0.05
      ? `${fmtKw(live.batteryPower)} kW`
      : live.batteryPower < -0.05
        ? `${fmtKw(-live.batteryPower)} kW`
        : t('common.idle');
  const gridText =
    live.grid < -0.05 ? `${fmtKw(-live.grid)} kW ↑` : live.grid > 0.05 ? `${fmtKw(live.grid)} kW ↓` : '0 W';

  return (
    <motion.svg
      viewBox="0 0 340 312"
      className={cn('mx-auto w-full max-w-[360px]', className)}
      initial={{ opacity: 0, scale: 0.96 }}
      animate={{ opacity: 1, scale: 1 }}
      transition={{ duration: 0.5, delay: 0.25, ease: 'easeOut' }}
      role="img"
      aria-label={t('flow.aria')}
    >
      {edges.map((e) => (
        <FlowEdgeLayer key={e.id} edge={e} uid={uid} />
      ))}

      <FlowNode
        x={NODES.fv.x}
        y={NODES.fv.y}
        label={t('flow.fv')}
        valueText={`${fmtKw(live.production)} kW`}
        active={live.production > 0.1}
        glowColor={palette.solar}
        onClick={() => navigate('/inversores')}
      >
        <SolarPanel size={28} style={{ color: live.production > 0.1 ? palette.solar : 'var(--text-faint)' }} />
      </FlowNode>

      <FlowNode
        x={NODES.home.x}
        y={NODES.home.y}
        label={t('flow.home')}
        valueText={`${fmtKw(live.consumption)} kW`}
        active={live.consumption > 0.1}
        glowColor={palette.consumo}
      >
        <House size={30} style={{ color: palette.consumo }} />
      </FlowNode>

      <FlowNode
        x={NODES.battery.x}
        y={NODES.battery.y}
        label={t('common.battery')}
        valueText={batteryText}
        active={Math.abs(live.batteryPower) > 0.1}
        glowColor={palette.bateria}
        onClick={() => navigate('/bateria')}
      >
        <span className="flex flex-col items-center leading-none">
          <BatteryCharging size={22} style={{ color: palette.bateria }} />
          <span className="mt-1 text-[11px] font-bold tabular-nums" style={{ color: palette.bateria }}>
            {Math.round(live.soc)}%
          </span>
        </span>
      </FlowNode>

      <FlowNode
        x={NODES.grid.x}
        y={NODES.grid.y}
        label={t('common.grid')}
        valueText={gridText}
        active={Math.abs(live.grid) > 0.1}
        glowColor={live.grid < 0 ? palette.redVertido : palette.redCompra}
        onClick={() => navigate('/historico')}
      >
        <UtilityPole size={26} style={{ color: live.grid < 0 ? palette.redVertido : palette.redCompra }} />
      </FlowNode>
    </motion.svg>
  );
}

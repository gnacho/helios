import { useId } from 'react';
import { Area, AreaChart, ResponsiveContainer } from 'recharts';

interface SparklineProps {
  data: number[];
  color: string;
  height?: number;
  /** Punto final "en vivo". */
  liveDot?: boolean;
}

/** AreaChart mínimo sin ejes con gradiente semántico y punto final en vivo. */
export default function Sparkline({ data, color, height = 40, liveDot = true }: SparklineProps) {
  const id = useId().replace(/[:]/g, '');
  const points = data.map((v, i) => ({ i, v }));
  const last = points[points.length - 1];

  return (
    <div style={{ width: '100%', height }} aria-hidden>
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={points} margin={{ top: 2, right: 2, bottom: 2, left: 2 }}>
          <defs>
            <linearGradient id={`spark-${id}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={color} stopOpacity={0.3} />
              <stop offset="100%" stopColor={color} stopOpacity={0} />
            </linearGradient>
          </defs>
          <Area
            type="monotone"
            dataKey="v"
            stroke={color}
            strokeWidth={1.8}
            fill={`url(#spark-${id})`}
            isAnimationActive
            animationDuration={900}
            dot={false}
            activeDot={false}
          />
        </AreaChart>
      </ResponsiveContainer>
      {liveDot && last && last.v > 0 && (
        <span className="relative float-right -mt-2 mr-1 flex h-1.5 w-1.5">
          <span className="absolute inline-flex h-full w-full animate-ping-soft rounded-full opacity-60" style={{ backgroundColor: color }} />
          <span className="relative inline-flex h-1.5 w-1.5 rounded-full" style={{ backgroundColor: color }} />
        </span>
      )}
    </div>
  );
}

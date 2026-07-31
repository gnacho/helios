import { useState } from 'react';
import { Cell, Pie, PieChart, ResponsiveContainer, Sector } from 'recharts';
import { useTranslation } from 'react-i18next';
import { useEnergyColors } from '@/lib/colors';
import { fmtEnergy, fmtPct } from '@/lib/format';
import { useAnimatedNumber } from '@/lib/useAnimatedNumber';
import { cn } from '@/lib/utils';

export interface SourceSplit {
  /** Solar directa (kWh). */
  solar: number;
  /** Batería (kWh). */
  bateria: number;
  /** Red (kWh). */
  red: number;
}

interface EnergySourceDonutProps {
  split: SourceSplit;
  height?: number;
}

interface SectorShapeProps {
  cx: number;
  cy: number;
  innerRadius: number;
  outerRadius: number;
  startAngle: number;
  endAngle: number;
  fill: string;
}

const renderActiveShape = (props: unknown) => {
  const { cx, cy, innerRadius, outerRadius, startAngle, endAngle, fill } = props as SectorShapeProps;
  return (
    <Sector
      cx={cx}
      cy={cy}
      innerRadius={innerRadius}
      outerRadius={(outerRadius ?? 0) + 6}
      startAngle={startAngle}
      endAngle={endAngle}
      fill={fill}
      cornerRadius={6}
    />
  );
};

/**
 * Donut «¿De dónde vino tu energía?»: solar directa / batería / red.
 * Segmento activo (hover/tap): se expande +6px y el centro muestra su dato.
 */
export default function EnergySourceDonut({ split, height = 320 }: EnergySourceDonutProps) {
  const palette = useEnergyColors();
  const { t } = useTranslation();
  const [activeIndex, setActiveIndex] = useState<number>(-1);

  const total = Math.max(0, split.solar + split.bateria + split.red);
  const animatedTotal = useAnimatedNumber(total);

  const slices = [
    { key: 'solar', label: t('historico.solarDirect'), value: split.solar, color: palette.solar },
    { key: 'bateria', label: t('common.battery'), value: split.bateria, color: palette.bateria },
    { key: 'red', label: t('common.grid'), value: split.red, color: palette.redCompra },
  ].filter((s) => s.value > 0.005);

  const active = activeIndex >= 0 ? slices[activeIndex] : undefined;

  return (
    <div className="flex h-full flex-col">
      <div className="relative flex-1" style={{ minHeight: height * 0.62 }}>
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={slices}
              dataKey="value"
              nameKey="label"
              innerRadius="62%"
              outerRadius="88%"
              startAngle={90}
              endAngle={-270}
              paddingAngle={2}
              cornerRadius={5}
              stroke="none"
              activeIndex={activeIndex < 0 ? undefined : activeIndex}
              activeShape={renderActiveShape}
              onMouseEnter={(_, i) => setActiveIndex(i)}
              onMouseLeave={() => setActiveIndex(-1)}
              onClick={(_, i) => setActiveIndex((cur) => (cur === i ? -1 : i))}
              isAnimationActive
              animationDuration={900}
              animationBegin={120}
            >
              {slices.map((s) => (
                <Cell key={s.key} fill={s.color} className="cursor-pointer outline-none" />
              ))}
            </Pie>
          </PieChart>
        </ResponsiveContainer>

        {/* Centro: total o dato del segmento activo */}
        <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center text-center">
          {active ? (
            <>
              <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-faint">{active.label}</p>
              <p className="font-display text-[22px] font-semibold leading-tight text-app">
                {fmtEnergy(active.value)} <span className="text-[0.6em] font-medium text-faint">kWh</span>
              </p>
              <p className="text-xs font-semibold" style={{ color: active.color }}>
                {total > 0 ? fmtPct((active.value / total) * 100) : 0} %
              </p>
            </>
          ) : (
            <>
              <p className="font-display text-2xl font-semibold leading-none text-app">
                {fmtEnergy(animatedTotal)} <span className="text-[0.6em] font-medium text-faint">kWh</span>
              </p>
              <p className="mt-1 text-xs text-faint">{t('historico.consumed')}</p>
            </>
          )}
        </div>
      </div>

      {/* Leyenda con valores absolutos */}
      <ul className="mt-2 flex flex-col gap-1.5 px-1 pb-1">
        {slices.map((s, i) => (
          <li key={s.key}>
            <button
              onMouseEnter={() => setActiveIndex(i)}
              onMouseLeave={() => setActiveIndex(-1)}
              onClick={() => setActiveIndex((cur) => (cur === i ? -1 : i))}
              className={cn(
                'flex w-full items-center gap-2 rounded-lg px-2 py-1 text-left text-[13px] transition-colors',
                activeIndex === i ? 'bg-surface-2' : 'hover:bg-surface-2/60',
              )}
            >
              <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ backgroundColor: s.color }} />
              <span className="text-muted">{s.label}</span>
              <span className="ml-auto font-semibold tabular-nums text-app">{fmtEnergy(s.value)} kWh</span>
              <span className="w-10 text-right text-xs tabular-nums text-faint">
                {total > 0 ? fmtPct((s.value / total) * 100) : 0} %
              </span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}

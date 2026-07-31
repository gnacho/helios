import { useMemo, useState } from 'react';
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { useTranslation } from 'react-i18next';
import type { PowerPoint } from '@/data/types';
import { useEnergyColors } from '@/lib/colors';
import { fmtKw, fmtTime } from '@/lib/format';

const BUCKET_MIN = 15;
const X_TICKS = [0, 240, 480, 720, 960, 1200, 1440];

interface BatteryPowerChartProps {
  data: PowerPoint[];
  height?: number;
}

interface BucketRow {
  t: number;
  carga: number | null;
  descarga: number | null;
}

function PowerTooltip({ active, payload, label }: { active?: boolean; payload?: { dataKey?: string | number; value?: number | null }[]; label?: number }) {
  const { t } = useTranslation();
  if (!active || !payload || payload.length === 0 || label === undefined) return null;
  const row = payload.find((p) => p.value !== null && p.value !== undefined);
  if (!row || row.value === null || row.value === undefined) return null;
  const charging = row.dataKey === 'carga';
  return (
    <div className="rounded-xl border border-app bg-surface/95 px-3 py-2 shadow-lg backdrop-blur-md">
      <p className="text-xs font-semibold text-app">
        {fmtTime(label)} · {charging ? t('common.charging') : t('common.discharging')} {fmtKw(Math.abs(row.value))} kW
      </p>
    </div>
  );
}

/**
 * §4 Potencia de carga/descarga: BarChart divergente a 15 min —
 * carga (emerald) hacia arriba, descarga (amber) hacia abajo, eje 0 centrado.
 */
export default function BatteryPowerChart({ data, height = 260 }: BatteryPowerChartProps) {
  const palette = useEnergyColors();
  const { t } = useTranslation();
  const [activeIdx, setActiveIdx] = useState<number | null>(null);

  const rows = useMemo<BucketRow[]>(() => {
    const buckets: BucketRow[] = [];
    for (let b = 0; b < (24 * 60) / BUCKET_MIN; b++) {
      const start = b * BUCKET_MIN;
      const pts = data.filter((p) => p.t >= start && p.t < start + BUCKET_MIN);
      const avg = pts.length > 0 ? pts.reduce((s, p) => s + p.batteryPower, 0) / pts.length : 0;
      buckets.push({
        t: start,
        carga: avg > 0.005 ? avg : null,
        descarga: avg < -0.005 ? avg : null,
      });
    }
    return buckets;
  }, [data]);

  const dim = (i: number) => (activeIdx === null || activeIdx === i ? 1 : 0.45);

  return (
    <section className="helios-card shadow-card dark:shadow-card-dark" aria-label={t('bateria.powerAria')}>
      <div className="flex flex-wrap items-center gap-2 px-4 pb-1 pt-4 sm:px-5">
        <div className="mr-auto">
          <h2 className="text-[15px] font-semibold text-app">{t('bateria.powerTitle')}</h2>
          <p className="text-xs text-faint">{t('bateria.powerSubtitle')}</p>
        </div>
        <span className="inline-flex items-center gap-1.5 text-xs text-muted">
          <span className="h-2 w-2 rounded-full" style={{ backgroundColor: palette.bateria }} /> {t('bateria.charge')}
        </span>
        <span className="inline-flex items-center gap-1.5 text-xs text-muted">
          <span className="h-2 w-2 rounded-full" style={{ backgroundColor: palette.solar }} /> {t('bateria.discharge')}
        </span>
      </div>

      <div className="px-1 pb-2">
        <ResponsiveContainer width="100%" height={height} className="max-lg:!h-[220px]">
          <BarChart
            data={rows}
            margin={{ top: 10, right: 12, bottom: 0, left: 4 }}
            onMouseMove={(s) => setActiveIdx(typeof s?.activeTooltipIndex === 'number' ? s.activeTooltipIndex : null)}
            onMouseLeave={() => setActiveIdx(null)}
          >
            <CartesianGrid vertical={false} stroke="var(--line)" strokeOpacity={0.6} strokeDasharray="3 6" />
            <XAxis
              dataKey="t"
              type="number"
              domain={[0, 1440]}
              ticks={X_TICKS}
              tickFormatter={(v: number) => fmtTime(v)}
              tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
              axisLine={false}
              tickLine={false}
            />
            <YAxis
              domain={[-2.6, 2.6]}
              ticks={[-2, -1, 0, 1, 2]}
              width={30}
              tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
              axisLine={false}
              tickLine={false}
            />
            <Tooltip content={<PowerTooltip />} cursor={{ fill: 'var(--surface-2)', opacity: 0.5 }} />
            <ReferenceLine y={0} stroke="var(--line)" strokeWidth={1.5} />
            <Bar dataKey="carga" name={t('bateria.charge')} stackId="p" fill={palette.bateria} radius={[3, 3, 0, 0]} animationDuration={450}>
              {rows.map((r, i) => (
                <Cell key={r.t} opacity={dim(i)} />
              ))}
            </Bar>
            <Bar dataKey="descarga" name={t('bateria.discharge')} stackId="p" fill={palette.solar} radius={[0, 0, 3, 3]} animationDuration={450}>
              {rows.map((r, i) => (
                <Cell key={r.t} opacity={dim(i)} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>
    </section>
  );
}

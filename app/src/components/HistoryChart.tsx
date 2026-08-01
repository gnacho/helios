import { SOLAR_GRADIENT } from '@/lib/colors';
import { useMemo } from 'react';
import {
  Area,
  AreaChart,
  Bar,
  CartesianGrid,
  Cell,
  ComposedChart,
  Line,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { format } from 'date-fns';
import { useTranslation } from 'react-i18next';
import { dateLocale } from '@/i18n';
import type { HistoryDay, PowerPoint } from '@/data/types';
import { STEP_MIN } from '@/data/types';
import type { Period } from '@/lib/historyStats';
import { useEnergyColors } from '@/lib/colors';
import { fmtEnergy, fmtKw, fmtPct, fmtTime } from '@/lib/format';

const AXIS_TICKS = [0, 180, 360, 540, 720, 900, 1080, 1260, 1440];

interface BarRow {
  /** Clave única (fecha ISO o mes). */
  key: string;
  /** Etiqueta del eje X. */
  label: string;
  /** Cabecera del tooltip: `Lun 8` / `julio`. */
  caption: string;
  date: Date;
  production: number;
  consumption: number;
  autoconsumo: number;
  isToday: boolean;
}

interface HistoryChartProps {
  period: Period;
  days: HistoryDay[];
  /** Serie de potencia del día (solo periodo Día). */
  daySeries: PowerPoint[];
  /** "Ahora" simulado en minutos (marcador AHORA si el día es hoy). */
  nowMin: number;
  /** Si el periodo Día mostrado es hoy. */
  isToday: boolean;
  /** "Hoy" mock: marca la barra del día actual al 100 % de opacidad. */
  today: Date;
  height?: number;
  /** Drill-down: click en barra de día. */
  onDrillDay: (date: Date) => void;
  /** Drill-down: click en barra de mes (periodo Año). */
  onDrillMonth: (date: Date) => void;
}

// ── Tooltips ─────────────────────────────────────────────────────────────────

interface TooltipPayload {
  name: string;
  value: number;
  color?: string;
  payload?: BarRow;
}

function BarsTooltip({ active, payload }: { active?: boolean; payload?: TooltipPayload[] }) {
  if (!active || !payload || payload.length === 0) return null;
  const row = payload[0]?.payload;
  if (!row) return null;
  return (
    <div className="rounded-xl border border-app bg-surface/95 px-3 py-2 shadow-lg backdrop-blur-md">
      <p className="mb-1 text-xs font-semibold capitalize text-app">{row.caption}</p>
      <p className="flex items-center gap-1.5 text-xs text-muted">
        <span className="h-2 w-2 rounded-full" style={{ backgroundColor: 'var(--c-solar)' }} />
        Producción <span className="ml-auto pl-3 font-semibold text-app">{fmtEnergy(row.production)} kWh</span>
      </p>
      <p className="flex items-center gap-1.5 text-xs text-muted">
        <span className="h-2 w-2 rounded-full" style={{ backgroundColor: 'var(--c-consumo)' }} />
        Consumo <span className="ml-auto pl-3 font-semibold text-app">{fmtEnergy(row.consumption)} kWh</span>
      </p>
      <p className="flex items-center gap-1.5 text-xs text-muted">
        <span className="h-2 w-2 rounded-full" style={{ backgroundColor: 'var(--c-bateria)' }} />
        Autoconsumo <span className="ml-auto pl-3 font-semibold text-app">{fmtPct(row.autoconsumo)} %</span>
      </p>
    </div>
  );
}

function DayTooltip({ active, payload, label }: { active?: boolean; payload?: TooltipPayload[]; label?: number }) {
  if (!active || !payload || payload.length === 0 || label === undefined) return null;
  return (
    <div className="rounded-xl border border-app bg-surface/95 px-3 py-2 shadow-lg backdrop-blur-md">
      <p className="mb-1 text-xs font-semibold text-app">{fmtTime(label)}</p>
      {payload.map((p) => (
        <p key={p.name} className="flex items-center gap-1.5 text-xs text-muted">
          <span className="h-2 w-2 rounded-full" style={{ backgroundColor: p.color }} />
          {p.name} <span className="ml-auto pl-3 font-semibold text-app">{fmtKw(p.value)} kW</span>
        </p>
      ))}
    </div>
  );
}

// ── Componente ───────────────────────────────────────────────────────────────

/**
 * Gráfica principal del histórico:
 *  - Día: curvas de potencia producción vs consumo (sin replay).
 *  - Semana/Mes: barras agrupadas por día + línea punteada de autoconsumo %.
 *  - Año: mismas barras agregadas por mes (12 grupos).
 * Click en barra → drill-down (Día o Mes).
 */
export default function HistoryChart({
  period,
  days,
  daySeries,
  nowMin,
  isToday,
  today,
  height = 320,
  onDrillDay,
  onDrillMonth,
}: HistoryChartProps) {
  const palette = useEnergyColors();
  const { t, i18n } = useTranslation();

  // Filas de barras: por día (semana/mes) o agregadas por mes (año).
  const barRows = useMemo<BarRow[]>(() => {
    if (period === 'ano') {
      const byMonth = new Map<number, BarRow>();
      for (const d of days) {
        const m = d.date.getMonth();
        let row = byMonth.get(m);
        if (!row) {
          const date = new Date(d.date.getFullYear(), m, 1);
          row = {
            key: `${d.date.getFullYear()}-${m}`,
            label: format(date, 'MMM', { locale: dateLocale() }),
            caption: format(date, 'MMMM', { locale: dateLocale() }),
            date,
            production: 0,
            consumption: 0,
            autoconsumo: 0,
            isToday: false,
          };
          byMonth.set(m, row);
        }
        row.production += d.productionKwh;
        row.consumption += d.consumptionKwh;
        row.autoconsumo += d.autoconsumoPct * d.productionKwh;
      }
      const rows = Array.from(byMonth.values()).sort((a, b) => a.date.getTime() - b.date.getTime());
      for (const r of rows) r.autoconsumo = r.production > 0 ? r.autoconsumo / r.production : 0;
      return rows;
    }
    return days.map((d) => ({
      key: d.date.toISOString(),
      label: period === 'semana' ? format(d.date, 'EEE d', { locale: dateLocale() }) : `${d.date.getDate()}`,
      caption: format(d.date, 'EEE d', { locale: dateLocale() }),
      date: d.date,
      production: d.productionKwh,
      consumption: d.consumptionKwh,
      autoconsumo: d.autoconsumoPct,
      isToday:
        d.date.getFullYear() === today.getFullYear() &&
        d.date.getMonth() === today.getMonth() &&
        d.date.getDate() === today.getDate(),
    }));
  }, [period, days, today, i18n.language]);

  if (period === 'dia') {
    return (
      <ResponsiveContainer width="100%" height={height} className="max-lg:!h-[240px]">
        <AreaChart data={daySeries} margin={{ top: 10, right: 12, bottom: 0, left: 4 }}>
          <defs>
            <linearGradient id="hist-grad-prod" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={palette.solar} stopOpacity={0.25} />
              <stop offset="100%" stopColor={palette.solar} stopOpacity={0} />
            </linearGradient>
            <linearGradient id="hist-grad-cons" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={palette.consumo} stopOpacity={0.18} />
              <stop offset="100%" stopColor={palette.consumo} stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid vertical={false} stroke="var(--line)" strokeOpacity={0.6} strokeDasharray="3 6" />
          <XAxis
            dataKey="t"
            type="number"
            domain={[0, 1440]}
            ticks={AXIS_TICKS}
            tickFormatter={(v: number) => fmtTime(v)}
            tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
            axisLine={false}
            tickLine={false}
          />
          <YAxis
            domain={[0, 7]}
            width={30}
            tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
            axisLine={false}
            tickLine={false}
          />
          <Tooltip content={<DayTooltip />} cursor={{ stroke: 'var(--text-faint)', strokeDasharray: '3 3', strokeOpacity: 0.5 }} />
          <Area
            name={t('common.production')}
            type="monotone"
            dataKey="production"
            stroke={palette.solar}
            strokeWidth={2.5}
            fill="url(#hist-grad-prod)"
            animationDuration={1200}
            dot={false}
            activeDot={{ r: 4 }}
          />
          <Area
            name={t('common.consumption')}
            type="monotone"
            dataKey="consumption"
            stroke={palette.consumo}
            strokeWidth={2}
            fill="url(#hist-grad-cons)"
            animationDuration={1200}
            animationBegin={300}
            dot={false}
            activeDot={{ r: 4 }}
          />
          {isToday && (
            <ReferenceLine
              x={Math.round(nowMin / STEP_MIN) * STEP_MIN}
              stroke={palette.solar}
              strokeWidth={2}
              label={{ value: 'AHORA', position: 'insideTopLeft', fill: SOLAR_GRADIENT.from, fontSize: 10, fontWeight: 700 }}
            />
          )}
        </AreaChart>
      </ResponsiveContainer>
    );
  }

  const handleBarClick = (row: BarRow) => {
    if (period === 'ano') onDrillMonth(row.date);
    else onDrillDay(row.date);
  };

  return (
    <ResponsiveContainer width="100%" height={height} className="max-lg:!h-[240px]">
      <ComposedChart data={barRows} margin={{ top: 10, right: 4, bottom: 0, left: 4 }} barCategoryGap="28%" barGap={3}>
        <CartesianGrid vertical={false} stroke="var(--line)" strokeOpacity={0.6} strokeDasharray="3 6" />
        <XAxis
          dataKey="label"
          tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
          axisLine={false}
          tickLine={false}
          interval={period === 'mes' ? 2 : 0}
        />
        <YAxis
          yAxisId="left"
          width={30}
          tick={{ fontSize: 12, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
          axisLine={false}
          tickLine={false}
        />
        <YAxis
          yAxisId="right"
          orientation="right"
          domain={[0, 100]}
          width={30}
          ticks={[0, 50, 100]}
          tickFormatter={(v: number) => `${v}%`}
          tick={{ fontSize: 11, fill: 'var(--text-faint)', fontFamily: 'Inter' }}
          axisLine={false}
          tickLine={false}
        />
        <Tooltip content={<BarsTooltip />} cursor={{ fill: 'var(--surface-2)', opacity: 0.6 }} />
        <Bar
          yAxisId="left"
          name={t('common.production')}
          dataKey="production"
          fill={palette.solar}
          radius={[4, 4, 0, 0]}
          animationDuration={550}
          onClick={(data) => handleBarClick(data.payload as BarRow)}
          className="cursor-pointer"
        >
          {barRows.map((row) => (
            <Cell key={row.key} fillOpacity={row.isToday ? 1 : 0.7} />
          ))}
        </Bar>
        <Bar
          yAxisId="left"
          name={t('common.consumption')}
          dataKey="consumption"
          fill={palette.consumo}
          radius={[4, 4, 0, 0]}
          animationDuration={550}
          animationBegin={80}
          onClick={(data) => handleBarClick(data.payload as BarRow)}
          className="cursor-pointer"
        >
          {barRows.map((row) => (
            <Cell key={row.key} fillOpacity={row.isToday ? 1 : 0.7} />
          ))}
        </Bar>
        <Line
          yAxisId="right"
          name={t('common.autoconsumo')}
          type="monotone"
          dataKey="autoconsumo"
          stroke={palette.bateria}
          strokeWidth={2}
          strokeDasharray="5 4"
          dot={{ r: 2.5, fill: palette.bateria, strokeWidth: 0 }}
          animationDuration={900}
          animationBegin={400}
        />
      </ComposedChart>
    </ResponsiveContainer>
  );
}

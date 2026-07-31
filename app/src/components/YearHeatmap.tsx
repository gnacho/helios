import { useEffect, useMemo, useRef, useState } from 'react';
import { motion } from 'framer-motion';
import { format } from 'date-fns';
import { useTranslation } from 'react-i18next';
import { dateLocale, fmtDayMonth, fmtDayMonthLong } from '@/i18n';
import type { HistoryDay } from '@/data/types';
import { useEnergyColors } from '@/lib/colors';
import { fmtEnergy } from '@/lib/format';

const CELL = 12;
const GAP = 3;
const PITCH = CELL + GAP;
const LEFT = 22; // etiquetas L/X/V
const TOP = 18; // etiquetas de mes
const COLS = 53;

interface HeatCell {
  key: string;
  col: number;
  row: number;
  date: Date;
  kwh: number;
  /** Nivel 0–4 de intensidad. */
  level: number;
}

interface YearHeatmapProps {
  /** 365 días sintéticos terminando "hoy". */
  days: HistoryDay[];
  today: Date;
  /** Drill-down al periodo Día de esa fecha. */
  onSelectDay: (date: Date) => void;
}

/** Días L/X/V en las filas 0/2/4 (semana empieza en lunes). */
const ROW_LABELS: { row: number; key: string }[] = [
  { row: 0, key: 'historico.wdMon' },
  { row: 2, key: 'historico.wdWed' },
  { row: 4, key: 'historico.wdFri' },
];

/**
 * Heatmap anual tipo GitHub (53 × 7, celdas 12px): intensidad amber según la
 * producción diaria. Hover → tooltip; click → drill-down al día. En móvil el
 * contenedor hace scroll horizontal y arranca mostrando los últimos meses.
 */
export default function YearHeatmap({ days, today, onSelectDay }: YearHeatmapProps) {
  const palette = useEnergyColors();
  const { t, i18n } = useTranslation();
  const scrollRef = useRef<HTMLDivElement>(null);
  const [hovered, setHovered] = useState<HeatCell | null>(null);
  const [tooltipPos, setTooltipPos] = useState<{ x: number; y: number } | null>(null);

  const { cells, monthLabels } = useMemo(() => {
    const max = Math.max(1, ...days.map((d) => d.productionKwh));
    const start = days[0]?.date;
    const cells: HeatCell[] = [];
    const monthLabels: { col: number; label: string }[] = [];
    if (!start) return { cells, monthLabels };

    // Lunes de la semana que contiene el primer día.
    const firstMonday = new Date(start.getFullYear(), start.getMonth(), start.getDate());
    firstMonday.setDate(firstMonday.getDate() - ((firstMonday.getDay() + 6) % 7));

    let prevMonth = -1;
    for (const d of days) {
      const monday = new Date(d.date.getFullYear(), d.date.getMonth(), d.date.getDate());
      monday.setDate(monday.getDate() - ((monday.getDay() + 6) % 7));
      const col = Math.round((monday.getTime() - firstMonday.getTime()) / (7 * 86_400_000));
      const row = (d.date.getDay() + 6) % 7;
      const level = Math.min(4, Math.max(1, Math.ceil((d.productionKwh / max) * 4)));
      cells.push({
        key: d.date.toISOString(),
        col: Math.min(col, COLS - 1),
        row,
        date: d.date,
        kwh: d.productionKwh,
        level,
      });
      // Etiqueta de mes cuando cambia el mes del lunes de la columna.
      if (monday.getMonth() !== prevMonth) {
        monthLabels.push({ col: Math.min(col, COLS - 1), label: format(monday, 'MMM', { locale: dateLocale() }) });
        prevMonth = monday.getMonth();
      }
    }
    return { cells, monthLabels };
  }, [days, i18n.language]);

  // En móvil, el scroll arranca a la derecha (últimos meses).
  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollLeft = el.scrollWidth;
  }, []);

  const levelOpacity = [0, 0.28, 0.48, 0.72, 1];
  const fillFor = (level: number) => (level === 0 ? 'var(--surface-2)' : palette.solar);

  const showTooltip = (cell: HeatCell, target: SVGRectElement) => {
    const container = scrollRef.current;
    if (!container) return;
    const cRect = container.getBoundingClientRect();
    const r = target.getBoundingClientRect();
    setTooltipPos({ x: r.left - cRect.left + r.width / 2 + container.scrollLeft, y: r.top - cRect.top });
    setHovered(cell);
  };

  const width = LEFT + COLS * PITCH;
  const height = TOP + 7 * PITCH - GAP;

  return (
    <div ref={scrollRef} className="relative overflow-x-auto pb-1" role="img" aria-label={t('historico.heatmapAria')}>
      <svg width={width} height={height} className="block" style={{ minWidth: width }}>
        {/* Etiquetas de mes */}
        {monthLabels.map((m) => (
          <text
            key={`${m.col}-${m.label}`}
            x={LEFT + m.col * PITCH}
            y={10}
            fontSize={10}
            fill="var(--text-faint)"
            fontFamily="Inter"
          >
            {m.label}
          </text>
        ))}
        {/* Etiquetas L/X/V */}
        {ROW_LABELS.map((r) => (
          <text
            key={r.key}
            x={0}
            y={TOP + r.row * PITCH + CELL - 2}
            fontSize={9}
            fill="var(--text-faint)"
            fontFamily="Inter"
          >
            {t(r.key)}
          </text>
        ))}
        {/* Celdas (cascada por columnas: ~8ms por columna) */}
        {cells.map((c) => {
          const isHovered = hovered?.key === c.key;
          const isTodayCell =
            c.date.getFullYear() === today.getFullYear() &&
            c.date.getMonth() === today.getMonth() &&
            c.date.getDate() === today.getDate();
          return (
            <motion.rect
              key={c.key}
              x={LEFT + c.col * PITCH}
              y={TOP + c.row * PITCH}
              width={CELL}
              height={CELL}
              rx={3}
              fill={fillFor(c.level)}
              fillOpacity={c.level === 0 ? 1 : levelOpacity[c.level]}
              stroke={isHovered ? 'var(--text)' : isTodayCell ? 'var(--text-faint)' : 'none'}
              strokeWidth={isHovered || isTodayCell ? 1 : 0}
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ delay: c.col * 0.008, duration: 0.18 }}
              whileHover={{ scale: 1.25 }}
              style={{ transformBox: 'fill-box', transformOrigin: 'center', cursor: 'pointer' }}
              role="button"
              tabIndex={0}
              aria-label={t('historico.cellAria', { date: fmtDayMonthLong(c.date), kwh: fmtEnergy(c.kwh) })}
              onMouseEnter={(e) => showTooltip(c, e.currentTarget as unknown as SVGRectElement)}
              onMouseLeave={() => setHovered(null)}
              onFocus={(e) => showTooltip(c, e.currentTarget as unknown as SVGRectElement)}
              onBlur={() => setHovered(null)}
              onClick={() => onSelectDay(c.date)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  onSelectDay(c.date);
                }
              }}
            />
          );
        })}
      </svg>

      {/* Tooltip de celda: `12 jul · 42,8 kWh` */}
      {hovered && tooltipPos && (
        <div
          className="pointer-events-none absolute z-10 -translate-x-1/2 whitespace-nowrap rounded-lg border border-app bg-surface px-2 py-1 text-[11px] font-semibold text-app shadow-lg"
          style={{ left: tooltipPos.x, top: Math.max(0, tooltipPos.y - 30) }}
        >
          {fmtDayMonth(hovered.date)} · {fmtEnergy(hovered.kwh)} kWh
        </div>
      )}

      {/* Leyenda de intensidad */}
      <div className="mt-2 flex items-center justify-end gap-1 pr-1 text-[10px] text-faint">
        <span className="mr-1">{t('historico.less')}</span>
        {levelOpacity.map((op, i) => (
          <span
            key={i}
            className="inline-block h-[10px] w-[10px] rounded-[3px]"
            style={{ backgroundColor: i === 0 ? 'var(--surface-2)' : palette.solar, opacity: i === 0 ? 1 : op }}
          />
        ))}
        <span className="ml-1">{t('historico.more')}</span>
      </div>
    </div>
  );
}

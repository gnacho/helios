import { useNavigate } from 'react-router';
import { motion } from 'framer-motion';
import { Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import Sparkline from '@/components/Sparkline';
import { numLocale } from '@/i18n';
import { fmtEnergy, fmtKw } from '@/lib/format';
import { useAnimatedNumber } from '@/lib/useAnimatedNumber';

interface InverterCardProps {
  name: string;
  model: string;
  kwp: number;
  /** kW actuales del inversor. */
  nowKw: number;
  /** kWh acumulados hoy. */
  todayKwh: number;
  /** Serie del día del inversor (kW cada 5 min) para el sparkline. */
  series: number[];
  /** Porcentaje que aporta al total de hoy (0–100). */
  sharePct: number;
  color: string;
  tab: string;
  index?: number;
}

/** Mini-tarjeta de inversor con sparkline y barra de proporción. */
export default function InverterCard({
  name,
  model,
  kwp,
  nowKw,
  todayKwh,
  series,
  sharePct,
  color,
  tab,
  index = 0,
}: InverterCardProps) {
  const navigate = useNavigate();
  const { t } = useTranslation();
  const animatedNow = useAnimatedNumber(nowKw);
  const kwpFmt = new Intl.NumberFormat(numLocale(), { minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(kwp);

  return (
    <motion.button
      initial={{ opacity: 0, y: 24 }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, amount: 0.2 }}
      transition={{ duration: 0.5, delay: index * 0.12, ease: [0.25, 1, 0.5, 1] }}
      onClick={() => navigate(`/inversores?tab=${tab}`)}
      className="helios-card group w-full p-5 text-left shadow-card hover:-translate-y-0.5 hover:shadow-lg active:scale-[0.98] dark:shadow-card-dark"
    >
      <div className="flex items-center gap-3">
        <span className="flex h-9 w-9 items-center justify-center rounded-xl" style={{ backgroundColor: `${color}1F`, color }}>
          <Zap size={18} strokeWidth={2.2} />
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate text-[15px] font-semibold text-app">{name}</p>
          <p className="text-xs text-faint">{model}</p>
        </div>
        <span className="inline-flex items-center gap-1.5 rounded-full bg-emerald-500/12 px-2 py-0.5 text-[11px] font-semibold text-emerald-600 dark:text-emerald-400">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" /> {t('common.online')}
        </span>
        <span className="hidden text-xs font-medium text-faint sm:inline">{kwpFmt} kWp</span>
      </div>

      <div className="mt-4 flex items-end justify-between gap-4">
        <div>
          <p className="font-display text-2xl font-semibold tracking-[-0.01em]" style={{ color }} aria-live="off">
            {fmtKw(animatedNow)}
            <span className="ml-1 text-[0.6em] font-medium text-faint">kW</span>
          </p>
          <p className="mt-0.5 text-sm text-muted">
            {t('inverter.today')} <span className="font-semibold text-app">{fmtEnergy(todayKwh)} kWh</span>
          </p>
        </div>
        <div className="w-[140px] shrink-0 transition-[filter] duration-200 group-hover:brightness-110">
          <Sparkline data={series} color={color} height={48} />
        </div>
      </div>

      <div className="mt-4">
        <div className="flex items-center justify-between text-[11px] font-medium text-faint">
          <span>{t('inverter.shareToday')}</span>
          <span className="font-semibold" style={{ color }}>
            {Math.round(sharePct)} %
          </span>
        </div>
        <div className="mt-1 h-1 w-full overflow-hidden rounded-full bg-surface-2">
          <motion.div
            className="h-full rounded-full"
            style={{ backgroundColor: color }}
            initial={{ width: 0 }}
            whileInView={{ width: `${sharePct}%` }}
            viewport={{ once: true }}
            transition={{ duration: 0.8, ease: 'easeOut', delay: 0.6 }}
          />
        </div>
      </div>
    </motion.button>
  );
}

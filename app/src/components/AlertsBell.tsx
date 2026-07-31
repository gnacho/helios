import { Bell } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { useEnergyData } from '@/data/EnergyDataProvider';
import { cn } from '@/lib/utils';

export default function AlertsBell() {
  const { getLivePower } = useEnergyData();
  const { t } = useTranslation();
  const alerts = getLivePower().alerts ?? [];
  const top = alerts.some((a) => a.severity === 'critical')
    ? 'bg-rose-500'
    : alerts.some((a) => a.severity === 'warning')
      ? 'bg-amber-500'
      : 'bg-sky-500';

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          aria-label={t('alerts.label', { count: alerts.length })}
          className="relative rounded-full border border-app bg-surface p-2 text-muted transition-colors hover:text-app"
        >
          <Bell size={16} />
          {alerts.length > 0 && <span className={cn('absolute right-1.5 top-1.5 h-1.5 w-1.5 rounded-full', top)} />}
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-72">
        {alerts.length === 0 ? (
          <div className="px-3 py-2.5 text-xs text-muted">{t('alerts.empty')}</div>
        ) : (
          alerts.map((a) => (
            <div key={a.id} className="flex items-start gap-2 px-3 py-2">
              <span
                className={cn(
                  'mt-1 h-2 w-2 shrink-0 rounded-full',
                  a.severity === 'critical' ? 'bg-rose-500' : a.severity === 'warning' ? 'bg-amber-500' : 'bg-sky-500',
                )}
              />
              <span className="text-xs leading-snug text-app">{a.text}</span>
            </div>
          ))
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

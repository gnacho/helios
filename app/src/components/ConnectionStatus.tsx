import { useTranslation } from 'react-i18next';
import { useEnergyData } from '@/data/EnergyDataProvider';
import { LIVE_STALE_MS } from '@/data/types';
import { cn } from '@/lib/utils';

/**
 * Pill de estado de conexión HAOS.
 * conectado (verde) · reconectando (amber) · demo (violeta, "Modo demo").
 * Si el SSE lleva más de LIVE_STALE_MS en silencio (conexión muerta sin onerror,
 * típico en móvil suspendido), gana el aviso "sin datos" aunque el navegador
 * siga creyendo que está conectado.
 */
export default function ConnectionStatus({ compact = false }: { compact?: boolean }) {
  const { connectionStatus, liveUpdatedAt, now } = useEnergyData();
  const { t } = useTranslation();

  const ageSec = liveUpdatedAt ? Math.floor((now.getTime() - liveUpdatedAt) / 1000) : null;
  const stale = connectionStatus !== 'demo' && ageSec !== null && ageSec * 1000 > LIVE_STALE_MS;

  const staleText = (secs: number) =>
    secs < 60
      ? t('connection.staleSec', { count: secs })
      : secs < 3600
        ? t('connection.staleMin', { count: Math.floor(secs / 60) })
        : t('connection.staleHour', { count: Math.floor(secs / 3600) });

  const config = stale
    ? { dot: 'bg-amber-500', text: staleText(ageSec), ping: true }
    : {
        connected: { dot: 'bg-emerald-500', text: t('connection.connected'), ping: true },
        reconnecting: { dot: 'bg-amber-500', text: t('connection.reconnecting'), ping: true },
        demo: { dot: 'bg-violet-500', text: t('connection.demo'), ping: false },
      }[connectionStatus];

  return (
    <span
      className={cn(
        'inline-flex h-8 items-center gap-2 rounded-full border border-app bg-surface px-3',
        'text-xs font-medium text-muted',
      )}
      title={t('connection.title')}
    >
      <span className="relative flex h-2 w-2">
        {config.ping && (
          <span className={cn('absolute inline-flex h-full w-full rounded-full opacity-60 animate-ping-soft', config.dot)} />
        )}
        <span className={cn('relative inline-flex h-2 w-2 rounded-full', config.dot)} />
      </span>
      {!compact && <span>{config.text}</span>}
      {compact && <span className="sr-only">{config.text}</span>}
    </span>
  );
}

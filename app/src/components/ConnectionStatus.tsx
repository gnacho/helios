import { useEnergyData } from '@/data/EnergyDataProvider';
import { cn } from '@/lib/utils';

/**
 * Pill de estado de conexión HAOS.
 * conectado (verde) · reconectando (amber) · demo (violeta, "Modo demo").
 */
export default function ConnectionStatus({ compact = false }: { compact?: boolean }) {
  const { connectionStatus } = useEnergyData();

  const config = {
    connected: { dot: 'bg-emerald-500', text: 'HAOS · Local', ping: true },
    reconnecting: { dot: 'bg-amber-500', text: 'Reconectando…', ping: true },
    demo: { dot: 'bg-violet-500', text: 'Modo demo', ping: false },
  }[connectionStatus];

  return (
    <span
      className={cn(
        'inline-flex h-8 items-center gap-2 rounded-full border border-app bg-surface px-3',
        'text-xs font-medium text-muted',
      )}
      title="Estado de la conexión con Home Assistant"
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

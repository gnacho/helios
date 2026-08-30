import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { RefreshCw, X } from 'lucide-react';
import { apiFetch } from '@/data/api-client';
import { UpdateDialog } from '@/components/UpdateDialog';
import { CHECK_KEY, CHECK_INTERVAL, DISMISS_KEY, getDismissed, onRibbonSignal } from '@/lib/update-check';

const REPO_URL = 'https://github.com/gnacho/helios';

export default function UpdateRibbon() {
  const { t } = useTranslation();
  const [state, setState] = useState<'idle' | 'checking' | 'uptodate' | 'available' | 'error'>('idle');
  const [latestVersion, setLatestVersion] = useState('');
  const [dialogOpen, setDialogOpen] = useState(false);

  useEffect(() => {
    let stale = false;
    const run = async () => {
      try {
        const me = await apiFetch<{ user?: { role?: string } }>('/api/auth/me').catch(() => null);
        if (me?.user?.role !== 'admin') return;

        const last = Number(window.localStorage.getItem(CHECK_KEY) || 0);
        if (Date.now() - last < CHECK_INTERVAL) return;
        window.localStorage.setItem(CHECK_KEY, String(Date.now()));

        setState('checking');
        const status = await apiFetch<{ current: string; latest: string; available: boolean }>('/api/update/status');
        if (stale) return;
        if (!status?.available || !status.latest) {
          setState('uptodate');
        } else if (status.latest === getDismissed()) {
          setState('uptodate');
        } else {
          setLatestVersion(status.latest);
          setState('available');
        }
      } catch {
        if (!stale) setState('error');
      }
    };
    void run();
    const off = onRibbonSignal((latest) => {
      if (stale) return;
      if (latest !== getDismissed()) {
        setLatestVersion(latest);
        setState('available');
      }
    });
    return () => { stale = true; off(); };
  }, []);

  const dismissVersion = useCallback(() => {
    try { window.localStorage.setItem(DISMISS_KEY, latestVersion); } catch { /* sin storage */ }
    setState('uptodate');
  }, [latestVersion]);

  if (state !== 'available') return null;

  return (
    <>
      <div
        role="status"
        className="mb-4 flex items-center gap-2.5 rounded-xl border border-amber-500/35 bg-amber-500/10 px-3.5 py-2.5 text-[13px] font-semibold text-amber-500"
      >
        <RefreshCw className="h-4 w-4 shrink-0" />
        <span>{t('ajustes.about.updateAvailable', { version: latestVersion })}</span>
        <a
          href={`${REPO_URL}/releases`}
          target="_blank"
          rel="noreferrer"
          className="hidden h-8 shrink-0 items-center rounded-lg border border-amber-500/40 px-3 text-xs font-medium text-amber-500 transition-colors hover:bg-amber-500/15 sm:flex"
        >
          {t('ajustes.about.viewRelease')}
        </a>
        <button
          type="button"
          onClick={() => setDialogOpen(true)}
          className="ml-auto flex h-8 shrink-0 items-center rounded-lg border border-amber-500/40 bg-amber-500 px-3 text-xs font-medium text-white transition-colors hover:brightness-110"
        >
          {t('ajustes.about.updateNow')}
        </button>
        <button
          type="button"
          onClick={dismissVersion}
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-amber-500/40 text-amber-500 transition-colors hover:bg-amber-500/10"
          aria-label={t('ajustes.about.dismiss')}
        >
          <X className="h-4 w-4" />
        </button>
      </div>
      <UpdateDialog open={dialogOpen} onOpenChange={setDialogOpen} />
    </>
  );
}

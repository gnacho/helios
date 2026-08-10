import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { RefreshCw, X } from 'lucide-react';
import { apiFetch, apiPost } from '@/data/api-client';
import pkg from '../../package.json';

const CHECK_KEY = 'helios-last-update-check';
const DISMISS_KEY = 'helios-release-dismissed';
const CHECK_INTERVAL = 7 * 24 * 60 * 60 * 1000;
const REPO_URL = 'https://github.com/gnacho/helios';

function compareSemver(a: string, b: string): number {
  const pa = a.replace(/^v/, '').split('.').map((n) => parseInt(n, 10) || 0);
  const pb = b.replace(/^v/, '').split('.').map((n) => parseInt(n, 10) || 0);
  for (let i = 0; i < 3; i++) {
    if ((pa[i] || 0) !== (pb[i] || 0)) return (pa[i] || 0) - (pb[i] || 0);
  }
  return 0;
}

function getDismissed(): string {
  try { return window.localStorage.getItem(DISMISS_KEY) ?? ''; } catch { return ''; }
}

export default function UpdateRibbon() {
  const { t } = useTranslation();
  const [state, setState] = useState<'idle' | 'checking' | 'uptodate' | 'available' | 'error'>('idle');
  const [latestVersion, setLatestVersion] = useState('');
  const [applying, setApplying] = useState(false);

  useEffect(() => {
    let stale = false;
    const run = async () => {
      try {
        const me = await apiFetch<{ user?: { role?: string } }>('/api/auth/me').catch(() => null);
        if (me?.user?.role !== 'admin') return;

        const last = Number(window.localStorage.getItem(CHECK_KEY) || 0);
        if (Date.now() - last < CHECK_INTERVAL) return;
        window.localStorage.setItem(CHECK_KEY, String(Date.now()));

        const repo = REPO_URL.match(/github\.com\/([^/]+\/[^/.]+)/)?.[1];
        if (!repo) return;
        setState('checking');
        const res = await fetch(`https://api.github.com/repos/${repo}/releases/latest`, {
          headers: { Accept: 'application/vnd.github+json' },
        });
        let version = '';
        if (res.ok) {
          const data = (await res.json()) as { tag_name?: string };
          version = data.tag_name || '';
        } else if (res.status === 404) {
          const tagRes = await fetch(`https://api.github.com/repos/${repo}/tags?per_page=1`, {
            headers: { Accept: 'application/vnd.github+json' },
          });
          if (tagRes.ok) {
            const tags = (await tagRes.json()) as { name?: string }[];
            version = tags[0]?.name || '';
          }
        }
        if (stale) return;
        const ver = version.replace(/^v/, '');
        if (!version || compareSemver(version, pkg.version) <= 0) setState('uptodate');
        else if (ver === getDismissed()) setState('uptodate');
        else {
          setLatestVersion(ver);
          setState('available');
        }
      } catch {
        if (!stale) setState('error');
      }
    };
    void run();
    return () => { stale = true; };
  }, []);

  const dismissVersion = useCallback(() => {
    try { window.localStorage.setItem(DISMISS_KEY, latestVersion); } catch { /* sin storage */ }
    setState('uptodate');
  }, [latestVersion]);

  if (state !== 'available') return null;

  const apply = async () => {
    if (applying) return;
    setApplying(true);
    try {
      await apiPost<{ ok: boolean }>('/api/update/apply');
      setTimeout(() => window.location.reload(), 2500);
    } catch {
      setApplying(false);
    }
  };

  return (
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
        onClick={() => void apply()}
        disabled={applying}
        className="ml-auto flex h-8 shrink-0 items-center rounded-lg border border-amber-500/40 bg-amber-500 px-3 text-xs font-medium text-white transition-colors hover:brightness-110 disabled:opacity-60"
      >
        {applying ? t('ajustes.about.applying') : t('ajustes.about.updateNow')}
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
  );
}

const CHECK_KEY = 'helios-last-update-check';
const DISMISS_KEY = 'helios-release-dismissed';
const CHECK_INTERVAL = 4 * 60 * 60 * 1000;
const EVENT_NAME = 'helios-update-available';

export { CHECK_KEY, DISMISS_KEY, CHECK_INTERVAL };

export function invalidateThrottle(): void {
  try { window.localStorage.removeItem(CHECK_KEY); } catch { /* no storage */ }
}

export function notifyRibbon(latest: string): void {
  invalidateThrottle();
  window.dispatchEvent(new CustomEvent(EVENT_NAME, { detail: { latest } }));
}

export function getDismissed(): string {
  try { return window.localStorage.getItem(DISMISS_KEY) ?? ''; } catch { return ''; }
}

export function onRibbonSignal(cb: (latest: string) => void): () => void {
  const handler = (e: Event) => {
    const detail = (e as CustomEvent<{ latest: string }>).detail;
    if (detail?.latest) cb(detail.latest);
  };
  window.addEventListener(EVENT_NAME, handler);
  return () => window.removeEventListener(EVENT_NAME, handler);
}

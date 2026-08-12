// apply-update.ts — apply de actualización async (patrón app-auto-update).
// El server escribe un flag y un systemd .path lanza helios-update.sh (root);
// aquí se sondea /api/version hasta que el build (release-id del marker)
// CAMBIA — el server se reinicia con el código nuevo — o timeout. Devuelve
// true si se confirmó el cambio de build.
import { apiFetch } from '@/data/api-client';

const POLL_MS = 2000;
const TIMEOUT_MS = 90 * 1000; // ~90s (el deploy del script + restart tardan ~30-60s)

export async function applyRelease(): Promise<boolean> {
  const before = await apiFetch<{ build?: string }>('/api/version').catch(() => ({ build: '' }));
  await apiFetch<{ ok: boolean }>('/api/update/apply', { method: 'POST' }); // 202: flag escrito
  const started = Date.now();
  let done = false;
  while (!done && Date.now() - started < TIMEOUT_MS) {
    await new Promise((r) => setTimeout(r, POLL_MS));
    try {
      const v = await apiFetch<{ build?: string }>('/api/version');
      // build = release-id del marker; cambia tras cada deploy. Confirmamos
      // cuando difiere del build previo al apply (server ya reinició).
      if (v?.build && v.build !== before.build) done = true;
    } catch {
      /* el server puede estar reiniciándose: seguir sondeando */
    }
  }
  return done;
}

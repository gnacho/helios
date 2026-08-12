// update.js — estado y aplicación de actualizaciones (patrón Keynest/Deltos,
// skill app-auto-update): detecta la última release ESTABLE del repo
// (releases/latest, tag v*) y, si hay versión nueva, la aplica ejecutando
// helios-update.sh (deploy/, versionado en el repo: releases + checksums +
// marker semver). El server NO se auto-aplica en runtime: el endpoint escribe
// un flag en el dir de datos (escribible) y un systemd .path
// (helios-update.path) lo detecta y lanza helios-update.service (root, oneshot)
// on-demand. El script hace el deploy + systemctl restart. El apply es
// asíncrono: el front sondea /api/version hasta que el build cambia.
import { writeFileSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { kvGet, kvSet } from './db.js'

const REPO = process.env.GITHUB_REPO || 'gnacho/helios'
const MARKER = process.env.RELEASE_MARKER || '/opt/helios/.release-id'
const CACHE_KEY = 'gh_latest_release'
const CACHE_TTL = 5 * 60 * 1000

// Versión semver instalada (marker lo escribe helios-update.sh tras cada deploy).
export function currentId() {
  try {
    return readFileSync(MARKER, 'utf8').trim()
  } catch {
    return ''
  }
}

// Última release ESTABLE (releases/latest, tag v*), no la prerelease "latest"
// de main. Caché en kv con TTL 5 min para no pegar a la API de GitHub en cada
// llamada (rate-limit 60/h por IP sin token).
async function latestId(db) {
  const cached = kvGet(db, CACHE_KEY)
  if (cached) {
    try {
      const c = JSON.parse(cached)
      if (Date.now() - c.at < CACHE_TTL) return c.id
    } catch { /* noop */ }
  }
  const res = await fetch(`https://api.github.com/repos/${REPO}/releases/latest`, {
    headers: { 'User-Agent': 'helios-updater', Accept: 'application/vnd.github+json' },
    signal: AbortSignal.timeout(10000),
  })
  if (!res.ok) return null
  const data = await res.json()
  const id = String(data.tag_name ?? '').replace(/^v/, '')
  kvSet(db, CACHE_KEY, JSON.stringify({ at: Date.now(), id }))
  return id
}

// Comparación semver numérica: '0.10.0' > '0.9.0'; prefijos 'v' ignorados.
function compareSemver(a, b) {
  const pa = String(a).replace(/^v/, '').split('.').map((n) => parseInt(n, 10) || 0)
  const pb = String(b).replace(/^v/, '').split('.').map((n) => parseInt(n, 10) || 0)
  for (let i = 0; i < 3; i++) {
    if ((pa[i] || 0) !== (pb[i] || 0)) return (pa[i] || 0) - (pb[i] || 0)
  }
  return 0
}

export async function updateStatus(db) {
  const current = currentId()
  const latest = await latestId(db).catch(() => null)
  const available = Boolean(latest && current && compareSemver(latest, current) > 0)
  return { current, latest, available }
}

// El endpoint de apply NO ejecuta el script directamente (el servicio va
// sandboxeado: User=helios + ProtectSystem=full + NoNewPrivileges, así que un
// hijo hereda el sandbox y no puede escribir /opt/helios ni systemctl restart).
// En su lugar escribe un flag en el dir de datos (escribible); un systemd
// .path (helios-update.path) lo detecta y lanza helios-update.service (root,
// oneshot) on-demand. Devuelve true si el flag se escribió.
export function requestUpdate(dataDir) {
  const flag = join(dataDir, '.update-requested')
  try {
    writeFileSync(flag, new Date().toISOString())
    return true
  } catch {
    return false
  }
}

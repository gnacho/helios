import { writeFileSync, readFileSync, watchFile, unwatchFile } from 'node:fs'
import { join } from 'node:path'
import { kvGet, kvSet } from './db.js'

const REPO = process.env.GITHUB_REPO || 'gnacho/helios'
const MARKER = process.env.RELEASE_MARKER || '/opt/helios/.release-id'
const CACHE_KEY = 'gh_latest_release'
const CACHE_TTL = 5 * 60 * 1000
const PROGRESS_FILE = 'update-progress.json'
const PROGRESS_STALE_MS = 15 * 60 * 1000

const listeners = new Set()

export function subscribe(fn) {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

function broadcast(payload) {
  for (const fn of listeners) {
    try { fn(payload) } catch { /* listener muerto */ }
  }
}

export function currentId() {
  try {
    return readFileSync(MARKER, 'utf8').trim()
  } catch {
    return ''
  }
}

async function latestRelease(db) {
  const cached = kvGet(db, CACHE_KEY)
  if (cached) {
    try {
      const c = JSON.parse(cached)
      if (Date.now() - c.at < CACHE_TTL) return c
    } catch { /* noop */ }
  }
  const res = await fetch(`https://api.github.com/repos/${REPO}/releases/latest`, {
    headers: { 'User-Agent': 'helios-updater', Accept: 'application/vnd.github+json' },
    signal: AbortSignal.timeout(10000),
  })
  if (!res.ok) return null
  const data = await res.json()
  const id = String(data.tag_name ?? '').replace(/^v/, '')
  const body = String(data.body ?? '').trim()
  const entry = { at: Date.now(), id, body }
  kvSet(db, CACHE_KEY, JSON.stringify(entry))
  return entry
}

function compareSemver(a, b) {
  const pa = String(a).replace(/^v/, '').split('.').map((n) => parseInt(n, 10) || 0)
  const pb = String(b).replace(/^v/, '').split('.').map((n) => parseInt(n, 10) || 0)
  for (let i = 0; i < 3; i++) {
    if ((pa[i] || 0) !== (pb[i] || 0)) return (pa[i] || 0) - (pb[i] || 0)
  }
  return 0
}

export function readProgress(dataDir) {
  try {
    const raw = readFileSync(join(dataDir, PROGRESS_FILE), 'utf8')
    const p = JSON.parse(raw)
    if (p.ts && Date.now() - p.ts > PROGRESS_STALE_MS) return null
    return p
  } catch {
    return null
  }
}

export async function updateStatus(db, dataDir) {
  const current = currentId()
  const rel = await latestRelease(db).catch(() => null)
  const latest = rel?.id ?? null
  const latestBody = rel?.body ?? null
  const available = Boolean(latest && current && compareSemver(latest, current) > 0)
  const progress = dataDir ? readProgress(dataDir) : null
  const updating = progress ? { step: progress.step, progress: progress.pct ?? 0 } : false
  return {
    current,
    latest,
    latestBody,
    available,
    canApply: available && !updating,
    updating,
    repo: REPO,
  }
}

export function requestUpdate(dataDir) {
  const flag = join(dataDir, '.update-requested')
  try {
    writeFileSync(flag, new Date().toISOString())
    return true
  } catch {
    return false
  }
}

export function watchProgress(dataDir) {
  const file = join(dataDir, PROGRESS_FILE)
  let lastMtime = 0
  watchFile(file, { interval: 500 }, (curr) => {
    if (curr.mtimeMs <= lastMtime) return
    lastMtime = curr.mtimeMs
    const p = readProgress(dataDir)
    if (p) broadcast({ type: 'progress', step: p.step, pct: p.pct ?? 0 })
  })
  return () => unwatchFile(file)
}

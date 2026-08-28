import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  AlertTriangle,
  ArrowRight,
  Check,
  Circle,
  DownloadCloud,
  Loader2,
} from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Progress } from '@/components/ui/progress'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { apiFetch } from '@/data/api-client'

export interface UpdateStatusInfo {
  current: string
  latest: string | null
  latestBody?: string | null
  available: boolean
  canApply: boolean
  updating: false | { step: string; progress?: number }
  error?: string | null
}

const STEP_ORDER = ['fetch', 'download', 'verify', 'install', 'restart'] as const

type Phase = 'confirm' | 'progress' | 'restarting' | 'error'

interface UpdateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  initialStatus?: UpdateStatusInfo | null
}

export function UpdateDialog({ open, onOpenChange, initialStatus }: UpdateDialogProps) {
  const { t } = useTranslation()
  const [phase, setPhase] = useState<Phase>('confirm')
  const [status, setStatus] = useState<UpdateStatusInfo | null>(initialStatus ?? null)
  const [step, setStep] = useState<string>('fetch')
  const [pct, setPct] = useState<number>(0)
  const [errorCode, setErrorCode] = useState<string | null>(null)
  const [ackDowntime, setAckDowntime] = useState(false)

  const esRef = useRef<EventSource | null>(null)
  const pollRef = useRef<number | null>(null)
  const uptimeRef = useRef<number>(Number.MAX_SAFE_INTEGER)
  const phaseRef = useRef<Phase>('confirm')
  const fetchFailRef = useRef(0)

  const setPhaseBoth = (p: Phase) => {
    phaseRef.current = p
    setPhase(p)
  }

  const closeStream = useCallback(() => {
    if (esRef.current) {
      esRef.current.close()
      esRef.current = null
    }
    if (pollRef.current !== null) {
      window.clearInterval(pollRef.current)
      pollRef.current = null
    }
  }, [])

  useEffect(() => closeStream, [closeStream])

  const waitAndReload = useCallback(() => {
    if (phaseRef.current === 'restarting') return
    setPhaseBoth('restarting')
    const deadline = Date.now() + 90_000
    const id = window.setInterval(async () => {
      try {
        const res = await fetch('/health', { signal: AbortSignal.timeout(2000) })
        if (res.ok) {
          const j = (await res.json().catch(() => null)) as { uptime?: number } | null
          const fresh = j != null && typeof j.uptime === 'number' && j.uptime < uptimeRef.current
          if (fresh || Date.now() > deadline) {
            window.clearInterval(id)
            window.location.reload()
          }
        }
      } catch {
        /* reiniciando */
      }
    }, 1500)
  }, [])

  const handleStatus = useCallback(
    (st: UpdateStatusInfo) => {
      setStatus(st)
      fetchFailRef.current = 0
      if (st.error) {
        setErrorCode(st.error)
        setPhaseBoth('error')
        closeStream()
        return
      }
      if (st.updating) {
        if (phaseRef.current === 'confirm') setPhaseBoth('progress')
        setStep(st.updating.step)
        setPct(st.updating.progress ?? 0)
        if (st.updating.step === 'done') {
          closeStream()
          waitAndReload()
        }
      } else if (phaseRef.current === 'progress') {
        closeStream()
        waitAndReload()
      }
    },
    [closeStream, waitAndReload],
  )

  const startPolling = useCallback(() => {
    if (pollRef.current !== null) return
    const poll = async () => {
      try {
        const st = await apiFetch<UpdateStatusInfo>('/api/update/status')
        handleStatus(st)
      } catch {
        fetchFailRef.current += 1
        if (fetchFailRef.current >= 2 && phaseRef.current === 'progress') {
          closeStream()
          waitAndReload()
        }
      }
    }
    void poll()
    pollRef.current = window.setInterval(poll, 2000)
  }, [closeStream, handleStatus, waitAndReload])

  const startStream = useCallback(() => {
    try {
      const es = new EventSource('/api/update/stream')
      esRef.current = es
      es.addEventListener('update', (ev) => {
        try {
          handleStatus(JSON.parse((ev as MessageEvent).data) as UpdateStatusInfo)
        } catch {
          /* payload corrupto */
        }
      })
      es.onerror = () => {
        if (phaseRef.current === 'restarting') return
        if (step === 'restart' || step === 'done') {
          closeStream()
          waitAndReload()
          return
        }
        closeStream()
        startPolling()
      }
    } catch {
      startPolling()
    }
  }, [closeStream, handleStatus, startPolling, waitAndReload, step])

  useEffect(() => {
    if (open) {
      setPhaseBoth('confirm')
      setStep('fetch')
      setPct(0)
      setErrorCode(null)
      setAckDowntime(false)
      fetchFailRef.current = 0
      if (!initialStatus) {
        apiFetch<UpdateStatusInfo>('/api/update/status')
          .then((j) => {
            if (!j) return
            setStatus(j)
            if (j.updating) {
              setPhaseBoth('progress')
              startStream()
            }
          })
          .catch(() => undefined)
      } else {
        setStatus(initialStatus)
        if (initialStatus.updating) {
          setPhaseBoth('progress')
          startStream()
        }
      }
    } else {
      closeStream()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const apply = async () => {
    try {
      const res = await fetch('/health', { signal: AbortSignal.timeout(2000) })
      if (res.ok) {
        const j = (await res.json()) as { uptime?: number }
        if (typeof j.uptime === 'number') uptimeRef.current = j.uptime
      }
    } catch {
      /* sin baseline */
    }
    try {
      await apiFetch('/api/update/apply', { method: 'POST' })
      setPhaseBoth('progress')
      startStream()
    } catch {
      setErrorCode('network')
      setPhaseBoth('error')
    }
  }

  const activeIdx = STEP_ORDER.indexOf(step as (typeof STEP_ORDER)[number])
  const busy = phase === 'progress' || phase === 'restarting'

  const changelogLines = useMemo(
    () =>
      (status?.latestBody ?? '')
        .split('\n')
        .map((l) => l.trim())
        .filter((l) => l && !/^(co-authored-by|signed-off-by|reviewed-by):/i.test(l)),
    [status?.latestBody],
  )

  return (
    <Dialog open={open} onOpenChange={(o) => !busy && onOpenChange(o)}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <DownloadCloud className="h-5 w-5 text-accent" strokeWidth={1.75} aria-hidden="true" />
            {t('update.dialog.title')}
          </DialogTitle>
          {phase === 'confirm' && <DialogDescription>{t('update.dialog.desc')}</DialogDescription>}
        </DialogHeader>

        {phase === 'confirm' && (
          <div className="flex flex-col gap-4">
            <div className="flex items-center justify-center gap-3 rounded-xl border border-border bg-surface px-4 py-3">
              <span className="font-mono text-sm text-text-secondary">{status?.current ?? '...'}</span>
              <ArrowRight className="h-4 w-4 text-text-muted" strokeWidth={1.75} aria-hidden="true" />
              <span className="font-mono text-sm font-semibold text-accent">
                {status?.latest ?? '...'}
              </span>
            </div>
            {changelogLines.length > 0 && (
              <div className="flex flex-col gap-1.5">
                <p className="text-xs font-semibold uppercase tracking-wider text-text-muted">
                  {t('update.dialog.changelogTitle')}
                </p>
                <div className="max-h-44 overflow-y-auto rounded-xl border border-border bg-surface px-3.5 py-2.5">
                  <ul className="flex flex-col gap-1">
                    {changelogLines.map((l, i) => (
                      <li key={i} className="flex items-start gap-2 text-caption leading-snug text-text-secondary">
                        <span className="mt-1 h-1 w-1 shrink-0 rounded-full bg-text-muted/60" aria-hidden="true" />
                        {l.replace(/^[-*]\s+/, '')}
                      </li>
                    ))}
                  </ul>
                </div>
              </div>
            )}
            <label className="flex cursor-pointer items-start gap-2.5 rounded-xl bg-warn/10 px-3.5 py-2.5 text-caption leading-snug text-warn">
              <Checkbox
                checked={ackDowntime}
                onCheckedChange={(v) => setAckDowntime(v === true)}
                className="mt-0.5"
                aria-label={t('update.dialog.downNotice')}
              />
              <span>{t('update.dialog.downNotice')}</span>
            </label>
            <DialogFooter>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                {t('update.dialog.cancel')}
              </Button>
              <Button
                onClick={() => void apply()}
                disabled={!ackDowntime || !status?.canApply}
              >
                <DownloadCloud className="h-4 w-4" strokeWidth={1.75} aria-hidden="true" />
                {t('update.dialog.start')}
              </Button>
            </DialogFooter>
          </div>
        )}

        {phase === 'progress' && (
          <div className="flex flex-col gap-4" role="status">
            <ul className="flex flex-col gap-2.5">
              {STEP_ORDER.map((s, i) => {
                const done = activeIdx > i || step === 'done'
                const active = activeIdx === i && step !== 'done'
                return (
                  <li key={s} className="flex items-center gap-2.5 text-sm">
                    {done ? (
                      <Check className="h-4 w-4 shrink-0 text-ok" strokeWidth={2} aria-hidden="true" />
                    ) : active ? (
                      <Loader2
                        className="h-4 w-4 shrink-0 animate-spin text-accent"
                        strokeWidth={2}
                        aria-hidden="true"
                      />
                    ) : (
                      <Circle className="h-4 w-4 shrink-0 text-text-muted/40" strokeWidth={2} aria-hidden="true" />
                    )}
                    <span className={done ? 'text-text-secondary' : active ? 'text-text-primary font-medium' : 'text-text-muted'}>
                      {t(`update.step.${s}`)}
                    </span>
                  </li>
                )
              })}
            </ul>
            <div className="flex flex-col gap-1.5">
              <Progress value={pct} aria-label={t('update.dialog.progressLabel')} />
              <div className="flex items-center justify-between text-caption text-text-muted">
                <span>{t(`update.step.${step}`)}...</span>
                <span className="font-mono">{pct}%</span>
              </div>
            </div>
            <p className="text-center text-caption text-text-muted">{t('update.dialog.hideHint')}</p>
          </div>
        )}

        {phase === 'restarting' && (
          <div className="flex flex-col items-center gap-3 py-4" role="status">
            <Loader2 className="h-10 w-10 animate-spin text-accent" strokeWidth={1.5} aria-hidden="true" />
            <p className="text-sm font-medium text-text-primary">{t('update.dialog.restarting')}</p>
            <p className="text-caption text-text-muted">{t('update.dialog.reloadSoon')}</p>
          </div>
        )}

        {phase === 'error' && (
          <div className="flex flex-col gap-4">
            <div className="flex items-start gap-3 rounded-xl bg-danger/10 px-4 py-3">
              <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-danger" strokeWidth={1.75} aria-hidden="true" />
              <div className="min-w-0">
                <p className="text-sm font-medium text-danger">{t('update.dialog.failed')}</p>
                {errorCode && <p className="mt-0.5 font-mono text-caption text-text-muted">{errorCode}</p>}
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                {t('update.dialog.close')}
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

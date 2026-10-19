import WebSocket from 'ws'
import { EventEmitter } from 'node:events'

export interface HAEntityState {
  state?: string
  attributes?: Record<string, unknown>
  lastUpdated?: string
}

export interface HAStatsRow {
  start: string
  end?: string
  mean?: number | null
  state?: number | null
  sum?: number | null
  min?: number | null
  max?: number | null
}

export type HAStats = Record<string, HAStatsRow[]>

export interface HAHistoryRow {
  state: string
  last_changed: string
  attributes?: Record<string, unknown>
}

export interface HAStatisticsQuery {
  startTime: string
  endTime: string
  statisticIds: string[]
  period: string
  types: string[]
}

export interface HAHistoryQuery {
  startTime: string
  endTime: string
  entityId: string
}

interface HAPendingCall {
  resolve: (value: unknown) => void
  reject: (reason: Error) => void
  timer: NodeJS.Timeout
}

interface HAEntityChange {
  s?: string
  a?: Record<string, unknown>
  lu?: string
}

interface HAEntityEvent {
  a?: Record<string, HAEntityChange>
  c?: Record<string, { '+': HAEntityChange; '-': unknown }>
}

interface HAMessage {
  type: string
  id?: number
  message?: string
  success?: boolean
  result?: unknown
  error?: { code?: string; message?: string }
  event?: HAEntityEvent
}

export class HAClient extends EventEmitter {
  httpUrl: string
  wsUrl: string
  token: string
  entities: Map<string, HAEntityState>
  pending: Map<number, HAPendingCall>
  msgId: number
  ws: WebSocket | null
  connected: boolean
  stopped: boolean
  retries: number
  subscribedIds: string[]

  constructor(httpUrl: string, token: string) {
    super()
    this.httpUrl = httpUrl
    this.wsUrl = httpUrl.replace(/^http/, 'ws') + '/api/websocket'
    this.token = token
    this.entities = new Map()
    this.pending = new Map()
    this.msgId = 0
    this.ws = null
    this.connected = false
    this.stopped = false
    this.retries = 0
    this.subscribedIds = []
  }

  start(): void {
    this.stopped = false
    this._connect()
  }

  stop(): void {
    this.stopped = true
    if (this.ws) this.ws.terminate()
  }

  _connect(): void {
    if (this.stopped) return
    const ws = new WebSocket(this.wsUrl, { handshakeTimeout: 10000 })
    this.ws = ws

    ws.on('message', (raw) => {
      let msg
      try {
        msg = JSON.parse(raw.toString())
      } catch {
        return
      }
      this._handle(msg)
    })

    ws.on('close', () => {
      this._onClose()
    })
    ws.on('error', (err) => {
      this.emit('wsError', err.message)
      try {
        ws.terminate()
      } catch {}
    })
  }

  _onClose(): void {
    const wasConnected = this.connected
    this.connected = false
    for (const { reject, timer } of this.pending.values()) {
      clearTimeout(timer)
      reject(new Error('ws closed'))
    }
    this.pending.clear()
    if (wasConnected) this.emit('disconnected')
    if (this.stopped) return
    const base = Math.min(30000, 1000 * 2 ** Math.min(this.retries, 5))
    const delay = Math.round(base * (0.5 + Math.random()))
    this.retries += 1
    setTimeout(() => this._connect(), delay)
  }

  _handle(msg: HAMessage): void {
    if (msg.type === 'auth_required') {
      this.ws!.send(JSON.stringify({ type: 'auth', access_token: this.token }))
      return
    }
    if (msg.type === 'auth_ok') {
      this.connected = true
      this.retries = 0
      this.emit('connected')
      if (this.subscribedIds.length) this.subscribeEntities(this.subscribedIds)
      return
    }
    if (msg.type === 'auth_invalid') {
      this.emit('fatal', `auth_invalid: ${msg.message || 'token rechazado'}`)
      this.stopped = true
      this.ws!.terminate()
      return
    }
    if (msg.type === 'result') {
      const p = this.pending.get(msg.id!)
      if (!p) return
      this.pending.delete(msg.id!)
      clearTimeout(p.timer)
      if (msg.success) p.resolve(msg.result)
      else p.reject(new Error(msg.error?.message || `error ${msg.error?.code}`))
      return
    }
    if (msg.type === 'event' && msg.event) {
      this._applyEntityEvent(msg.event)
    }
  }

  _applyEntityEvent(event: HAEntityEvent): void {
    if (event.a) {
      for (const [id, v] of Object.entries(event.a)) {
        this.entities.set(id, { state: v.s, attributes: v.a ?? {}, lastUpdated: v.lu })
        this.emit('entity', id)
      }
    }
    if (event.c) {
      for (const [id, diff] of Object.entries(event.c)) {
        if (diff['+']) {
          const cur = this.entities.get(id) || { state: undefined, attributes: {}, lastUpdated: undefined }
          const plus = diff['+']
          this.entities.set(id, {
            state: plus.s !== undefined ? plus.s : cur.state,
            attributes: plus.a !== undefined ? plus.a : cur.attributes,
            lastUpdated: plus.lu !== undefined ? plus.lu : cur.lastUpdated,
          })
          this.emit('entity', id)
        } else if (diff['-']) {
          this.entities.delete(id)
        }
      }
    }
  }

  call(message: Record<string, unknown>): Promise<unknown> {
    if (!this.connected || !this.ws) return Promise.reject(new Error('no conectado'))
    const ws = this.ws
    const id = ++this.msgId
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id)
        reject(new Error('timeout ws'))
      }, 60000)
      this.pending.set(id, { resolve, reject, timer })
      ws.send(JSON.stringify({ id, ...message }))
    })
  }

  subscribeEntities(ids: string[]): Promise<unknown> {
    this.subscribedIds = ids
    return this.call({ type: 'subscribe_entities', entity_ids: ids }).catch((err: Error) => {
      this.emit('wsError', `subscribe_entities: ${err.message}`)
    })
  }

  /** Llama a un servicio de HAOS por websocket (target/entity_id separado). */
  callService(domain: string, service: string, serviceData?: Record<string, unknown>): Promise<unknown> {
    const msg: Record<string, unknown> = { type: 'call_service', domain, service }
    const data = serviceData ?? {}
    if (data.entity_id) msg.target = { entity_id: data.entity_id }
    const rest: Record<string, unknown> = { ...data }
    delete rest.entity_id
    if (Object.keys(rest).length > 0) msg.service_data = rest
    return this.call(msg)
  }

  getState(id: string): HAEntityState | undefined {
    return this.entities.get(id)
  }

  statisticsDuringPeriod(query: HAStatisticsQuery): Promise<HAStats> {
    return this.call({
      type: 'recorder/statistics_during_period',
      start_time: query.startTime,
      end_time: query.endTime,
      statistic_ids: query.statisticIds,
      period: query.period,
      types: query.types,
    }) as Promise<HAStats>
  }

  // Historial de estados vía REST (no existe equivalente websocket para
  // /api/history/period). Lo usan las extensiones para sensores SIN
  // state_class (sin statistics): p.ej. el contador de energía del cargador.
  // Devuelve la lista de estados de UNA entidad: [{ state, last_changed }].
  async historyDuringPeriod(query: HAHistoryQuery): Promise<HAHistoryRow[]> {
    const url =
      `${this.httpUrl}/api/history/period/${encodeURIComponent(query.startTime)}` +
      `?filter_entity_id=${encodeURIComponent(query.entityId)}` +
      `&end_time=${encodeURIComponent(query.endTime)}&minimal_response`
    const res = await fetch(url, { headers: { Authorization: `Bearer ${this.token}` } })
    if (!res.ok) throw new Error(`history HTTP ${res.status}`)
    const data = await res.json()
    return Array.isArray(data) && Array.isArray(data[0]) ? (data[0] as HAHistoryRow[]) : []
  }
}

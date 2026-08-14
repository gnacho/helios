import WebSocket from 'ws'
import { EventEmitter } from 'node:events'

export class HAClient extends EventEmitter {
  constructor(httpUrl, token) {
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

  start() {
    this.stopped = false
    this._connect()
  }

  stop() {
    this.stopped = true
    if (this.ws) this.ws.terminate()
  }

  _connect() {
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

  _onClose() {
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

  _handle(msg) {
    if (msg.type === 'auth_required') {
      this.ws.send(JSON.stringify({ type: 'auth', access_token: this.token }))
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
      this.ws.terminate()
      return
    }
    if (msg.type === 'result') {
      const p = this.pending.get(msg.id)
      if (!p) return
      this.pending.delete(msg.id)
      clearTimeout(p.timer)
      if (msg.success) p.resolve(msg.result)
      else p.reject(new Error(msg.error?.message || `error ${msg.error?.code}`))
      return
    }
    if (msg.type === 'event' && msg.event) {
      this._applyEntityEvent(msg.event)
    }
  }

  _applyEntityEvent(event) {
    if (event.a) {
      for (const [id, v] of Object.entries(event.a)) {
        this.entities.set(id, { state: v.s, attributes: v.a || {}, lastUpdated: v.lu })
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

  call(message) {
    if (!this.connected || !this.ws) return Promise.reject(new Error('no conectado'))
    const id = ++this.msgId
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id)
        reject(new Error('timeout ws'))
      }, 60000)
      this.pending.set(id, { resolve, reject, timer })
      this.ws.send(JSON.stringify({ id, ...message }))
    })
  }

  subscribeEntities(ids) {
    this.subscribedIds = ids
    return this.call({ type: 'subscribe_entities', entity_ids: ids }).catch((err) => {
      this.emit('wsError', `subscribe_entities: ${err.message}`)
    })
  }

  getState(id) {
    return this.entities.get(id)
  }

  statisticsDuringPeriod({ startTime, endTime, statisticIds, period, types }) {
    return this.call({
      type: 'recorder/statistics_during_period',
      start_time: startTime,
      end_time: endTime,
      statistic_ids: statisticIds,
      period,
      types,
    })
  }

  // Historial de estados vía REST (no existe equivalente websocket para
  // /api/history/period). Lo usan las extensiones para sensores SIN
  // state_class (sin statistics): p.ej. el contador de energía del cargador.
  // Devuelve la lista de estados de UNA entidad: [{ state, last_changed }].
  async historyDuringPeriod({ startTime, endTime, entityId }) {
    const url =
      `${this.httpUrl}/api/history/period/${encodeURIComponent(startTime)}` +
      `?filter_entity_id=${encodeURIComponent(entityId)}` +
      `&end_time=${encodeURIComponent(endTime)}&minimal_response`
    const res = await fetch(url, { headers: { Authorization: `Bearer ${this.token}` } })
    if (!res.ok) throw new Error(`history HTTP ${res.status}`)
    const data = await res.json()
    return Array.isArray(data) && Array.isArray(data[0]) ? data[0] : []
  }
}

/* ============================================================
   Helios web — app.js
   Cielo procedural (scroll = un día completo), gráficas en vivo,
   teatro sticky, ticker, slider y lightbox.
   ============================================================ */
(() => {
  'use strict'

  const $ = (s) => document.querySelector(s)
  const clamp = (v, a, b) => Math.min(b, Math.max(a, v))
  const ss = (a, b, x) => { const t = clamp((x - a) / (b - a), 0, 1); return t * t * (3 - 2 * t) }
  const pad = (n) => String(n).padStart(2, '0')
  const rgb = (arr, a = 1) => `rgba(${arr[0]},${arr[1]},${arr[2]},${a})`

  function mulberry32(seed) {
    let s = seed >>> 0
    return () => {
      s |= 0; s = (s + 0x6d2b79f5) | 0
      let t = Math.imul(s ^ (s >>> 15), 1 | s)
      t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296
    }
  }

  /* ── Microcopy traducible de widgets dinámicos ── */
  const MICRO = {
    es: {
      charge: 'Cargando', discharge: 'Descargando', full: 'Batería llena', idle: 'En reposo',
      exp: '→ red', imp: '← red',
      chargeT: 'Batería cargando', dischargeT: 'Batería descargando', fullT: 'Batería al 100 %', idleT: 'Batería en espera',
      grid: 'Red', cons: 'Consumo', today: 'hoy', saved: 'ahorrados',
      stats: '0 nubes · 100 % AGPL-3.0',
    },
    en: {
      charge: 'Charging', discharge: 'Discharging', full: 'Battery full', idle: 'Idle',
      exp: '→ grid', imp: '← grid',
      chargeT: 'Battery charging', dischargeT: 'Battery discharging', fullT: 'Battery at 100 %', idleT: 'Battery standing by',
      grid: 'Grid', cons: 'Load', today: 'today', saved: 'saved',
      stats: '0 clouds · 100 % AGPL-3.0',
    },
  }

  /* ── Simulación solar (misma familia que el mock de la demo) ── */
  const solarCurve = (h) => {
    if (h < 6.2 || h > 21.8) return 0
    const g = (x, mu, s, a) => a * Math.exp(-((x - mu) ** 2) / (2 * s * s))
    return Math.max(0.02, g(h, 12.9, 3.3, 4.2) + g(h, 13.9, 4.1, 1.3))
  }
  const consumptionCurve = (h) => {
    const g = (x, mu, s, a) => a * Math.exp(-((x - mu) ** 2) / (2 * s * s))
    return 0.55 + g(h, 8.2, 1.6, 0.38) + g(h, 20.9, 1.8, 0.55) + 0.16 * Math.sin(Math.PI * (h - 6) / 15)
  }
  const kwhToday = (hNow) => {
    let acc = 0, prev = solarCurve(372 / 60)
    for (let m = 372 + 5; m <= hNow * 60; m += 5) {
      const v = solarCurve(m / 60)
      acc += ((v + prev) / 2) * (5 / 60)
      prev = v
    }
    return acc
  }
  const hourNow = () => { const d = new Date(); return d.getHours() + d.getMinutes() / 60 + d.getSeconds() / 3600 }
  const timeStr = () => { const d = new Date(); return `${pad(d.getHours())}:${pad(d.getMinutes())}` }

  function batteryState(h) {
    if (h >= 8 && h < 14.5) return { soc: 40 + 60 * (h - 8) / 6.5, mode: 'charge' }
    if (h >= 18 && h < 22.5) return { soc: 100 - 60 * (h - 18) / 4.5, mode: 'discharge' }
    if (h >= 14.5 && h < 18) return { soc: 100, mode: 'full' }
    return { soc: 40, mode: 'idle' }
  }

  /* ── Cielo: keyframes de fase ── */
  const SKY_KEYS = [
    { t: 0.00, top: [11, 16, 48], mid: [74, 63, 122], hor: [224, 122, 79] },
    { t: 0.14, top: [56, 116, 181], mid: [131, 180, 222], hor: [233, 224, 196] },
    { t: 0.50, top: [68, 146, 208], mid: [157, 205, 236], hor: [246, 240, 222] },
    { t: 0.78, top: [94, 129, 186], mid: [214, 158, 122], hor: [244, 178, 99] },
    { t: 0.92, top: [52, 44, 97], mid: [94, 72, 131], hor: [196, 106, 102] },
    { t: 1.00, top: [11, 16, 48], mid: [28, 31, 66], hor: [48, 44, 86] },
  ]
  const SUN_KEYS = [
    { t: 0.00, x: 0.14, y: 0.70, r: 30, c: [255, 214, 125] },
    { t: 0.14, x: 0.30, y: 0.34, r: 22, c: [255, 244, 205] },
    { t: 0.50, x: 0.56, y: 0.27, r: 20, c: [255, 247, 214] },
    { t: 0.78, x: 0.78, y: 0.30, r: 26, c: [255, 208, 150] },
    { t: 0.92, x: 0.86, y: 0.72, r: 38, c: [255, 157, 92] },
    { t: 1.00, x: 0.86, y: 0.85, r: 40, c: [255, 150, 84] },
  ]
  function lerpK(kf, t, key) {
    if (t <= kf[0].t) return kf[0][key]
    for (let i = 1; i < kf.length; i++) {
      const a = kf[i - 1], b = kf[i]
      if (t <= b.t) {
        const f = (t - a.t) / (b.t - a.t)
        const av = a[key], bv = b[key]
        if (typeof av === 'number') return av + (bv - av) * f
        return av.map((v, j) => v + (bv[j] - v) * f)
      }
    }
    return kf[kf.length - 1][key]
  }

  /* ── Cielo: canvas ── */
  const skyCanvas = $('#sky')
  const skyCtx = skyCanvas.getContext('2d')
  let SW = 0, SH = 0
  const dprOf = () => Math.min(3, window.devicePixelRatio || 1)
  const MAX_CSS = 4096
  const MAX_PHYS = 8192
  function fitCanvas(canvas, cw, ch, maxCss) {
    const dpr = dprOf()
    const w = cw != null ? cw : canvas.clientWidth
    const h = ch != null ? ch : canvas.clientHeight
    if (!w || !h || w > maxCss || h > maxCss) return null
    canvas.width = Math.min(w * dpr, MAX_PHYS)
    canvas.height = Math.min(h * dpr, MAX_PHYS)
    return { w, h, dpr }
  }
  function resizeSky() {
    const f = fitCanvas(skyCanvas, window.innerWidth, window.innerHeight, MAX_PHYS)
    if (!f) return
    SW = f.w; SH = f.h
    skyCtx.setTransform(f.dpr, 0, 0, f.dpr, 0, 0)
  }

  const rnd = mulberry32(20260811)
  const stars = []
  for (let i = 0; i < 170; i++) {
    stars.push({ x: rnd(), y: rnd() * 0.85, r: 0.4 + rnd() * 1.3, tw: 0.5 + rnd() * 2, ph: rnd() * 6.28, a: 0.25 + rnd() * 0.75 })
  }
  const CONST_C = [0.72, 0.16]
  const CONST_R = []
  for (let k = 0; k < 8; k++) {
    const a = k * Math.PI / 4
    CONST_R.push([CONST_C[0] + Math.cos(a) * 0.05, CONST_C[1] + Math.sin(a) * 0.055])
  }
  const meteors = []
  let nextMeteor = 5

  function drawConstellation(ctx, sf) {
    const a = 0.65 * sf
    const cx = CONST_C[0] * SW, cy = CONST_C[1] * SH
    ctx.strokeStyle = rgb([255, 255, 255], a * 0.45)
    ctx.lineWidth = 1
    ctx.beginPath()
    for (const p of CONST_R) { ctx.moveTo(cx, cy); ctx.lineTo(p[0] * SW, p[1] * SH) }
    ctx.stroke()
    ctx.fillStyle = rgb([255, 255, 255], a)
    for (const p of CONST_R) { ctx.beginPath(); ctx.arc(p[0] * SW, p[1] * SH, 1.6, 0, 7); ctx.fill() }
    ctx.beginPath(); ctx.arc(cx, cy, 2, 0, 7); ctx.fill()
  }

  function updateMeteors(ctx, sf) {
    nextMeteor -= 1 / 60
    if (nextMeteor <= 0 && sf > 0.45) {
      meteors.push({
        x: SW * (0.2 + Math.random() * 0.6), y: SH * (0.05 + Math.random() * 0.3),
        vx: -(0.35 + Math.random() * 0.3) * SW / 1400, vy: (0.18 + Math.random() * 0.15) * SH / 900,
        life: 1,
      })
      nextMeteor = 5 + Math.random() * 9
    }
    for (let i = meteors.length - 1; i >= 0; i--) {
      const m = meteors[i]
      m.life -= 1 / 60
      if (m.life <= 0) { meteors.splice(i, 1); continue }
      m.x += m.vx * 2.2; m.y += m.vy * 2.2
      const a = Math.min(1, m.life * 2.2)
      const tl = 16
      const g = ctx.createLinearGradient(m.x, m.y, m.x - m.vx * tl, m.y - m.vy * tl)
      g.addColorStop(0, `rgba(255,255,255,${a.toFixed(3)})`)
      g.addColorStop(1, 'rgba(255,255,255,0)')
      ctx.strokeStyle = g; ctx.lineWidth = 1.4
      ctx.beginPath(); ctx.moveTo(m.x, m.y); ctx.lineTo(m.x - m.vx * tl, m.y - m.vy * tl)
      ctx.stroke()
    }
  }

  function drawSky(t, h) {
    const ctx = skyCtx
    ctx.clearRect(0, 0, SW, SH)
    const top = lerpK(SKY_KEYS, t, 'top'), mid = lerpK(SKY_KEYS, t, 'mid'), hor = lerpK(SKY_KEYS, t, 'hor')
    const grad = ctx.createLinearGradient(0, 0, 0, SH)
    grad.addColorStop(0, rgb(top)); grad.addColorStop(0.55, rgb(mid)); grad.addColorStop(1, rgb(hor))
    ctx.fillStyle = grad
    ctx.fillRect(0, 0, SW, SH)

    const starF = clamp(1 - ss(0.04, 0.17, t) + ss(0.86, 0.96, t), 0, 1)
    const nightF = clamp(0.2 + 0.8 * (Math.cos((h - 14) / 24 * Math.PI * 2) + 1) / 2, 0.2, 1)
    const sf = starF * nightF
    const time = performance.now() / 1000

    if (sf > 0.03) {
      for (const s of stars) {
        const a = s.a * sf * (0.6 + 0.4 * Math.sin(time * s.tw + s.ph))
        if (a < 0.03) continue
        const x = s.x * SW, y = s.y * SH
        ctx.fillStyle = `rgba(255,255,255,${a.toFixed(3)})`
        ctx.beginPath(); ctx.arc(x, y, s.r * 1.2, 0, 7); ctx.fill()
        if (s.r > 1.35) {
          const g = ctx.createRadialGradient(x, y, 0, x, y, s.r * 6)
          g.addColorStop(0, `rgba(255,255,255,${(a * 0.25).toFixed(3)})`)
          g.addColorStop(1, 'rgba(255,255,255,0)')
          ctx.fillStyle = g; ctx.beginPath(); ctx.arc(x, y, s.r * 6, 0, 7); ctx.fill()
        }
      }
      drawConstellation(ctx, sf)
      updateMeteors(ctx, sf)
    }

    // banda ámbar en el horizonte (amanecer / atardecer)
    const glowF = clamp(1 - Math.abs(t - 0.06) * 7, 0, 1) + clamp(1 - Math.abs(t - 0.87) * 7, 0, 1)
    if (glowF > 0.02) {
      const gy = SH * 0.72, gh = SH * 0.14
      const g = ctx.createLinearGradient(0, gy, 0, gy + gh)
      g.addColorStop(0, rgb([255, 170, 90], 0)); g.addColorStop(0.5, rgb([255, 170, 90], 0.28 * glowF)); g.addColorStop(1, rgb([255, 170, 90], 0))
      ctx.fillStyle = g
      ctx.fillRect(0, gy, SW, gh)
    }

    // sol
    const sx = lerpK(SUN_KEYS, t, 'x'), sy = lerpK(SUN_KEYS, t, 'y'), sr = lerpK(SUN_KEYS, t, 'r'), sc = lerpK(SUN_KEYS, t, 'c')
    if (sy * SH < SH * 0.8) {
      const px = sx * SW, py = sy * SH
      const g = ctx.createRadialGradient(px, py, 0, px, py, sr * 7)
      g.addColorStop(0, rgb(sc, 0.45)); g.addColorStop(1, rgb(sc, 0))
      ctx.fillStyle = g; ctx.beginPath(); ctx.arc(px, py, sr * 7, 0, 7); ctx.fill()
      ctx.fillStyle = rgb(sc); ctx.beginPath(); ctx.arc(px, py, sr * 0.42, 0, 7); ctx.fill()
      ctx.fillStyle = 'rgba(255,255,255,0.85)'; ctx.beginPath(); ctx.arc(px, py, sr * 0.24, 0, 7); ctx.fill()
    }

    // luna
    if (sf > 0.25) {
      const mx = SW * 0.8, my = SH * 0.2, mr = Math.min(SW, SH) * 0.045
      const g = ctx.createRadialGradient(mx, my, 0, mx, my, mr * 5)
      g.addColorStop(0, `rgba(232,233,244,${(0.35 * sf).toFixed(3)})`); g.addColorStop(1, 'rgba(232,233,244,0)')
      ctx.fillStyle = g; ctx.beginPath(); ctx.arc(mx, my, mr * 5, 0, 7); ctx.fill()
      ctx.fillStyle = 'rgba(232,233,244,0.95)'; ctx.beginPath(); ctx.arc(mx, my, mr, 0, 7); ctx.fill()
      ctx.fillStyle = rgb(lerpK(SKY_KEYS, t, 'top'), 0.95)
      ctx.beginPath(); ctx.arc(mx + mr * 0.32, my - mr * 0.22, mr * 0.86, 0, 7); ctx.fill()
    }
  }

  function scrollPhase() {
    const doc = document.documentElement
    const max = doc.scrollHeight - window.innerHeight
    return clamp(max > 0 ? window.scrollY / max : 0, 0, 1)
  }

  /* ── Gráfica del día ── */
  function drawDayChart(canvas, labels) {
    const f = fitCanvas(canvas, null, null, MAX_CSS)
    if (!f) return
    const dpr = f.dpr, w = f.w, h = f.h
    const ctx = canvas.getContext('2d')
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    const L = labels ? 34 : 8, R = 10, T = 10, B = labels ? 20 : 8
    const pw = w - L - R, ph = h - T - B
    const maxY = 6
    const now = hourNow()
    const nowX = L + (clamp(now, 6.2, 21.8) - 6.2) / 15.6 * pw
    const pts = []
    for (let m = 372; m <= 1308; m += 5) {
      const v = solarCurve(m / 60)
      pts.push({ x: L + ((m - 372) / 936) * pw, y: T + ph * (1 - v / maxY), v })
    }

    const grad = ctx.createLinearGradient(0, T, 0, T + ph)
    grad.addColorStop(0, 'rgba(255,194,75,0.30)'); grad.addColorStop(1, 'rgba(255,194,75,0.02)')
    ctx.beginPath(); ctx.moveTo(pts[0].x, T + ph)
    for (const p of pts) ctx.lineTo(p.x, p.y)
    ctx.lineTo(pts[pts.length - 1].x, T + ph)
    ctx.closePath(); ctx.fillStyle = grad; ctx.fill()

    ctx.lineWidth = 2
    ctx.strokeStyle = 'rgba(255,194,75,0.95)'
    ctx.shadowColor = 'rgba(255,194,75,0.5)'; ctx.shadowBlur = 8
    ctx.beginPath()
    for (const p of pts) { if (p.x > nowX) break; if (p.x === pts[0].x && p.x <= nowX) ctx.moveTo(p.x, p.y); else ctx.lineTo(p.x, p.y) }
    ctx.stroke()
    ctx.shadowBlur = 0
    ctx.strokeStyle = 'rgba(255,255,255,0.30)'
    ctx.setLineDash([4, 5])
    ctx.beginPath()
    let started = false
    for (const p of pts) {
      if (p.x >= nowX) { if (!started) { ctx.moveTo(p.x, p.y); started = true } else ctx.lineTo(p.x, p.y) }
    }
    ctx.stroke(); ctx.setLineDash([])

    ctx.strokeStyle = 'rgba(255,255,255,0.25)'
    ctx.setLineDash([2, 4])
    ctx.beginPath(); ctx.moveTo(nowX, T); ctx.lineTo(nowX, T + ph); ctx.stroke()
    ctx.setLineDash([])

    if (now >= 6.2 && now <= 21.8) {
      const yNow = T + ph * (1 - clamp(solarCurve(now), 0, maxY) / maxY)
      const g = ctx.createRadialGradient(nowX, yNow, 0, nowX, yNow, 16)
      g.addColorStop(0, 'rgba(255,194,75,0.9)'); g.addColorStop(1, 'rgba(255,194,75,0)')
      ctx.fillStyle = g; ctx.beginPath(); ctx.arc(nowX, yNow, 16, 0, 7); ctx.fill()
      ctx.fillStyle = '#FFE9B0'; ctx.beginPath(); ctx.arc(nowX, yNow, 4, 0, 7); ctx.fill()
    }

    if (labels) {
      ctx.fillStyle = 'rgba(255,255,255,0.55)'
      ctx.font = '9px JetBrains Mono, monospace'
      ctx.textAlign = 'center'
      for (const hh of [6, 9, 12, 15, 18, 21]) ctx.fillText(String(hh), L + ((hh - 6.2) / 15.6) * pw, h - 6)
    }
  }

  /* ── Histórico: última semana (producción vs consumo, como la app) ── */
  function roundRectPath(ctx, x, y, w, h, r) {
    ctx.beginPath()
    if (ctx.roundRect) { ctx.roundRect(x, y, w, h, r); return }
    ctx.moveTo(x + r, y); ctx.lineTo(x + w - r, y); ctx.arcTo(x + w, y, x + w, y + r, r)
    ctx.lineTo(x + w, y + h); ctx.lineTo(x, y + h); ctx.lineTo(x, y + r); ctx.arcTo(x, y, x + r, y, r)
    ctx.closePath()
  }
  function drawHist(canvas) {
    const f = fitCanvas(canvas, null, null, MAX_CSS)
    if (!f) return
    const dpr = f.dpr, w = f.w, h = f.h
    const ctx = canvas.getContext('2d')
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    const lang = document.documentElement.lang || 'es'
    const r = mulberry32(7)
    const month = new Date().getMonth() + 1
    const peak = Math.max(0, Math.cos(2 * Math.PI * (month - 6.5) / 12)) ** 1.5
    const nowH = hourNow()
    const days = []
    for (let i = 6; i >= 0; i--) {
      const d = new Date()
      d.setDate(d.getDate() - i)
      const base = (30 + peak * 8) * (0.85 + r() * 0.35)
      const isToday = i === 6
      let prod = base
      let cons = base * (0.38 + r() * 0.12)
      if (isToday) {
        const f = clamp((nowH - 6.2) / 15.6, 0.06, 1)
        prod = base * f
        cons = cons * clamp((nowH - 6.2) / 15.6, 0.35, 1)
      }
      days.push({ d, prod, cons, isToday })
    }
    const maxY = Math.max(40, Math.ceil((Math.max(...days.map((x) => Math.max(x.prod, x.cons))) + 4) / 10) * 10)
    const L = 28, R = 8, T = 6, B = 16
    const pw = w - L - R, ph = h - T - B
    ctx.font = '9px JetBrains Mono, monospace'
    ctx.textAlign = 'right'
    for (let v = 0; v <= maxY; v += 10) {
      const y = T + ph * (1 - v / maxY)
      ctx.strokeStyle = 'rgba(255,255,255,0.08)'
      ctx.beginPath(); ctx.moveTo(L, y); ctx.lineTo(L + pw, y); ctx.stroke()
      ctx.fillStyle = 'rgba(255,255,255,0.5)'
      ctx.fillText(String(v), L - 5, y + 3)
    }
    const n = 7, gap = 10
    const bw = (pw - gap * (n - 1)) / n
    const inner = bw / 2
    days.forEach((row, i) => {
      const x = L + i * (bw + gap)
      const op = row.isToday ? 1 : 0.7
      const prodH = (row.prod / maxY) * ph
      ctx.fillStyle = `rgba(255,194,75,${(0.9 * op).toFixed(2)})`
      roundRectPath(ctx, x, T + ph - prodH, inner - 2, prodH, 3)
      ctx.fill()
      const consH = (row.cons / maxY) * ph
      ctx.fillStyle = `rgba(52,211,153,${(0.9 * op).toFixed(2)})`
      roundRectPath(ctx, x + inner + 2, T + ph - consH, inner - 2, consH, 3)
      ctx.fill()
      ctx.fillStyle = 'rgba(255,255,255,0.5)'
      ctx.textAlign = 'center'
      ctx.fillText(row.d.toLocaleDateString(lang, { weekday: 'short', day: 'numeric' }), x + bw / 2, h - 4)
    })
  }

  /* ── Stats hero ── */
  const heroLiveTime = $('#heroLiveTime'), widgetTime = $('#widgetTime')
  const statNow = $('#statNow'), statToday = $('#statToday'), statMoney = $('#statMoney')
  const batChargedVal = $('#batChargedVal'), batDischargedVal = $('#batDischargedVal')
  function batteryKwhToday(hNow) {
    let ch = 0, ds = 0, prevC = 0, prevD = 0
    for (let m = 372; m <= hNow * 60; m += 5) {
      const h = m / 60
      const surplus = solarCurve(h) - consumptionCurve(h) - 0.8
      const c = h >= 8 && h < 15 && surplus > 0 ? Math.min(1.8, surplus) : 0
      const d = h >= 18 && h < 22.5 && surplus < -0.2 ? Math.min(1.8, -surplus) : 0
      ch += ((c + prevC) / 2) * (5 / 60)
      ds += ((d + prevD) / 2) * (5 / 60)
      prevC = c; prevD = d
    }
    return [ch, ds]
  }
  function updateStats() {
    const h = hourNow()
    const t = timeStr()
    heroLiveTime.textContent = t
    widgetTime.textContent = t
    const solarN = solarCurve(h)
    const kWh = kwhToday(h)
    statNow.textContent = `${solarN.toFixed(1)} kW`
    statToday.textContent = `${kWh.toFixed(1)} kWh`
    statMoney.textContent = `${(kWh * 0.135).toFixed(2)} €`
    const [ch, ds] = batteryKwhToday(h)
    batChargedVal.textContent = `${ch.toFixed(1)} kWh`
    batDischargedVal.textContent = `${ds.toFixed(1)} kWh`
  }

  /* ── Batería ── */
  const ringFg = $('#ringFg'), batterySoc = $('#batterySoc'), batteryStatus = $('#batteryStatus')
  let curSoc = 100
  function updateBattery(h) {
    const st = batteryState(h)
    curSoc += (st.soc - curSoc) * 0.08
    ringFg.style.strokeDashoffset = String(314.16 * (1 - curSoc / 100))
    batterySoc.textContent = String(Math.round(curSoc))
    batteryStatus.textContent = MICRO[document.documentElement.lang || 'es'][st.mode]
  }

  /* ── Flujo de energía (réplica del widget de la app) ── */
  const EDGE_COLORS = { 'fv-home': '#FBBF24', 'fv-battery': '#34D399', 'fv-grid': '#22D3EE', 'battery-home': '#34D399', 'grid-home': '#FB7185', 'grid-battery': '#FB7185' }
  const NODE_COLORS = { fv: '#FBBF24', home: '#60A5FA', battery: '#34D399' }
  const edgeEls = {}, edgePaths = {}, edgeParts = {}, edgeValues = {}, edgeT = {}, prevActive = {}, edgeLen = {}
  document.querySelectorAll('.flow-edge').forEach((el) => {
    const id = el.dataset.edge
    edgeEls[id] = el
    edgePaths[id] = el.querySelector('path')
    edgeParts[id] = [...el.querySelectorAll('.flow-parts circle')]
    edgeLen[id] = edgePaths[id].getTotalLength()
    prevActive[id] = false
    edgeT[id] = 0
  })
  const nodeEls = {}, nodeRing = {}, nodeGlow = {}
  document.querySelectorAll('.flow-node').forEach((el) => {
    const n = el.dataset.node
    nodeEls[n] = el
    nodeRing[n] = el.querySelector('.node-ring')
    nodeGlow[n] = el.querySelector('.node-glow')
  })
  const flowFvVal = $('#flowFvVal'), flowHomeVal = $('#flowHomeVal'), flowBatVal = $('#flowBatVal'), flowGridVal = $('#flowGridVal'), flowBatSoc = $('#flowBatSoc'), gridIcon = $('#flowGridIcon')
  const flowLive = { production: 0, consumption: 0, batteryPower: 0, grid: 0, soc: 100 }
  const flowDuration = (kw) => Math.max(0.8, Math.min(2.6, 2.6 - kw * 0.28))
  function computeEdges(live) {
    const fvToHome = Math.min(live.production, live.consumption)
    const charging = Math.max(0, live.batteryPower)
    const discharging = Math.max(0, -live.batteryPower)
    const exporting = Math.max(0, -live.grid)
    const importing = Math.max(0, live.grid)
    const solarSurplus = Math.max(0, live.production - live.consumption)
    const gridToBattery = Math.max(0, Math.min(charging - solarSurplus, importing))
    const fvToBattery = Math.max(0, charging - gridToBattery)
    return { 'fv-home': fvToHome, 'fv-battery': fvToBattery, 'fv-grid': exporting, 'battery-home': discharging, 'grid-home': importing, 'grid-battery': gridToBattery }
  }
  function updateFlowWidget() {
    const lang = document.documentElement.lang || 'es'
    const edges = computeEdges(flowLive)
    for (const id in edges) {
      const active = edges[id] > 0.05
      edgeValues[id] = edges[id]
      edgeEls[id].classList.toggle('active', active)
      edgePaths[id].style.stroke = active ? EDGE_COLORS[id] : 'rgb(255 255 255 / 0.16)'
      edgeParts[id].forEach((c) => { c.style.fill = active ? EDGE_COLORS[id] : 'transparent' })
      if (active && !prevActive[id]) edgeT[id] = 0
      prevActive[id] = active
    }
    const live = flowLive
    const setNode = (name, active, color) => {
      nodeEls[name].classList.toggle('active', active)
      nodeRing[name].style.stroke = active ? color : 'rgb(255 255 255 / 0.25)'
      nodeGlow[name].style.stroke = active ? color : 'transparent'
    }
    setNode('fv', live.production > 0.1, NODE_COLORS.fv)
    setNode('home', live.consumption > 0.1, NODE_COLORS.home)
    setNode('battery', Math.abs(live.batteryPower) > 0.1, NODE_COLORS.battery)
    const gridActive = Math.abs(live.grid) > 0.1
    const gridColor = live.grid < 0 ? '#22D3EE' : '#FB7185'
    setNode('grid', gridActive, gridColor)
    gridIcon.style.color = gridActive ? gridColor : 'rgb(255 255 255 / 0.55)'
    flowFvVal.textContent = `${live.production.toFixed(2)} kW`
    flowHomeVal.textContent = `${live.consumption.toFixed(2)} kW`
    flowBatVal.textContent = live.batteryPower > 0.05 ? `${live.batteryPower.toFixed(2)} kW` : live.batteryPower < -0.05 ? `${(-live.batteryPower).toFixed(2)} kW` : MICRO[lang].idle
    flowGridVal.textContent = live.grid < -0.05 ? `${(-live.grid).toFixed(2)} kW ↑` : live.grid > 0.05 ? `${live.grid.toFixed(2)} kW ↓` : '0 W'
    flowBatSoc.textContent = String(Math.round(live.soc))
  }
  let lastFlowTs = 0
  function updateFlowParticles(ts) {
    const dt = Math.min(0.05, (ts - lastFlowTs) / 1000)
    lastFlowTs = ts
    let any = false
    for (const id in prevActive) {
      if (!prevActive[id]) continue
      any = true
      const len = edgeLen[id]
      edgeT[id] = (edgeT[id] + dt / flowDuration(edgeValues[id])) % 1
      const parts = edgeParts[id]
      for (let k = 0; k < parts.length; k++) {
        const off = ((edgeT[id] + k / parts.length) % 1) * len
        const p = edgePaths[id].getPointAtLength(off)
        parts[k].setAttribute('cx', p.x.toFixed(2))
        parts[k].setAttribute('cy', p.y.toFixed(2))
      }
    }
    return any
  }
  function updateFlow(h) {
    const solarN = solarCurve(h), consN = consumptionCurve(h)
    const surplus = solarN - consN - 0.8
    let batPower = 0
    if (curSoc < 98 && surplus > 0) batPower = Math.min(1.8, surplus)
    else if (curSoc > 45 && surplus < -0.2) batPower = -Math.min(1.8, -surplus)
    flowLive.production = solarN
    flowLive.consumption = consN
    flowLive.batteryPower = batPower
    flowLive.grid = surplus - batPower
    flowLive.soc = curSoc
    updateFlowWidget()
  }

  /* ── Ticker ── */
  const tickerTrack = $('#tickerTrack')
  function buildTicker() {
    const lang = document.documentElement.lang || 'es'
    const M = MICRO[lang]
    const h = hourNow()
    const solarN = solarCurve(h), consN = consumptionCurve(h)
    const surplus = solarN - consN - 0.8
    const exp = Math.max(0, surplus), imp = Math.max(0, -surplus)
    const bs = batteryState(h)
    const kWh = kwhToday(h)
    const items = [
      `Solis <b>${(solarN * 0.62).toFixed(1)}</b> kW`,
      `Fox <b>${(solarN * 0.38).toFixed(1)}</b> kW`,
      `${M[bs.mode + 'T']} <b>${Math.round(bs.soc)}</b> %`,
      exp > 0 ? `${M.grid} → <b>${exp.toFixed(1)}</b> kW` : `${M.grid} ← <b>${imp.toFixed(1)}</b> kW`,
      `${M.cons} <b>${consN.toFixed(1)}</b> kW`,
      `<b>${kWh.toFixed(1)}</b> kWh ${M.today}`,
      `<b>${(kWh * 0.135).toFixed(2)}</b> € ${M.saved}`,
      M.stats,
    ]
    tickerTrack.innerHTML = items.map((s) => `<span>${s} ·</span>`).join('') + items.map((s) => `<span>${s} ·</span>`).join('')
    tickerTrack.style.animation = 'none'
    void tickerTrack.offsetWidth
    tickerTrack.style.animation = ''
  }

  /* ── Slider + lightbox ── */
  function setupSlider() {
    const track = $('#sliderTrack')
    const slides = track.children.length
    const dotsWrap = $('#sliderDots')
    for (let i = 0; i < slides; i++) {
      const d = document.createElement('button')
      d.className = 'slider-dot' + (i ? '' : ' active')
      d.setAttribute('aria-label', 'Slide ' + (i + 1))
      dotsWrap.appendChild(d)
    }
    const dots = [...dotsWrap.children]
    let idx = 0
    const go = (i) => {
      idx = (i + slides) % slides
      track.style.transform = `translateX(-${idx * 100}%)`
      dots.forEach((d, j) => d.classList.toggle('active', j === idx))
    }
    $('#sliderPrev').addEventListener('click', () => go(idx - 1))
    $('#sliderNext').addEventListener('click', () => go(idx + 1))
    dots.forEach((d, j) => d.addEventListener('click', () => go(j)))

    const lb = $('#lightbox'), lbImg = $('#lightboxImg')
    const close = () => { lb.hidden = true; document.body.style.overflow = '' }
    track.querySelectorAll('.slide img').forEach((img) => {
      img.addEventListener('click', () => { lbImg.src = img.src; lb.hidden = false; document.body.style.overflow = 'hidden' })
    })
    $('#lightboxClose').addEventListener('click', close)
    lb.addEventListener('click', (e) => { if (e.target === lb) close() })
    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') close()
      if (e.key === 'ArrowLeft') go(idx - 1)
      if (e.key === 'ArrowRight') go(idx + 1)
    })
  }

  /* ── Reveal ── */
  function setupReveal() {
    const els = document.querySelectorAll('.section-head, .feature-card, .how-step, .slide, .faq-item, .thanks-card, .cta-final, .statute')
    els.forEach((el) => el.classList.add('reveal'))
    const io = new IntersectionObserver((entries) => {
      entries.forEach((en) => { if (en.isIntersecting) { en.target.classList.add('visible'); io.unobserve(en.target) } })
    }, { threshold: 0.12 })
    els.forEach((el) => io.observe(el))
  }

  /* ── Init ── */
  function init() {
    const i18n = window.heliosI18n
    i18n.applyLang(i18n.detectLang())
    $('#langBtn').addEventListener('click', () => {
      i18n.applyLang(document.documentElement.lang === 'es' ? 'en' : 'es')
    })
    document.addEventListener('langchange', buildTicker)

    resizeSky()
    window.addEventListener('resize', resizeSky)

    buildTicker()
    setInterval(buildTicker, 10000)
    setupSlider()
    setupReveal()
    updateFlow(hourNow())

    let lastDom = 0
    const chartState = new Map()
    function chartNeedsRedraw(canvas) {
      const key = canvas.clientWidth + 'x' + canvas.clientHeight
      const st = chartState.get(canvas) || { size: '', min: -1 }
      const dirty = key !== st.size || new Date().getMinutes() !== st.min
      st.size = key; st.min = new Date().getMinutes()
      chartState.set(canvas, st)
      return dirty
    }
    function frame(ts) {
      const h = hourNow()
      drawSky(scrollPhase(), h)
      const heroChart = $('#heroChart'), theaterChart = $('#theaterChart'), theaterHist = $('#theaterHist')
      if (chartNeedsRedraw(heroChart)) drawDayChart(heroChart, true)
      if (chartNeedsRedraw(theaterChart)) drawDayChart(theaterChart, false)
      if (chartNeedsRedraw(theaterHist)) drawHist(theaterHist)
      updateFlowParticles(ts)
      if (ts - lastDom > 1000) {
        lastDom = ts
        updateStats()
        updateBattery(h)
        updateFlow(h)
      }
      requestAnimationFrame(frame)
    }
    requestAnimationFrame(frame)
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init)
  else init()
})()

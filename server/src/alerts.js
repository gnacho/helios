// alerts.js — motor de alertas push de Helios (alertas de SISTEMA de energía).
//
// Evalúa cada minuto el estado en vivo (computeLive + atributos crudos del
// scraper Solis) con detección de FLANCO y anti-rebote por ticks consecutivos:
// cada condición dispara UNA notificación al entrar y otra de recuperación al
// salir; no hay reenvíos mientras la condición persiste.
//
// Triggers (decisión del usuario 2-Ago-2026):
// - inversor_offline / inversor_ok: scraper.attributes.inverterOnline === 0,
//   sostenido 2 ticks (2 min) para no avisar por un hueco puntual del scraper.
//   SOLO se evalúa con scraper fresco (<15 min) y de DÍA (sun.sun
//   above_horizon): el inversor se apaga cada noche (decisión del usuario:
//   "está offline todas las noches") y alertar con datos viejos es avisar sin
//   información nueva (2-Ago: datos stale desde las 23:33 toda la noche).
//   Si sun.sun no está disponible se evalúa igualmente (fail-open: es una
//   alerta crítica y sun.sun es de las entidades más fiables de HAOS).
// - corte_red / corte_red_ok: HEURÍSTICA — con red presente la pinza de red
//   oscila siempre (import/export); en isla (backup Solis) grid ≈ 0 sostenido
//   mientras la casa sigue consumiendo desde batería/PV. Firma: scraper fresco
//   (<15 min) + grid < 0,02 kW + consumo > 0,1 kW + (batería descargando o PV
//   cubriendo el consumo), sostenido 3 ticks. Ajustar si da falsos positivos.
// - fox_offline / fox_ok: la pinza del Fox (pvFox) en 'unavailable'/'unknown'
//   sostenido 3 ticks, SOLO de día (el Fox se apaga cada noche igual que el
//   Solis; evaluar 24/7 sería falso positivo nocturno). Severidad high, no
//   critical: la casa sigue funcionando, solo falta producción.
// - bateria_baja: SOC ≤ reserva (install_config.batteryReservePct, defecto 20).
//   Se rearma cuando SOC > reserva + 5.
// - resumen_diario: scheduler propio a las 21:00 local con los KPI del día.
//
// notifyFn inyectable para tests (defecto: notifyAll de push.js).
// Cada disparo queda en audit_log (action 'alert:<tipo>') para poder ajustar
// umbrales con historial real (2-Ago-2026: corte_red no tenía historial y no
// se podía evaluar su tasa de falsos positivos).
import { kvGet, audit } from './db.js'
import { ENTITIES } from './config.js'
import { notifyAll } from './push.js'

const TICKS_INVERSOR_OFFLINE = 2
const TICKS_FOX_OFFLINE = 3
const TICKS_CORTE_RED = 3
const TICKS_RED_RECUPERADA = 2
const HISTERESIS_SOC = 5
export const HORA_RESUMEN_DIARIO = 21 // hora local

export function createAlertsEngine({ db, ha, solar, notifyFn = notifyAll }) {
  const estado = {
    inversor: { mal: 0, alertado: false },
    fox: { mal: 0, alertado: false },
    red: { mal: 0, ok: 0, alertado: false },
    bateria: { alertado: false },
  }

  function disparar(tipo, datos, opciones) {
    notifyFn(db, tipo, datos, opciones)
    try {
      audit(db, 'system', null, 'alert:' + tipo, { severity: opciones?.severity || 'normal', ...datos })
    } catch {
      /* el audit nunca debe romper la alerta */
    }
  }

  function reservaPct() {
    try {
      const cfg = JSON.parse(kvGet(db, 'install_config') || '{}')
      return typeof cfg.batteryReservePct === 'number' ? cfg.batteryReservePct : 20
    } catch {
      return 20
    }
  }

  function tick() {
    if (!ha.connected) return // sin HAOS no hay datos fiables; la alerta HAOS ya se ve por SSE
    const live = solar.computeLive(ha)
    const scraper = ha.getState(ENTITIES.scraper)
    const attrs = scraper?.attributes || {}
    const lastUpd = attrs.lastUpdate ? new Date(attrs.lastUpdate).getTime() : null
    const fresco = lastUpd !== null && Date.now() - lastUpd < 15 * 60000

    // ── Inversor offline: solo de día y con scraper fresco ──────────────
    const sun = ha.getState(ENTITIES.sun)
    const deDia = !sun || sun.state === 'above_horizon' // fail-open si falta sun.sun
    if (fresco && deDia) {
      if (attrs.inverterOnline === 0) {
        estado.inversor.mal++
        if (!estado.inversor.alertado && estado.inversor.mal >= TICKS_INVERSOR_OFFLINE) {
          estado.inversor.alertado = true
          disparar('inversor_offline', {}, { severity: 'critical' })
        }
      } else {
        if (estado.inversor.alertado) {
          estado.inversor.alertado = false
          disparar('inversor_ok', {}, { severity: 'normal' })
        }
        estado.inversor.mal = 0
      }
    }

    // ── Fox offline: pinza unavailable/unknown, solo de día ─────────────
    if (deDia) {
      const foxState = ha.getState(ENTITIES.pvFox)?.state
      if (foxState === 'unavailable' || foxState === 'unknown') {
        estado.fox.mal++
        if (!estado.fox.alertado && estado.fox.mal >= TICKS_FOX_OFFLINE) {
          estado.fox.alertado = true
          disparar('fox_offline', {}, { severity: 'high' })
        }
      } else {
        if (estado.fox.alertado) {
          estado.fox.alertado = false
          disparar('fox_ok', {}, { severity: 'normal' })
        }
        estado.fox.mal = 0
      }
    }

    // ── Corte de red (heurística, ver cabecera) ─────────────────────────
    const gridMag = Math.abs(Number(attrs.currentGridPower)) || 0
    const sinRed =
      fresco &&
      gridMag < 0.02 &&
      live.consumption > 0.1 &&
      (live.batteryPower < -0.05 || live.production > live.consumption)
    if (sinRed) {
      estado.red.mal++
      estado.red.ok = 0
      if (!estado.red.alertado && estado.red.mal >= TICKS_CORTE_RED) {
        estado.red.alertado = true
        disparar('corte_red', {}, { severity: 'critical' })
      }
    } else {
      estado.red.mal = 0
      if (estado.red.alertado) {
        estado.red.ok++
        if (estado.red.ok >= TICKS_RED_RECUPERADA) {
          estado.red.alertado = false
          estado.red.ok = 0
          disparar('corte_red_ok', {}, { severity: 'normal' })
        }
      }
    }

    // ── Batería baja (SOC ≤ reserva) ────────────────────────────────────
    const reserva = reservaPct()
    if (!estado.bateria.alertado && live.soc > 0 && live.soc <= reserva) {
      estado.bateria.alertado = true
      disparar('bateria_baja', { soc: live.soc, reserva }, { severity: 'high' })
    } else if (estado.bateria.alertado && live.soc > reserva + HISTERESIS_SOC) {
      estado.bateria.alertado = false
    }
  }

  return { tick, estado }
}

// Resumen diario: KPI del día (producción/consumo/autoconsumo) a todos los
// usuarios con el tipo activado. Severidad normal (respeta quiet hours).
export async function enviarResumenDiario(db, ha, solar, notifyFn = notifyAll) {
  const kpis = await solar.getKpis(ha, solar.todayStr(), db)
  const datos = {
    produccion: kpis.productionKwh,
    consumo: kpis.consumptionKwh,
    autoconsumo: Math.round(kpis.autoconsumoPct),
  }
  await notifyFn(db, 'resumen_diario', datos, { severity: 'normal' })
  try {
    audit(db, 'system', null, 'alert:resumen_diario', { severity: 'normal', ...datos })
  } catch {
    /* el audit nunca debe romper la alerta */
  }
}

// Scheduler del resumen: mismo patrón que scheduleNightly de index.js pero a
// las 21:00 local (la consolidación nocturna corre a las 00:10).
export function scheduleResumenDiario(db, ha, solar, notifyFn) {
  const now = new Date()
  const next = new Date(now)
  next.setHours(HORA_RESUMEN_DIARIO, 0, 0, 0)
  if (next.getTime() <= now.getTime()) next.setDate(next.getDate() + 1)
  setTimeout(async () => {
    try {
      await enviarResumenDiario(db, ha, solar, notifyFn)
      console.log('[helios] resumen diario enviado')
    } catch (err) {
      console.error('[helios] resumen diario error:', err.message)
    }
    scheduleResumenDiario(db, ha, solar, notifyFn)
  }, next.getTime() - now.getTime()).unref()
}

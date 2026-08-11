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
// - corte_red / corte_red_ok: FIRMA DIFERENCIAL con las pinzas Tongou del
//   cuadro de la vivienda (no el inversor). En un corte real con EPS del Solis
//   activo, el circuito NO respaldado (vivienda_medidor_power) cae a ~0 W
//   mientras el circuito RESPALDADO (medidor_respaldo_power) sigue funcionando
//   desde la batería. Firma: scraper fresco (<15 min) + gridMag bajo + pinza no
//   respaldada <50 W + pinza respaldada >30 W + batería DESCARGANDO (<-0.05 kW,
//   porque sin red el respaldo se alimenta de la batería y cargarla sería
//   imposible), sostenido 3 ticks. La cláusula de batería descargando filtra los
//   dropouts Zigbee de la pinza no respaldada. NO usa la señal del Fox
//   (foxess_r_volt / running_state) porque una caída del Modbus del Fox es
//   indistinguible de un corte real (incidente 12-Jul-2025: r_volt≈0 durante
//   20 h con la vivienda consumiendo 1.5 kW = dropout de comms, no corte
//   eléctrico). En un apagón total sin EPS ambas pinzas caen a 0 y esta alerta
//   no dispara; lo cubre inversor_offline por anti-isla.
// - fox_offline / fox_ok: la pinza del Fox (pvFox) en 'unavailable'/'unknown'
//   sostenido 3 ticks, SOLO de día (el Fox se apaga cada noche igual que el
//   Solis; evaluar 24/7 sería falso positivo nocturno). Severidad high, no
//   critical: la casa sigue funcionando, solo falta producción.
// - bateria_baja: SOC ≤ reserva (install_config.batteryReservePct, defecto 20).
//   Se rearma cuando SOC > reserva + 5.
// - resumen_diario: scheduler propio que se envía al ANOCHECER (next_setting de
//   sun.sun + un offset para asentar las últimas lecturas) con los KPI del día.
//   Fallback a hora fija si falta el dato solar. Ver proximoEnvioResumen.
//
// notifyFn inyectable para tests (defecto: notifyAll de push.js).
// Cada disparo queda en audit_log (action 'alert:<tipo>') para poder ajustar
// umbrales con historial real (2-Ago-2026: corte_red no tenía historial y no
// se podía evaluar su tasa de falsos positivos).
import { kvGet, kvSet, audit } from './db.js'
import { getInstall } from './install.js'
import { notifyAll } from './push.js'

const TICKS_INVERSOR_OFFLINE = 2
const TICKS_FOX_OFFLINE = 3
const TICKS_CORTE_RED = 3
const TICKS_RED_RECUPERADA = 2
const HISTERESIS_SOC = 5
export const HORA_RESUMEN_DIARIO = 21 // hora local (fallback si no hay dato solar)
// Minutos tras el anochecer antes de enviar el resumen: dejan que el último
// intervalo de producción del día quede registrado antes de leer los KPI.
export const RESUMEN_OFFSET_MIN = 10
// Reintento breve mientras esperamos a que sun.sun llegue como estado al
// arrancar/reconectar HAOS (el acuse de subscribe_entities precede a los
// estados). Si el fallback aún queda lejos, reprogramamos al anochecer real.
const RECHECK_BOOT_MS = 30_000

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

  // corte_red NO es detectable de forma fiable con los sensores actuales: la
  // pinza del circuito NO respaldado está casi siempre a ~0 W por consumo bajo
  // (no por corte), y el scraper no expone tensión de red ni flag de EPS/isla.
  // Firma observada en producción (9-Ago-2026): disparos que siguen la ventana
  // de descarga de la batería en autoconsumo nublado normal, sin evento de red.
  // → Desactivado por defecto (9-Ago-2026, issue #19). `inversor_offline`
  // cubre un apagón total y `fox_offline` la pérdida de la pinza del Fox.
  // Para rehabilitarlo: `install_config.corteRedEnabled = true` (kv).
  function corteRedEnabled() {
    try {
      const cfg = JSON.parse(kvGet(db, 'install_config') || '{}')
      return cfg.corteRedEnabled === true
    } catch {
      return false
    }
  }

  function tick() {
    if (!ha.connected) return // sin HAOS no hay datos fiables; la alerta HAOS ya se ve por SSE
    const t = getInstall()
    const live = solar.computeLive(ha)
    // Estado del inversor: sensor HAOS con atributos (statusAttrsId, opcional).
    // Sin él no hay señal de online/scraper → la alerta inversor no se evalúa.
    const statusAttrs = t.statusAttrsId ? ha.getState(t.statusAttrsId)?.attributes || {} : {}
    const lastUpd = statusAttrs.lastUpdate ? new Date(statusAttrs.lastUpdate).getTime() : null
    const fresco = lastUpd !== null && Date.now() - lastUpd < 15 * 60000

    // ── Inversor offline: solo con statusAttrs, de día y con datos frescos ──
    const sun = ha.getState(t.sun)
    const deDia = !sun || sun.state === 'above_horizon' // fail-open si falta sun.sun
    if (t.statusAttrsId && fresco && deDia) {
      if (statusAttrs.inverterOnline === 0) {
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

    // ── Inversor 2 offline (fox): pinza unavailable/unknown, solo de día ──
    // Solo si la topología tiene un 2º inversor (la alerta fox_offline está
    // pensada para la pinza local del Fox).
    const inv2 = t.inverters[1]
    if (deDia && inv2?.powerId) {
      const inv2State = ha.getState(inv2.powerId)?.state
      if (inv2State === 'unavailable' || inv2State === 'unknown') {
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

    // ── Corte de red (firma diferencial de pinzas, ver cabecera) ───────
    // Desactivado por defecto: ver corteRedEnabled(). Requiere además la
    // topología con circuitos respaldado/no-respaldado y grid por atributos
    // (si no, no hay firma).
    if (corteRedEnabled() && t.grid.mode === 'attrs' && t.consumption.respaldoId && t.consumption.noRespaldadaId) {
    const gridMag = Math.abs(Number(statusAttrs.currentGridPower)) || 0
    const respaldoKw = live.respaldoKw ?? 0
    const noRespaldadaKw = live.noRespaldadaKw ?? 0
    // La batería debe estar DESCARGANDO: en un corte real con EPS del Solis
    // sosteniendo el respaldo, la energía viene de la batería (sin red no se
    // podría cargar). Esto filtra los dropouts de la pinza no respaldada
    // (caso real 9-Ago-2026: pinza Zigbee en dropout reportando 0 W con la
    // red perfecta y la batería cargando desde FV → disparó falso crítico).
    const sinRed =
      fresco &&
      gridMag < 0.05 &&
      noRespaldadaKw < 0.05 &&
      respaldoKw > 0.03 &&
      live.batteryPower < -0.05
    if (sinRed) {
      estado.red.mal++
      estado.red.ok = 0
      if (!estado.red.alertado && estado.red.mal >= TICKS_CORTE_RED) {
        estado.red.alertado = true
        disparar('corte_red', { gridMag, respaldoKw, noRespaldadaKw, batteryPower: live.batteryPower, fresco }, { severity: 'critical' })
      }
    } else {
      estado.red.mal = 0
      if (estado.red.alertado) {
        estado.red.ok++
        if (estado.red.ok >= TICKS_RED_RECUPERADA) {
          estado.red.alertado = false
          estado.red.ok = 0
          disparar('corte_red_ok', { gridMag, respaldoKw, noRespaldadaKw, batteryPower: live.batteryPower, fresco }, { severity: 'normal' })
        }
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

// Calcula el próximo momento de envío del resumen a partir del anochecer.
// sun.sun.attributes.next_setting es SIEMPRE el próximo anochecer (instante
// futuro, hoy si no ha anochecido o mañana si ya): disparamos sunset +
// RESUMEN_OFFSET_MIN. Si no hay dato solar (HAOS caído), fallback a hora fija
// HORA_RESUMEN_DIARIO (hoy si no ha pasado, mañana en caso contrario). Así nunca
// se salta un día. La guarda anti-doble-envío del mismo día la lleva
// ejecutarResumenSiToca (persistida en kv). `ahora` se inyecta para tests.
export function proximoEnvioResumen(ha, ahora = new Date()) {
  const sunsetIso = ha.getState(getInstall().sun)?.attributes?.next_setting
  const sunset = sunsetIso ? new Date(sunsetIso) : null
  if (sunset && Number.isFinite(sunset.getTime())) {
    return new Date(sunset.getTime() + RESUMEN_OFFSET_MIN * 60000)
  }
  const fb = new Date(ahora)
  fb.setHours(HORA_RESUMEN_DIARIO, 0, 0, 0)
  if (fb.getTime() <= ahora.getTime()) fb.setDate(fb.getDate() + 1)
  return fb
}

// Clave de día local (YYYY-MM-DD) — la hora del anochecer es local y la guarda
// debe contar por día calendario del servidor.
function fechaLocalKey(d) {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const dia = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${dia}`
}

const KV_RESUMEN_FECHA = 'resumen_ultimo_envio'

// Envía el resumen SOLO si no se ha enviado ya hoy (guarda persistida en kv,
// sobrevive a reinicios). Devuelve true si envió. Evita el doble-envío cuando
// el path fallback (21:00) y el path sunset (+offset) caen el mismo día.
export async function ejecutarResumenSiToca(db, ha, solar, notifyFn = notifyAll) {
  const ahora = new Date()
  if (kvGet(db, KV_RESUMEN_FECHA) === fechaLocalKey(ahora)) return false
  await enviarResumenDiario(db, ha, solar, notifyFn)
  kvSet(db, KV_RESUMEN_FECHA, fechaLocalKey(ahora))
  return true
}

// Scheduler del resumen: se envía al anochecer (ver proximoEnvioResumen); la
// consolidación nocturna corre a las 00:10. Delay con suelo de 1 min por
// seguridad anti-bucle cerrado.
export function scheduleResumenDiario(db, ha, solar, notifyFn) {
  const ahora = new Date()
  const next = proximoEnvioResumen(ha, ahora)
  const sunsetDisponible = !!ha.getState(getInstall().sun)?.attributes?.next_setting
  const delay = next.getTime() - ahora.getTime()
  // Carrera de arranque: al iniciar (o justo tras reconectar HAOS) sun.sun
  // puede no haber llegado todavía como estado. Si el dato no está y el
  // fallback aún queda lejos, reintentamos en 30 s para programar al anochecer
  // real; si sigue sin haber sunset al acercarnos al fallback, usamos la hora
  // fija (HAOS caído todo el día).
  if (!sunsetDisponible && delay > RECHECK_BOOT_MS) {
    setTimeout(() => scheduleResumenDiario(db, ha, solar, notifyFn), RECHECK_BOOT_MS).unref()
    return
  }
  console.log(`[helios] resumen diario programado para ${next.toISOString()}`)
  setTimeout(async () => {
    try {
      const envio = await ejecutarResumenSiToca(db, ha, solar, notifyFn)
      console.log(envio ? '[helios] resumen diario enviado' : '[helios] resumen diario ya enviado hoy, omitido')
    } catch (err) {
      console.error('[helios] resumen diario error:', err.message)
    }
    scheduleResumenDiario(db, ha, solar, notifyFn)
  }, Math.max(delay, 60_000)).unref()
}

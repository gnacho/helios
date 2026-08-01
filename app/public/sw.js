/*
 * Helios — service worker (notificaciones push).
 * Mínimo a propósito: SIN caché ni fetch handler (la app siempre va a red;
 * el SW existe solo para Web Push). Solo se registra en contextos seguros
 * (HTTPS o localhost); en LAN HTTP `navigator.serviceWorker` es undefined.
 * Actualización: SKIP_WAITING desde la UI (Ajustes → Acerca de) si se añade.
 */

self.addEventListener('install', () => self.skipWaiting())
self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim())
})

self.addEventListener('message', (event) => {
  if (event.data && event.data.type === 'SKIP_WAITING') self.skipWaiting()
})

// === PUSH: recepción =========================================================
// REGLA CRÍTICA: TODO evento push termina en showNotification() — si no,
// Chrome muestra un aviso genérico y Safari REVOCA el permiso.
// El servidor envía payload híbrido: campos planos (title/body/url/tag) para
// este handler + bloque "web_push" (Declarative Web Push, Safari/iOS 18.4+).
self.addEventListener('push', (event) => {
  event.waitUntil(
    (async () => {
      let datos = {}
      try {
        datos = event.data ? event.data.json() : {}
      } catch {
        datos = {} // payload corrupto: se muestra el fallback igualmente
      }
      await self.registration.showNotification(datos.title || 'Helios', {
        body: datos.body || 'Tienes una alerta de energía',
        icon: '/pwa-icon-192.png',
        badge: '/pwa-icon-192.png',
        tag: datos.tag || 'default', // coalescing: mismo tag reemplaza la anterior
        renotify: true,
        data: { url: datos.url || '/' },
        // NO actions/image/requireInteraction: no soportados en iOS.
      })
    })(),
  )
})

// === PUSH: click en la notificación ==========================================
self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const url = (event.notification.data && event.notification.data.url) || '/'
  event.waitUntil(
    clients
      .matchAll({ type: 'window', includeUncontrolled: true })
      .then((lista) => {
        for (const cliente of lista) {
          if (cliente.url === url && 'focus' in cliente) return cliente.focus()
        }
        return clients.openWindow(url)
      }),
  )
})

// === PUSH: renovación automática de la suscripción ===========================
// Red de seguridad (cobertura irregular entre navegadores): re-suscribe y
// re-envía al servidor (upsert por endpoint). La higiene principal es el
// borrado por 404/410 en el sender.
self.addEventListener('pushsubscriptionchange', (event) => {
  event.waitUntil(
    self.registration.pushManager
      .subscribe({
        userVisibleOnly: true,
        applicationServerKey:
          event.oldSubscription && event.oldSubscription.options
            ? event.oldSubscription.options.applicationServerKey
            : undefined,
      })
      .then((sub) =>
        fetch('/api/push/subscribe', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(sub),
        }),
      ),
  )
})

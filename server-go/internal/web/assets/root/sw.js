const ctx = self;
ctx.addEventListener("install", () => ctx.skipWaiting());
ctx.addEventListener("activate", (event) => {
  event.waitUntil(ctx.clients.claim());
});
ctx.addEventListener("message", (event) => {
  if (event.data && event.data.type === "SKIP_WAITING") ctx.skipWaiting();
});
ctx.addEventListener("push", (event) => {
  event.waitUntil(
    (async () => {
      let datos = {};
      try {
        datos = event.data ? event.data.json() : {};
      } catch {
        datos = {};
      }
      await ctx.registration.showNotification(datos.title || "Helios", {
        body: datos.body || "Tienes una alerta de energ\xEDa",
        icon: "/pwa-icon-192.png",
        badge: "/pwa-icon-192.png",
        tag: datos.tag || "default",
        // coalescing: mismo tag reemplaza la anterior
        renotify: true,
        data: { url: datos.url || "/" }
        // NO actions/image/requireInteraction: no soportados en iOS.
      });
    })()
  );
});
ctx.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = event.notification.data && event.notification.data.url || "/";
  event.waitUntil(
    ctx.clients.matchAll({ type: "window", includeUncontrolled: true }).then((lista) => {
      for (const cliente of lista) {
        if (cliente.url === url && "focus" in cliente) return cliente.focus();
      }
      return ctx.clients.openWindow(url);
    })
  );
});
ctx.addEventListener("pushsubscriptionchange", (event) => {
  event.waitUntil(
    ctx.registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: event.oldSubscription && event.oldSubscription.options ? event.oldSubscription.options.applicationServerKey : void 0
    }).then(
      (sub) => fetch("/api/push/subscribe", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(sub)
      })
    )
  );
});

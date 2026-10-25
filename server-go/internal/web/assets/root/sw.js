// Helios service worker: push notifications plus an offline shell.
// The app is server rendered, so the cache is runtime only: whatever page the
// user opened last is what shows up with no network, and the assets keep
// working because they carry a versioned query string.
const ctx = self;

const SHELL = "helios-shell-v1";
const ASSETS = "helios-assets-v1";
const OFFLINE_URL = "/offline.html";
// Root files served from the binary's embed (see handleRootFile).
const ROOT_FILES = new Set([
  "/favicon.svg",
  "/favicon-32.png",
  "/apple-touch-icon.png",
  "/pwa-icon-192.png",
  "/pwa-icon-512.png",
  "/manifest.webmanifest",
  "/empty-solar.svg",
  OFFLINE_URL,
]);
const PRECACHE = [OFFLINE_URL, "/favicon.svg", "/manifest.webmanifest", "/pwa-icon-192.png"];

ctx.addEventListener("install", (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(SHELL);
      // One by one: a single 404 must not abort the whole install.
      await Promise.all(PRECACHE.map((url) => cache.add(url).catch(() => {})));
      await ctx.skipWaiting();
    })()
  );
});

ctx.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      const keep = new Set([SHELL, ASSETS]);
      const names = await caches.keys();
      await Promise.all(names.filter((name) => !keep.has(name)).map((name) => caches.delete(name)));
      await ctx.clients.claim();
    })()
  );
});

// Live data, Datastar streams and session changes never go to the cache.
function isLive(url) {
  return (
    url.pathname.startsWith("/api/") ||
    url.pathname.startsWith("/ui/") ||
    url.pathname === "/health" ||
    url.pathname === "/login" ||
    url.pathname === "/logout"
  );
}

ctx.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;
  let url;
  try {
    url = new URL(req.url);
  } catch {
    return;
  }
  if (url.origin !== ctx.location.origin || isLive(url)) return;

  // Pages: network first, last visit as the fallback, offline notice if none.
  if (req.mode === "navigate") {
    event.respondWith(
      (async () => {
        try {
          const res = await fetch(req);
          if (res && res.ok) {
            const cache = await caches.open(SHELL);
            cache.put(req, res.clone()).catch(() => {});
          }
          return res;
        } catch {
          const cache = await caches.open(SHELL);
          return (
            (await cache.match(req)) ||
            (await cache.match(OFFLINE_URL)) ||
            new Response("Sin conexión", { status: 503, headers: { "Content-Type": "text/plain; charset=utf-8" } })
          );
        }
      })()
    );
    return;
  }

  // Assets: cache first. Their URLs carry ?v=<hash>, so a release changes them
  // and the old entry simply stops being requested.
  if (url.pathname.startsWith("/assets/") || ROOT_FILES.has(url.pathname)) {
    event.respondWith(
      (async () => {
        const cache = await caches.open(ASSETS);
        const hit = await cache.match(req);
        if (hit) return hit;
        try {
          const res = await fetch(req);
          if (res && res.ok) cache.put(req, res.clone()).catch(() => {});
          return res;
        } catch {
          return new Response("", { status: 504 });
        }
      })()
    );
  }
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
        body: datos.body || "Tienes una alerta de energía",
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

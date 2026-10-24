// gestures.js - static helper (no app logic): horizontal swipe between the
// page's tabs, mirroring the React useTabSwipe behavior. Vertical gestures
// are left to the browser (scroll / pull-to-refresh).
(function () {
  "use strict";
  var sx = 0, sy = 0, tracking = false;

  function tabLinks() {
    var strip = document.querySelector("[data-swipe-tabs]");
    if (!strip) return null;
    var links = Array.prototype.slice.call(strip.querySelectorAll("a[href*='=']"));
    if (links.length < 2) return null;
    var params = links.map(function (a) {
      return new URL(a.href, location.href).searchParams;
    });
    var keyName = params[0].hasOwnProperty("inv") || params.some(function (p) { return p.has("inv"); }) ? "inv" : "tab";
    var keys = params.map(function (p) { return p.get(keyName); });
    var current = new URLSearchParams(location.search).get(keyName) || keys[0];
    var idx = keys.indexOf(current);
    return { keyName: keyName, keys: keys, idx: idx, links: links };
  }

  document.addEventListener("touchstart", function (e) {
    if (e.touches.length !== 1) return;
    sx = e.touches[0].clientX;
    sy = e.touches[0].clientY;
    tracking = true;
  }, { passive: true });

  document.addEventListener("touchend", function (e) {
    if (!tracking) return;
    tracking = false;
    var t = tabLinks();
    if (!t || t.idx === -1) return;
    var dx = e.changedTouches[0].clientX - sx;
    var dy = e.changedTouches[0].clientY - sy;
    if (Math.abs(dx) < 70 || Math.abs(dy) > 60) return;
    var next = t.idx + (dx < 0 ? 1 : -1);
    if (next < 0 || next >= t.keys.length) return;
    var url = new URL(t.links[next].href, location.href);
    // go() keeps SPA-less navigation; the view transition animates it.
    location.assign(url.pathname + url.search);
  }, { passive: true });
})();

// Autosubmit de inputs de fichero (picker de foto del cargador): sin esto el
// navegador no envía nada hasta pulsar un botón y el gesto queda a medias.
document.addEventListener("change", function (e) {
  var el = e.target;
  if (el && el.matches && el.matches("input[data-autosubmit]") && el.form) {
    el.form.requestSubmit();
  }
});

// Service worker: la PWA (manifest + push) la sirve el backend; aqui solo se
// registra. Sin esto las notificaciones push dejaban de llegar en la UI templ.
if ("serviceWorker" in navigator) {
  window.addEventListener("load", function () {
    navigator.serviceWorker.register("/sw.js").catch(function () {});
  });
}

// Navegacion sin recargar: al cambiar de menu se parchea el contenido y se
// actualiza la URL con pushState, asi que atras/adelante deben volver a pedir
// el fragmento. Si la ruta no es del menu (p.ej. un enlace con parametros), se
// recarga la pagina entera.
window.addEventListener("popstate", function () {
  var path = window.location.pathname;
  var enlaces = document.querySelectorAll('a[href="' + path + '"]');
  for (var i = 0; i < enlaces.length; i++) {
    if (enlaces[i].hasAttribute("data-on:click__prevent")) {
      enlaces[i].click();
      return;
    }
  }
  window.location.reload();
});

// gestures.js - static helper (no app logic): drag-to-swipe between the
// page's tabs, mirroring the React useTabSwipe hook. The server paints every
// panel side by side in a track, so the neighbour is already there and the
// finger moves it 1:1. Vertical gestures stay with the browser.
(function () {
  "use strict";
  var MIN_DX = 12;          // if smaller, it is a tap, not a drag
  var SNAP_FRACTION = 0.22; // of the pager width, like the React hook
  var EDGE_DAMPING = 0.35;  // elastic resistance when there is no neighbour
  var desktop = window.matchMedia("(min-width: 1024px)");
  var drag = null;

  function trackOf(pager) {
    return pager.querySelector("[data-pager-track]");
  }

  function panelsOf(pager) {
    var track = trackOf(pager);
    return track ? Array.prototype.slice.call(track.children) : [];
  }

  function activeIndex(pager) {
    var i = parseInt(pager.getAttribute("data-active") || "0", 10);
    return isNaN(i) ? 0 : i;
  }

  function moveTo(pager, index, animate) {
    var track = trackOf(pager);
    if (!track) return;
    track.style.transition = animate
      ? "transform 220ms cubic-bezier(.22,1,.36,1)"
      : "none";
    track.style.transform = "translate3d(" + (-index * 100) + "%, 0, 0)";
    if (animate) {
      pager.setAttribute("data-active", String(index));
      paintTabs(index);
      pushURL(index);
    }
    fitHeight(pager, animate);
  }

  // fitHeight keeps the clipped wrapper as tall as the visible panel only.
  function fitHeight(pager, animate) {
    if (desktop.matches) {
      // On desktop only the active panel is laid out (the rest are display:none
      // at lg), so any translate would push it out of the viewport.
      var wideTrack = trackOf(pager);
      if (wideTrack) {
        wideTrack.style.transform = "";
        wideTrack.style.transition = "";
      }
      pager.style.height = "";
      pager.style.transition = "";
      return;
    }
    var panel = panelsOf(pager)[activeIndex(pager)];
    if (!panel) return;
    pager.style.transition = animate ? "height 220ms cubic-bezier(.22,1,.36,1)" : "none";
    pager.style.height = panel.offsetHeight + "px";
  }

  function tabs() {
    var strip = document.querySelector("[data-swipe-tabs]");
    return strip ? Array.prototype.slice.call(strip.querySelectorAll("a[role='tab']")) : [];
  }

  // paintTabs moves the pill and the selected state without asking the server.
  function paintTabs(index) {
    tabs().forEach(function (a, j) {
      var on = j === index;
      a.setAttribute("aria-selected", on ? "true" : "false");
      a.classList.toggle("text-app", on);
      a.classList.toggle("text-faint", !on);
      a.classList.toggle("hover:text-muted", !on);
      var pill = a.querySelector(".vt-slide-tab");
      if (on && !pill) {
        pill = document.createElement("span");
        pill.className = "vt-slide-tab absolute inset-0 rounded-full bg-surface-2 shadow-inner";
        a.insertBefore(pill, a.firstChild);
      } else if (!on && pill) {
        pill.remove();
      }
      var dot = a.querySelector("[data-tab-dot]");
      if (dot) dot.style.opacity = on ? "1" : "0.4";
      if (a.hasAttribute("data-tab-color")) {
        a.style.color = on ? a.getAttribute("data-tab-color") : "";
      }
    });
  }

  // pushURL keeps the address bar in sync (replace: no extra history entry).
  function pushURL(index) {
    var link = tabs()[index];
    if (!link) return;
    var url = new URL(link.getAttribute("href"), location.href);
    if (url.pathname + url.search === location.pathname + location.search) return;
    history.replaceState(history.state, "", url.pathname + url.search);
  }

  function sync() {
    if (drag) return; // never fight the finger
    Array.prototype.forEach.call(document.querySelectorAll("[data-pager]"), function (pager) {
      moveTo(pager, activeIndex(pager), false);
    });
  }

  document.addEventListener("touchstart", function (e) {
    drag = null;
    if (e.touches.length !== 1 || desktop.matches) return;
    var t = e.target;
    if (t.closest && t.closest("nav, [data-swipe-tabs], [role='dialog'], [data-no-swipe], input, textarea, select")) return;
    var pager = t.closest ? t.closest("[data-pager]") : null;
    if (!pager || panelsOf(pager).length < 2) return;
    drag = {
      pager: pager,
      x0: e.touches[0].clientX,
      y0: e.touches[0].clientY,
      index: activeIndex(pager),
      axis: null,
      dx: 0
    };
  }, { passive: true, capture: true });

  document.addEventListener("touchmove", function (e) {
    if (!drag || e.touches.length !== 1) return;
    var dx = e.touches[0].clientX - drag.x0;
    var dy = e.touches[0].clientY - drag.y0;
    if (drag.axis === null) {
      if (Math.abs(dx) < MIN_DX) return;
      // Vertical-ish gesture: leave it to the browser (scroll).
      if (Math.abs(dx) <= Math.abs(dy) * 1.2) {
        drag = null;
        return;
      }
      drag.axis = "x";
    }
    e.preventDefault();
    drag.dx = dx;
    var track = trackOf(drag.pager);
    if (!track) return;
    var count = panelsOf(drag.pager).length;
    var atEdge = (dx < 0 && drag.index === count - 1) || (dx > 0 && drag.index === 0);
    var offset = atEdge ? dx * EDGE_DAMPING : dx;
    var base = -drag.index * drag.pager.clientWidth;
    track.style.transition = "none";
    track.style.transform = "translate3d(" + (base + offset) + "px, 0, 0)";
  }, { passive: false, capture: true });

  function endDrag() {
    if (!drag) return;
    var d = drag;
    drag = null;
    if (d.axis !== "x") return;
    var count = panelsOf(d.pager).length;
    var next = d.index;
    if (Math.abs(d.dx) / d.pager.clientWidth >= SNAP_FRACTION) {
      var target = d.index + (d.dx < 0 ? 1 : -1);
      if (target >= 0 && target < count) next = target;
    }
    moveTo(d.pager, next, true);
  }

  document.addEventListener("touchend", endDrag, { passive: true, capture: true });
  document.addEventListener("touchcancel", endDrag, { passive: true, capture: true });

  // Panels change height with the data and with the language: keep the clip
  // on the visible one. The pager is re-created by every Datastar patch, so a
  // MutationObserver is the cheapest hook to re-apply the geometry.
  var pending = false;
  function scheduleSync() {
    if (pending || drag) return;
    pending = true;
    requestAnimationFrame(function () {
      pending = false;
      sync();
    });
  }
  new MutationObserver(scheduleSync).observe(document.body, { childList: true, subtree: true });
  window.addEventListener("resize", scheduleSync);
  desktop.addEventListener("change", scheduleSync);
  window.addEventListener("load", scheduleSync);
  sync();
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

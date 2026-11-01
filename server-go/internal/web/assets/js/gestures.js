// gestures.js - static helper (no app logic): drag-to-swipe between the
// page's tabs, mirroring the React useTabSwipe hook. The server paints every
// panel side by side in a track, so the neighbour is already there and the
// finger moves it 1:1. Vertical gestures stay with the browser.
(function () {
  "use strict";
  var MIN_DX = 12;          // if smaller, it is a tap, not a drag
  var SNAP_FRACTION = 0.22; // of the pager width, like the React hook
  var EDGE_DAMPING = 0.35;  // elastic resistance when there is no neighbour
  var SLIDE_MS = 300;       // commit: long enough to read as a glide
  var BACK_MS = 220;        // snap back to the tab we were on
  var SLIDE_EASE = "cubic-bezier(.32,.72,0,1)"; // decelerates late, no snap
  var FLICK_VELOCITY = 0.4; // px/ms that commit below SNAP_FRACTION
  var VELOCITY_WINDOW = 90; // ms of drag samples used for the flick speed
  var desktop = window.matchMedia("(min-width: 1024px)");
  var reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
  var drag = null;

  // motionOff covers both the system setting and the app's own switch, which
  // lands as a class on <html> (same condition anim.templ uses).
  function motionOff() {
    return reduceMotion.matches || document.documentElement.classList.contains("reduce-motion");
  }
  // settle holds, per pager, the moment its slide ends. A stream patch that
  // lands inside that window must wait: writing the final transform with no
  // transition is what made the swipe look like a single frame (#217).
  var settle = new WeakMap();

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

  function moveTo(pager, index, animate, ms, ease) {
    var track = trackOf(pager);
    if (!track) return;
    var motion = animate && !motionOff();
    ms = ms || SLIDE_MS;
    ease = ease || SLIDE_EASE;
    track.style.transition = motion ? "transform " + ms + "ms " + ease : "none";
    track.style.transform = "translate3d(" + (-index * 100) + "%, 0, 0)";
    if (animate) {
      pager.setAttribute("data-active", String(index));
      // Mark the slide before touching the DOM: paintTabs mutates the tree and
      // that mutation reaches the observer, so a sync could otherwise cancel
      // the transition we are about to start.
      if (motion) {
        settle.set(pager, performance.now() + ms + 20);
        scheduleSyncAfter(ms + 40);
      } else {
        settle.delete(pager);
      }
      paintTabs(index, motion ? ms : 0, ease);
      pushURL(index);
    } else {
      settle.delete(pager);
    }
    fitHeight(pager, motion, ms, ease);
  }

  // fitHeight keeps the clipped wrapper as tall as the visible panel only.
  function fitHeight(pager, animate, ms, ease) {
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
    pager.style.transition = animate
      ? "height " + (ms || SLIDE_MS) + "ms " + (ease || SLIDE_EASE)
      : "none";
    pager.style.height = panel.offsetHeight + "px";
  }

  function tabs() {
    var strip = document.querySelector("[data-swipe-tabs]");
    return strip ? Array.prototype.slice.call(strip.querySelectorAll("a[role='tab']")) : [];
  }

  // paintTabs moves the pill and the selected state without asking the server.
  // On a swipe the pill travels with the panel (FLIP) instead of appearing in
  // the new tab: a tap already glides through the view-transition group, and
  // both paths should look the same (#217).
  function paintTabs(index, ms, ease) {
    var list = tabs();
    var from = null;
    list.forEach(function (a) {
      var p = a.querySelector(".vt-slide-tab");
      if (p) from = p.getBoundingClientRect();
    });
    list.forEach(function (a, j) {
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
    var to = list[index] && list[index].querySelector(".vt-slide-tab");
    if (!from || !to || !ms || !to.animate) return;
    var r = to.getBoundingClientRect();
    if (!r.width || !r.height) return;
    var dx = from.left - r.left;
    var dy = from.top - r.top;
    var sx = from.width / r.width;
    var sy = from.height / r.height;
    if (!dx && !dy && sx === 1 && sy === 1) return;
    to.animate([
      { transform: "translate(" + dx + "px," + dy + "px) scale(" + sx + "," + sy + ")", transformOrigin: "0 0" },
      { transform: "none", transformOrigin: "0 0" }
    ], { duration: ms, easing: ease });
  }

  // pushURL keeps the address bar in sync (replace: no extra history entry).
  function pushURL(index) {
    var link = tabs()[index];
    if (!link) return;
    // A malformed href must not abort the gesture half way.
    try {
      var url = new URL(link.getAttribute("href"), location.href);
      if (url.pathname + url.search === location.pathname + location.search) return;
      history.replaceState(history.state, "", url.pathname + url.search);
    } catch (err) {
      /* keep the swipe, the address bar is cosmetic */
    }
  }

  function sync() {
    if (drag) return; // never fight the finger
    var wait = 0;
    Array.prototype.forEach.call(document.querySelectorAll("[data-pager]"), function (pager) {
      var until = settle.get(pager);
      if (until !== undefined) {
        var left = until - performance.now();
        if (left > 0) {
          // Mid-slide: re-applying the geometry with no transition here is
          // what turned a swipe into a jump (#217). Come back when it ends.
          if (left > wait) wait = left;
          return;
        }
        settle.delete(pager);
      }
      moveTo(pager, activeIndex(pager), false);
    });
    if (wait) scheduleSyncAfter(wait + 40);
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
      dx: 0,
      samples: [{ x: e.touches[0].clientX, t: e.timeStamp }]
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
    drag.samples.push({ x: e.touches[0].clientX, t: e.timeStamp });
    if (drag.samples.length > 8) drag.samples.shift();
    var track = trackOf(drag.pager);
    if (!track) return;
    var count = panelsOf(drag.pager).length;
    var atEdge = (dx < 0 && drag.index === count - 1) || (dx > 0 && drag.index === 0);
    var offset = atEdge ? dx * EDGE_DAMPING : dx;
    var base = -drag.index * drag.pager.clientWidth;
    track.style.transition = "none";
    track.style.transform = "translate3d(" + (base + offset) + "px, 0, 0)";
  }, { passive: false, capture: true });

  // flickVelocity returns the horizontal speed of the last VELOCITY_WINDOW ms
  // in px/ms. A finger that stops before lifting reads as slow on purpose.
  function flickVelocity(samples, now) {
    if (!samples || samples.length < 2) return 0;
    var last = samples[samples.length - 1];
    if (now && now - last.t > 120) return 0;
    var first = last;
    for (var i = samples.length - 1; i >= 0; i--) {
      first = samples[i];
      if (last.t - samples[i].t >= VELOCITY_WINDOW) break;
    }
    var dt = last.t - first.t;
    if (dt < 24) return 0;
    return (last.x - first.x) / dt;
  }

  function endDrag(e) {
    if (!drag) return;
    var d = drag;
    drag = null;
    if (d.axis !== "x") return;
    var count = panelsOf(d.pager).length;
    var next = d.index;
    var width = d.pager.clientWidth || 1;
    var v = flickVelocity(d.samples, e && e.timeStamp);
    // A fast flick commits even when it covers less than SNAP_FRACTION: that
    // was the other half of the abrupt feel, a clear gesture bouncing back.
    var flick = Math.abs(v) >= FLICK_VELOCITY && (v < 0) === (d.dx < 0);
    if (Math.abs(d.dx) >= MIN_DX && (Math.abs(d.dx) / width >= SNAP_FRACTION || flick)) {
      var target = d.index + (d.dx < 0 ? 1 : -1);
      if (target >= 0 && target < count) next = target;
    }
    moveTo(d.pager, next, true, next === d.index ? BACK_MS : SLIDE_MS, SLIDE_EASE);
  }

  document.addEventListener("touchend", endDrag, { passive: true, capture: true });
  document.addEventListener("touchcancel", endDrag, { passive: true, capture: true });

  // Panels change height with the data and with the language: keep the clip
  // on the visible one. The pager is re-created by every Datastar patch, so a
  // MutationObserver is the cheapest hook to re-apply the geometry.
  var pending = false;
  var settleTimer = 0;
  // scheduleSyncAfter re-runs the geometry once a slide is over, so a patch
  // that arrived mid-animation still gets applied.
  function scheduleSyncAfter(ms) {
    clearTimeout(settleTimer);
    settleTimer = setTimeout(scheduleSync, ms);
  }
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

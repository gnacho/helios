/**
 * countup.js - make rendered numbers count up.
 *
 * Opt-in: add `data-countup` to an element whose text is a number (optionally
 * with prefix/suffix like "kW"). On first paint the number animates from 0 to
 * its server-rendered value; when a live update (SSE signal merge) rewrites
 * the text, it animates from the previous value to the new one instead.
 * No animation under reduce-motion (Ajustes switch or the OS setting).
 *
 * Vanilla and dependency-free: the same exception as gestures.js.
 */
(() => {
  'use strict';


  const reduceMotion = () =>
    document.documentElement.classList.contains('reduce-motion') ||
    window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  // Splits "Solar 12,4 kWh" into ["Solar ", "12,4", " kWh"]: the number is the
  // LAST numeric token, so labels that contain digits keep their text.
  const NUM_RE = /^(\D*?)(-?\d[\d\s\u00a0.,']*)(\D*)$/;

  function parseText(text) {
    const m = NUM_RE.exec(text);
    if (!m) return null;
    const raw = m[2].replace(/[\s\u00a0']/g, '');
    const sepIdx = Math.max(raw.lastIndexOf('.'), raw.lastIndexOf(','));
    let dec = 0;
    let canon = raw;
    if (sepIdx !== -1) {
      const tail = raw.length - sepIdx - 1;
      if (tail === 1 || tail === 2) {
        dec = tail; // short tail = decimal mark: "12,4" / "0.35"
        canon = raw.slice(0, sepIdx).replace(/[.,]/g, '') + '.' + raw.slice(sepIdx + 1);
      } else {
        canon = raw.replace(/[.,]/g, ''); // long tail = group separator: "1.234"
      }
    }
    const value = parseFloat(canon);
    if (!isFinite(value)) return null;
    return { prefix: m[1], suffix: m[3], value, dec };
  }

  function formatter(dec) {
    try {
      return new Intl.NumberFormat(document.documentElement.lang || 'es', {
        minimumFractionDigits: dec,
        maximumFractionDigits: dec,
      });
    } catch (_) {
      return new Intl.NumberFormat('es', { minimumFractionDigits: dec, maximumFractionDigits: dec });
    }
  }

  function animate(el, from, to, info) {
    const dur = 800;
    const easeOut = (t) => 1 - Math.pow(1 - t, 3);
    const fmt = formatter(info.dec);
    if (el._countupRaf) cancelAnimationFrame(el._countupRaf);
    // t0 starts at the FIRST animation frame: performance.now() at call time
    // can be far ahead of the next frame timestamp (hidden/headless tabs) and
    // the whole animation would collapse into its last frame.
    let t0 = null;
    const frame = (now) => {
      if (t0 === null) t0 = now;
      const k = Math.min(1, (now - t0) / dur);
      const v = from + (to - from) * easeOut(k);
      el.textContent = info.prefix + fmt.format(v) + info.suffix;
      if (k < 1) {
        el._countupRaf = requestAnimationFrame(frame);
      } else {
        el._countupRaf = null;
      }
    };
    el._countupRaf = requestAnimationFrame(frame);
  }

  function maybeAnimate(el) {
    if (el._countupRaf) return; // already counting; the final value lands alone
    const info = parseText(el.textContent);
    if (!info) return;
    const prev = el._countupPrev;
    el._countupPrev = info.value;
    if (prev === undefined) {
      animate(el, 0, info.value, info); // first paint: from zero
    } else if (prev !== info.value) {
      animate(el, prev, info.value, info); // live update: from previous
    }
  }

  const scan = (root) => {
    if (reduceMotion()) return;
    root.querySelectorAll('[data-countup]').forEach(maybeAnimate);
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => scan(document));
  } else {
    scan(document);
  }

  // Live updates: Datastar rewrites the text of [data-text] nodes on signal
  // merges, so watch character data and re-trigger from the previous value.
  const obs = new MutationObserver((muts) => {
    if (reduceMotion()) return;
    for (const m of muts) {
      if (m.type === 'childList') {
        // SSE fragments can re-insert whole sections: count their numbers too.
        m.addedNodes.forEach((n) => {
          if (n.nodeType !== 1) return;
          if (n.hasAttribute && n.hasAttribute('data-countup')) maybeAnimate(n);
          if (n.querySelectorAll) n.querySelectorAll('[data-countup]').forEach(maybeAnimate);
        });
        continue;
      }
      const el = m.target.nodeType === 3 ? m.target.parentElement : m.target;
      if (el && el.hasAttribute && el.hasAttribute('data-countup') && !el._countupRaf) {
        maybeAnimate(el);
      }
    }
  });
  const startObs = () => {
    obs.observe(document.documentElement, { childList: true, subtree: true, characterData: true });
  };
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', startObs);
  } else {
    startObs();
  }
})();

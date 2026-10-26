// Custom dropdown for the charger schedule controls. The native select opens a
// full screen wheel on Android and hides the rest of the form, so the schedule
// fields use a small popover of their own. Values live in a hidden input so the
// datastar click expressions keep reading them with getElementById.
(function () {
  function panelOf(wrap) {
    return wrap.querySelector("[data-drop-panel]");
  }

  function closePanel(wrap) {
    var panel = panelOf(wrap);
    var btn = wrap.querySelector("[data-drop-btn]");
    if (panel) panel.classList.add("hidden");
    if (btn) btn.setAttribute("aria-expanded", "false");
  }

  function closeAll(except) {
    document.querySelectorAll("[data-drop]").forEach(function (wrap) {
      if (wrap !== except) closePanel(wrap);
    });
  }

  document.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-drop-btn]");
    if (btn) {
      var wrap = btn.closest("[data-drop]");
      var panel = panelOf(wrap);
      var open = !panel.classList.contains("hidden");
      closeAll(wrap);
      panel.classList.toggle("hidden", open);
      btn.setAttribute("aria-expanded", open ? "false" : "true");
      return;
    }

    var opt = e.target.closest("[data-drop-opt]");
    if (opt) {
      var box = opt.closest("[data-drop]");
      var list = panelOf(box);
      var trigger = box.querySelector("[data-drop-btn]");
      var label = box.querySelector("[data-drop-label]");
      var input = box.querySelector("input[type=hidden]");
      if (input) input.value = opt.dataset.value;
      if (label) label.textContent = opt.dataset.label;
      list.querySelectorAll("[data-drop-opt]").forEach(function (o) {
        var on = o === opt;
        o.setAttribute("aria-selected", on ? "true" : "false");
        o.classList.toggle("bg-brand/10", on);
        o.classList.toggle("text-brand", on);
        o.classList.toggle("font-semibold", on);
        o.classList.toggle("text-app", !on);
      });
      closePanel(box);
      if (trigger) trigger.focus();
      return;
    }

    if (!e.target.closest("[data-drop]")) closeAll(null);
  });

  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape") closeAll(null);
  });
})();

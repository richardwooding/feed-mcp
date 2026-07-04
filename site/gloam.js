/*!
 * gloam.js — optional, dependency-free behaviors for the gloam design language.
 * https://github.com/richardwooding/gloam · MIT
 *
 * Everything is data-attribute driven and no-ops when the elements are absent,
 * so you can include it unconditionally. Runs on DOMContentLoaded.
 *
 *   Copy button:  <button class="gl-copy" data-gl-copy="snippetId">Copy</button>
 *                 <code id="snippetId">…</code>
 *   Tabs:         <button class="gl-lang" data-gl-tab="one" ...>One</button>
 *                 container: <div data-gl-tabs>…buttons…</div>
 *                 panels:    <div data-gl-panel="one">…</div>  (hidden unless selected)
 *   Mobile nav:   <button data-gl-nav-toggle="navId">☰</button>  <nav id="navId">…</nav>
 *   Theme toggle: <button data-gl-theme-toggle aria-label="Toggle color theme">…</button>
 *                 flips light/dark on <html data-theme>, persisted in localStorage.
 *   Year:         <span data-gl-year></span>
 */
(function () {
  "use strict";
  function ready(fn) {
    if (document.readyState !== "loading") fn();
    else document.addEventListener("DOMContentLoaded", fn);
  }

  // Effective theme: an explicit data-theme wins, else the system preference.
  // (Reading the computed --gl-bg is unreliable — browsers serialize it as rgb().)
  function isDark() {
    var explicit = document.documentElement.getAttribute("data-theme");
    if (explicit) return explicit === "dark";
    return !window.matchMedia("(prefers-color-scheme: light)").matches;
  }

  // Restore a persisted theme as early as this script runs. For flash-free
  // restore, also add this one-liner in <head> (runs before first paint):
  //   <script>try{var t=localStorage.getItem("gl-theme");if(t)document.documentElement.setAttribute("data-theme",t)}catch(e){}</script>
  try {
    var savedTheme = localStorage.getItem("gl-theme");
    if (savedTheme === "light" || savedTheme === "dark") {
      document.documentElement.setAttribute("data-theme", savedTheme);
    }
  } catch (e) { /* ignore */ }

  ready(function () {
    // Current year.
    document.querySelectorAll("[data-gl-year]").forEach(function (el) {
      el.textContent = new Date().getFullYear();
    });

    // Theme toggle: flip light/dark, persist the choice, reflect it in aria-pressed.
    var themeToggles = document.querySelectorAll("[data-gl-theme-toggle]");
    function syncToggles() {
      var dark = isDark();
      themeToggles.forEach(function (b) { b.setAttribute("aria-pressed", String(dark)); });
    }
    syncToggles();
    themeToggles.forEach(function (btn) {
      btn.addEventListener("click", function () {
        var next = isDark() ? "light" : "dark";
        document.documentElement.setAttribute("data-theme", next);
        try { localStorage.setItem("gl-theme", next); } catch (e) { /* ignore */ }
        syncToggles();
      });
    });

    // Copy-to-clipboard buttons.
    document.querySelectorAll("[data-gl-copy]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        var target = document.getElementById(btn.getAttribute("data-gl-copy"));
        if (!target) return;
        var text = target.textContent;
        function done() {
          var old = btn.textContent;
          btn.textContent = "Copied";
          btn.classList.add("done");
          setTimeout(function () { btn.textContent = old; btn.classList.remove("done"); }, 1400);
        }
        if (navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(text).then(done).catch(fallback);
        } else {
          fallback();
        }
        function fallback() {
          var ta = document.createElement("textarea");
          ta.value = text; ta.style.position = "absolute"; ta.style.left = "-9999px";
          document.body.appendChild(ta); ta.select();
          // Only signal success if the copy actually happened.
          try { if (document.execCommand("copy")) done(); } catch (e) { /* ignore */ }
          document.body.removeChild(ta);
        }
      });
    });

    // Pill tabs: clicking [data-gl-tab=X] inside [data-gl-tabs] shows [data-gl-panel=X].
    document.querySelectorAll("[data-gl-tabs]").forEach(function (group) {
      var tabs = group.querySelectorAll("[data-gl-tab]");
      // Scope panels to the nearest ancestor that actually contains them, so
      // multiple tab groups on a page don't toggle each other's panels — while
      // still working when the group and its panels sit in sibling columns
      // (e.g. hero pills in one column, terminal panels in the other).
      var container = group.parentElement;
      while (container && !container.querySelector("[data-gl-panel]")) {
        container = container.parentElement;
      }
      if (!container) container = document;
      function select(name) {
        tabs.forEach(function (t) { t.setAttribute("aria-selected", String(t.getAttribute("data-gl-tab") === name)); });
        container.querySelectorAll("[data-gl-panel]").forEach(function (p) {
          p.hidden = p.getAttribute("data-gl-panel") !== name;
        });
      }
      tabs.forEach(function (t) {
        t.addEventListener("click", function () { select(t.getAttribute("data-gl-tab")); });
      });
      var initial = group.querySelector('[data-gl-tab][aria-selected="true"]') || tabs[0];
      if (initial) select(initial.getAttribute("data-gl-tab"));
    });

    // Mobile nav toggle; closes when a link inside is tapped.
    document.querySelectorAll("[data-gl-nav-toggle]").forEach(function (btn) {
      var nav = document.getElementById(btn.getAttribute("data-gl-nav-toggle"));
      if (!nav) return;
      btn.addEventListener("click", function () { nav.classList.toggle("open"); });
      nav.addEventListener("click", function (e) {
        // closest("a") so clicks on elements nested inside a link still close.
        if (e.target.closest && e.target.closest("a")) nav.classList.remove("open");
      });
    });
  });
})();

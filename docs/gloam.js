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
      // Capture the label once and clear any pending reset, so rapid repeat
      // clicks can't latch "Copied" as the restore text.
      var originalHTML = btn.innerHTML;
      var resetTimer = null;
      btn.addEventListener("click", function () {
        var target = document.getElementById(btn.getAttribute("data-gl-copy"));
        if (!target) return;
        var text = target.textContent;
        function done() {
          btn.textContent = "Copied";
          btn.classList.add("done");
          if (resetTimer) clearTimeout(resetTimer);
          // Restore via innerHTML so any nested markup (icons) survives.
          resetTimer = setTimeout(function () { btn.innerHTML = originalHTML; btn.classList.remove("done"); }, 1400);
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
      // Only toggle panels this group owns, so two tab groups sharing an
      // ancestor don't hide each other's panels.
      var owned = Object.create(null);
      tabs.forEach(function (t) { owned[t.getAttribute("data-gl-tab")] = true; });
      function select(name) {
        tabs.forEach(function (t) { t.setAttribute("aria-selected", String(t.getAttribute("data-gl-tab") === name)); });
        container.querySelectorAll("[data-gl-panel]").forEach(function (p) {
          var panelName = p.getAttribute("data-gl-panel");
          if (owned[panelName]) p.hidden = panelName !== name;
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
      btn.addEventListener("click", function () {
        var open = nav.classList.toggle("open");
        btn.setAttribute("aria-expanded", String(open));
      });
      nav.addEventListener("click", function (e) {
        // closest("a") so clicks on elements nested inside a link still close.
        if (e.target.closest && e.target.closest("a")) {
          nav.classList.remove("open");
          btn.setAttribute("aria-expanded", "false");
        }
      });
    });

    // Carousel: a scroll-snap track with prev/next, generated dots, and
    // motion-safe autoplay. No-ops without a viewport; degrades to a plain
    // scrollable row when JS is off. Autoplay respects prefers-reduced-motion.
    document.querySelectorAll("[data-gl-carousel]").forEach(function (root) {
      var viewport = root.querySelector("[data-gl-carousel-viewport]");
      if (!viewport) return;
      var slides = Array.prototype.slice.call(viewport.children);
      if (!slides.length) return;
      var prev = root.querySelector("[data-gl-carousel-prev]");
      var next = root.querySelector("[data-gl-carousel-next]");
      var dotsBox = root.querySelector("[data-gl-carousel-dots]");
      var reduce = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;

      // Index of the slide whose left edge is nearest the current scroll offset.
      function current() {
        var best = 0, bestD = Infinity;
        for (var i = 0; i < slides.length; i++) {
          var d = Math.abs(slides[i].offsetLeft - viewport.scrollLeft);
          if (d < bestD) { bestD = d; best = i; }
        }
        return best;
      }
      function goTo(i) {
        i = (i + slides.length) % slides.length;
        viewport.scrollTo({ left: slides[i].offsetLeft, behavior: reduce ? "auto" : "smooth" });
      }

      var dots = [];
      if (dotsBox) {
        slides.forEach(function (_, i) {
          var b = document.createElement("button");
          b.className = "gl-carousel-dot";
          b.type = "button";
          b.setAttribute("aria-label", "Go to slide " + (i + 1));
          b.addEventListener("click", function () { goTo(i); });
          dotsBox.appendChild(b);
          dots.push(b);
        });
      }
      function sync() {
        var c = current();
        dots.forEach(function (d, i) { d.setAttribute("aria-current", String(i === c)); });
      }
      sync();

      if (prev) prev.addEventListener("click", function () { goTo(current() - 1); });
      if (next) next.addEventListener("click", function () { goTo(current() + 1); });

      var ticking = false;
      viewport.addEventListener("scroll", function () {
        if (ticking) return;
        ticking = true;
        window.requestAnimationFrame(function () { sync(); ticking = false; });
      });

      // Motion-safe autoplay; pause on hover/focus and when the tab is hidden.
      if (!reduce) {
        var timer = null;
        function start() { if (!timer) timer = setInterval(function () { goTo(current() + 1); }, 6000); }
        function stop() { if (timer) { clearInterval(timer); timer = null; } }
        root.addEventListener("pointerenter", stop);
        root.addEventListener("pointerleave", start);
        root.addEventListener("focusin", stop);
        root.addEventListener("focusout", start);
        document.addEventListener("visibilitychange", function () { if (document.hidden) { stop(); } else { start(); } });
        start();
      }
    });
  });
})();

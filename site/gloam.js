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
 *   Year:         <span data-gl-year></span>
 */
(function () {
  "use strict";
  function ready(fn) {
    if (document.readyState !== "loading") fn();
    else document.addEventListener("DOMContentLoaded", fn);
  }

  ready(function () {
    // Current year.
    document.querySelectorAll("[data-gl-year]").forEach(function (el) {
      el.textContent = new Date().getFullYear();
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
          try { document.execCommand("copy"); done(); } catch (e) { /* ignore */ }
          document.body.removeChild(ta);
        }
      });
    });

    // Pill tabs: clicking [data-gl-tab=X] inside [data-gl-tabs] shows [data-gl-panel=X].
    document.querySelectorAll("[data-gl-tabs]").forEach(function (group) {
      var tabs = group.querySelectorAll("[data-gl-tab]");
      function select(name) {
        tabs.forEach(function (t) { t.setAttribute("aria-selected", String(t.getAttribute("data-gl-tab") === name)); });
        document.querySelectorAll("[data-gl-panel]").forEach(function (p) {
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
        if (e.target.tagName === "A") nav.classList.remove("open");
      });
    });
  });
})();

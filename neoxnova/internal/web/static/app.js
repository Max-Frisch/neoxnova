/*
 * app.js — progressive-enhancement helpers for the Neo-XNova shell.
 *
 * Kept tiny and CSP-safe ("script-src 'self'"): all dynamic styling is applied
 * through the CSSOM (element.style), never inline style attributes.
 */
(function () {
  "use strict";

  var WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  var MONTHS = [
    "Jan", "Feb", "Mar", "Apr", "May", "Jun",
    "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"
  ];

  function pad(n) {
    return n < 10 ? "0" + n : "" + n;
  }

  function formatServerTime(date) {
    return (
      pad(date.getDate()) + " " +
      WEEKDAYS[date.getDay()] + " " +
      MONTHS[date.getMonth()] + " " +
      pad(date.getHours()) + ":" +
      pad(date.getMinutes()) + ":" +
      pad(date.getSeconds())
    );
  }

  // applyBars fills every [data-pct] element (resource capacity, progress bars)
  // to the percentage captured server-side.
  function applyBars(root) {
    var scope = root && root.querySelectorAll ? root : document;
    var bars = scope.querySelectorAll("[data-pct]");
    for (var i = 0; i < bars.length; i++) {
      var pct = parseFloat(bars[i].getAttribute("data-pct"));
      if (isNaN(pct)) {
        continue;
      }
      if (pct < 0) {
        pct = 0;
      } else if (pct > 100) {
        pct = 100;
      }
      bars[i].style.width = pct + "%";
    }
  }

  // startClock keeps the server clock ticking, offset by the server/client skew
  // captured at load so it never drifts with the browser timezone.
  function startClock() {
    var el = document.getElementById("server-clock");
    if (!el) {
      return;
    }
    var base = parseInt(el.getAttribute("data-t"), 10);
    if (isNaN(base)) {
      return;
    }
    var offsetMs = base * 1000 - Date.now();
    var render = function () {
      el.textContent = formatServerTime(new Date(Date.now() + offsetMs));
    };
    render();
    window.setInterval(render, 1000);
  }

  // wirePlanetSelect navigates on planet dropdown changes.
  function wirePlanetSelect() {
    var select = document.getElementById("planet-select");
    if (!select) {
      return;
    }
    select.addEventListener("change", function () {
      if (select.value) {
        window.location.href = select.value;
      }
    });
  }

  // wireMaxButtons fills a quantity input with the server-computed affordable
  // maximum when a "Max" button is clicked. Uses a delegated listener so it
  // keeps working after htmx swaps.
  function wireMaxButtons() {
    document.addEventListener("click", function (evt) {
      var el = evt.target;
      while (el && el !== document && !el.classList.contains("unit-max")) {
        el = el.parentNode;
      }
      if (!el || el === document) {
        return;
      }
      evt.preventDefault();
      var input = document.getElementById(el.getAttribute("data-target"));
      if (input) {
        input.value = el.getAttribute("data-max");
      }
    });
  }

  function init() {
    applyBars(document);
    startClock();
    wirePlanetSelect();
    wireMaxButtons();
  }

  document.addEventListener("DOMContentLoaded", init);

  // Re-apply dynamic widths after htmx swaps (e.g. the polled resource bar).
  document.addEventListener("htmx:afterSwap", function (evt) {
    applyBars(evt.target);
  });
})();

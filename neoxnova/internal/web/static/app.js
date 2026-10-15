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

  // formatNumber inserts thousands separators (matches fnum server-side).
  function formatNumber(n) {
    return String(n).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  }

  // hms renders seconds as a fixed HH:MM:SS clock (matches the server queue
  // counters and card durations).
  function hms(secs) {
    secs = Math.max(0, Math.floor(secs || 0));
    var h = Math.floor(secs / 3600);
    var m = Math.floor((secs % 3600) / 60);
    var s = secs % 60;
    return pad(h) + ":" + pad(m) + ":" + pad(s);
  }

  // renderCostTotal builds the same cost-row markup the server renders.
  function renderCostTotal(cost) {
    var parts = "";
    if (cost.m > 0) {
      parts += '<span class="cost-row"><img src="/static/niburu/icons/metall_mini.gif" alt="Metal"/>' + formatNumber(cost.m) + "</span>";
    }
    if (cost.c > 0) {
      parts += '<span class="cost-row"><img src="/static/niburu/icons/kristall_mini.gif" alt="Crystal"/>' + formatNumber(cost.c) + "</span>";
    }
    if (cost.d > 0) {
      parts += '<span class="cost-row"><img src="/static/niburu/icons/deuterium_mini.gif" alt="Deuterium"/>' + formatNumber(cost.d) + "</span>";
    }
    return '<div class="cost">' + parts + "</div>";
  }

  // updateLevelPreview sums the embedded per-level cost/time table from the
  // card's next level up to the chosen target and updates the shown totals
  // (no extra request; the table is generated server-side from internal/game).
  function updateLevelPreview(input) {
    var card = input.closest(".gcard");
    var form = input.form;
    if (!card || !form) {
      return;
    }
    var table;
    try {
      table = JSON.parse(form.getAttribute("data-cost-table") || "[]");
    } catch (e) {
      return;
    }
    var min = parseInt(input.getAttribute("min"), 10);
    if (isNaN(min)) {
      min = 0;
    }
    var target = parseInt(input.value, 10);
    if (isNaN(target) || target < min) {
      target = min;
    }
    var totals = { m: 0, c: 0, d: 0, t: 0 };
    for (var i = 0; i < table.length; i++) {
      var row = table[i];
      if (row.l < min) {
        continue;
      }
      if (row.l > target) {
        break;
      }
      totals.m += row.m;
      totals.c += row.c;
      totals.d += row.d;
      totals.t += row.t;
    }
    var costEl = card.querySelector("[data-cost-total]");
    if (costEl) {
      costEl.innerHTML = renderCostTotal(totals);
    }
    var durEl = card.querySelector("[data-duration-total]");
    if (durEl) {
      durEl.textContent = hms(totals.t);
    }
  }

  // wireLevelPreview updates a card's live cost/time as its target level input
  // changes. Delegated so it survives htmx swaps.
  function wireLevelPreview() {
    document.addEventListener("input", function (evt) {
      var el = evt.target;
      if (el && el.hasAttribute && el.hasAttribute("data-level-input")) {
        updateLevelPreview(el);
      }
    });
  }

  function init() {
    applyBars(document);
    startClock();
    wirePlanetSelect();
    wireMaxButtons();
    wireLevelPreview();
  }

  document.addEventListener("DOMContentLoaded", init);

  // Re-apply dynamic widths after htmx swaps (e.g. the polled resource bar).
  document.addEventListener("htmx:afterSwap", function (evt) {
    applyBars(evt.target);
  });
})();

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

  // serverOffsetMs is the captured server-vs-client clock skew (ms). Countdowns
  // and the clock both derive from it so local timezone/latency never drift.
  var serverOffsetMs = null;

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

  // serverNowMs returns the current server wall-clock time in ms, using the skew
  // captured from #server-clock on first use (0 when the element is absent).
  function serverNowMs() {
    if (serverOffsetMs === null) {
      var el = document.getElementById("server-clock");
      var base = el ? parseInt(el.getAttribute("data-t"), 10) : NaN;
      serverOffsetMs = isNaN(base) ? 0 : base * 1000 - Date.now();
    }
    return Date.now() + serverOffsetMs;
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
    serverOffsetMs = base * 1000 - Date.now();
    var render = function () {
      el.textContent = formatServerTime(new Date(serverNowMs()));
    };
    render();
    window.setInterval(render, 1000);
  }

  // tickCountdowns updates every [data-countdown] element (absolute server epoch
  // in unix seconds) so build/research/fleet ETAs count down live with no extra
  // requests. Default format is HH:MM:SS; data-cd-format="dur" picks the compact
  // "2h 5m" style used by fleet ETAs. Re-queried every tick so htmx swaps are
  // picked up automatically.
  function tickCountdowns() {
    var els = document.querySelectorAll("[data-countdown]");
    if (els.length === 0) {
      return;
    }
    var now = serverNowMs();
    for (var i = 0; i < els.length; i++) {
      var end = parseInt(els[i].getAttribute("data-countdown"), 10);
      if (isNaN(end)) {
        continue;
      }
      var secs = Math.max(0, Math.round(end - now / 1000));
      els[i].textContent = els[i].getAttribute("data-cd-format") === "dur" ? dur(secs) : hms(secs);
    }
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
        input.dispatchEvent(new Event("input", { bubbles: true }));
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

  // dur renders seconds as "2h 5m" / "5m 30s" / "45s" (matches the server fleet
  // ETA formatter in data.go).
  function dur(secs) {
    secs = Math.max(0, Math.floor(secs || 0));
    var h = Math.floor(secs / 3600);
    var m = Math.floor((secs % 3600) / 60);
    var s = secs % 60;
    if (h > 0) {
      return h + "h " + m + "m";
    }
    if (m > 0) {
      return m + "m " + s + "s";
    }
    return s + "s";
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

  // updateUnitPreview scales a ship/defense card's per-unit cost and duration by
  // the quantity typed into its input (units are linear: qty × per-unit), so the
  // cumulative resources "count up" as the number grows. The per-unit values ride
  // along on the form as data-* attributes (generated server-side).
  function updateUnitPreview(input) {
    var card = input.closest(".gcard");
    var form = input.form;
    if (!card || !form) {
      return;
    }
    var per;
    try {
      per = JSON.parse(form.getAttribute("data-unit-cost") || "{}");
    } catch (e) {
      return;
    }
    var qty = parseInt(input.value, 10);
    if (isNaN(qty) || qty < 1) {
      qty = 1;
    }
    var costEl = card.querySelector("[data-cost-total]");
    if (costEl) {
      costEl.innerHTML = renderCostTotal({
        m: (per.m || 0) * qty,
        c: (per.c || 0) * qty,
        d: (per.d || 0) * qty
      });
    }
    var durEl = card.querySelector("[data-duration-total]");
    var perT = parseInt(form.getAttribute("data-unit-time"), 10);
    if (durEl && !isNaN(perT)) {
      durEl.textContent = hms(perT * qty);
    }
  }

  // updateGovernatorTarget previews the Achievement-Point cost of improving a
  // governator to the typed level, using the cumulative AP table embedded on the
  // form (server-generated from game.Governator.APCost).
  function updateGovernatorTarget(input) {
    var form = input.form;
    if (!form) {
      return;
    }
    var cum;
    try {
      cum = JSON.parse(form.getAttribute("data-ap-cum") || "[]");
    } catch (e) {
      return;
    }
    var level = parseInt(form.getAttribute("data-gov-level"), 10) || 0;
    var max = parseInt(form.getAttribute("data-gov-max"), 10) || level;
    var target = parseInt(input.value, 10);
    if (isNaN(target) || target <= level) {
      target = level + 1;
    }
    if (target > max) {
      target = max;
    }
    var el = document.getElementById(form.getAttribute("data-ap-id"));
    if (el) {
      el.textContent = formatNumber((cum[target] || 0) - (cum[level] || 0));
    }
    // Raise the shown bonus percent with the selected level, like a resource
    // cost counting up (mirrors governators.js GubPriceAP).
    var bonus;
    try {
      bonus = JSON.parse(form.getAttribute("data-gov-bonus") || "[]");
    } catch (e) {
      return;
    }
    var pct = bonus[target];
    if (pct === undefined) {
      return;
    }
    var card = input.closest(".gov-card");
    if (!card) {
      return;
    }
    var effects = card.querySelectorAll("[data-gov-effect]");
    for (var i = 0; i < effects.length; i++) {
      effects[i].textContent = (effects[i].getAttribute("data-gov-sign") || "+") + pct + "%";
    }
  }

  // updateGovernatorDays scales a governator's per-day Dark Matter price by the
  // number of days typed into the hire form.
  function updateGovernatorDays(input) {
    var form = input.form;
    if (!form) {
      return;
    }
    var perDay = parseInt(form.getAttribute("data-perday"), 10) || 0;
    var days = parseInt(input.value, 10);
    if (isNaN(days) || days < 1) {
      days = 1;
    }
    var el = document.getElementById(form.getAttribute("data-price-id"));
    if (el) {
      el.textContent = formatNumber(perDay * days);
    }
  }

  // wireCardPreviews updates a card's live cost/time as its target-level or
  // quantity input changes. Delegated so it survives htmx swaps.
  function wireCardPreviews() {
    document.addEventListener("input", function (evt) {
      var el = evt.target;
      if (!el || !el.hasAttribute) {
        return;
      }
      if (el.hasAttribute("data-level-input")) {
        updateLevelPreview(el);
      } else if (el.hasAttribute("data-unit-input")) {
        updateUnitPreview(el);
      } else if (el.hasAttribute("data-gov-target")) {
        updateGovernatorTarget(el);
      } else if (el.hasAttribute("data-gov-days")) {
        updateGovernatorDays(el);
      }
    });
  }

  function init() {
    applyBars(document);
    startClock();
    wirePlanetSelect();
    wireMaxButtons();
    wireCardPreviews();
    tickCountdowns();
    window.setInterval(tickCountdowns, 1000);
  }

  document.addEventListener("DOMContentLoaded", init);

  // Re-apply dynamic widths after htmx swaps (e.g. the polled resource bar).
  document.addEventListener("htmx:afterSwap", function (evt) {
    applyBars(evt.target);
  });
})();

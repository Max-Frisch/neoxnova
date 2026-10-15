// Expedition panel slider labels: exp_time 1-10 -> 0.25-2.5 h,
// exp_speed 1-10 -> 10-100 %. Kept in a static file so the page CSP
// (script-src 'self') allows it.
(function () {
  function bind(inputId, outId, fmt) {
    var input = document.getElementById(inputId);
    var out = document.getElementById(outId);
    if (!input || !out) return;
    var update = function () { out.textContent = fmt(Number(input.value)); };
    input.addEventListener("input", update);
    update();
  }
  bind("exp_time", "exp_time_out", function (v) { return (v * 0.25).toFixed(2) + " h"; });
  bind("exp_speed", "exp_speed_out", function (v) { return v * 10 + "%"; });
})();

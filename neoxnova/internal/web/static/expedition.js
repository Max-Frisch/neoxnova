// Expedition panel slider labels. exp_time 1-10 -> 0.25-2.5 h nominal, and the
// real hold the fleet actually waits is that nominal compressed by the universe
// fleet-speed multiplier (matching game.CalculateHoldDuration server-side).
// exp_speed 1-10 -> 10-100 %. Kept in a static file so the page CSP
// (script-src 'self') allows it.
(function () {
  // dur renders seconds as "Hh Mm" / "Mm Ss" / "Ss", matching the server's dur().
  function dur(secs) {
    secs = Math.max(0, Math.floor(secs || 0));
    var h = Math.floor(secs / 3600);
    var m = Math.floor((secs % 3600) / 60);
    var s = secs % 60;
    if (h > 0) return h + "h " + m + "m";
    if (m > 0) return m + "m " + s + "s";
    return s + "s";
  }

  // realHold is the fleet-speed-compressed hold in seconds; a non-zero nominal
  // hold never collapses below one second.
  function realHold(expTime, fleetSpeed) {
    var nominal = expTime * 0.25 * 3600;
    if (nominal <= 0) return 0;
    var fs = fleetSpeed > 0 ? fleetSpeed : 1;
    var scaled = Math.round(nominal / fs);
    return scaled < 1 ? 1 : scaled;
  }

  function bind(inputId, outId, fmt) {
    var input = document.getElementById(inputId);
    var out = document.getElementById(outId);
    if (!input || !out) return;
    var update = function () { out.textContent = fmt(Number(input.value)); };
    input.addEventListener("input", update);
    update();
  }

  var expTime = document.getElementById("exp_time");
  var fleetSpeed = expTime ? Number(expTime.getAttribute("data-fleet-speed")) : 1;
  if (!(fleetSpeed > 0)) fleetSpeed = 1;

  bind("exp_time", "exp_time_out", function (v) {
    return (v * 0.25).toFixed(2) + " h · real " + dur(realHold(v, fleetSpeed));
  });
  bind("exp_speed", "exp_speed_out", function (v) { return v * 10 + "%"; });
})();

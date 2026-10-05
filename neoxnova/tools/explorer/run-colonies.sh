#!/usr/bin/env bash
# run-colonies.sh - start/stop detached incremental build daemons for acc2 colonies.
#
# Each colony runs its own `httpbot.mjs resolve` process inside a tmux session and
# is wrapped in a restart loop, so a crash/exit self-heals. A separate `bonus`
# session periodically loads the Online Bonus page. Safe to re-run (existing
# sessions are skipped).
#
# Usage (on the Azure VM, from neoxnova/tools/explorer):
#   bash run-colonies.sh start     # launch one build daemon per colony + bonus
#   bash run-colonies.sh status    # list running daemons
#   bash run-colonies.sh logs      # tail all logs
#   bash run-colonies.sh stop      # kill all daemons
#
# Tunables (env vars, or edit the defaults here):
#   EXPLORER_COLO_COORDS  comma list of colony coords   (default 2:186:9,2:186:10,2:186:11)
#   EXPLORER_COLO_PLAN    goals json                    (default plans/colo-grow.json)
#   EXPLORER_BUILDER_BUMP_SEC   Robot/Nanite bump when mine build >= this (default 360 = 6 min)
#   EXPLORER_ENERGY_SATS        satellites queued on energy deficit / target (default 200)
#   EXPLORER_QUEUE_WAIT_MS      idle poll interval      (default 12000)
#   EXPLORER_FETCH_TIMEOUT_MS   per-HTTP-request timeout (default 60000; slow server!)
#   EXPLORER_MIN/MAX_DELAY_MS   inter-request delay      (default 1200 / 2500)
#   EXPLORER_BONUS_EVERY_S      Online Bonus poll period (default 900)
#   EXPLORER_COLO_STAGGER       seconds between daemon starts (default 10)
#
# To change the "done" state / max levels, edit plans/colo-grow.json:
#   buildings     fixed targets (the bootstrap the bot must reach)
#   gradual       codes auto-raised +1 per cycle once fixed targets are met
#   caps          per-code maximum for `gradual` (and bumpBuilders) - the final "done"
#   bumpBuilders  Robot(14)/Nanite(15), +1 each when a mine build exceeds the threshold
#   ships         unit targets, e.g. {"212":200} Solar Satellites
#   order         build priority

set -uo pipefail
cd "$(dirname "$0")"

NODE_FLAGS="--max-old-space-size=96"
COORDS="${EXPLORER_COLO_COORDS:-2:186:9,2:186:10,2:186:11}"
PLAN="${EXPLORER_COLO_PLAN:-plans/colo-grow.json}"
BUMP="${EXPLORER_BUILDER_BUMP_SEC:-360}"
SATS="${EXPLORER_ENERGY_SATS:-200}"
WAIT="${EXPLORER_QUEUE_WAIT_MS:-12000}"
TIMEOUT="${EXPLORER_FETCH_TIMEOUT_MS:-60000}"
MIN_DELAY="${EXPLORER_MIN_DELAY_MS:-1200}"
MAX_DELAY="${EXPLORER_MAX_DELAY_MS:-2500}"
BONUS_EVERY="${EXPLORER_BONUS_EVERY_S:-900}"
STAGGER="${EXPLORER_COLO_STAGGER:-10}"
SESS_PREFIX="colo-"
BONUS_SESS="bonus"
# Slow game server: allow long requests and space them out.
ENV="EXPLORER_FETCH_TIMEOUT_MS=$TIMEOUT EXPLORER_MIN_DELAY_MS=$MIN_DELAY EXPLORER_MAX_DELAY_MS=$MAX_DELAY EXPLORER_BUILDER_BUMP_SEC=$BUMP EXPLORER_ENERGY_SATS=$SATS EXPLORER_QUEUE_WAIT_MS=$WAIT"

detect_cps() {
  env $ENV node $NODE_FLAGS httpbot.mjs planets \
    | node -e '
      let d = "";
      process.stdin.on("data", (c) => (d += c)).on("end", () => {
        let planets = [];
        try { planets = JSON.parse(d).planets || []; } catch { process.exit(3); }
        for (const w of process.argv[1].split(",")) {
          const p = planets.find((x) => x.coords === w);
          if (p) console.log(`${p.id} ${p.coords} ${p.name}`);
          else console.error(`[!] no planet found for ${w}`);
        }
      });' "$COORDS"
}

start_bonus() {
  if tmux has-session -t "$BONUS_SESS" 2>/dev/null; then
    echo "[=] $BONUS_SESS already running"
    return
  fi
  mkdir -p data
  tmux new-session -d -s "$BONUS_SESS" \
    "cd '$PWD' && while true; do env $ENV node $NODE_FLAGS httpbot.mjs get 'game.php?page=bonus' >/dev/null 2>&1; echo \"[\$(date +%T)] online bonus checked\"; sleep $BONUS_EVERY; done >> 'data/bonus.log' 2>&1"
  echo "[+] started $BONUS_SESS (Online Bonus every ${BONUS_EVERY}s) -> data/bonus.log"
}

start() {
  echo "[*] plan=$PLAN coords=$COORDS bump=${BUMP}s sats=$SATS timeout=${TIMEOUT}ms"
  local found=0
  while read -r cp coords name; do
    [ -z "${cp:-}" ] && continue
    found=1
    local sess="${SESS_PREFIX}${cp}" log="data/colo-${cp}.log"
    if tmux has-session -t "$sess" 2>/dev/null; then
      echo "[=] $sess already running ($coords $name)"
      continue
    fi
    mkdir -p data
    tmux new-session -d -s "$sess" \
      "cd '$PWD' && while true; do env $ENV node $NODE_FLAGS httpbot.mjs resolve --goals '$PLAN' --cp $cp --steps 1000000; echo \"[\$(date +%T)] resolver exited; restart in 30s\"; sleep 30; done >> '$log' 2>&1"
    echo "[+] started $sess ($coords $name) -> $log"
    sleep "$STAGGER"
  done < <(detect_cps)
  if [ "$found" = 0 ]; then
    echo "[!] no matching planets; check EXPLORER_COLO_COORDS" >&2
    return 1
  fi
  start_bonus
  echo "[*] done. 'bash run-colonies.sh status' / 'logs' to watch, 'stop' to halt."
}

status() {
  tmux ls 2>/dev/null | grep -E "^(${SESS_PREFIX}[0-9]+|${BONUS_SESS}):" \
    || echo "[*] no colony daemons running"
}

stop() {
  local any=0
  for s in $(tmux ls 2>/dev/null | grep -oE "^(${SESS_PREFIX}[0-9]+|${BONUS_SESS}):" | tr -d ':'); do
    tmux kill-session -t "$s" && echo "[-] stopped $s"
    any=1
  done
  [ "$any" = 0 ] && echo "[*] nothing to stop"
}

logs() {
  local files=(data/colo-*.log data/bonus.log)
  [ -e "${files[0]}" ] || { echo "[*] no logs yet"; return; }
  tail -n 20 -f "${files[@]}"
}

case "${1:-start}" in
  start) start ;;
  status) status ;;
  stop) stop ;;
  logs) logs ;;
  *) echo "usage: bash run-colonies.sh {start|status|logs|stop}" >&2; exit 2 ;;
esac

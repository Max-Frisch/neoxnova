#!/usr/bin/env bash
# run-expharvest.sh - continuously collect expedition outcomes + fight reports.
#
# Polls the Expedition messages (messcat=15) and Combat messages (messcat=3),
# appending new rows/reports to data/expeditions.json / data/expedition-reports.json.
# Idempotent, so it is safe to run one daemon per account (one per host).
#
# Usage (from neoxnova/tools/explorer):
#   bash run-expharvest.sh start [tag]   # launch tmux session expharv-<tag>
#   bash run-expharvest.sh status
#   bash run-expharvest.sh logs [tag]
#   bash run-expharvest.sh stop [tag]
#
# Tunables: EXPLORER_HARVEST_EVERY_S (default 180), EXPLORER_FETCH_TIMEOUT_MS.

set -uo pipefail
cd "$(dirname "$0")"

NODE_FLAGS="--max-old-space-size=96"
EVERY="${EXPLORER_HARVEST_EVERY_S:-180}"
TIMEOUT="${EXPLORER_FETCH_TIMEOUT_MS:-60000}"

run_loop() {
  local tag="$1" log="data/exp-harvest-${1}.log"
  mkdir -p data
  while true; do
    { echo "[$(date +%T)] harvest"
      EXPLORER_FETCH_TIMEOUT_MS="$TIMEOUT" node $NODE_FLAGS httpbot.mjs exp-log
      EXPLORER_FETCH_TIMEOUT_MS="$TIMEOUT" node $NODE_FLAGS httpbot.mjs exp-report
    } >> "$log" 2>&1
    sleep "$EVERY"
  done
}

case "${1:-start}" in
  start)
    tag="${2:-$(hostname)}"
    sess="expharv-$tag"
    if tmux has-session -t "$sess" 2>/dev/null; then echo "[=] $sess already running"; exit 0; fi
    mkdir -p data
    tmux new-session -d -s "$sess" "cd '$PWD' && bash run-expharvest.sh loop $tag"
    echo "[+] started $sess (every ${EVERY}s) -> data/exp-harvest-${tag}.log"
    ;;
  loop) run_loop "${2:-$(hostname)}" ;;
  status) tmux ls 2>/dev/null | grep -E '^expharv-' || echo "[*] no harvest daemon running" ;;
  logs) tail -n 30 -f "data/exp-harvest-${2:-$(hostname)}.log" ;;
  stop)
    tag="${2:-$(hostname)}"
    tmux kill-session -t "expharv-$tag" 2>/dev/null && echo "[-] stopped expharv-$tag" || echo "[*] not running"
    ;;
  *) echo "usage: bash run-expharvest.sh {start|status|logs|stop} [tag]" >&2; exit 2 ;;
esac

#!/usr/bin/env bash
# run-bonus.sh - periodically load the Online Bonus page (game.php?page=bonus).
#
# Visiting the page is what "clicks" the Online Bonus button: it grants Dark
# Matter + Peaceful XP whenever the button is up. Safe to re-run; one daemon per
# account/host.
#
# Usage (from neoxnova/tools/explorer):
#   bash run-bonus.sh start [tag]   # launch tmux session bonus-<tag>
#   bash run-bonus.sh status
#   bash run-bonus.sh logs [tag]
#   bash run-bonus.sh stop [tag]
#
# Tunables: EXPLORER_BONUS_EVERY_S (default 900), EXPLORER_FETCH_TIMEOUT_MS.

set -uo pipefail
cd "$(dirname "$0")"

NODE_FLAGS="--max-old-space-size=96"
EVERY="${EXPLORER_BONUS_EVERY_S:-900}"
TIMEOUT="${EXPLORER_FETCH_TIMEOUT_MS:-60000}"

run_loop() {
  local tag="$1" log="data/bonus-${1}.log"
  mkdir -p data
  while true; do
    { echo "[$(date +%T)] online bonus checked"
      EXPLORER_FETCH_TIMEOUT_MS="$TIMEOUT" node $NODE_FLAGS httpbot.mjs get 'game.php?page=bonus' >/dev/null 2>&1
    } >> "$log" 2>&1
    sleep "$EVERY"
  done
}

case "${1:-start}" in
  start)
    tag="${2:-$(hostname)}"
    sess="bonus-$tag"
    if tmux has-session -t "$sess" 2>/dev/null; then echo "[=] $sess already running"; exit 0; fi
    mkdir -p data
    tmux new-session -d -s "$sess" "cd '$PWD' && bash run-bonus.sh loop $tag"
    echo "[+] started $sess (every ${EVERY}s) -> data/bonus-${tag}.log"
    ;;
  loop) run_loop "${2:-$(hostname)}" ;;
  status) tmux ls 2>/dev/null | grep -E '^bonus-' || echo "[*] no bonus daemon running" ;;
  logs) tail -n 30 -f "data/bonus-${2:-$(hostname)}.log" ;;
  stop)
    tag="${2:-$(hostname)}"
    tmux kill-session -t "bonus-$tag" 2>/dev/null && echo "[-] stopped bonus-$tag" || echo "[*] not running"
    ;;
  *) echo "usage: bash run-bonus.sh {start|status|logs|stop} [tag]" >&2; exit 2 ;;
esac

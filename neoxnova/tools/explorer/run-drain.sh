#!/usr/bin/env bash
# run-drain.sh - keep the burn-down expedition sender (drain.mjs) alive in tmux,
# one account per host. drain.mjs loops internally; this wrapper restarts it if
# it ever exits. Writes its own log to data/drain-<tag>.log.
#
# Usage (from neoxnova/tools/explorer):
#   bash run-drain.sh start [tag]
#   bash run-drain.sh status
#   bash run-drain.sh logs [tag]
#   bash run-drain.sh stop [tag]
#
# Tunables: EXPLORER_DRAIN_RESTART_S (default 30).

set -uo pipefail
cd "$(dirname "$0")"

NODE_FLAGS="--max-old-space-size=96"
RESTART="${EXPLORER_DRAIN_RESTART_S:-30}"

run_loop() {
  local tag="$1" sup="data/drain-${1}.supervisor.log"
  mkdir -p data
  while true; do
    # drain.mjs writes its own data/drain-<tag>.log; keep stdout/err separate so
    # its console echo is not duplicated into that same file.
    node $NODE_FLAGS drain.mjs "$tag" >> "$sup" 2>&1
    echo "[$(date +%T)] drain.mjs $tag exited; restarting in ${RESTART}s" >> "$sup"
    sleep "$RESTART"
  done
}

case "${1:-start}" in
  start)
    tag="${2:-$(hostname)}"
    sess="drain-$tag"
    if tmux has-session -t "$sess" 2>/dev/null; then echo "[=] $sess already running"; exit 0; fi
    mkdir -p data
    tmux new-session -d -s "$sess" "cd '$PWD' && bash run-drain.sh loop $tag"
    echo "[+] started $sess -> data/drain-${tag}.log"
    ;;
  loop) run_loop "${2:-$(hostname)}" ;;
  status) tmux ls 2>/dev/null | grep -E '^drain-' || echo "[*] no drain daemon running" ;;
  logs) tail -n 30 -f "data/drain-${2:-$(hostname)}.log" ;;
  stop)
    tag="${2:-$(hostname)}"
    tmux kill-session -t "drain-$tag" 2>/dev/null && echo "[-] stopped drain-$tag" || echo "[*] not running"
    ;;
  *) echo "usage: bash run-drain.sh {start|status|logs|stop} [tag]" >&2; exit 2 ;;
esac

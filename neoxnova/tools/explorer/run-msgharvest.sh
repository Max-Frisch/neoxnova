#!/usr/bin/env bash
# run-msgharvest.sh - continuously collect messages of every category.
#
# Runs the full-category message scan (spy/player/alliance/combat/system/
# transport/expedition/game/construction) and merges new rows into
# data/messages.json, then prints the accumulating statistics. Idempotent, so it
# is safe to run one daemon per account (one per host).
#
# By default it only pages a few sites (new messages land at the top), which is
# cheap enough to run repeatedly. For a one-time deep backfill use the bare
# command without EXPLORER_MSG_MAX_SITES:
#   node --max-old-space-size=192 httpbot.mjs msg-scan
#
# Usage (from neoxnova/tools/explorer):
#   bash run-msgharvest.sh start [tag]   # launch tmux session msgharv-<tag>
#   bash run-msgharvest.sh status
#   bash run-msgharvest.sh logs [tag]
#   bash run-msgharvest.sh stop [tag]
#
# Tunables: EXPLORER_MSG_EVERY_S (default 900), EXPLORER_MSG_MAX_SITES (default 6),
#           EXPLORER_FETCH_TIMEOUT_MS.

set -uo pipefail
cd "$(dirname "$0")"

NODE_FLAGS="--max-old-space-size=192"
EVERY="${EXPLORER_MSG_EVERY_S:-900}"
SITES="${EXPLORER_MSG_MAX_SITES:-6}"
TIMEOUT="${EXPLORER_FETCH_TIMEOUT_MS:-60000}"

run_loop() {
  local tag="$1" log="data/msg-harvest-${1}.log"
  mkdir -p data
  while true; do
    { echo "[$(date +%T)] msg-scan --max-sites $SITES"
      EXPLORER_FETCH_TIMEOUT_MS="$TIMEOUT" node $NODE_FLAGS httpbot.mjs msg-scan --max-sites "$SITES"
    } >> "$log" 2>&1
    sleep "$EVERY"
  done
}

case "${1:-start}" in
  start)
    tag="${2:-$(hostname)}"
    sess="msgharv-$tag"
    if tmux has-session -t "$sess" 2>/dev/null; then echo "[=] $sess already running"; exit 0; fi
    mkdir -p data
    tmux new-session -d -s "$sess" "cd '$PWD' && bash run-msgharvest.sh loop $tag"
    echo "[+] started $sess (every ${EVERY}s, ${SITES} sites) -> data/msg-harvest-${tag}.log"
    ;;
  loop) run_loop "${2:-$(hostname)}" ;;
  status) tmux ls 2>/dev/null | grep -E '^msgharv-' || echo "[*] no message harvest daemon running" ;;
  logs) tail -n 30 -f "data/msg-harvest-${2:-$(hostname)}.log" ;;
  stop)
    tag="${2:-$(hostname)}"
    tmux kill-session -t "msgharv-$tag" 2>/dev/null && echo "[-] stopped msgharv-$tag" || echo "[*] not running"
    ;;
  *) echo "usage: bash run-msgharvest.sh {start|status|logs|stop} [tag]" >&2; exit 2 ;;
esac

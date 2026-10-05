#!/usr/bin/env bash
# run-one-colony.sh <cp> [plan] - detached restart loop for a single planet,
# Windows-friendly equivalent of one `run-colonies.sh` session: runs resolve
# until verify reports every target met, then stops itself. Sats are queued by
# resolve whenever energy goes negative.
set -uo pipefail
cd "$(dirname "$0")"

CP="${1:?usage: run-one-colony.sh <cp> [plan]}"
PLAN="${2:-plans/colo-grow.json}"
LOG="data/colo-${CP}.log"
NODE_FLAGS="--max-old-space-size=96"
mkdir -p data

export EXPLORER_FETCH_TIMEOUT_MS="${EXPLORER_FETCH_TIMEOUT_MS:-60000}"
export EXPLORER_MIN_DELAY_MS="${EXPLORER_MIN_DELAY_MS:-1200}"
export EXPLORER_MAX_DELAY_MS="${EXPLORER_MAX_DELAY_MS:-2500}"
export EXPLORER_BUILDER_BUMP_SEC="${EXPLORER_BUILDER_BUMP_SEC:-360}"
export EXPLORER_ENERGY_SATS="${EXPLORER_ENERGY_SATS:-200}"
export EXPLORER_QUEUE_WAIT_MS="${EXPLORER_QUEUE_WAIT_MS:-12000}"

while true; do
  node $NODE_FLAGS httpbot.mjs resolve --goals "$PLAN" --cp "$CP" --steps 1000000 >>"$LOG" 2>&1
  if node $NODE_FLAGS httpbot.mjs verify --goals "$PLAN" --cp "$CP" >>"$LOG" 2>&1; then
    echo "[$(date +%T)] verify MET; daemon stopping" >>"$LOG"
    break
  fi
  echo "[$(date +%T)] verify NOT MET; restart in 30s" >>"$LOG"
  sleep 30
done

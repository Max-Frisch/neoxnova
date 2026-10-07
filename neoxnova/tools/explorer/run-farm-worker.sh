#!/usr/bin/env bash
# run-farm-worker.sh <acc> <cp> [main|site] - persistent build worker for ONE planet.
#
# Keeps a single session and continuously feeds that planet's shipyard at its
# rate (EXPLORER_UNIT_SECONDS). Launch one per ship-building planet so all
# shipyards build in parallel. Self-logs to data/farmworker-<acc>-<cp>.log.
set -uo pipefail
cd "$(dirname "$0")"

ACC="${1:?usage: run-farm-worker.sh <acc> <cp> [main|site]}"
CP="${2:?usage: run-farm-worker.sh <acc> <cp> [main|site]}"
KIND="${3:-site}"
PLAN="plans/farm-${ACC}-${KIND}.json"
LOG="data/farmworker-${ACC}-${CP}.log"

export EXPLORER_FETCH_TIMEOUT_MS="${EXPLORER_FETCH_TIMEOUT_MS:-60000}"
export EXPLORER_UNIT_SECONDS="${FARM_UNIT_SECONDS:-90}"
export EXPLORER_UNIT_BATCH="${EXPLORER_UNIT_BATCH:-8000}"

echo "[$(date +%T)] worker ${ACC} cp=${CP} kind=${KIND}" >>"$LOG"
while true; do
  node --max-old-space-size=96 httpbot.mjs worker "$CP" "$PLAN" --interval 10 >>"$LOG" 2>&1
  echo "[$(date +%T)] worker ${CP} exited; restart in 15s" >>"$LOG"
  sleep 15
done

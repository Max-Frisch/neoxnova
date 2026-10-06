#!/usr/bin/env bash
# run-farm-build.sh - rolling farm build+pool loop (one account per host).
#
#   bash run-farm-build.sh acc1     # Windows (via run-farm.ps1)
#   bash run-farm-build.sh acc2     # VM (tmux farm-acc2)
#
# Each cycle: refresh main levels -> farm-plan.mjs -> if phase=build, resolve each
# site's goal, then bundle-pool each colony's BB/HC/BR to main (mission 4=deploy).
# Self-logs to data/farm-build-<acc>.log. Tunables: FARM_BUILD_EVERY_S (300).
set -uo pipefail
cd "$(dirname "$0")"

ACC="${1:?usage: run-farm-build.sh <acc>}"
mkdir -p data
LOG="data/farm-build-${ACC}.log"
exec >>"$LOG" 2>&1

NODE=(node --max-old-space-size=96)
BATCH="${EXPLORER_UNIT_BATCH:-3000}"
EVERY="${FARM_BUILD_EVERY_S:-300}"
CFG="plans/farm-sites.json"

read_cfg() { CFG="$CFG" ACC="$ACC" node -e "process.stdout.write(String(JSON.parse(require('fs').readFileSync(process.env.CFG,'utf8'))[process.env.ACC][process.argv[1]]))" "$1"; }
MAIN_CP="$(read_cfg mainCp)"
MAIN_COORDS="$(read_cfg mainCoords)"

mapfile -t SITES < <(CFG="$CFG" ACC="$ACC" node -e "JSON.parse(require('fs').readFileSync(process.env.CFG,'utf8'))[process.env.ACC].sites.forEach(s=>console.log(s))")
echo "[$(date +%T)] farm-build ${ACC} start: main=${MAIN_CP} ${MAIN_COORDS} sites=${SITES[*]}"

while true; do
  echo "[$(date +%T)] build cycle"
  "${NODE[@]}" httpbot.mjs levels --cp "$MAIN_CP" --out "data/farm-main-${ACC}.json"
  "${NODE[@]}" farm-plan.mjs --acc "$ACC" plan

  PHASE=$(ACC="$ACC" node -e "process.stdout.write(JSON.parse(require('fs').readFileSync('data/farm-state-'+process.env.ACC+'.json','utf8')).phase)")
  if [ "$PHASE" = "build" ]; then
    EXPLORER_UNIT_BATCH="$BATCH" "${NODE[@]}" httpbot.mjs resolve --goals "plans/farm-${ACC}-main.json" --cp "$MAIN_CP" --steps 60
    for cp in "${SITES[@]}"; do
      [ "$cp" = "$MAIN_CP" ] && continue
      EXPLORER_UNIT_BATCH="$BATCH" "${NODE[@]}" httpbot.mjs resolve --goals "plans/farm-${ACC}-site.json" --cp "$cp" --steps 60
      "${NODE[@]}" httpbot.mjs levels --cp "$cp" --out "data/farm-site-${ACC}-${cp}.json"
      POOL=$(ACC="$ACC" CP="$cp" node -e "const s=(JSON.parse(require('fs').readFileSync('data/farm-site-'+process.env.ACC+'-'+process.env.CP+'.json','utf8')).ships)||{};const b=+s['207']||0,h=+s['203']||0,r=+s['219']||0;if(b||h||r)process.stdout.write('207:'+b+',203:'+h+',219:'+r)")
      if [ -n "$POOL" ]; then
        "${NODE[@]}" httpbot.mjs fleet "$MAIN_COORDS" 4 "$POOL" 10 --cp "$cp"
        echo "[$(date +%T)] pooled $cp -> $MAIN_COORDS : $POOL"
      fi
    done
  else
    echo "[$(date +%T)] phase=$PHASE; awaiting send"
  fi
  sleep "$EVERY"
done

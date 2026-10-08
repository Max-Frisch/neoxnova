#!/usr/bin/env bash
# run-farm-build.sh - rolling farm PLANNER + POOLER (one account per host).
#
#   bash run-farm-build.sh acc1     # Windows (via run-farm.ps1)
#   bash run-farm-build.sh acc2     # VM (tmux farm-acc2)
#
# Ship building itself is done by one persistent per-planet worker
# (`httpbot.mjs worker <cp> <plan>`; see run-farm-worker.sh) so all shipyards
# build in parallel. This loop only: refreshes main levels -> farm-plan.mjs
# (writes the per-planet goal files) -> pools each colony's ships to main.
# Self-logs to data/farm-build-<acc>.log. Tunable: FARM_BUILD_EVERY_S (30).
set -uo pipefail
cd "$(dirname "$0")"

ACC="${1:?usage: run-farm-build.sh <acc>}"
mkdir -p data
LOG="data/farm-build-${ACC}.log"
exec >>"$LOG" 2>&1

NODE=(node --max-old-space-size=96)
EVERY="${FARM_BUILD_EVERY_S:-30}"
# Only pool a colony once it has accumulated a real batch, so we don't spam tiny
# deploy fleets (each is a fleet movement; expo + pooling must stay under the cap).
# Threshold = config `poolMin` (re-read every cycle), else $FARM_POOL_MIN, else 5000.
# It MUST sit below the per-site build target: the site goal is share(slots*grow*S),
# so a threshold above it makes colonies hoard ships forever while the main planet
# starves and expedition slots sit idle.
POOL_FLOOR="${FARM_POOL_MIN:-5000}"
CFG="plans/farm-sites.json"

read_cfg() { CFG="$CFG" ACC="$ACC" node -e "process.stdout.write(String(JSON.parse(require('fs').readFileSync(process.env.CFG,'utf8'))[process.env.ACC][process.argv[1]]))" "$1"; }
MAIN_CP="$(read_cfg mainCp)"
MAIN_COORDS="$(read_cfg mainCoords)"

mapfile -t SITES < <(CFG="$CFG" ACC="$ACC" node -e "JSON.parse(require('fs').readFileSync(process.env.CFG,'utf8'))[process.env.ACC].sites.forEach(s=>console.log(s))")
echo "[$(date +%T)] farm-planner ${ACC} start: main=${MAIN_CP} ${MAIN_COORDS} sites=${SITES[*]}"

while true; do
  echo "[$(date +%T)] planner cycle"
  "${NODE[@]}" httpbot.mjs levels --cp "$MAIN_CP" --out "data/farm-main-${ACC}.json"
  "${NODE[@]}" farm-plan.mjs --acc "$ACC" plan
  POOL_MIN=$(CFG="$CFG" ACC="$ACC" POOL_FLOOR="$POOL_FLOOR" node -e 'const c=JSON.parse(require("fs").readFileSync(process.env.CFG,"utf8"))[process.env.ACC]||{};process.stdout.write(String(c.poolMin||process.env.POOL_FLOOR))')
  for cp in "${SITES[@]}"; do
    [ "$cp" = "$MAIN_CP" ] && continue
    "${NODE[@]}" httpbot.mjs levels --cp "$cp" --out "data/farm-site-${ACC}-${cp}.json"
    POOL=$(ACC="$ACC" CP="$cp" POOL_MIN="$POOL_MIN" CFG="$CFG" node -e '
      const cfg=JSON.parse(require("fs").readFileSync(process.env.CFG,"utf8"))[process.env.ACC];
      const x=cfg.comp||{}; const codes=[x.main||"207",x.wall,x.cargo,x.recycler||"219"].filter(Boolean);
      const s=(JSON.parse(require("fs").readFileSync("data/farm-site-"+process.env.ACC+"-"+process.env.CP+".json","utf8")).ships)||{};
      let tot=0; const parts=[];
      for(const c of codes){const n=+s[c]||0; if(n>0){tot+=n; parts.push(c+":"+n);}}
      if(tot>=+process.env.POOL_MIN) process.stdout.write(parts.join(","));')
    if [ -n "$POOL" ]; then
      "${NODE[@]}" httpbot.mjs fleet "$MAIN_COORDS" 4 "$POOL" 10 --cp "$cp"
      echo "[$(date +%T)] pooled $cp -> $MAIN_COORDS : $POOL"
    fi
  done
  sleep "$EVERY"
done

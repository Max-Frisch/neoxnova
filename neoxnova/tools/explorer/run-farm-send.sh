#!/usr/bin/env bash
# run-farm-send.sh - rolling farm expedition send loop (one account per host).
#
#   bash run-farm-send.sh acc1     # Windows (via run-farm.ps1)
#   bash run-farm-send.sh acc2     # VM (tmux farmsend-acc2)
#
# Waits until state phase=ready and no real cmd=1 expedition is active, then
# fires the full 7-fleet set and hands S over to the next build cycle. Never
# sends a partial round. Self-logs to data/farm-send-<acc>.log. Tunable:
# FARM_SEND_EVERY_S.
set -uo pipefail
cd "$(dirname "$0")"

ACC="${1:?usage: run-farm-send.sh <acc>}"
mkdir -p data
LOG="data/farm-send-${ACC}.log"
exec >>"$LOG" 2>&1

NODE=(node --max-old-space-size=96)
EVERY="${FARM_SEND_EVERY_S:-60}"
echo "[$(date +%T)] farm-send ${ACC} start"

while true; do
  PHASE=$(ACC="$ACC" node -e "process.stdout.write(JSON.parse(require('fs').readFileSync('data/farm-state-'+process.env.ACC+'.json','utf8')).phase)" 2>/dev/null || echo "?")
  if [ "$PHASE" != "ready" ]; then sleep "$EVERY"; continue; fi

  # NOTE: only real cmd=1 sends are `Expedition (A)`. The stuck cmd=2 ghosts
  # ("Expedition at Hostail sector (R)", fleetID null) are ignored — they never
  # land and do NOT block sends (counter may exceed cap).
  STATS=$("${NODE[@]}" httpbot.mjs exp-state 2>/dev/null | ACC="$ACC" node -e '
    let d="";process.stdin.on("data",c=>d+=c).on("end",()=>{
      const i=d.indexOf("{"),j=d.lastIndexOf("}");
      if(i<0||j<0){process.stdout.write("ERR 0 0 0");return;}
      const st=JSON.parse(d.slice(i,j+1));
      const cap=st.expeditionSlots?Number(st.expeditionSlots):7;
      const active=(st.fleets||[]).filter(f=>/Expedition\s*\(A\)/i.test(f.mission)).length;
      process.stdout.write(active+" "+cap+" "+(st.fleets||[]).length);
    });')
  read -r ACTIVE CAP TOTAL <<<"$STATS"
  if [ "$ACTIVE" = "ERR" ] || [ "$ACTIVE" != "0" ]; then
    echo "[$(date +%T)] phase=ready but not idle (activeExpo=$ACTIVE cap=$CAP rows=$TOTAL); wait"
    sleep "$EVERY"; continue
  fi

  OLD_S=$(ACC="$ACC" node -e "process.stdout.write(String(JSON.parse(require('fs').readFileSync('data/farm-state-'+process.env.ACC+'.json','utf8')).S))")
  OLD_BR=$(ACC="$ACC" node -e "process.stdout.write(String(JSON.parse(require('fs').readFileSync('data/farm-state-'+process.env.ACC+'.json','utf8')).br))")
  SET="207:${OLD_S},203:$((5 * OLD_S)),219:${OLD_BR},202:1,204:1,205:1,206:1"

  "${NODE[@]}" farm-plan.mjs --acc "$ACC" sent
  echo "[$(date +%T)] firing 7 fleets: $SET"
  "${NODE[@]}" httpbot.mjs expedition "$SET" 7 1 10
  sleep "$EVERY"
done

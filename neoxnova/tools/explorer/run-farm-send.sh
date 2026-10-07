#!/usr/bin/env bash
# run-farm-send.sh - rolling farm expedition send loop (one account per host).
#
#   bash run-farm-send.sh acc1     # Windows (via run-farm.ps1)
#   bash run-farm-send.sh acc2     # VM (tmux farmsend-acc2)
#
# Keeps every expedition slot busy: as soon as a slot frees (a fleet returns) and
# the main planet can field another fleet of S, one is sent — never waiting for a
# whole round while slots sit idle. S grows once per full rotation (active==0),
# ratio-capped (see farm-plan.mjs). Slot count = min(config, live expeditionSlots)
# so acc1 picks up its 8th slot automatically when the upgrade lands.
# Self-logs to data/farm-send-<acc>.log. Tunables: FARM_SEND_EVERY_S (30).
set -uo pipefail
cd "$(dirname "$0")"

ACC="${1:?usage: run-farm-send.sh <acc>}"
mkdir -p data
LOG="data/farm-send-${ACC}.log"
exec >>"$LOG" 2>&1

NODE=(node --max-old-space-size=96)
EVERY="${FARM_SEND_EVERY_S:-30}"
CFG="plans/farm-sites.json"
MAIN_CP=$(CFG="$CFG" ACC="$ACC" node -e "process.stdout.write(String(JSON.parse(require('fs').readFileSync(process.env.CFG,'utf8'))[process.env.ACC].mainCp))")
CONFIG_SLOTS=$(CFG="$CFG" ACC="$ACC" node -e "const c=JSON.parse(require('fs').readFileSync(process.env.CFG,'utf8'))[process.env.ACC]||{};process.stdout.write(String(c.slots||7))")
echo "[$(date +%T)] farm-send ${ACC} start (config slots=${CONFIG_SLOTS})"

state_get() { ACC="$ACC" node -e "const s=JSON.parse(require('fs').readFileSync('data/farm-state-'+process.env.ACC+'.json','utf8'));process.stdout.write(String(s['$1']))"; }
# max fleets we can field right now, capped at FREE ($3), given S ($1) and BR ($2)
afford() {
  ACC="$ACC" S="$1" BR="$2" FREE="$3" node -e '
    const s=(JSON.parse(require("fs").readFileSync("data/farm-main-"+process.env.ACC+".json","utf8")).ships)||{};
    const S=+process.env.S,br=+process.env.BR,free=+process.env.FREE;
    const bb=+s["207"]||0,hc=+s["203"]||0,r=+s["219"]||0;
    const small=Math.min(+s["202"]||0,+s["204"]||0,+s["205"]||0,+s["206"]||0);
    let n=Math.min(free,Math.floor(bb/S),Math.floor(hc/(5*S)),br>0?Math.floor(r/br):free,small);
    if(!isFinite(n)||n<0)n=0;
    process.stdout.write(String(n));' 2>/dev/null
}

while true; do
  # only real cmd=1 sends are `Expedition (A)`; `(R)` ghosts (fleetID null) are ignored
  STATS=$("${NODE[@]}" httpbot.mjs exp-state 2>/dev/null | ACC="$ACC" node -e '
    let d="";process.stdin.on("data",c=>d+=c).on("end",()=>{
      const i=d.indexOf("{"),j=d.lastIndexOf("}");
      if(i<0||j<0){process.stdout.write("ERR 0 0");return;}
      const st=JSON.parse(d.slice(i,j+1));
      const det=st.expeditionSlots?Number(st.expeditionSlots):0;
      const active=(st.fleets||[]).filter(f=>/Expedition\s*\(A\)/i.test(f.mission)).length;
      process.stdout.write(active+" "+det+" "+(st.fleets||[]).length);
    });')
  read -r ACTIVE DETECTED TOTAL <<<"$STATS"
  if [ "$ACTIVE" = "ERR" ]; then echo "[$(date +%T)] exp-state unreadable; wait"; sleep "$EVERY"; continue; fi

  SLOTS=$CONFIG_SLOTS
  if [ "${DETECTED:-0}" -ge 1 ] 2>/dev/null && [ "$DETECTED" -lt "$SLOTS" ]; then SLOTS=$DETECTED; fi
  # persist detected slot count so the builder sizes the round to match
  ACC="$ACC" SLOTS="$SLOTS" node -e 'const p="data/farm-state-"+process.env.ACC+".json",f=require("fs"),s=JSON.parse(f.readFileSync(p,"utf8"));if(s.slots!==Number(process.env.SLOTS)){s.slots=Number(process.env.SLOTS);f.writeFileSync(p,JSON.stringify(s,null,2));}' 2>/dev/null

  FREE=$((SLOTS - ACTIVE))
  if [ "$FREE" -le 0 ]; then sleep "$EVERY"; continue; fi

  # refresh main ships for the affordability check (also feeds farm-plan sent)
  "${NODE[@]}" httpbot.mjs levels --cp "$MAIN_CP" --out "data/farm-main-${ACC}.json" >/dev/null 2>&1

  S=$(state_get S); BR=$(state_get br)
  N=$(afford "$S" "$BR" "$FREE"); case "$N" in ''|*[!0-9]*) N=0;; esac

  if [ "$ACTIVE" -eq 0 ] && [ "$N" -ge 1 ]; then
    # full rotation home -> grow S (ratio-capped), then use the new S
    "${NODE[@]}" farm-plan.mjs --acc "$ACC" sent
    S=$(state_get S); BR=$(state_get br)
    N=$(afford "$S" "$BR" "$FREE"); case "$N" in ''|*[!0-9]*) N=0;; esac
  fi

  if [ "$N" -lt 1 ]; then echo "[$(date +%T)] slots free=${FREE}/${SLOTS} active=${ACTIVE} but not enough ships for 1 fleet (S=$S); wait"; sleep "$EVERY"; continue; fi

  SET="207:${S},203:$((5 * S)),219:${BR},202:1,204:1,205:1,206:1"
  echo "[$(date +%T)] firing ${N} fleet(s): slots=${SLOTS} active=${ACTIVE} S=${S} br=${BR} :: $SET"
  "${NODE[@]}" httpbot.mjs expedition "$SET" "$N" 1 10
  sleep "$EVERY"
done

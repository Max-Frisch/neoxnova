#!/usr/bin/env bash
# run-farm-send.sh - rolling farm expedition send loop (one account per host).
#
#   bash run-farm-send.sh acc1     # Windows (via run-farm.ps1)
#   bash run-farm-send.sh acc2     # VM (tmux farmsend-acc2)
#
# Keeps every expedition slot busy: as soon as a slot frees (a fleet returns) and
# the main planet can field another fleet of S, one is sent — never waiting for a
# whole round while slots sit idle. S grows once per full rotation (active==0),
# ratio-capped (see farm-plan.mjs). Slot count = the LIVE expeditionSlots (config
# only a fallback), so every slot the account has is kept busy.
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

# Active = any real expedition row (outbound `(A)` OR returning `(R)`). The only
# things to exclude are acc1's permanent ghosts: "Expedition at Hostail sector (R)"
# (fleetID null, never land). Counting both legs stops mid-return re-fires.
# Prints "active slots total" (or "ERR 0 0" when the page is unreadable).
exp_active() {
  "${NODE[@]}" httpbot.mjs exp-state 2>/dev/null | ACC="$ACC" node -e '
    let d="";process.stdin.on("data",c=>d+=c).on("end",()=>{
      const i=d.indexOf("{"),j=d.lastIndexOf("}");
      if(i<0||j<0){process.stdout.write("ERR 0 0");return;}
      let st;try{st=JSON.parse(d.slice(i,j+1));}catch(e){process.stdout.write("ERR 0 0");return;}
      const active=(st.fleets||[]).filter(f=>/Expedition/i.test(f.mission)&&!/Hostail/i.test(f.mission)).length;
      const det=st.expeditionSlots?Number(st.expeditionSlots):0;
      process.stdout.write(active+" "+det+" "+(st.fleets||[]).length);
    });'
}

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

# farm-main-<acc>.json is written by `levels`; treat it as usable only if refreshed
# within MAX seconds. A stale read makes `afford` over-count and, after a send whose
# new fleet was not listed yet, previously caused an oversized re-fire.
levels_fresh() {
  ACC="$ACC" MAX="${1:-90}" node -e '
    const fs=require("fs");
    try{const s=JSON.parse(fs.readFileSync("data/farm-main-"+process.env.ACC+".json","utf8"));
      const t=s.updatedAt?Date.parse(s.updatedAt):0;
      process.exit(t && (Date.now()-t)/1000 <= +process.env.MAX && s.ships ? 0 : 1);
    }catch(e){process.exit(1);}'
}

GROWN=0   # has S been grown for the current rotation?
while true; do
  STATS=$(exp_active)
  read -r ACTIVE DETECTED TOTAL <<<"$STATS"
  if [ "$ACTIVE" = "ERR" ]; then echo "[$(date +%T)] exp-state unreadable; wait"; sleep "$EVERY"; continue; fi

  # A real fleet out means we have left the "all home" state: allow the NEXT full
  # rotation to grow S again.
  [ "$ACTIVE" -gt 0 ] && GROWN=0

  # Follow the live slot count so every available slot is kept busy (config is only
  # the fallback when the page doesn't report one).
  SLOTS=$CONFIG_SLOTS
  if [ "${DETECTED:-0}" -ge 1 ] 2>/dev/null; then SLOTS=$DETECTED; fi
  # persist detected slot count so the builder sizes the round to match
  ACC="$ACC" SLOTS="$SLOTS" node -e 'const p="data/farm-state-"+process.env.ACC+".json",f=require("fs"),s=JSON.parse(f.readFileSync(p,"utf8"));if(s.slots!==Number(process.env.SLOTS)){s.slots=Number(process.env.SLOTS);f.writeFileSync(p,JSON.stringify(s,null,2));}' 2>/dev/null

  FREE=$((SLOTS - ACTIVE))
  if [ "$FREE" -le 0 ]; then sleep "$EVERY"; continue; fi

  # refresh main ships (also feeds farm-plan sent); skip the cycle if the refresh
  # failed so an order is never sized from stale counts.
  "${NODE[@]}" httpbot.mjs levels --cp "$MAIN_CP" --out "data/farm-main-${ACC}.json" >/dev/null 2>&1
  if ! levels_fresh 90; then echo "[$(date +%T)] main levels refresh stale/failed; wait"; sleep "$EVERY"; continue; fi

  S=$(state_get S); BR=$(state_get br)

  # Grow S exactly once per full rotation. `sent` recomputes S from the PRE-send
  # total, so calling it again after firing (the new fleet not listed yet -> active
  # still 0) would inflate S. GROWN pins it to one call until fleets are seen out.
  if [ "$ACTIVE" -eq 0 ] && [ "$GROWN" -eq 0 ]; then
    "${NODE[@]}" farm-plan.mjs --acc "$ACC" sent
    GROWN=1
    S=$(state_get S); BR=$(state_get br)
  fi

  N=$(afford "$S" "$BR" "$FREE"); case "$N" in ''|*[!0-9]*) N=0;; esac
  if [ "$N" -lt 1 ]; then echo "[$(date +%T)] slots free=${FREE}/${SLOTS} active=${ACTIVE} but not enough ships for 1 fleet (S=$S); wait"; sleep "$EVERY"; continue; fi

  SET="207:${S},203:$((5 * S)),219:${BR},202:1,204:1,205:1,206:1"
  echo "[$(date +%T)] firing ${N} fleet(s): slots=${SLOTS} active=${ACTIVE} S=${S} br=${BR} :: $SET"
  "${NODE[@]}" httpbot.mjs expedition "$SET" "$N" 1 10

  # Confirm the send registered. If it did not, the next cycle retries with the
  # SAME S (GROWN still set; the fresh-levels check blocks an over-sized retry).
  sleep 8
  CHK=$(exp_active | cut -d' ' -f1)
  if [ "$CHK" = "ERR" ] || [ "${CHK:-0}" -lt 1 ] 2>/dev/null; then
    echo "[$(date +%T)] send not confirmed (active=$CHK); will retry next cycle"
  fi
  sleep "$EVERY"
done

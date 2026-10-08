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
# Composition (config) + the hull actually flying (state — the ramp flips it).
COMP=$(CFG="$CFG" ACC="$ACC" node -e "const c=JSON.parse(require('fs').readFileSync(process.env.CFG,'utf8'))[process.env.ACC]||{};const x=c.comp||{};process.stdout.write([x.main||'207',x.wall||'',x.wallPer||5,x.cargo||'',x.recycler||'219'].join('|'))")
IFS='|' read -r C_MAIN C_WALL C_WALLPER C_CARGO C_RECY <<<"$COMP"
echo "[$(date +%T)] farm-send ${ACC} start (config slots=${CONFIG_SLOTS} main=${C_MAIN} wall=${C_WALL:-none} cargo=${C_CARGO:-none} recy=${C_RECY})"

state_get() { ACC="$ACC" node -e "const s=JSON.parse(require('fs').readFileSync('data/farm-state-'+process.env.ACC+'.json','utf8'));const v=s['$1'];process.stdout.write(v==null?'':String(v))"; }

# Active = any real expedition row (outbound `(A)` OR returning `(R)`). The only
# things to exclude are acc1's permanent ghosts: "Expedition at Hostail sector (R)"
# (fleetID null, never land). Counting both legs stops mid-return re-fires.
# Prints "active slots total inflight" where inflight = summed ship count of the
# real fleets (from the live page, so the planner can size S accurately; the old
# run-log reconstruction over-counted). "ERR 0 0 0" when the page is unreadable.
exp_active() {
  "${NODE[@]}" httpbot.mjs exp-state 2>/dev/null | ACC="$ACC" node -e '
    let d="";process.stdin.on("data",c=>d+=c).on("end",()=>{
      const i=d.indexOf("{"),j=d.lastIndexOf("}");
      if(i<0||j<0){process.stdout.write("ERR 0 0 0");return;}
      let st;try{st=JSON.parse(d.slice(i,j+1));}catch(e){process.stdout.write("ERR 0 0 0");return;}
      const real=(st.fleets||[]).filter(f=>/Expedition/i.test(f.mission)&&!/Hostail/i.test(f.mission));
      const det=st.expeditionSlots?Number(st.expeditionSlots):0;
      const inflight=real.reduce((a,f)=>a+(Number(String(f.number||"0").replace(/[^0-9]/g,""))||0),0);
      process.stdout.write(real.length+" "+det+" "+(st.fleets||[]).length+" "+inflight);
    });'
}

# max fleets we can field right now, capped at FREE ($3), given S ($1), recycler
# count/fleet BR ($2) and cargo count/fleet CARGON ($4). Composition codes/ratios
# come from the environment (MAIN/WALL/WALLPER/CARGOC/RECY).
afford() {
  ACC="$ACC" S="$1" BR="$2" CARGON="$3" FREE="$4" \
  MAIN="$MAIN" WALL="$C_WALL" WALLPER="$C_WALLPER" CARGOC="$C_CARGO" RECY="$C_RECY" node -e '
    const s=(JSON.parse(require("fs").readFileSync("data/farm-main-"+process.env.ACC+".json","utf8")).ships)||{};
    const S=+process.env.S,br=+process.env.BR,cargo=+process.env.CARGON,free=+process.env.FREE;
    const main=+s[process.env.MAIN]||0, wall=+s[process.env.WALL]||0, cg=+s[process.env.CARGOC]||0, r=+s[process.env.RECY]||0;
    const small=Math.min(+s["202"]||0,+s["204"]||0,+s["205"]||0,+s["206"]||0);
    let n=Math.min(free,Math.floor(main/S),
      process.env.WALL?Math.floor(wall/(+process.env.WALLPER*S)):free,
      process.env.CARGOC&&cargo>0?Math.floor(cg/cargo):free,
      br>0?Math.floor(r/br):free, small);
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

while true; do
  STATS=$(exp_active)
  read -r ACTIVE DETECTED TOTAL INFLIGHT <<<"$STATS"
  if [ "$ACTIVE" = "ERR" ]; then echo "[$(date +%T)] exp-state unreadable; wait"; sleep "$EVERY"; continue; fi

  # Follow the live slot count so every available slot is kept busy (config is only
  # the fallback when the page doesn't report one).
  SLOTS=$CONFIG_SLOTS
  if [ "${DETECTED:-0}" -ge 1 ] 2>/dev/null; then SLOTS=$DETECTED; fi
  # persist detected slot count + active fleets so the planner sizes the round and
  # reconstructs the in-flight ships (account-wide ratio gate) from the same view.
  ACC="$ACC" SLOTS="$SLOTS" ACTIVE="$ACTIVE" node -e 'const p="data/farm-state-"+process.env.ACC+".json",f=require("fs"),s=JSON.parse(f.readFileSync(p,"utf8"));let ch=false;if(s.slots!==Number(process.env.SLOTS)){s.slots=Number(process.env.SLOTS);ch=true;}if(s.active!==Number(process.env.ACTIVE)){s.active=Number(process.env.ACTIVE);ch=true;}if(ch)f.writeFileSync(p,JSON.stringify(s,null,2));' 2>/dev/null

  # refresh main ships (also feeds farm-plan sent); skip the cycle if the refresh
  # failed so an order is never sized from stale counts.
  "${NODE[@]}" httpbot.mjs levels --cp "$MAIN_CP" --out "data/farm-main-${ACC}.json" >/dev/null 2>&1
  if ! levels_fresh 90; then echo "[$(date +%T)] main levels refresh stale/failed; wait"; sleep "$EVERY"; continue; fi

  # Size S from the WHOLE-fleet capacity EVERY cycle — even when every slot is busy
  # (FREE==0). `--active` + `--inflight-ships` describe the fleets currently flying
  # (live exp-state ship counts); the planner reconstructs their MAIN count so S
  # fills all slots instead of ratcheting ~2x too high off the run log.
  "${NODE[@]}" farm-plan.mjs --acc "$ACC" sent --active "$ACTIVE" \
    --inflight-ships "${INFLIGHT:-0}" --inflight-count "${ACTIVE:-0}"
  S=$(state_get S); BR=$(state_get br); CARGON=$(state_get cargo)
  MAIN=$(state_get main); MAIN=${MAIN:-$C_MAIN}
  CARGON=${CARGON:-0}

  FREE=$((SLOTS - ACTIVE))
  if [ "$FREE" -le 0 ]; then sleep "$EVERY"; continue; fi

  N=$(afford "$S" "$BR" "$CARGON" "$FREE"); case "$N" in ''|*[!0-9]*) N=0;; esac
  if [ "$N" -lt 1 ]; then echo "[$(date +%T)] slots free=${FREE}/${SLOTS} active=${ACTIVE} but not enough ships for 1 fleet (main=${MAIN} S=$S); wait"; sleep "$EVERY"; continue; fi

  SET="${MAIN}:${S}"
  [ -n "$C_WALL" ] && SET="${SET},${C_WALL}:$((C_WALLPER * S))"
  [ -n "$C_CARGO" ] && [ "$CARGON" -gt 0 ] && SET="${SET},${C_CARGO}:${CARGON}"
  SET="${SET},${C_RECY}:${BR},202:1,204:1,205:1,206:1"
  echo "[$(date +%T)] firing ${N} fleet(s): slots=${SLOTS} active=${ACTIVE} S=${S} br=${BR} :: $SET"
  # One fleet per POST. A single `exp_num=N` request was unreliable (rejected /
  # only partly applied) and left slots idle; N separate single-fleet sends land
  # deterministically and each is logged as its own run.
  # Always target the main planet explicitly: the session's "current" planet
  # drifts to whichever colony a worker last touched, and the expedition then
  # fails (posts from a planet that doesn't hold the fleet).
  for ((k = 0; k < N; k++)); do
    "${NODE[@]}" httpbot.mjs expedition "$SET" 1 1 10 --cp "$MAIN_CP"
  done

  # Confirm the send registered; if it did not, the next cycle retries (the fresh
  # `levels` guard blocks an order sized from stale counts).
  sleep 8
  CHK=$(exp_active | cut -d' ' -f1)
  if [ "$CHK" = "ERR" ] || [ "${CHK:-0}" -le "${ACTIVE:-0}" ] 2>/dev/null; then
    echo "[$(date +%T)] send not confirmed (active=$CHK, was $ACTIVE); will retry next cycle"
  fi
  sleep "$EVERY"
done

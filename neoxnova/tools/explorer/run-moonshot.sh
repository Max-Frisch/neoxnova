#!/usr/bin/env bash
# run-moonshot.sh - one-off moonshot operation: attack Account2's galaxy-2
# planets with 2500 Battle Recyclers (219) twice each, launched from the planet
# at 2:188:9 (cp 1648). Waits until the deployed BRs have landed, then fires in
# a slot-aware loop (max 30 fleets). 2:193:20 is excluded (already has a moon).
#
# Usage: bash run-moonshot.sh [start|loop|status|logs|stop] [tag]
#
# Tunables: MM_SRC_CP, MM_SHIP, MM_PER, MM_PER_PLANET, MM_MIN_BR, MM_SPEED.
set -uo pipefail
cd "$(dirname "$0")"

NODE_FLAGS="--max-old-space-size=96"
NODE="node $NODE_FLAGS"
SRC_CP="${MM_SRC_CP:-1648}"
SHIP="${MM_SHIP:-219}"
PER="${MM_PER:-2500}"
PER_PLANET="${MM_PER_PLANET:-2}"
MIN_BR="${MM_MIN_BR:-75000}"
SPEED="${MM_SPEED:-10}"

TARGETS=(
  2:178:20 2:182:20 2:184:20 2:185:20 2:186:20
  2:187:20 2:189:20 2:191:20 2:192:19 2:193:1
  2:195:20 2:196:20 2:197:20 2:198:20 2:199:20
)
# Env override: MM_TARGETS="2:178:20 2:184:20 ..." replaces the default list.
if [ -n "${MM_TARGETS:-}" ]; then read -r -a TARGETS <<<"$MM_TARGETS"; fi

br_count() {
  local html n
  html=$($NODE httpbot.mjs dump "page=shipyard&mode=fleet&cp=$SRC_CP" 2>/dev/null)
  n=$(printf '%s' "$html" | grep -aoE "id=\"val_${SHIP}\"[^>]*>[0-9.]+" | grep -aoE '[0-9.]+$' | tr -d '.')
  echo "${n:-0}"
}

fleet_used_max() {
  local html fm used max
  html=$($NODE httpbot.mjs dump "page=fleetTable" 2>/dev/null)
  fm=$(printf '%s' "$html" | grep -aoE 'Fleet [0-9]+ / [0-9]+' | head -1)
  read -r _ used _ max <<<"$fm"
  echo "${used:-0} ${max:-30}"
}

run_loop() {
  local tag="$1" log="data/moonshot-${1}.log"
  mkdir -p data
  log() { echo "[$(date +%T)] $*" | tee -a "$log"; }

  log "moonshot START src=cp$SRC_CP ship=$SHIP per=$PER x$PER_PLANET on ${#TARGETS[@]} targets"
  log "targets: ${TARGETS[*]}"

  while :; do
    local br; br=$(br_count)
    if [ "$br" -ge "$MIN_BR" ]; then log "BRs ready: $br >= $MIN_BR"; break; fi
    log "waiting for BRs at cp$SRC_CP: $br / $MIN_BR"
    sleep 10
  done

  local sent=0 total=$(( ${#TARGETS[@]} * PER_PLANET ))
  for t in "${TARGETS[@]}"; do
    for i in $(seq 1 "$PER_PLANET"); do
      while :; do
        read -r used max <<<"$(fleet_used_max)"
        [ "$used" -lt "$max" ] && break
        log "slots full ($used/$max), waiting"
        sleep 20
      done
      local out
      out=$($NODE httpbot.mjs fleet "$t" 1 "${SHIP}:${PER}" "$SPEED" --cp "$SRC_CP" 2>&1)
      if echo "$out" | grep -q 'OK sent'; then
        sent=$((sent + 1))
        log "OK [$sent/$total] $t attack $i/$(($PER_PLANET)) :: $(echo "$out" | grep -aoE '\[fleet\] OK sent.*' | head -1)"
      else
        log "FAIL $t attack $i :: $(echo "$out" | grep -aoE '\[fleet\].*' | tail -1)"
      fi
    done
  done
  log "moonshot DONE sent=$sent/$total"
}

case "${1:-start}" in
  start)
    tag="${2:-acc1}"
    sess="moonshot-$tag"
    if tmux has-session -t "$sess" 2>/dev/null; then echo "[=] $sess already running"; exit 0; fi
    mkdir -p data
    # tmux new-session does not inherit custom vars, so forward the resolved
    # parameters explicitly into the loop invocation.
    prefix="MM_SRC_CP='$SRC_CP' MM_SHIP='$SHIP' MM_PER='$PER' MM_PER_PLANET='$PER_PLANET' MM_MIN_BR='$MIN_BR' MM_SPEED='$SPEED' MM_TARGETS='${MM_TARGETS:-}'"
    tmux new-session -d -s "$sess" "cd '$PWD' && $prefix bash run-moonshot.sh loop $tag"
    echo "[+] started $sess -> data/moonshot-${tag}.log (per_planet=$PER_PLANET targets=${#TARGETS[@]})"
    ;;
  loop) run_loop "${2:-acc1}" ;;
  status) tmux ls 2>/dev/null | grep -E '^moonshot-' || echo "[*] no moonshot daemon running" ;;
  logs) tail -n 40 -f "data/moonshot-${2:-acc1}.log" ;;
  stop)
    tag="${2:-acc1}"
    tmux kill-session -t "moonshot-$tag" 2>/dev/null && echo "[-] stopped moonshot-$tag" || echo "[*] not running"
    ;;
  *) echo "usage: bash run-moonshot.sh {start|status|logs|stop} [tag]" >&2; exit 2 ;;
esac

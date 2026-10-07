# Rolling expedition farm — runbook & plan

Hands-off rolling farm for both accounts. One set of scripts per account keeps
the shipyard building the expedition set, pools it to the main planet, and
re-fires 7 expeditions whenever expedition slots free up. The agent only
bootstraps and tunes; **never** load full datasets — read file tails only.

Referenced from `BACKLOG.md`. Everything runs from `neoxnova/tools/explorer/`.

> Status: **LIVE** (since 2026-10-06/07) on both accounts. See `BACKLOG.md` "Live state"
> for the current cycle/S and the BR-gate fix.

## Accounts & hosts
- **acc1 Bratwurst** — main `3:125:12` (cp `1593`); runs locally on Windows
  (PowerShell detached loops; **no tmux**).
- **acc2 TheBob** — main `2:188:16`; runs on Azure VM `azure-bot`
  (`ssh -F neoxnova/secrets/ssh/config azure-bot`), tmux.
- Keep as-is tonight. Moving acc1 onto the VM (shared public IP) is deferred
  — it needs an account selector (see "Known gaps").
- **VM sync rule:** local commit → `git push` →
  `ssh -F neoxnova/secrets/ssh/config azure-bot 'git -C ~/neoxnova fetch && git -C ~/neoxnova reset --hard origin/main'`.

## Expedition set (per fleet)
| role | code | count | notes |
|---|---|---|---|
| BB (Battleship) | `207` | `S` | main damage |
| HC (Heavy Cargo) | `203` | `5 * S` | held at 5:1 to BB |
| BR (Battle Recycler) | `219` | `round(S / 250)` | use your 250:1 rule |
| Light Cargo | `202` | `1` | |
| Light Fighter | `204` | `1` | |
| Heavy Fighter | `205` | `1` | |
| Cruiser | `206` | `1` | |

Exactly **one of each small ship** per fleet (Light Cargo / LF / HF / Cruiser;
the 1-of-each is a probe for the "unlock all ship-find types" theory).

> **Do not send Spy Probe (`210`)** — spies sent to slot 21 / an expedition
> target throw an error on this server. It is excluded from the set entirely.
>
> Any other ship can be sent; the server rejects the whole set with "Not all
> ships are available." only if one of them is *short on the planet* (e.g. Light
> Cargo before it has been built). The build step produces the 1-of-each.

Set string for the `expedition` command:
`207:S,203:5S,219:BR,202:1,204:1,205:1,206:1`

### Fleet points (unit cost / 1e6 metal+crystal, excl. deuterium)
Verified against `testdata/niburus_catalog.json`:

| code | metal | crystal | points |
|---|---:|---:|---:|
| 207 Battleship | 41 000 | 17 000 | **0.058** |
| 203 Heavy Cargo | 6 000 | 6 000 | **0.012** |
| 219 Battle Recycler | 1 000 000 | 600 000 | **1.6** |

`pts = 0.058*S + 0.012*5S + 1.6*round(S/250)` → S=42 000 ≈ **5 225 pts/fleet**.
This crosses the suspected **5 k** Arsenal tier (threshold semantics still
unconfirmed; user believes tiers are 5k/50k/250k). Higher tiers need S ≥ ~400 k
(50 k) / ~2 M (250 k) — far out of reach for now.

## Growth rule
- **S is endogenous.** After a *full rotation* (all 7 fleets home and pooled at
  main), `S = max(42000, min(floor(have_BB/7), floor(have_HC/35)))` — capped by
  the **scarcest** fleet component (BB:HC = 1:5) so a bloated stockpile of one
  ship cannot balloon S beyond what the other can field; then rebuild toward
  `7*S` and send 7.
- Ship-find loot + continuous production make each cycle's S larger than the
  last; no fixed increment to tune.
- Start at **S = 42 000** (HC 210 000) once, then hand over to the rule.
- **BR** always `round(S/250)` (S=42k → 168; = 20 BR per 5 000 BB).
- Ratios to maintain: BB:HC = 1:5, BB:BR = 250:1; exactly 1 of each small ship.

## Key command facts (verified in `httpbot.mjs`)
- `expedition <code:count,…> [num] [time] [speed] [--pve N] [--cp id]`
  (`httpbot.mjs:737`). `num` = fleets sent **now**, each consuming the full
  composition ⇒ 7 fleets of S need **7*S BB**. Use `num=7 time=1 speed=10`.
  Never pass `--pve` (cmd=2 leaks ghost fleets; `docs/EXPEDITIONS_LIVE_2026-10-06.md` §4).
- `fleet <g:s:p> <mission> <code:count,…> [speed] [--dry] [--cp id]`
  (`httpbot.mjs:671`). Mission **4 = deploy**. Target must be a real celestial.
- `exp-state` (`httpbot.mjs:771`) prints JSON `{expeditionSlots, used, fleets}`.
  The cap is **7 on both accounts**, but the counter is **not enforced** (live
  2026-10-06: fired at `7/7` → `8/7` → `9/7`). acc1 carries **6 permanent
  `cmd=2` ghosts** — rows `"Expedition at Hostail sector (R)"` with `fleetID:
  null` that never land. **Real cmd=1 sends are `"Expedition (A)"` and have a
  fleetID.** The send daemon counts only `(A)` rows, so the ghosts can't stall
  it. (The `used` counter includes ghosts — ignore it; don't gate on `used`.)
- `levels --cp <id> --out <file>` writes `{buildings, research, ships, defenses,
  resources}`; `ships["207"]` = BB count, `buildings["21"]` = shipyard level.
- `resolve --goals <file> --cp <id> --steps N`; `EXPLORER_UNIT_BATCH=<n>` caps
  units queued per submission. Ship goals are absolute counts to *have* on that
  planet; `resolve` exits when met, so regenerating the goal file each cycle is
  how S moves (chosen strategy).

## Daemons

### Build — `run-farm-build.sh <acc>` (tight loop, ~60 s)
1. `node httpbot.mjs levels --cp <main> --out data/farm-main-<acc>.json`.
2. `node farm-plan.mjs --acc <acc>` → recompute S, write
   `plans/farm-<acc>-main.json` (BB/HC **share** + the **whole** BR need + 7× each
   small ship) and `plans/farm-<acc>-site.json` (BB/HC share only) and
   `data/farm-state-<acc>.json`.
3. Per build site: `EXPLORER_UNIT_BATCH=8000 node httpbot.mjs resolve --goals
   <plan> --cp <id> --steps 60` (main uses `-main`, colonies use `-site`). `resolve`
   blocks while units build and refills as each batch drains, so the shipyard queue
   stays busy without oversized single orders.
4. Pool each colony → main (one bundled send, never per single ship):
   `node httpbot.mjs fleet <mainCoords> 4 207:<n>,203:<n>,219:<n> 10 --cp <colony>`
   with `<n>` = live counts from that colony's `levels`.
5. Write state; sleep 300 s.

### Send — `run-farm-send.sh <acc>` (loop ~60 s)
1. `node httpbot.mjs exp-state` → count **real** outgoing expeditions
   (`mission` matches `Expedition (A)`); ignore the `cmd=2` `(R)` ghosts.
2. If no real expedition is active (`(A)` count == 0) **and** state phase is
   `ready`: `node httpbot.mjs expedition "207:S,203:5S,219:BR,202:1,204:1,205:1,206:1" 7 1 10`.
   (Don't gate on the `used` counter — it includes ghosts.)
3. Sleep 60 s; fast/delay returns are staggered, re-fire when the round is home.
4. Guard total fleet slots: 7 expo + pooling ≤ **25** total movements.

### State machine (`data/farm-state-<acc>.json`)
```json
{ "cycle": 0, "S": 42000, "br": 168, "phase": "build", "lastSendAt": null }
```
- `build` → when main holds ≥ `7*S` BB (and ≥ `35*S` HC, ≥ `7*br` BR, small ships
  present) set `phase="ready"`.
- `ready` + 0 outgoing expeditions → send 7, then `cycle++`, recompute
  `S = max(42000, min(have_BB_before_send/7, have_HC_before_send/35))` for the
  **next** cycle, `phase="build"`. (Compute S from the pre-send total so it grows,
  not from 0; cap by the scarcest component.)
- Never send partial rounds with a stale S; if not all 7 slots are free, wait.

## Scripts to create (next session)
- `tools/explorer/farm-plan.mjs` — recompute S, emit the two goal JSONs + state.
- `tools/explorer/run-farm-build.sh` — build + pool loop.
- `tools/explorer/run-farm-send.sh` — expedition send loop.
- `tools/explorer/run-farm.ps1` — acc1 detached launcher (mirror
  `run-colony-daemon.ps1`), starts the two loops with logging to `data/`.
- `plans/farm-sites.json` — config filled by recon (below).

### `plans/farm-sites.json` (template)
```json
{
  "acc1": { "mainCp": "1593", "mainCoords": "3:125:12",
            "sites": ["<mainCp>", "<same-system cp>", "<3:124 cp>"] },
  "acc2": { "mainCp": "<recon>", "mainCoords": "2:188:16",
            "sites": ["<mainCp>", "<2:188 cp>", "<2:187/2:186 cp>"] }
}
```

## Recon checklist (one-time, next session)
- `node httpbot.mjs planets` on both accounts → cp/name/coords of every planet.
- `node httpbot.mjs levels --cp <id> --out data/lv-<id>.json` for **main + nearest
  system + 1 further out** only. acc1: `1593` + same-system `3:125:*` +
  `3:124:*`. acc2: main + nearest of `2:188/*`, `2:187/*`, `2:186/*`.
- Record per site: **shipyard level** (`buildings["21"]`), resources, and flight
  time to main (deploy). Prefer sites within the same system / one system out to
  keep deploy + expedition times short.
- Pick 2–4 build sites per account; fill `plans/farm-sites.json`.

## Launch
- **Bootstrap (once, before the daemons):** build the full starter on the
  resource-rich main so the first round fires quickly:
  `node farm-plan.mjs --acc <acc> starter` → `resolve --goals
  plans/farm-<acc>-starter.json --cp <main> --steps 1000000`; then `farm-plan
  --acc <acc> sent`, fire the 7-set manually, and start the daemons below.
- **acc1 (Windows):** `powershell -File run-farm.ps1 -Acc acc1` (spawns both
  loops detached; logs `data/farm-build-acc1.log`, `data/farm-send-acc1.log`).
- **acc2 (VM):** `tmux new -d -s farm-acc2 'bash run-farm-build.sh acc2'` and
  `tmux new -d -s farmsend-acc2 'bash run-farm-send.sh acc2'`; harvest already
  runs as `expharv-acc2`.
- Harvest daemons: acc1 local loop, acc2 tmux `expharv-acc2` (`run-expharvest.sh`).

## Track (agent glance, tails only)
- `data/exp-harvest-<acc>.log` summary lines + `exp-report` fight reports:
  enemy comp, def/atk ratio, BB vs HC loss split (fodder effect).
- Ship-find recovered types (does 1-of-each unlock all?), BR debris, combat XP,
  Arsenal upgrade finds, net BB/HC delta.
- Update `BACKLOG.md` periodically; **commit only when asked**.

## Safety / guardrails
- Confirm the set string parses with a **1-fleet smoke send** before any mass
  send: `expedition "207:1,203:5,219:1,202:1,204:1,205:1,206:1" 1 1 10`
  and verify the `slots now x/y` line. (Args are `num time speed` — only three
  numbers; the set needs the 1-of-each small ships, incl. Light Cargo, present.)
- Watch fleet-slot use (7 expo + pooling ≤ 25); back off pooling near the cap.
- **Never** send DD (`226`), Black Moon (`216`), or Frigate (`227`) — leave parked.
- If a black hole eats a fleet, just rebuild.
- BR is very expensive (1.6 M pts each) — do not over-buy; respect `S/250`.

## Known gaps / risks
- `httpbot.mjs:33` hardcodes `NIBURU_USER/PASS` from `secrets/explorer.env`; only
  `EXPLORER_*`/`NIBURU_BASE_URL` can come from env (`:31`). One checkout = one
  account. Running both on the VM needs `EXPLORER_ACCOUNT` support (or a second
  checkout). Deferred.
- Same-public-IP for both accounts is a multi-account/link risk; `deploy-vm.sh`
  intentionally kept acc2 on a separate IP. Account interaction (acc1 harvest
  hub was `2:188:9`, acc2 territory) must stop.
- VM is 2 vCPU / 1 GB — if consolidating, merge build+send into one sequential
  script per account (→ 4 node procs) and keep `--max-old-space-size=96`.
- Arsenal threshold still ambiguous (5k vs 75k; 5k/50k/250k tiers). This farm's
  ~5 225 pts/fleet is the live probe.

## Open items
- acc2 exact build sites + flight times (recon).
- Whether HC growth is wanted from day one or only once ship-finds look rare.
- Whether to grow S from day one or only after the first full rotation
  (current plan: build the 7× starter, send, then let S grow endogenously).

## Next session — first steps
1. Read `BACKLOG.md` + this file.
2. VM sync (commit → push → `reset --hard`); confirm harvest daemons.
3. Recon both accounts; fill `plans/farm-sites.json`.
4. Create the four scripts; smoke-test the send parse with one small fleet.
5. Bootstrap: build the 7× starter at main, fire 7, start the daemons.
6. Update `BACKLOG.md`; commit only when asked.

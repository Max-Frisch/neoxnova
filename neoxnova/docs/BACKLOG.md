# BACKLOG / status — read first; update at task end; keep < 80 lines

## Live state
- Stack: `docker compose` (`neoxnova_postgres`, `neoxnova_redis`); run `go`/`make` from `neoxnova/`.
- **Both rolling farms ONLINE (2026-10-07).** acc1 local (build+send+harvest+bonus), acc2 VM.
  **Slot-aware top-up** (`run-farm-send.sh` loop 30 s): `slots = min(cfg, expeditionSlots)`
  (**acc2 = 8 now**, acc1 = 7→8 auto-detected), `active` = real exps (any `/Expedition/` row
  not `/Hostail/` — the `(A)`/`(R)` letter is NOT the ghost test; a real fleet flips A→R with
  `fleetID:null` on return). Fires `n = min(free, affordable)` fleets so freed slots never idle;
  `farm-plan sent` grows S once per full rotation: `S = max(42000, min(⌊BB/slots⌋, ⌊HC/(5·slots)⌋))`.
  Latest: acc2 cycle=4 S 98 314→61 764 (scaled to HC in flight) → recovered as ships landed.
- **Ship production model:** shipyard is a **`Building: N per second` factory** (per-ship
  Duration rounds to `00h 00m 00s` = continuous rate, e.g. HC `288/s`). `parse.mjs` now reads
  `perSec`; `httpbot resolve` sizes each order to `EXPLORER_UNIT_SECONDS` of throughput (~90 s)
  and re-submits ~2.5 s after it completes — no fixed 30/60 s poll gap. Also stopped the
  unit `[!] clearing stale pending` re-order (was double-queuing batches → overbuild).
- **Adaptive BB:** `farm-plan` queues **zero BB whenever main HC < 5×BB** (HC is the crystal-gated
  bottleneck); resumes automatically. `plans/farm-sites.json`: `mineGoals` {2:44, 71:10},
  `pauseBB` override. acc2 was 1.75M BB vs 274k HC (needs 5:1) → S was HC-capped, BB idle.
- **`httpbot trade <buy> <code:amt,...>`:** resource trader, value ratio 1:2:4, **250 DM per
  call → only BIG lump trades (billions)**. Site plan feeds crystal mine via resolve.
- **Parallel build (`6f24227`):** server allows concurrent sessions per account, so run **one
  persistent `httpbot worker <cp> <plan>` per ship-building planet** — all shipyards build at once.
  Session is reused across processes via `data/session-<user>.json` (login once; relogin only when a
  request proves logout). `run-farm-build.sh` is now **planner + pooler only**; workers launched by
  `run-farm-worker.sh <acc> <cp> main|site`. Live: acc2 4 workers (`1598 @288/s`, `1672/1673 @216/s`,
  `1674 @192/s`); acc1 3 site workers `@192/s` (main idle — target met). Check `tmux ls` (VM) /
  `run-farm-worker` procs (acc1).
- **Farm bug fixed `eb79f39`,`9fcbcab`:** acc2 sat `phase=build` 8 h+ — the ready gate required
  main `BR >= 7*br=2023` but BR collapsed to 294 post-send and crystal-poor colonies owned the BR
  share, so `resolve` never rebuilt it (BB/HC over-built to 2.0M/3.4M meanwhile). Fix: **main carries
  the whole BR need**, sites build BB/HC only. Deploy = commit→push→VM `reset --hard`.
- acc1 (local, Bratwurst `3:125:12`): espionage L21; colony builders stopped (caps); **moon present**
  (<10 000 km). Host back on this session.
- acc2 (VM `azure-bot`, TheBob `2:188:16`): colony builders `colo-1687/88/89/96` running, `1697/98` done.
  VM tmux `farm-acc2` (build), `farmsend-acc2` (send), `expharv-acc2` (harvest), `bonus-acc2`
  (`run-bonus.sh`, `page=bonus` every 900 s).
- VM sync rule: local commit -> `push` -> `ssh azure-bot 'git -C ~/neoxnova reset --hard origin/main'`.
- **Rolling farm (LIVE since 2026-10-06/07):** runbook/spec in `docs/ROLLING_FARM.md` (S-rule line
  below is now stale — see Live state). Set = `207:S,203:5S,219:round(S/250)` + 1 each of
  `202/204/205/206` (no Spy Probe `210` — errors at slot 21).
- **Expo matrix (2026-10-06):** both accounts, `cmd=1`, main planets, `time=1`, `speed=10`.
  **Round 1** (08:49Z) = **0 combat / 14 fleets**. **Round 2** re-fired 7 arms (09:30–09:37Z);
  1 delayed returner per account, both landed ~10:04Z. **Round 3** (10:06Z) fired on **both**
  accounts, 7 arms: `217:100` · `217:50+204:500` · `217:50+226:100` · `217:50+215:100` ·
  `226:100` · `207:100` · `217:100+204:1000`. Arm 4 replaces the recorded `217:50+216:20`
  (Black Moons: acc1 has 6, acc2 has 0) with a Battle Cruiser escort both own. Real cap is **7
  on both**; acc1's `13/7` counter is the false legacy `cmd=2` shadow-slot display bug (ignore).
  Harvest daemons (`run-expharvest.sh`: tmux `expharv-acc2` on VM, local loop) write
  `data/expeditions.json`, `data/expedition-reports.json`; sends in `data/expedition-runs.json`.
  **Round 4** (10:36Z) re-fired the same 7 small arms on both. **Round 5 = BIG (both, ~11:05Z):**
  built up both fleets via `resolve plans/exp-big.json` (builds are near-instant here) →
  **acc1 ~22.3k / acc2 ~20.5k fleet points**; fired 7 big arms each: `226:1000` (5k pts) ·
  `226:2000` (10k) · `217:1000` · `217:1000+204:3000` · `207:4000` · `225:2000` · `219:1000`.
  **Results (both idle, all home): rounds 4+5 rolled 0 combats** (13.3 % pirate roll missed on 28
  expeditions) ⇒ **ratio above 503 pts still unmeasured**. Fleet deltas before-r5→end net-positive
  for every class on both accounts, except acc2 `219:1000` **lost to a black hole** (msg 219935).
  Black-hole rate: acc1 **0/70**, acc2 **1/35**. Ratio table + §: `docs/EXPEDITIONS_LIVE_2026-10-06.md` §6.
  **Next: keep firing big arms until a high-point combat triggers; then build toward ~50k.**
- **Arsenal (2026-10-06):** in-game page `game.php?page=arsenal`; help = `game.php?page=manualinfo&id=10`
  (Russian). Upgrades drop from (a) regular `cmd=1` expedition "Infinite distances" **only if the
  sent fleet ≥ 75,000 fleet points** (1 pt = 1,000,000 metal+crystal excl. deuterium; pirates
  encounter 13.3 %, 10 % find after win); (b) Hostail `cmd=2`: Barbarians 8 % (laser/ion/jet/light
  armor/light shield), Pirates 11 % (ion/plasma/impulse/medium), Aliens 14 % (plasma/grav/hyperspace/
  heavy); +1 % find per 10 combat levels. Activation: first 10 levels 100 %; above 10 −2 % per
  success, floor 75 %; fail = −10 % of last value. Details: `docs/ARSENAL_LIVE_2026-10-06.md`.
  User's memory says the tiers are **5k/50k/250k** (light/medium/heavy) and doubts the doc's single
  75k; round 5 (≥5k) is the first probe. "light/medium/heavy" also = Barbarian/Pirate/Alien Hostail
  tiers.

## Open backlog (ordered; one item per session)
1. **Expedition resolver (Go)** — capture done (see below); model `MissionExpedition` in
   `internal/engine`: outcome roll, loot, and points-scaled enemy. BLOCKERS: enemy formula unknown;
   `cmd=2` leaks ghost fleets (avoid). Notes: `docs/EXPEDITIONS_LIVE_2026-10-06.md`.
1b. **Expedition enemy formula** — composition-controlled samples (pure vs escort vs
    **meatshield/"Schussfang"**); stop poking `--pve`. Rounds **2 & 3 produced fight reports**
    (`exp-report`, messcat=3) — full table in `docs/EXPEDITIONS_LIVE_2026-10-06.md` §6. Result:
    enemy points ≈ **~0.7× sent fleet points at large sizes** (DD100 → 500 pts vs enemy 342) but a
    **fixed minimum template + high variance** at small sizes (BT10 → 2–23 pts). Enemy is a fixed mix
    (Heavy Cargo/LF/Cruiser/BB/Star Fighter/BT/Destroyer) incl. types never sent; escorts do **not**
    protect cargo (`217` survival not better with DD escort). **Ratio converges to ~0.68 by 500 pts**
    (full table in `docs/EXPEDITIONS_LIVE_2026-10-06.md` §6); rounds 4+5 (big) rolled **0 combats**,
    so behaviour >503 pts is still unmeasured. Next: fire more big arms to catch a high-point fight.
1c. **Arsenal upgrades (Go model)** — catalog + drop rules captured in
    `docs/ARSENAL_LIVE_2026-10-06.md`. Model once expedition resolver exists; needs the fleet-point
    threshold semantics (doc says 75k; user believes 5k/50k/250k tiers) confirmed live — round 5 is
    the first live probe. Hostail `cmd=2` is the only tier-targeted route but is the ghost-fleet path.
2. **Incoming-fleet view** (transport/attack/espionage) — mission text+colour per planet so online
   defenders see/react before arrival (currently only espionage is visible via its reports endpoint).
3. **Auto-builder base (Go)** — blueprint per planet + account research; design in `docs/AUTO_BUILD_DESIGN.md`.
4. **Moons** — acc1 `3:125:12` now HAS a moon (live, spawn confirmed; diameter <10 000).
   Rules to model: **≥10 000 km = indestructible**; below that destroyable by Deathstar.
   Scope: moonbase, moon buildings (sensor phalanx/jump gate), creation (debris) + destruction.
5. **TOTP 2FA** (auth 2nd factor on `internal/auth`).
6. **Full game-loop integration test** — register→login→colonize→build→research→shipyard→dispatch→
   battle→report→recycle→abandon.

## Done (newest first)
- acc2 rolling farm live (build+send+harvest) + Online-Bonus daemon `run-bonus.sh` (untracked)
- expedition live capture + tooling (`httpbot` `expedition`/`exp-state`/`exp-log`); docs/EXPEDITIONS_LIVE_2026-10-06
- espionage: full report + counter-espionage + reports routes — `9074c93`
- espionage live matrix doc; explorer fleet step-2 mission fix — `2c19386`, `602a324`
- colony caps robot 21 / nanite 10 / mines 44/42/39 — `b69fdd2`
- relocation (teleport) + DM field expansion + attack lockout — `2d15674`, `97ca89f`
- combat engine, missions, auth, planets, combat reports — `9fa5787`
- (older history: `git log --oneline`)

## Deep dives (read on demand)
`docs/ROLLING_FARM.md` · `docs/EXPLORER.md` · `docs/BALANCE_DATA_NEEDED.md` · `docs/COMBAT_FINDINGS.md` / `COMBAT_MODEL.md` /
`COMBAT_TEST_PLAN.md` · `docs/ESPIONAGE_LIVE_2026-10-06.md` · `docs/EXPEDITIONS_LIVE_2026-10-06.md` ·
`docs/ARSENAL_LIVE_2026-10-06.md` · `docs/AUTO_BUILD_DESIGN.md` · `docs/BACKUP.md`

## Open questions / blockers
- Counter-espionage exact formula unknown; using approximation + ships-only detection.
- Moon spawn chance: owner testing manually (deferred).
- Defense-vs-ships detection not isolated live (targets always had ships).

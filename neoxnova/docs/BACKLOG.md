# BACKLOG / status — read first; update at task end; keep < 80 lines

## Live state
- Stack: `docker compose`; run `go`/`make` from `neoxnova/`. **Deploy = local commit → push →
  VM `ssh -F neoxnova/secrets/ssh/config azure-bot 'git -C ~/neoxnova reset --hard origin/main'`.**
- **Rolling farm ONLINE both accounts (2026-10-07).** `run-farm-build.sh` = planner+pooler (writes
  `plans/farm-<acc>-{main,site}.json`; pools colony ships → main once a colony holds
  ≥`FARM_POOL_MIN`=50k, so expo+pool movements stay under the fleet-slot cap). `run-farm-worker.sh
  <acc> <cp> main|site` = **one persistent worker per ship-building planet** (`httpbot worker`), all
  shipyards build **in parallel** (server allows concurrent sessions; session reused via
  `data/session-<user>.json`, relogin only when a request proves logout). acc1 = local detached bash;
  acc2 = VM tmux (`farm-acc2` + `farmw-acc2-{1598,1672,1673,1674}`, plus `farmsend-acc2`,`expharv-acc2`,`bonus-acc2`).
- **acc1 loop durability (2026-10-07):** the local build+send loops are detached bash children and
  **died ~13:01** (their parent shell closed) while workers/harvest/bonus survived, so no expeditions
  fired for ~2 h. Restarted with `run-farm.ps1 start`; `run-farm.ps1 status` + log mtimes are the check.
- **Ship building is SHIP-ONLY; mines/conveyors are handled MANUALLY** (removed from automation). Shipyard
  is a `Building: N per second` factory (`perSec` in `parse.mjs`); `resolve` sizes each order to
  `EXPLORER_UNIT_SECONDS`(90 s) of throughput and re-submits ~2.5 s after it completes (no poll gap).
  **Adaptive BB:** queue **zero BB while main HC < 5×BB** (HC is the bottleneck); resumes automatically.
- **Expo send (`run-farm-send.sh`, 30 s):** slots = the **LIVE `expeditionSlots`** (config is only a
  fallback; 2026-10-07: **acc1 = 9, acc2 = 9**), so every slot is kept busy. `active` = `/Expedition/`
  rows **not `/Hostail/`** (the `(A)`/`(R)` letter is NOT the ghost test — a real fleet flips A→R with
  `fleetID:null` on return). Fires `n = min(free, affordable)`. acc1 carries **6 permanent Hostail
  ghosts** — `used` double-counts them; ignore.
- **S growth = build-capacity-limited (2026-10-07 — replaced the full-rotation `have/slots` rule).**
  The loop runs `farm-plan sent --active N` every cycle: `farm-plan` sums the WHOLE fleet — main + all
  site files + the N in-flight fleets (reconstructed from the last N `expedition-runs.json` sends) —
  then `S = ⌊fleet/slots⌋` (capped by BB and 5·HC) and **ratchets** (`max(prev, cap)`), so combat
  losses are rebuilt toward the high-water rather than shrinking the plan. Build targets are
  `S · growth` (`growth`=3 in `farm-sites.json`) so the goal always sits ABOVE the current fleet and
  every shipyard builds flat out — throughput (all sites), not a target, is the only limit. The loop
  skips a cycle when the main `levels` refresh is stale (never sizes orders from stale counts).
- **Worker stall fix 2026-10-07 (colonies "not building"):** `httpbot worker` remembered an in-memory
  pending unit as `have+want` and only cleared it when `have` reached it. The pooler draining a colony
  (`BB+HC+BR ≥ FARM_POOL_MIN`) drops `have`, so the expectation was never met → that worker stalled
  forever ("queues busy"). Now the pending stores `{base,exp,until}` and is dropped when `have < base`
  (drained) or the ETA deadline passes, so the worker re-submits instead of deadlocking.
- **`httpbot trade <buy> <code:amt,...>`:** trader, value 1:2:4 (deut→crystal = 1:2), **250 DM/call →
  only BIG lump trades (billions)**.
- acc1 Bratwurst `3:125:12` (local; moon present); sites = main `1593` + `1655/1656/1657`
  (`3:125:9-11`) + `1690/1692/1693` (`3:124:9-11`). acc2 TheBob `2:188:16` (VM); sites = `1598` +
  `1672/1673/1674` (`2:188:10-11` + `2:187:9`).
- Set = `207:S,203:5S,219:round(S/250)` + 1 each `202/204/205/206` (no Spy Probe `210`; errors at slot 21).

## Open backlog (ordered; one item per session)
0. **EXPAND BUILD SITES — DONE both.** acc1 = 7 sites (main `1593` + `1655/1656/1657` `3:125:9-11`
   + `1690/1692/1693` `3:124:9-11`). acc2 = 6 sites (main `1598` + `1672/1673` `2:188:10-11` +
   `1674/1675/1676` `2:187:9-11`; no `2:188:9` owned). All shipyard 14–16.
1. **Expedition resolver (Go)** — model `MissionExpedition` in `internal/engine`: outcome roll, loot,
   points-scaled enemy. BLOCKERS: enemy formula unknown; `cmd=2` leaks ghost fleets (avoid). Notes:
   `docs/EXPEDITIONS_LIVE_2026-10-06.md`.
1b. **Expedition enemy formula** — behavior >503 pts still unmeasured (rounds 4+5 rolled 0 combats);
   next: fire more big arms. Table in `docs/EXPEDITIONS_LIVE_2026-10-06.md` §6.
1c. **Arsenal upgrades (Go model)** — catalog + drop rules in `docs/ARSENAL_LIVE_2026-10-06.md`.
   **Resolved 2026-10-07:** the documented 75k fleet-point gate is NOT what delivers our finds — 4
   upgrade drawings (Jet engine ×2, Light armor, Laser weapons) dropped from ~7.7k–10.9k-pt fleets,
   ~10 % of combat wins, all from combat encounters. Remaining: activation/catalog rules + whether
   drop tier scales with fleet points (needs medium/heavy drops to appear).
2. **Incoming-fleet view** (transport/attack/espionage) — mission text+colour per planet for online
   defenders (currently only espionage via its reports endpoint).
3. **Auto-builder base (Go)** — blueprint per planet + account research; `docs/AUTO_BUILD_DESIGN.md`.
4. **Moons** — acc1 `3:125:12` has a moon (live; <10 000 km). Rules: ≥10 000 = indestructible; else
   destructible by Deathstar. Scope: moonbase, moon buildings, creation (debris) + destruction.
5. **TOTP 2FA** (auth 2nd factor on `internal/auth`).
6. **Full game-loop integration test** — register→…→abandon.

## Done (newest first)
- 2026-10-07: acc2 expanded to 6 build sites (`1675/1676` `2:187:10-11`); wrote
  `docs/ARSENAL_UPGRADES_IMPLEMENTATION.md` (next-session brief for the Go Arsenal/Market model;
  tiers = 5k/50k/250k, live routes/forms captured).
- 2026-10-07: acc1 expanded to 7 build sites (added `1690/1692/1693` `3:124:9-11`); S growth reworked
  to be build-capacity-limited (whole fleet incl. in-flight, monotonic ratchet, `growth` headroom so
  shipyard throughput is the only limit).
- 2026-10-07: fixed colony build deadlock (worker pending-unit cleared on pool-drain/deadline) and made
  the send loop follow the live slot count (used all 9 acc2 slots); deployed to the VM and restarted
  acc2 send+workers.
- 2026-10-07: acc1 build+send restarted after a ~2 h outage (loops died with their parent shell);
  send-loop S-inflation/over-fire race fixed (`GROWN` guard + stale-`levels` skip); cross-account
  expedition/Arsenal snapshot — upgrades drop far below the doc's 75k gate (see `ARSENAL_LIVE`).
- Parallel per-planet workers + session reuse; rate-aware unit batching; adaptive BB gating; `trade`
  command; slot auto-detect; (A)/(R) ghost-test fix — `6f24227`..`1de7ae0`.
- Rolling farm live (build+send+harvest+bonus) on both accounts; BR-gate deadlock fixed — `eb79f39`.
- expedition live capture + tooling (`httpbot` `expedition`/`exp-state`/`exp-log`); espionage routes;
  relocation; combat engine; auth — see `git log --oneline`.

## Deep dives (read on demand)
`docs/ROLLING_FARM.md` · `docs/EXPLORER.md` · `docs/ARSENAL_UPGRADES_IMPLEMENTATION.md` ·
`docs/BALANCE_DATA_NEEDED.md` · `docs/COMBAT_FINDINGS.md` /
`COMBAT_MODEL.md` / `COMBAT_TEST_PLAN.md` · `docs/ESPIONAGE_LIVE_2026-10-06.md` ·
`docs/EXPEDITIONS_LIVE_2026-10-06.md` · `docs/ARSENAL_LIVE_2026-10-06.md` · `docs/AUTO_BUILD_DESIGN.md`.

## Open questions / blockers
- Counter-espionage exact formula unknown (approximation + ships-only detection).
- Moon spawn chance: owner testing manually (deferred).
- Crystal is the fleet bottleneck: mine imbalance (metal mine ≫ crystal) + HC's 5:1 demand; sites stay
  crystal-poor. Manual mine/trader work only — automation must NOT build mines.

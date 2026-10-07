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
- **Ship building is SHIP-ONLY; mines/conveyors are handled MANUALLY** (removed from automation). Shipyard
  is a `Building: N per second` factory (`perSec` in `parse.mjs`); `resolve` sizes each order to
  `EXPLORER_UNIT_SECONDS`(90 s) of throughput and re-submits ~2.5 s after it completes (no poll gap).
  **Adaptive BB:** queue **zero BB while main HC < 5×BB** (HC is the bottleneck); resumes automatically.
- **Expo send (`run-farm-send.sh`, 30 s):** `slots = min(cfg 8, expeditionSlots)` **auto-adapts**
  (**acc1 = 6 now** — temp bonus expired; acc2 = 8). `active` = `/Expedition/` rows **not `/Hostail/`**
  (the `(A)`/`(R)` letter is NOT the ghost test — a real fleet flips A→R with `fleetID:null` on return).
  Fires `n = min(free, affordable)`; `farm-plan sent` grows `S = max(42000, min(⌊BB/slots⌋, ⌊HC/(5·slots)⌋))`.
  acc1 carries **6 permanent Hostail ghosts** — `used` double-counts them; ignore.
- **`httpbot trade <buy> <code:amt,...>`:** trader, value 1:2:4 (deut→crystal = 1:2), **250 DM/call →
  only BIG lump trades (billions)**.
- acc1 Bratwurst `3:125:12` (local; moon present). acc2 TheBob `2:188:16` (VM).
- Set = `207:S,203:5S,219:round(S/250)` + 1 each `202/204/205/206` (no Spy Probe `210`; errors at slot 21).

## Open backlog (ordered; one item per session)
0. **EXPAND BUILD SITES (next session — biggest win).** Use every shipyard: acc1 should be **main +
   6 colonies** (`3:125:9-12` + `3:124:*`); acc2 **main + 5** (`2:188:9-11` + `2:187:9-11`). Do
   `httpbot.mjs planets` recon on both, fill `plans/farm-sites.json` `sites`, launch one
   `run-farm-worker.sh` per new site (mind shipyard level + resources). Watch total process count and
   request rate. Verify all shipyards build simultaneously.
1. **Expedition resolver (Go)** — model `MissionExpedition` in `internal/engine`: outcome roll, loot,
   points-scaled enemy. BLOCKERS: enemy formula unknown; `cmd=2` leaks ghost fleets (avoid). Notes:
   `docs/EXPEDITIONS_LIVE_2026-10-06.md`.
1b. **Expedition enemy formula** — behavior >503 pts still unmeasured (rounds 4+5 rolled 0 combats);
   next: fire more big arms. Table in `docs/EXPEDITIONS_LIVE_2026-10-06.md` §6.
1c. **Arsenal upgrades (Go model)** — catalog + drop rules in `docs/ARSENAL_LIVE_2026-10-06.md`; needs
   the fleet-point threshold semantics confirmed live (doc 75k vs user's 5k/50k/250k tiers).
2. **Incoming-fleet view** (transport/attack/espionage) — mission text+colour per planet for online
   defenders (currently only espionage via its reports endpoint).
3. **Auto-builder base (Go)** — blueprint per planet + account research; `docs/AUTO_BUILD_DESIGN.md`.
4. **Moons** — acc1 `3:125:12` has a moon (live; <10 000 km). Rules: ≥10 000 = indestructible; else
   destructible by Deathstar. Scope: moonbase, moon buildings, creation (debris) + destruction.
5. **TOTP 2FA** (auth 2nd factor on `internal/auth`).
6. **Full game-loop integration test** — register→…→abandon.

## Done (newest first)
- Parallel per-planet workers + session reuse; rate-aware unit batching; adaptive BB gating; `trade`
  command; slot auto-detect; (A)/(R) ghost-test fix — `6f24227`..`1de7ae0`.
- Rolling farm live (build+send+harvest+bonus) on both accounts; BR-gate deadlock fixed — `eb79f39`.
- expedition live capture + tooling (`httpbot` `expedition`/`exp-state`/`exp-log`); espionage routes;
  relocation; combat engine; auth — see `git log --oneline`.

## Deep dives (read on demand)
`docs/ROLLING_FARM.md` · `docs/EXPLORER.md` · `docs/BALANCE_DATA_NEEDED.md` · `docs/COMBAT_FINDINGS.md` /
`COMBAT_MODEL.md` / `COMBAT_TEST_PLAN.md` · `docs/ESPIONAGE_LIVE_2026-10-06.md` ·
`docs/EXPEDITIONS_LIVE_2026-10-06.md` · `docs/ARSENAL_LIVE_2026-10-06.md` · `docs/AUTO_BUILD_DESIGN.md`.

## Open questions / blockers
- Counter-espionage exact formula unknown (approximation + ships-only detection).
- Moon spawn chance: owner testing manually (deferred).
- Crystal is the fleet bottleneck: mine imbalance (metal mine ≫ crystal) + HC's 5:1 demand; sites stay
  crystal-poor. Manual mine/trader work only — automation must NOT build mines.

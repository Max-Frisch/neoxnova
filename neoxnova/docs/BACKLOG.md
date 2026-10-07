# BACKLOG / status — read first; update at task end; keep < 80 lines

## Live state
- Stack: `docker compose`; run `go`/`make` from `neoxnova/`. **Deploy = local commit → push →
  VM `ssh -F neoxnova/secrets/ssh/config azure-bot 'git -C ~/neoxnova reset --hard origin/main'`.**
- **Rolling farm ONLINE both accounts (2026-10-07).** `run-farm-build.sh` = planner+pooler (writes
  `plans/farm-<acc>-{main,site}.json`; pools colony ships → main once a colony holds
  ≥`FARM_POOL_MIN`=50k). `run-farm-worker.sh <acc> <cp> main|site` = one persistent worker per
  ship-building planet (all shipyards build in parallel; session reused via `data/session-<user>.json`,
  relogin only when a request proves logout). acc1 = local detached bash; acc2 = VM tmux (`farm-acc2` +
  `farmw-acc2-{1598,1672,1673,1674}`, plus `farmsend-acc2`,`expharv-acc2`,`bonus-acc2`).
- acc1 local build+send loops are detached bash children and once died with their parent shell (~2 h
  outage, 2026-10-07); restart with `run-farm.ps1 start`, check `run-farm.ps1 status` + log mtimes.
- **Ship building is SHIP-ONLY; mines/conveyors are MANUAL.** Shipyard = `Building: N per second`
  (`perSec` in `parse.mjs`); `resolve` sizes each order to `EXPLORER_UNIT_SECONDS`(90 s) and re-submits
  ~2.5 s after completion. **Adaptive BB:** zero BB while main HC < 5×BB.
- **Expo send (`run-farm-send.sh`, 30 s):** slots = LIVE `expeditionSlots` (config fallback; acc1/acc2
  = 9). `active` = `/Expedition/` rows **not `/Hostail/`** (a real fleet flips A→R with `fleetID:null`
  on return). acc1 carries **6 permanent Hostail ghosts** — `used` double-counts them; ignore.
- **S growth = build-capacity-limited:** `farm-plan sent --active N` sums the WHOLE fleet (main + all
  site files + N in-flight), then `S = ⌊fleet/slots⌋` (capped by BB and 5·HC), **ratcheted** so losses
  rebuild toward the high-water. Build targets = `S · growth` (`growth`=3) so throughput is the only
  limit. Loop skips a cycle when the main `levels` refresh is stale.
- **Worker stall fix:** pending unit stored `{base,exp,until}`, dropped when `have < base` (pool drain)
  or the ETA passes, so a drained colony re-submits instead of deadlocking.
- **`httpbot trade <buy> <code:amt,...>`:** value 1:2:4 (deut→crystal = 1:2), **250 DM/call → big lumps**.
- acc1 Bratwurst `3:125:12` (moon); sites = `1593` + `1655/1656/1657` (`3:125:9-11`) +
  `1690/1692/1693` (`3:124:9-11`). acc2 TheBob `2:188:16`; sites = `1598` + `1672/1673/1674`
  (`2:188:10-11` + `2:187:9`). All shipyard 14–16.
- Set = `207:S,203:5S,219:round(S/250)` + 1 each `202/204/205/206` (no Spy Probe `210`; slot 21 errors).

## Open backlog (ordered; one item per session)
0. **EXPAND BUILD SITES — DONE both** (acc1 7, acc2 6).
1. **Expedition resolver (Go)** — model `MissionExpedition` in `internal/engine`: outcome roll, loot,
   points-scaled enemy. BLOCKERS: enemy formula unknown; `cmd=2` leaks ghost fleets (avoid). Notes:
   `docs/EXPEDITIONS_LIVE_2026-10-06.md`.
1b. **Expedition enemy formula** — behavior >503 pts still unmeasured (rounds 4+5 rolled 0 combats);
   next: fire more big arms. Table in `docs/EXPEDITIONS_LIVE_2026-10-06.md` §6.
1c. **Arsenal upgrades (Go model)** — DONE 2026-10-07 (catalog, tiers 5k/50k/250k, activation rules,
   store, market, API, lot expiry, "Your Auctions"/remove, tests). Catalog re-verified against
   `docs/screenshots_arsenal/` (all 19 names/order/brackets match). REMAINING: (a) wire the
   ~10%-of-combat-win drop into the expedition resolver (item 1) via
   `game.DropPool`/`AddUpgradeItems`; (b) confirm tier gates the type
   pool vs the drop chance; (c) apply bonuses to production/combat; (d) `greid` keys for upgrades other
   than `combustion`; (e) `httpbot arsenal|market|activate|sell` tooling.
2. **Incoming-fleet view** (transport/attack/espionage) — mission text+colour per planet for online
   defenders (currently only espionage).
3. **Auto-builder base (Go)** — blueprint per planet + account research; `docs/AUTO_BUILD_DESIGN.md`.
4. **Moons** — acc1 `3:125:12` has a moon (live; <10 000 km). Rules: ≥10 000 = indestructible; else
   destructible by Deathstar. Scope: moonbase, moon buildings, creation (debris) + destruction.
5. **TOTP 2FA** (auth 2nd factor on `internal/auth`).
6. **Full game-loop integration test** — register→…→abandon.

## Done (newest first)
- 2026-10-07: **Arsenal/Market Go model** — `internal/game/arsenal.go` (19-upgrade catalog, tiers,
  activation), migration `0011_arsenal.sql`, `store.ArsenalStore` (activate/list/buy/remove/expire),
  `/api/v1/arsenal` + `/api/v1/market` handlers (incl. `market/mine`+`market/remove`), engine lot
  expiry, unit + DB integration tests; catalog confirmed against `docs/screenshots_arsenal/`.
- 2026-10-07: wrote `docs/ARSENAL_UPGRADES_IMPLEMENTATION.md`; acc2 expanded to 6 sites; acc1 to 7.
- 2026-10-07: S growth reworked to build-capacity-limited (whole fleet, monotonic ratchet, headroom).
- 2026-10-07: fixed colony build deadlock; send loop follows the live slot count; deployed to VM.
- 2026-10-07: acc1 build+send restarted after the ~2 h outage; fixed send-loop S-inflation race.
- Parallel per-planet workers + session reuse; rate-aware unit batching; adaptive BB gating; `trade`;
  slot auto-detect; (A)/(R) ghost-test fix — `6f24227`..`1de7ae0`. Rolling farm live — `eb79f39`.

## Deep dives (read on demand)
`docs/ROLLING_FARM.md` · `docs/EXPLORER.md` · `docs/ARSENAL_UPGRADES_IMPLEMENTATION.md` ·
`docs/BALANCE_DATA_NEEDED.md` · `docs/COMBAT_FINDINGS.md` / `COMBAT_MODEL.md` / `COMBAT_TEST_PLAN.md` ·
`docs/ESPIONAGE_LIVE_2026-10-06.md` · `docs/EXPEDITIONS_LIVE_2026-10-06.md` ·
`docs/ARSENAL_LIVE_2026-10-06.md` · `docs/AUTO_BUILD_DESIGN.md`.

## Open questions / blockers
- Counter-espionage exact formula unknown (approximation + ships-only detection).
- Moon spawn chance: owner testing manually (deferred).
- Crystal is the fleet bottleneck: mine imbalance (metal mine ≫ crystal) + HC's 5:1 demand; sites stay
  crystal-poor. Manual mine/trader work only — automation must NOT build mines.

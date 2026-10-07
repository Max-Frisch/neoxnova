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
  ~2.5 s after completion. **Symmetric ratio gate (account-wide):** the plan sums main + sites +
  in-flight (`st.active`) and builds ONLY the deficient type — HC-only while HC < 5·BB, BB-only while
  HC > 5·BB, both when balanced. (Was main-only and HC-biased, so HC ballooned while BB idled.)
  **Workers re-read the plan every resolve pass** (was once at startup → sites kept building a stale
  HC plan while `farm-plan` said `gate=BB`; restart or session-error was the only refresh).
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
   `game.DropPool`/`RollDrop`/`AddUpgradeItems`; (b) tier gates the type pool ONLY — DONE
   2026-10-07: exact light/medium/heavy sets in `arsenal.go` (`RollDrop` added; chance stays
   flat ~10%). ~~confirm tier gates the type pool vs the drop chance~~;
   (c) **apply bonuses to production/combat** — DONE 2026-10-07:
   `internal/game/unit_classes.go` (from live cards, fixture `testdata/niburus_unit_classes.json`,
   `httpbot card`/`cards`), weapon/armor/shield folded additively into the tech bonus
   (`DerivedStatBonus`), production 17/18/19 into `RecomputeProduction`, engine 11/12/13 into
   `FleetMaxSpeed`; resolver/`RecomputeCelestial`/`Dispatch` load `account_upgrades` and activation
   recomputes planets. OPEN: conveyor 14–16 — building 71/72/73 effect now measured
   (`docs/ARSENAL_UPGRADES_IMPLEMENTATION.md` §7: fleet `unitRate·L`, defense `·k(L)`,
   k=10+⌊(L+2)/4⌋); the upgrade folds in as a running additive percent on the total conveyor output
   (one Average +0.5%, ten = +5%) — semantics owner-confirmed 2026-10-07, only the live magnitude
   unverified (no items owned);
   (d) `greid` keys for upgrades other than `combustion`; (e) `httpbot arsenal|market|activate|sell`
   — DONE 2026-10-07 (`parseArsenalPage`/`parseMarketLots` + the four commands, dry unless `--go`).
1d. **Combat bonus model — per-weapon techs** — DONE 2026-10-07. `combat.go` `DerivedAttack`
   walks each unit's card weapon components: per component `base·(1+(weaponTech+arsenalWeapon)/100)`
   with `weaponTech` = 2%·Laser/Ion/Plasma (120/121/122), 4%·Graviton (199), 0 Standard; the sum is
   then `× (1+TechBonus(109)/100)`. Hull/shield keep arsenal armor/shield additive with 111/110.
   `CombatTechs` carries Laser/Ion/Plasma/Graviton, `loadCombatTechs` reads 120/121/122/199, seed
   includes them. Tests replay `real-big-acc1-acc2.report.json` (82/246/853/1538/3077) + the
   `unitStats.Attack == Σ base_w` invariant. Details: ARSENAL doc §8.
2. **Incoming-fleet view** (transport/attack/espionage) — DONE 2026-10-07:
   `GET /api/v1/planets/{id}/incoming-fleets` returns inbound OUTBOUND fleets with
   `mission_text`/`colour`/`hostile` (owner-checked, composition hidden); `game.MissionDisplayFor`
   maps missions to live labels. Colours are provisional pending a live incoming sample.
3. **Auto-builder base (Go)** — blueprint per planet + account research; `docs/AUTO_BUILD_DESIGN.md`.
4. **Moons** — acc1 `3:125:12` has a moon (live; <10 000 km). Rules: ≥10 000 = indestructible; else
   destructible by Deathstar. Scope: moonbase, moon buildings, creation (debris) + destruction.
5. **TOTP 2FA** (auth 2nd factor on `internal/auth`).
6. **Full game-loop integration test** — register→…→abandon.

## Done (newest first)
- 2026-10-07: **incoming-fleet view (item 2)** — `FleetStore.IncomingFleets` (owner-checked,
  OUTBOUND fleets targeting a celestial), `models.IncomingFleet`, `game.MissionDisplayFor`
  (live labels + colour + hostile flag), handler + route
  `GET /api/v1/planets/{id}/incoming-fleets`. Pure + DB-integration tests. Composition is not
  exposed; colours are provisional.
- 2026-10-07: **arsenal/market tooling (item 1c-e)** — `parseArsenalPage` + `parseMarketLots`
  in `parse.mjs`; `httpbot.mjs arsenal|market|activate <greid>|sell <type> <amt> <rate>` (the
  mutating two dry-run unless `--go`), verified offline against the live page captures.
- 2026-10-07: **arsenal drop tiers fixed** — `arsenal.go` light/medium/heavy pools now hold the
  owner-confirmed sets (light: 1,5,8,11,14,17,18,19; medium: 2,3,6,9,12,15; heavy: 4,7,10,13,16);
  `DropPool` is cumulative by tier; new `RollDrop` (flat ~10% chance, pick from the tier pool).
  Added a partition test + roll tests.
- 2026-10-07: **combat per-weapon techs (item 1d)** — `combat.go` `DerivedAttack` sums a unit's
  card weapon components (specific research + arsenal weapon additive per component; general
  Weapons tech 109 compounds the sum); `CombatTechs` gains Laser/Ion/Plasma/Graviton
  (120/121/122/199), `loadCombatTechs` + `CombatSeed` updated. New tests replay the live
  `real-big-acc1-acc2` report firepower/shield/hull and lock the `Σ base_w` invariant.
- 2026-10-07: **card + conveyor live findings** — `parseInfoCard` now captures every weapon
  component + bonus tooltips; fixture/`unit_classes.go` regenerated with `Weapons []WeaponClass`
  (HAR extractor added). Conveyor 71/72/73 throughput curve measured incl. Photon Cannon
  (`docs/ARSENAL_UPGRADES_IMPLEMENTATION.md` §7). Combat bonus formula re-derived from
  `data/combat/` reports → queued as item 1d / §8.
- 2026-10-07: **Arsenal upgrade bonuses applied** — per-unit classes extracted from the live info
  cards (202–228/401–419) into `internal/game/unit_classes.go` (fixture + `httpbot card`/`cards`);
  weapon/armor/shield added into the same percent sum as the tech bonus (`DerivedStatBonus`),
  production 17–19 into `RecomputeProduction`, engines 11–13 into `FleetMaxSpeed`; resolver +
  `RecomputeCelestial` + `Dispatch` read `account_upgrades`, activation recomputes planet production.
- 2026-10-07: **worker plan-refresh fix** — `cmdResolve` now re-reads the goals file each pass
  (kept old targets on a partial write) so long-lived workers follow ratio-gate flips; restarted all
  7 acc1 workers — sites switched from stale HC to `code 207` (BB).
- 2026-10-07: **farm ratio gate made symmetric + account-wide** (`farm-plan.mjs`): the plan now tallies
  main+sites+in-flight (`st.active`, persisted by the send loop) and builds only the deficient type, so
  the 5:1 HC:BB ratio converges instead of one type piling up idle; plan log shows `ratio`/`gate`.
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

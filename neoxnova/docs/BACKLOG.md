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
  ~2.5 s after completion. **Optional ratio gate (account-wide, only when `comp.wall` is set):** the plan
  sums main + sites + in-flight (`st.active`) and builds ONLY the deficient type. Currently OFF (wall-free meta).
  **Workers re-read the plan every resolve pass** (was once at startup → sites kept building a stale
  HC plan while `farm-plan` said `gate=BB`; restart or session-error was the only refresh).
- **Expo send (`run-farm-send.sh`, 30 s):** slots = LIVE `expeditionSlots` (config fallback; acc1/acc2
  = 9). `active` = `/Expedition/` rows **not `/Hostail/`** (a real fleet flips A→R with `fleetID:null`
  on return). acc1 carries **6 permanent Hostail ghosts** — `used` double-counts them; ignore.
- **S sizing = whole-fleet/slots (accurate, no ratchet):** `farm-plan sent --active N --inflight-ships M`
  sums the WHOLE fleet (main + all site files + the in-flight MAIN recovered from each airborne fleet's
  live `exp-state` ship count) and sets `S = ⌊fleet/slots⌋` so exactly `slots` fleets cover the account.
  The old run-log reconstruction over-counted (failed sends logged, `num`/stale S) and ratcheted S ~2x
  too high → half the slots idle; that ratchet is gone. Build targets = `S · growth` (`growth`=3).
  Sends are **one fleet per POST** (`exp_num>1` was unreliable). Loop skips a cycle when the main
  `levels` refresh is stale.
- **Worker stall fix:** pending unit stored `{base,exp,until}`, dropped when `have < base` (pool drain)
  or the ETA passes, so a drained colony re-submits instead of deadlocking.
- **`httpbot trade <buy> <code:amt,...>`:** value 1:2:4 (deut→crystal = 1:2), **250 DM/call → big lumps**.
- acc1 Bratwurst `3:125:12` (moon); sites = `1593` + `1655/1656/1657` (`3:125:9-11`) +
  `1690/1692/1693` (`3:124:9-11`). acc2 TheBob `2:188:16`; sites = `1598` + `1672/1673/1674/1675/1676`
  (`2:188:10-11` + `2:187:9-11`). **All shipyards upgraded to 18 (2026-10-08) → Frigate unlocked everywhere.**
- **Expo set = `227:S,219:br` + 1 each of every sub-Frigate ship `202/204/205/206/207/211/213/215/216/225/226`
  (no Spy Probe `210`; slot 21 errors). 2026-10-08: added the 7 heavies (207 BS, 211 PB, 213 SF, 215 BC,
  216 BM, 225 Galleon, 226 Destroyer) to inflate the expedition's fleet value/found scale — built on main
  only, one per fleet; the send gate now requires all 11 present.**
  Composition is config-driven in `plans/farm-sites.json` `comp` (`main`/`wall`/`cargo`/`recycler`/`ramp`);
  freighter/recycler ratios are **point-based** (`recyclerPoints`) so they survive a hull flip. `S` resets
  to capacity whenever `comp` changes; otherwise it ratchets. The optional wall ratio-gate is OFF.
- **Wall-free meta (2026-10-08):** the enemy mirrors our fleet ×~0.66 + a small template, so a fodder wall
  only absorbs our own alpha strike and inflates enemy HP — sim `cmd/exposim` shows `BB+5HC` loses 12.9%/fight
  vs **0.0%** wall-free (matches live ~1M HC/fight). Only Weapons/Shield/Armour research (109/110/111) is
  mirrored; specific weapon techs (120/121/122/199), arsenal, academy and governors are **attacker-only**.
- **Enemy W/S/A is ONE rolled value on all three stats, ~2.2× our 109 for aliens** (2026-10-08, report
  headers). Player shows three distinct values (acc1 +112/+85/+94); Pirates roll ~0.1–1.5× our 109 bonus,
  Aliens plateaud at **+202%** (acc1) / **+159%** (acc2). **Rule: keep 109/110/111 at the techtree floor**
  (Frigate 227 → 16/16/17; Battle Recycler 219 → 15/15/15) and invest in the non-mirrored bonuses, else the
  alien scales with us and its ~2.2× roll wipes the fleet (both accounts lost a full fleet ~1 min apart).
  Header now captured (`parseCombatReport.attackerInfo/defenderInfo` → `expedition-reports.json`); Go model
  `game.Combatant.FlatBonusPct`. Details `docs/EXPEDITIONS_LIVE_2026-10-06.md` §8.
- **Battleship → Frigate phase:** both accounts auto-flipped `main` to `227` once the Frigate fleet matched
  the BB fleet by points (`ramp.per`=690). Frigate is the best unlocked hull (crystal 0.25/pt vs HC 0.5,
  build tier 20/s ⇒ ~800M pts/s vs BB 7.7M). Recycler count is point-based (≈1 per fleet-S). BB/HC fleet is
  now surplus — **scrap via the black-market ship trader (50% loss) once no longer flying**.

## Open backlog (ordered; one item per session)
0. **EXPAND BUILD SITES — DONE both** (acc1 7, acc2 6).
1. **Expedition resolver (Go)** — model `MissionExpedition` in `internal/engine`: outcome roll, loot,
   enemy. Enemy formula now known (item 1b); remaining blocker: `cmd=2` leaks ghost fleets (avoid).
   Notes: `docs/EXPEDITIONS_LIVE_2026-10-06.md` §8.
1b. **Expedition enemy formula — DONE 2026-10-08.** Enemy = **mirror of the sent fleet × a single
    per-fleet roll ~0.6–0.9 (median 0.66) + a small random template** (LF/Cruiser/Star Fighter, tens
    to hundreds). Verified to S=814,397 BB (far past the old 503-pt ceiling); uniform across shared
    types ⇒ one roll per fleet. Small fleets sit on the template (old 2–40× ratios). **Enemy W/S/A is
    a second roll** — one value on all three stats, ~2.2× our mirrored 109 for aliens (see live-state
    bullet + §8). Data + tables: `docs/EXPEDITIONS_LIVE_2026-10-06.md` §8.
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
   maps missions to live labels. **Colours finalised 2026-10-08** from the live overview sample:
   hostility is now OWNERSHIP-based (any fleet not owned by the planet owner is hostile, incl.
   foreign espionage) and the palette is enemy attack red `#ff4d4d`, enemy spy orange `#ff9900`,
   own inbound green `#4caf50` (`MissionDisplayFor(mission, hostile)`).
3. **Auto-builder base (Go)** — blueprint per planet + account research; `docs/AUTO_BUILD_DESIGN.md`.
4. **Moons** — PARTIAL 2026-10-07. Live acc1 moon `cp=1725` @ `3:125:12`: diameter 8,426 km,
   created by combat at 20%, fields 62/63, Moon base 20. `game/moon.go` now holds the pure model:
   creation chance (`combat.MoonChance`), diameter, `MoonFieldsMax` (3/level; base 0; not classic
   `(d/1000)²`), `PhalanxRange` (L²−1), `JumpgateCooldown` (1h >>L), `MoonDestruction`
   (`(100−√S)·√D`) + Moon-base reduction (3%/2 levels), and the moon catalog (`moon_base` 41,
   `phalanx_sensor` 42, `jumpgate` 43 + moon-legal planet buildings). **Creation wired**
   (`resolveAttack` rolls `MoonCreation(res.MoonChance, seed)` on a player planet; inserts a MOON at
   the coords with `ON CONFLICT DO NOTHING`, recorded in the report). Details + open questions:
   `docs/MOONS.md`. REMAINING: `DESTROY_MOON` wiring (≥10,000 km immunity), Jumpgate jump (blocked —
   needs a 2nd **same-galaxy** moon; `mode=sendFleet` says "no other portal" with two L2 gates in
   galaxies 2 & 3), moon build/overview API + `resolveAttack` on a MOON target.
   **2nd moon acquired 2026-10-08**: acc1 now has `2:188:9` (diameter 8,544 km,
   5000 recyclers/attempt). **Phalanx DONE 2026-10-08**: works same-galaxy, reach `L²−1`, reveals
   any owner's fleet composition/ETA (`game.PhalanxInRange`, `parsePhalanx`, `httpbot phalanx`).
   **Debris question resolved**: only the creating battle's new debris drives the
   moon roll — a pre-existing field does NOT add to it (and diameter ignores the
   recycler count once the 20 % cap is hit; 8,544 = x=13). Details: `docs/MOONS.md`.
5. **TOTP 2FA** (auth 2nd factor on `internal/auth`).
6. **Full game-loop integration test** — register→…→abandon.

## Done (newest first)
- 2026-10-08: **moon #2 tooling + Phalanx live; Jumpgate blocked** — built Phalanx L2 + Jumpgate L1/L2
  on both acc1 moons (`cp=1772` 2:188:9, `cp=1725` 3:125:12). **Phalanx works**: `page=phalanx`
  (from galaxy view, sensor moon as current planet), range `L²−1`, **same-galaxy only**, reveals any
  owner's fleet composition/points/ETA. Added `game.PhalanxInRange` (+test), `parsePhalanx`,
  `httpbot phalanx <g:s:p> [1|3] --cp`. **Jumpgate blocked**: `page=information&mode=sendFleet`
  returns "You dont have another portal" from both moons despite two L2 gates → almost certainly
  same-galaxy; needs a same-galaxy moon to confirm. Gotcha: the current planet is account-global and
  raced by the farm workers (set+act in one process). Details `docs/MOONS.md`.
- 2026-10-08: **weak-alien win analyzed** — `e219ad37` (10:48, msg 244369): same comp as the 10:05
  wipe but enemy W/S/A **+90 %** vs +202 % (and a *larger* mirror, 12,231 vs 11,861 Frig) → **won**,
  lost 4,963 Frig + 6,474 BR, enemy annihilated, debris M 264.2 B / C 89.8 B ⇒ ~+171 B net (1.94×).
  Confirms the research roll, not the fleet roll, decides the fight. `docs/EXPEDITIONS…` §8.
- 2026-10-08: **enemy W/S/A research decoded — single rolled value ~2.2× our 109 for aliens** — the
  combat-report header (now parsed: `parseCombatReport.attackerInfo/defenderInfo` → `expedition-reports.json`)
  shows the NPC with ONE Weapons/Shield/Armour value vs our three. Pirates ~0.1–1.5× our 109 bonus; Aliens
  +202 %/+159 %. Both accounts lost a full Frigate fleet ~1 min apart to a ~2.2× alien. Rule recorded:
  cap 109/110/111 at the techtree floor (227 → 16/16/17) and invest in non-mirrored bonuses. Go:
  `game.Combatant.FlatBonusPct` + `cmd/exposim` pirate/alien scenarios; test added. §8.
- 2026-10-08: **fresh moon #2 sampled + field model corrected** — acc1's 2nd moon is
  **`cp=1772`** at `2:188:9` (coexists with planet Xusyty `cp=1648`; not in `httpbot
  planets`). Snapshot (nothing built): all moon structures 0, **fields `0 used / 3 max`**,
  diameter 8,544 km. A fresh moon has **1 base field** + the account's `+2` premium = 3
  (so the first Moon base can be built at once); Moon base 20 = `1+60+2 = 63`. Fixed
  `MoonFieldsMax(L) = 1 + 3L` (+test), updated `docs/MOONS.md`; captures under
  `data/moon2-acc1-*`.
- 2026-10-08: **upgrade-find harvest (acc1 5, acc2 4)** — find events live in the
  expedition category (`messcat=15`), text *"…create a drawing for an upgrade NAME (2 pc)"*;
  every find = **2 pc**. Found incl. **Hyperspace engine on both** (acc1 msg 240880,
  acc2 msg 241669) and heavy Grav gun/Heavy armor — but from **"ancient battlefield"
  ship finds as well as combat**. Fleet points are ambiguous: model `(M+C)/1e6` puts the
  drops at 101k (acc1 BB) / 162k (acc2 Frigate) — **under 250k yet heavy**; the live PHP
  formula `(M+C)*5/1000` puts them at ~500–800M (tier 3). **`ArsenalTier` 5k/50k/250k is
  suspect** — see `docs/ARSENAL_LIVE_2026-10-06.md` for the table.
- 2026-10-08: **one of each sub-Frigate ship per expedition (both accounts live)** — `farm-plan.mjs`
  `small` set + `run-farm-send.sh` gate/`SET` now carry `202/204/205/206/207/211/213/215/216/225/226`
  (1/fleet; 207/215/216/225/226 already on main, 211/213 built by the worker). In-flight ship-count
  reconstruction now subtracts 11 smalls/fleet. Split fallback also held until the full set is on main
  (it bypassed `afford`). Both loops restarted live (acc1 local, acc2 VM tmux); 211/213 now built and
  the next send carries all 11.
- 2026-10-08: **2nd/3rd Frigate pirate wins net-positive** — acc1 msgs 241795 (06:53, return 241815)
  and 241820 (07:03, return 241833). Losses M+C 76.802 B + 6.410 B = **83.211 B** (1,820 Frigate +
  6,507 Battle Recycler; +4.941 B Deut); debris hauled back exactly equals the reported fields:
  **169.047 B** (M 125.567 / C 43.480). Academy Standardisation (Fleet) L13 = −13 % rebuild ⇒ 72.394 B,
  **+96.653 B net / 2.34×** (−12 %: +95.821 B / 2.31×).
- 2026-10-08: **recycler ratio cut + colony pooling unblocked** — `plans/farm-sites.json` gains
  `comp.recyclerPoints=80e6` (≈1 recycler per 2 Frigates; was clamped to 1:1) and `poolMin=5000`.
  `run-farm-build.sh` now re-reads `poolMin` each cycle; the old fixed 50 k sat ABOVE the per-site
  build target (~3.9·S ≈ 32 k), so colonies hoarded ships forever and the main planet starved —
  the real cause of 2 idle expo slots (`not enough ships for 1 fleet`). `run-farm-send.sh` also gained
  a split fallback: when a full-S fleet won't fit but the main holds ≥`FARM_MIN_FLEET` (1000), it
  divides that batch over the FREE slots and flies smaller fleets (`per`/`brf`, ratio preserved) so
  slots stop idling while the fleet is under capacity. Note: the in-flight reconstruction assumes
  the new ratio, so S is briefly ~1.3× high until the 1:1 fleets land.
- 2026-10-08: **acc1 Frigate pirate fight is net-positive** — report `01a73067` (08 Oct 05:33, msg 241594):
  Frigate+Recycler comp `227:7630 + 219:7630` won vs the mirrored enemy. Our M+C losses 53.82 B
  (1,143 Frigate + 5,065 Battle Recycler); the 9-min-later return (msg 241609) hauled the full 50 %
  debris field home — M 94.19 B + C 32.71 B = **126.90 B** (Deut 0). With Academy Standardisation
  (Fleet) L13 = −13 % ship cost, rebuilding the loss is 46.83 B ⇒ **~80 B surplus / 2.71× recovery**.
- 2026-10-08: **expedition slots no longer idle (both accounts)** — `farm-plan.mjs` reconstructed
  in-flight ships from `expedition-runs.json`, which over-counted (attempts logged even on failure,
  `num`/stale S), ratcheting S ~2x too high: acc1 S=12.8k vs real cap ~6.4k, acc2 S=7.4k vs ~5.5k, so
  only ~4/9 slots flew. The send loop now reads each airborne fleet's ship count from the live
  `exp-state` and the planner recovers the MAIN count from it, sizing `S = whole-fleet/slots` with no
  ratchet; sends are one fleet per POST (`exp_num>1` was unreliable) and explicitly `--cp <mainCp>`
  (the session's current planet drifts to a colony a worker touched, and the POST then silently
  no-ops). acc1 S≈7.7k (8/9 slots), acc2 S≈4.2k. `cmd/exposim` gained the live Frigate+recycler comps
  (1:1 = 0.0% median / 3.8% worst-case losses; recyclers are the loot cargo). No Frigate combat/
  black-hole yet.
- 2026-10-08: **wall-free Frigate expedition comp (live, both accounts)** — `cmd/exposim` (new) swept
  comps on the real combat engine: the mirrored enemy makes fodder a liability (`BB+5HC` 12.9%/fight vs
  0.0% wall-free). Added Frigate (227) cost to the catalog, made `farm-plan.mjs` composition-configurable
  (`comp{main,wall,cargo,recycler,ramp}`, point-based ratios, S reset on comp change, auto-flip main to
  the ramp hull), generalized the send/pool scripts, upgraded every shipyard to 18, and flipped both
  accounts to a Frigate mono-fleet (`227:S,219:br,1×small`). BB/HC fleet now surplus.
- 2026-10-08: **expedition enemy formula resolved (item 1b)** — overnight reports (acc1 40 fights,
  acc2 42) show the enemy is the sent fleet mirrored × a per-fleet roll ~0.6–0.9 (median 0.66) plus
  a small random template; verified to S=814,397 BB. Win record flipped at scale (acc1 29W/11D/0L,
  acc2 37W/5D/0L). Black holes are account-asymmetric (acc1 2/461, acc2 19/493). Details
  `docs/EXPEDITIONS_LIVE_2026-10-06.md` §8.
- 2026-10-07: **moon creation wired (item 4)** — `resolveAttack` rolls
  `game.MoonCreation(res.MoonChance, CombatSeed^salt)` when an ATTACK lands on a player-owned
  planet; a success inserts a `MOON` at the same coordinates (0 fields/production, inherited temp,
  `ON CONFLICT DO NOTHING`) and stamps `CombatResult.MoonCreated/MoonDiameterKm` into the report.
  `MoonCreation` + diameter-cap added and tested.
- 2026-10-07: **moon capture + catalog (item 4)** — captured acc1's moon live (`cp=1725`):
  building levels/costs, fields 62/63, and the `page=information` cards for 41/42/43.
  `game/moon.go` gained `MoonFieldsMax` (3 fields/level, base 0), `PhalanxRange` (L²−1),
  `JumpgateCooldown`, `MoonBaseDestructionReduction` (3%/2 levels) and the moon structure
  catalog (41/42/43 + moon-legal planet buildings). `docs/MOONS.md` updated.
- 2026-10-07: **moon formulas (item 4, partial)** — `internal/game/moon.go` +
  `docs/MOONS.md`: standard creation chance (`combat.go:MoonChance`), diameter
  `floor(√(x+3p)·1000)` (verified 8,426 km at x=11/p=20), classic fields, and
  destruction `(100-√S)·√D` with the ≥10,000 km immunity. Captured the acc1 moon
  (`cp=1725`, 8,426 km) and the server's field/building rules + open questions.
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
`docs/ARSENAL_LIVE_2026-10-06.md` · `docs/AUTO_BUILD_DESIGN.md` · `docs/MOONS.md`.

## Open questions / blockers
- Counter-espionage exact formula unknown (approximation + ships-only detection).
- Expedition black-hole rate is **account-asymmetric** (acc1 4, acc2 22 in the harvested logs); cause
  unknown (luck vs size/speed). The "Moa Tikarr demands surrender" text is just the **info message for
  a pirate/alien expedition combat**, paired 1:1 with a combat report for the same fleet/time (owner
  confirmed 2026-10-08), not a dropped report. See `docs/EXPEDITIONS_LIVE_2026-10-06.md` §8.
- **Aliens are a total-loss risk** even for a winning comp: the enemy's single rolled W/S/A can reach
  ~2.2× our 109 (observed +202 %) and wipes the fleet. The roll's exact distribution vs account research
  is still open (only ~2 high samples); the harvest now records every header, so more data is incoming.
- **Moon creation is modelled** (debris chance + standard diameter); open: exact moon-destruction
  numbers (Battle Fortress count vs diameter, add vs mult Moon-base reduction), and Jumpgate
  cooldown/eligibility (needs a 2nd moon). See `docs/MOONS.md`.
- Crystal is the fleet bottleneck: mine imbalance (metal mine ≫ crystal); sites stay crystal-poor. Frigate
  was chosen partly for the lowest crystal intensity (0.25/pt). The surplus BB/HC fleet is the fastest
  crystal source — **scrap it via the black-market ship trader (50 % loss) once no longer flying.** Manual
  mine/trader work only — automation must NOT build mines.

## Next session (pick one)
- **Scrap the surplus BB/HC fleet** (ship trader) for crystal to fund Frigates; watch cargo/recycler sizing
  (`recyclerPoints` in `plans/farm-sites.json`) and tune if loot caps.
- **Moons item 4 continuation**: moon build/overview API (Moon base → fields), or `DESTROY_MOON`.
- **Auto-builder base (item 3)** — `docs/AUTO_BUILD_DESIGN.md`.
- **Expedition resolver (item 1)** — enemy formula now known (item 1b); model it in Go.

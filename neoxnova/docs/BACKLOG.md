# BACKLOG — kanban (todo / in-progress / done)

Read first; keep < 80 lines. One item per session. Full session history (pre-slim) lives in
`docs/BACKLOG_ARCHIVE.md`. Deeper context in the per-topic docs (see "Deep dives" below).

## Live state (operational cheat-sheet)
- acc1 `3:125:12` runs entirely on VM `azure-bot` (tmux `drain-acc1` + `bonus-acc1`),
  no local processes. acc2 retired (manual-only fallback).
- Burn-down = lean **BW 228** (`drain.mjs` v7 `bwlean-split-v7`, config `plans/farm-sites.json >
  acc1.drain`), 11 live slots, `per = floor((home+inflight)·0.97/slots)`.
- Rules: keep 109/110/111 at the techtree floor; resources are never cron-updated; live automation
  builds **ships only** (mines + ship-trader scrap are manual).
- Engine deploy = local commit → push → VM `git -C ~/neoxnova reset --hard origin/main`.
- Live tooling/runbooks/secrets now live in a **separate private sibling ops repo** (not tracked
  here); ops deploy is `git -C ~/neoxnova-ops pull`. No client/automation code in this repo.

## In progress
- [ ] **Ops split follow-ups** — engine de-branded + live-ops extracted to the private sibling
      repo; engine changes committed+pushed (`fc08b83`). Remaining: migrate the acc1 VM to the ops
      checkout (rename `secrets/explorer.env` keys `NIBURU_*`→`EXPLORER_*`, restart tmux
      daemons); optional `filter-repo` history purge. Full steps: private ops repo `docs/PLAYBOOK.md`.

## Todo (ordered; pick one per session)
Playability fixes (first human test): plan `docs/PLAYABILITY_FIXES_PLAN.md` — **done through B4**.
- [ ] **No messages were generated** (owner report) — the owner saw an action produce no
      message/inbox entries this session. Needs repro details (which action) before tracing the
      `messages`/`user_messages` insert path.
- [ ] **Combat XP + combat level** — a distinct progression (like the peaceful level but fed by
      combat, **not** the Bonus button; the button's "Experience points" are Academy points). The
      live "Received N combat experience" lines and the `An experience X / Y` level bar are in the
      ops combat captures; derive the award formula from the paired battle reports and wire it into
      the combat resolver (the `users.combat_xp` column and overview display already exist).
- [ ] **Peaceful: premium-tagged capture + effects wiring** — S6 refit the online-time curve
      (weighted quadratic `37n²−57.3n+4076`) + DM payout and added the `PeacefulPremium` model
      (`game/peaceful.go`). Still need a deliberately premium-tagged run at 5/10/15/20/25/50 to firm
      the constants, plus wiring `StudiesPct` into research time and the factory units/sec bonuses
      into shipyard time (captures: `info/Bonus Button Peaceful`).
- [ ] **Senate: Officers + governator gameplay effects** — Officers roster/cost (HAR capture needed);
      wire Building/Research/Fleet/combat governator bonuses (catalog fields already present).
- [ ] **Auto-builder Phase 3/4** — energy-gate decision, auto-colonize (`AUTO_BUILD_DESIGN.md` §12/§15).
- [ ] **TOTP 2FA** — second auth factor (`internal/auth`).
- [ ] **Full game-loop integration test** — register → … → abandon.
- [ ] **acc1 ops** — scrap surplus BB/HC via ship trader; watch cargo/recycler sizing.
- [ ] **(low) Missiles/Interceptors** — `503` + `502` (`docs/MOONS.md`).

## Done (recent only)
- 2026-10-17 — **Academy (full feature)**: ported the captured 3-branch tree (27 skills,
  `internal/game/academy.go`) — names, tier, prerequisite gates, effect text and per-level point
  costs fitted to the captured (level, cost) anchors; combat effects now fold in from the skill map
  (`1101`/`1102`/`1211`/`1302` attack, `1301`/`1302` shield+armour, `1308` RF reduction; `1103`/`1109`
  procs as before); `1104` Standardisation (Fleet) discounts every ship's build cost, `1207` Packing
  of cargo scales every ship's hold, `1205` Academy of Sciences adds IRN-connected labs, and `1206`
  Expanding Empire extends the colony cap. Flights get `1105` (+3% speed all drives) and `1107`/`1112`
  (fuel reduction); economy gets `1201`/`1202` production and `1204` storage; `1203` Magistracy
  (-2% research time/level) feeds `TechDuration`; the colony cap (`game.MaxColonies`: homeworld +
  universe `base_colonies` + Astrophysics, hard-capped, + academy) is enforced in `resolveColonize`.
  Migration `0026` adds `users.academy_points`; the **Online Bonus button** grants Academy points as
  its "Experience points" (owner-verified: peaceful 77 → 34 with the premium "Button Bonus ×4", i.e.
  an 8.5 base; `game.AcademyPointsBase`) — it no longer (wrongly) fed `combat_xp`, which is a
  separate combat-fed progression. New `store.AcademyStore` (upgrade with prereq/partial, branch reset
  at the captured 100%/75%/50% Antimatter/DM/free tiers) and `/academy` page + menu link (was a
  disabled item). Tests: cost anchors, effect aggregation, flight/fuel/research/cost/cargo/colony
  wiring, prereq/affordable, Bonus payout, store lifecycle, render + e2e buy. Still text-only / TBD:
  donate skill, achievements→points, `1108`/`1110`/`1111` procs, `1110`+double-shield defence procs
  (`1303`/`1311`/`1304`–`1306`), `1209`/`1210`, `1106` fleet slots, and the `42-acad-stat-probe`
  defender-branch capture.
- 2026-10-17 — **S6 Peaceful calibration + S7 Arsenal greid keys**: from the new Bonus captures,
  `game/peaceful.go` re-fits the online-time curve to a weighted quadratic (`37n²−57.3n+4076`,
  within ~1.5% of the L3/4/5/76/77 durations) and the DM payout central fit (`base 7000` /
  `slope 630`), and adds an explicit `PeacefulPremium` model (`Button Bonus ×4`, appearance +100%,
  XP +100% → half the online time) wired through `BonusButtonRewardPremium`/`PeacefulLevelStatePremium`
  and `BonusButtonInterval`; the zero value keeps the old no-premium behaviour. `arsenal.go` adds the
  confirmed `greid` keys (`l_conveyer`/`m_conveyer`/`h_conveyer`/`m_production`) and documents the
  "Bonus = activations×PerLevel, independent of available items" rule. Tests: curve anchors, premium
  multiplier/half-time/interval, greid-key resolution; DB peaceful-store lifecycle still green.
- 2026-10-17 — **Empire Overview S4–S5** (`page=imperium`): new `PlanetStore.EmpireOverview` loads
  every owned celestial in creation (id) order with per-celestial accrued stock
  (`update_celestial_resources`), cached hourly production incl. the moon Collider's DM/h, fields and
  the building/ship/defense matrices (three grouped `= ANY(ids)` queries — no N+1). New account-wide
  `GET /empire` page (`internal/web/empire.{go,templ}`) renders the transposed matrix with a **Σ**
  total column, per-column `Go` links, the All/Buildings/Ships/Defenses client-side filters
  (`app.js` `wireEmpireFilters`) and stubbed bulk-action links; topbar `imperia.png` link. Tests:
  store integration (order/home/stock/matrices/DM/h), render (`TestEmpireBodyMatrix`/`EmptyState`),
  e2e route. Follow-ups: Planetarium/Sensor module/Research Center + M7/M19/M32 rows once their
  catalogs land, and real `reduceresources`/`reducefleet` routes.
- 2026-10-17 — **Moon Collider S1–S3**: `game/collider.go` (id 69, ATM cost `850·1.35^(L-1)`,
  DM/h lookup `[4752,10454,17250]` + provisional quadratic past L3, combat/premium multiplier
  hooks); migration `0025` adds `celestial_objects.collider_level`/`collider_dm_prod_hourly` and
  account Dark Matter accrual (`update_user_dark_matter` + `users.dark_matter_calc_at/_acc/_prod_hourly`);
  `MoonStore.ColliderStatusFor`/`UpgradeCollider` (moon-only, spends `users.antimatter`, banks DM at
  the old rate first); `RecomputeCelestial` caches the output and re-sums the account DM/h; new
  `GET/POST /collider/{moon}` page + moon-page nav link; DM ticks in the resbar/overview. The
  Collider is deliberately **not** in the resource-cost structure catalog (ATM-bought), so it never
  enters the building queue and occupies no field. Tests: game math, store integration, render.
  Follow-up: capture Collider L3–6 to pin the production curve.
- 2026-10-17 — **Capture triage + Collider/Empire playbook**: reviewed the new `info/{Moons,Empire
  view,Academy,Bonus Button Peaceful}` recordings. Moons = first Collider capture (moon-only, ATM
  cost `850·1.35^(L-1)`, DM/h table 4752/10454/17250 at L1–3); Empire = full `page=imperium` matrix;
  "Academy" is actually Arsenal/Hangar/Defenses (adds `greid` keys `h_conveyer`/`m_production`);
  Bonus adds real payouts + the premium multiplier list. Wrote `docs/COLLIDER_EMPIRE_PLAYBOOK.md`
  (specs + S1–S7 session split) and folded it into the todo/backlog.
- 2026-10-17 — **Fleet: stationary sats get a label, not a dead input**: the Solar Satellite row now
  shows a muted italic "stationary" in place of the dispatch input (no max button, dimmed row),
  keeping the sprite/name/count and column alignment while removing the non-interactive spinner.
  Server still rejects `ship_212` via `game.Movable`/`store.ErrStationaryShip`.
  `TestFleetBodyStationary` asserts no input/name for 212 and the label instead.
- 2026-10-17 — **Seed survives migrate-then-seed**: `0016_test_universe` used the auto id sequence
  for the sandbox celestials, so on a fresh DB it grabbed ids 1–2 before `0002_seed` ran and the
  seed's homeworld/outpost inserts were skipped by `ON CONFLICT`. `0016` now jumps the
  `celestial_objects` sequence to 1000 first, so sandbox celestials take high ids and the documented
  `make migrate` → `make seed` order yields id 1 `1:1:1` and id 2 `1:2:3` (idempotent; both orders
  verified). `TestAbandonPlanet` no longer hardcodes the homeworld owner id 2 (it reads the owner),
  which was order-dependent.
- 2026-10-17 — **Fleet dispatch selector reskin**: the ship picker is now a grouped table (Battle
  fleet / Transports / Special) with capture sprites, per-type counts, a per-ship **max** button
  and an **All ships** button (fills every movable input). Solar Satellites stay listed but are
  non-selectable (disabled input, no max); `game.Movable` + `store.ErrStationaryShip` also reject a
  crafted dispatch of stationary units. New `fleet-all-max` JS handler and `.fleet-table` CSS.
  Tests: `TestFleetBodyReskin`/`TestFleetBodyStationary`/`TestFleetClassCatalogComplete`,
  `TestMovable`, `TestDispatchStationaryShip`.
- 2026-10-17 — **Buildings/Research cost colouring**: each resource line on the buildings and
  research cards turns red (not bold) when the planet/moon's current stock cannot cover that
  component of the next level. `ResourceStock` is loaded from the existing overview, and
  `costMask` is folded into each panel signature so the 5s status poll also repaints as resources
  accrue. The build forms embed the stock (`data-res-*`) so `app.js` keeps the colouring when the
  target level changes. Shipyard/defenses keep the plain cost rendering.
- 2026-10-17 — **Expedition hold scales with fleet speed**: the hold window was a fixed wall-clock
  duration, so only the flight legs shrank with `universes.fleet_speed`; a 15x expedition was not
  ~3x shorter than at 5x. New pure `game.CalculateHoldDuration(hold, fleetSpeed)` compresses the
  player-selected hold by the same factor (floored at 1s), applied to EXPEDITION and HOLD in
  `FleetStore.Dispatch`, so a whole mission now scales as 1/fleet_speed. The expedition panel
  (`expedition.templ`/`expedition.js`) shows the real hold next to the nominal slider value. Tests:
  `TestHoldDurationScalesWithFleetSpeed`/`FloorsAndZero`, `TestExpeditionHoldLabel`, updated
  hold/e2e integration assertions.
- 2026-10-17 — **Online Bonus DM not shown**: the payout *was* credited (`ClaimBonusButton`
  updates `users.dark_matter`), but the resbar always read 0 because `GetResourceState`/`GetOverview`
  never selected `dark_matter`/`antimatter` (only `stardust`). Both queries now `LEFT JOIN users`
  and populate `RealtimeResourceState.DarkMatter/Antimatter`; regression assertion in
  `TestPeacefulStoreLifecycle`.
- 2026-10-16 — **Shipyard per-unit >= 0**: queueing a large, very fast batch (500 solar satellites)
  floored the total duration to 1s, so `build_time_per_unit` (NUMERIC(10,2)) rounded to 0 and tripped
  the `> 0` check (`23514`). The column is informational only (batches resolve at `end_time`), so
  migration `0024_shipyard_per_unit_zero.sql` relaxes it to `>= 0`; regression test
  `TestEnqueueShipyardLargeFastBatch`.
- 2026-10-16 — **Peaceful level + Online Bonus button (first pass)**: `game/peaceful.go` (bonuses per
  level, online-time→level curve, bonus payout — constants calibrated from the "Bonus Button Peaceful"
  captures, tunable); migration `0023_peaceful.sql` (`users.peaceful_seconds/_seen_at`,
  `bonus_button_ready_at/_last_click_at`); `store.PeacefulStore` (capped/throttled online-time accrual,
  status, `ClaimBonusButton` with row-lock + cooldown → `ErrBonusNotReady`); web `page` middleware
  accrues time, real `/bonus` page + menu link (was a placeholder), overview shows the peaceful level,
  and the peaceful mine/energy % folds into the cached + displayed production. Tests: game formulas,
  store lifecycle, render.
- 2026-10-16 — **Graviton Research energy gate**: no resource cost, but now needs the planet's
  gross energy production (`energy_max`) ≥ `100000·3^level` (300k, 900k, 2.7M …). New
  `game.TechEnergyRequirement` + `TechDef.EnergyReqBase/Factor`; `store.ErrEnergyRequirement`
  gates `EnqueueResearch` (partial chain stops) and `InstantTech`; auto-builder `nextResearch`
  skips it; `/research` card and DM technology market show required/produced with met/unmet.
  Tests: `TestTechEnergyRequirement`, `TestEnqueueResearchEnergyGate`, `TestResearchCardEnergyGate`.
- 2026-10-16 — **Senate: Governators (`page=gubernators`)**: catalog `internal/game/senate.go` with the
  exact live formulas (AP cost `round(base·factor^(i-1))` summed; DM/day `round(base·factor^level)`;
  bonus `= +10% +1%/level`); migration `0022_senate.sql` (`users.achievement_points` +
  `user_governators`); `store.SenateStore` (irreversible AP improve, timed DM hire, engine expiry
  sweep); `/senate` hub + `/senate/governators` with live improve/Days previews in `app.js`. Active
  Resource/Energy governators fold into `game.ProductionBonus` (new `Energy` term) and the cached
  production columns. The improve input raises the shown bonus % live and both improve (AP) and hire
  (DM) clamp to the last affordable amount (partial), no-op at zero. Tests: formulas + affordability,
  store lifecycle/partial/production, render.
- 2026-10-16 — **Shipyard/defenses: cumulative cost + partial builds + live panels**: unit cards embed
  per-unit cost/time and `app.js` scales by the typed quantity; `EnqueueShipyard` caps an oversized
  batch to `affordableQty` (`QueueResult.Quantity`, "Queued N of M × …"); shipyard/defenses htmx-poll
  their status like buildings/research and `resolveShipyard` recomputes on solar-satellite completion
  (`energy_max`). Tests: `TestAffordableQty`, partial-enqueue + satellite-energy integration, i3 markup.
- 2026-10-16 — **Live client countdowns + simpler backdrop**: `[data-countdown]` ticker in `app.js`
  (reuses the `#server-clock` skew, re-queried each second so htmx swaps are picked up; `hms`/`dur`
  formats) drives build/research/shipyard queue rows, fleet/expedition/incoming ETAs and the
  jumpgate recharge — all client-side, zero extra requests. Body background switched from the tiled
  yellow-grid `bg_a.png` to a pure-CSS space gradient.
- 2026-10-16 — **Playability: research-gated prereqs + live build panels**: buildings page merges
  research levels into prereq checks (Nanite Factory was stuck at "Computer Technology 0/10"); the
  buildings/research panels are htmx-polled (`GET /{buildings,research}/{id}/status`, 5 s, `panelSig`
  → `204` when unchanged) so a finished build/research refreshes the cards/queue in place — no full
  reload, no clobbering a half-typed target level. Tests: `TestPanelSig`, i2 render, e2e unlock + polls.
- 2026-10-15 — **B4 Merchant of Fleets + DM Buildings/Technology + Voucher**: `game.MerchantRefund`
  (refund = `merchant_rate`% of base cost, default 50) + `game.DMMarketPrice` (divisor 1000); store
  `ScrapUnits` + `InstantStructure`/`InstantTech`; knobs `config/game.json`→`universes` (`0021`).
  Pages `/blackmarket/{merchant,buildings,technology,voucher}` (voucher redeem TBD). Tests in archive.
- 2026-10-15 — **B1–B3 black market**: standalone `/blackmarket` hub (6 tiles; `/market` → Arsenal);
  Resource Trader (`game.TradeQuote` + `TradeStore.ExecuteTrade`, live htmx preview, 250 DM, 4/2/1);
  Planetarium (fields/diameter/teleport via `game.FieldExpansionCost`/`RelocationCost` +
  `store.RelocatePlanet`). Tests: render + quote + e2e.
- 2026-10-15 — **Q1–Q4 chained queues + target UI**: `0020` multi-slot schema; `Enqueue*Chain` to a
  target level (slot-cap/poverty partial); `CancelQueue` by id (refund + tail renumber); editable
  `target_level` input + embedded `data-cost-table` summed live by `static/app.js`; all scopes render
  the whole queue. **C1**: `config/game.json` → `universes` via `ApplyGameSettings` on boot.
- 2026-10-15 — **P1/P2 perf**: `/static/` off the per-IP limiter + ETag/Cache-Control/gzip/304 +
  server timeouts; session/owned celestials resolved once (`userPrincipal`); Overview no double-accrue;
  resbar 2s→5s.
- 2026-10-15 — **Moons (item 4)**, **Web UI reskin I0–I5**, **Expeditions X1–X4**, **Stardust**.
- 2026-10-11 and earlier — Arsenal/Black-market pages, tech-tree + Simulator, frontend balance,
  drain `bwlean-split-v7`, 7+ acc1 colonies. See `docs/BACKLOG_ARCHIVE.md`.

## Deep dives (read on demand)
`PLAYABILITY_FIXES_PLAN.md` · `ARSENAL_UPGRADES_IMPLEMENTATION.md` · `BALANCE_DATA_NEEDED.md` · `COMBAT_FINDINGS.md` /
`COMBAT_MODEL.md` · `EXPEDITIONS_CALIBRATION_PLAN.md` · `AUTO_BUILD_DESIGN.md` · `MOONS.md` ·
`COLLIDER_EMPIRE_PLAYBOOK.md`.
Live-capture runbooks, the explorer client and raw balance data live in the private ops repo.

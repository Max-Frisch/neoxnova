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
- [ ] **Peaceful level / Online Bonus calibration** — `game/peaceful.go` constants are initial
      best-fits from only 2 captured levels (3 & 76); need tooltips at 1/5/10/15/20/25/50 (no XP
      premium) + bonus-button clicks with known premium state to fit the level-time curve, the six
      step divisors and the DM/XP payout. Also apply `StudiesPct` to research time and factory
      bonuses, and add the premium multipliers (`Button "Bonus"`, `Update of "Bonus" button`).
- [ ] **Senate: Officers + governator gameplay effects** — Officers roster/cost (HAR capture needed);
      wire Building/Research/Fleet/combat governator bonuses (catalog fields already present).
- [ ] **Auto-builder Phase 3/4** — energy-gate decision, auto-colonize (`AUTO_BUILD_DESIGN.md` §12/§15).
- [ ] **TOTP 2FA** — second auth factor (`internal/auth`).
- [ ] **Full game-loop integration test** — register → … → abandon.
- [ ] **acc1 ops** — scrap surplus BB/HC via ship trader; watch cargo/recycler sizing.
- [ ] **(low) Missiles/Interceptors** — `503` + `502` (`docs/MOONS.md`).

## Done (recent only)
- 2026-10-17 — **Buildings/Research cost colouring**: each resource line on the buildings and
  research cards turns red (not bold) when the planet/moon's current stock cannot cover that
  component of the next level. `ResourceStock` is loaded from the existing overview, and
  `costMask` is folded into each panel signature so the 5s status poll also repaints as resources
  accrue. Shipyard/defenses keep the plain cost rendering.
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
`COMBAT_MODEL.md` · `EXPEDITIONS_CALIBRATION_PLAN.md` · `AUTO_BUILD_DESIGN.md` · `MOONS.md`.
Live-capture runbooks, the explorer client and raw balance data live in the private ops repo.

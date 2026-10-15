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
- [ ] **Auto-builder Phase 3/4** — energy-gate decision, auto-colonize (`AUTO_BUILD_DESIGN.md` §12/§15).
- [ ] **TOTP 2FA** — second auth factor (`internal/auth`).
- [ ] **Full game-loop integration test** — register → … → abandon.
- [ ] **acc1 ops** — scrap surplus BB/HC via ship trader; watch cargo/recycler sizing.
- [ ] **(low) Missiles/Interceptors** — `503` + `502` (`docs/MOONS.md`).

## Done (recent only)
- 2026-10-15 — **B4 Merchant of Fleets + DM Buildings/Technology + Voucher**: `game.MerchantRefund`
  (refund = `merchant_rate`% of base cost, default **50**, from `info/MoF`; owner-provided, not HAR) +
  `game.DMMarketPrice` (DM = `ceil(cost / dm_market_divisor)`, default **1000**); store `ScrapUnits` +
  `InstantStructure`/`InstantTech`; knobs in `config/game.json` → `universes` (`0021_blackmarket.sql`).
  Pages `/blackmarket/{merchant,buildings,technology,voucher}` (voucher = captured layout, redeem TBD);
  all six hub tiles backed. Tests: `TestMerchantRefund`/`TestDMMarketPrice`, `TestScrapUnitsMerchant`,
  `TestInstant*DMMarket`, migration + render + e2e. Next: playability plan complete.
- 2026-10-15 — **B3 Planetarium page**: `GET /blackmarket/planetarium` — buy fields with DM
  (`game.FieldExpansionCost`), grow diameter with Stardust + local debris, teleport
  (`store.RelocatePlanet`). New `models.CelestialDetail` + `store.ListCelestialDetails`. Tests:
  `TestPlanetarium*` + e2e (resets the relocation attack-lock in `e2eReset`).
- 2026-10-15 — **B2 Resource Trader**: live M:C:D value **1:2:4** (4/2/1 amount ratio) + fixed
  **250 DM** per trade, verified from the ops HAR. `game.TradeQuote` (pure) + `store.TradeStore.
  ExecuteTrade` (accrue/row-lock, debit sold + DM, credit bought clamped to cap); `/blackmarket/
  trader` sell/buy/amount page + htmx preview. Tests: `TestTradeQuote*`, `TestExecuteTrade`,
  `TestTraderBodyRenders`/`TestTraderQuotePartial`, e2e trader subtest.
- 2026-10-15 — **B1 standalone `/blackmarket` hub**: six-tile hub (Trader + Planetarium backed, four
  disabled); `GET /market` 303s to `/arsenal?tab=market` (Arsenal owns the drawing auctions), the
  `/market/{list,buy,remove}` POSTs stay. Tests: `TestBlackMarketHub`/`…TilesDecoupled`/`…ComingSoon`,
  `TestShellRendersReskin`, e2e hub+redirect.
- 2026-10-15 — **Q4 target-level UI + live cost/time + multi-row queues**: editable `target_level`
  input + embedded per-level `data-cost-table` (summed live by `static/app.js`); all three scopes
  render the whole queue (frozen cost, ETA, per-row cancel). `Busy` dropped. Tests: `Test*BodyReskin`
  rewritten + e2e pass `target_level`.
- 2026-10-15 — **Q3 cancel by id + renumber/refund**: `CancelQueue(ctx,scope,queueID,ownerID)`
  refunds one order and pulls the tail forward (same-code orders shift down a level, difference
  refunded). Tests: `TestCancelQueueRefunds*ByID`, e2e cancel passes `queue_id`.
- 2026-10-15 — **Q2 chained queues**: `EnqueueStructure`/`EnqueueResearch` chain to a target level up
  to the slot cap / first unaffordable (partial); `EnqueueShipyard` chains per kind. Read models
  return `[]QueueEntrySummary`; engine unchanged. Tests: `TestEnqueue*Chain`, `…SlotCap/PartialOnPoverty`.
- 2026-10-15 — **C1 game tuning config**: `config/game.json` → `internal/config.LoadGameSettings` →
  `store.ApplyGameSettings` upserts speeds/slots onto the `universes` row on boot; `GAME_CONFIG_PATH`
  overrides. Tests: `TestLoadGameSettings*`, `TestApplyGameSettings*`.
- 2026-10-15 — **Q1 queue schema**: `0020_queue_slots.sql` adds `universes.build_queue_slots`(10)/
  `research_queue_slots`(4), drops the single-active partial unique indexes. Tests:
  `TestQueueSlotsMigrationIdempotent`, `TestQueueAllowsMultipleActive`.
- 2026-10-15 — **P1/P2 perf**: `/static/` outside the per-IP limiter + ETag/Cache-Control/`304`/gzip
  + server timeouts; session/owned celestials resolved once per request (`userPrincipal`), Overview
  no longer double-accrues, resbar poll 2s→5s. Tests: `TestStaticAssets*`, `TestResbar*`, `TestUserID*`.
- 2026-10-15 — **Moons (item 4)** (build/DESTROY_MOON/Jumpgate/Phalanx), **Web UI reskin I0–I5**,
  **Expeditions X1–X4**, **Stardust diameter sink**. Full notes: `docs/BACKLOG_ARCHIVE.md`.
- 2026-10-11 and earlier — Arsenal/Black-market pages, tech-tree + Simulator, frontend balance,
  drain `bwlean-split-v7`, 7+ acc1 colonies. See `docs/BACKLOG_ARCHIVE.md`.

## Deep dives (read on demand)
`PLAYABILITY_FIXES_PLAN.md` · `ARSENAL_UPGRADES_IMPLEMENTATION.md` · `BALANCE_DATA_NEEDED.md` · `COMBAT_FINDINGS.md` /
`COMBAT_MODEL.md` · `EXPEDITIONS_CALIBRATION_PLAN.md` · `AUTO_BUILD_DESIGN.md` · `MOONS.md`.
Live-capture runbooks, the explorer client and raw balance data live in the private ops repo.

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
Playability fixes (first human test): plan `docs/PLAYABILITY_FIXES_PLAN.md` — each row = one session.
- [ ] **Q4** target-level input + live cost/time (embedded JSON) + multi-row queue UI.
- [ ] **B1** `/blackmarket` hub (separate from Arsenal `/market`); stub unbuilt categories.
- [ ] **B2** Resource Trader (250 DM; 4/2/1 + fees) — capture rules from ops HAR.
- [ ] **B3** Planetarium page (fields/diameter/teleport UI over existing JSON endpoints).
- [ ] **B4 future** Merchant of Fleets / Buildings & Technology Market / Voucher.
- [ ] **Auto-builder Phase 3/4** — energy-gate decision, auto-colonize (`AUTO_BUILD_DESIGN.md` §12/§15).
- [ ] **TOTP 2FA** — second auth factor (`internal/auth`).
- [ ] **Full game-loop integration test** — register → … → abandon.
- [ ] **acc1 ops** — scrap surplus BB/HC via ship trader; watch cargo/recycler sizing.
- [ ] **(low) Missiles/Interceptors** — `503` + `502` (`docs/MOONS.md`).

## Done (recent only)
- 2026-10-15 — **Q3 cancel by id + renumber/refund**: `CancelQueue(ctx, scope, queueID, ownerID)`
  cancels one specific queued order (owner-checked, `FOR UPDATE`, scheduler-race safe) and refunds
  its frozen cost; the tail is pulled forward and later orders of the same code/tech shift down one
  level, the level-cost difference refunded (`renumberConstruction/Research/ShipyardTx`). Handlers
  parse a hidden `queue_id` form field (`queueRow` renders it). Tests: `TestCancelQueueRefunds*ByID`
  (middle-order renumber + refunds), e2e cancel subtests pass a `queue_id`. Next: Q4.
- 2026-10-15 — **Q2 chained queues**: `EnqueueStructure`/`EnqueueResearch` take a target level and
  chain orders (`start = MAX(end_time)`, per-order target = current+queued+1) up to the scope's
  slot cap or the first unaffordable level (partial); `EnqueueShipyard` chains per kind
  (ships/defenses separate, unlimited). `GetBuildings`/`GetResearch`/new `GetShipyardQueue` return
  `[]QueueEntrySummary`; web renders the head until Q4. `BlueprintStore.LoadState` projects queued
  orders and marks a scope busy only at its cap. Engine unchanged. Tests: `TestEnqueue*Chain`,
  `TestEnqueueStructureSlotCap/PartialOnPoverty`, `TestEnqueueShipyardChainsByKind`,
  `TestResolveConstructionChain`. Next: Q3.
- 2026-10-15 — **C1 game tuning config**: `config/game.json` (live-calibrated 4000/10000/15,
  build 10 / research 4) parsed by `internal/config.LoadGameSettings` (built-in defaults when
  absent, unknown keys rejected) and upserted onto the `universes` row for `UNIVERSE_ID` on boot
  via `store.ApplyGameSettings` (`cmd/server/main.go`); `GAME_CONFIG_PATH` overrides the path.
  No game-math change (`fleet_speed` already divides flight time; ships/expo round-trip scale,
  hold is player-set). Tests: `TestLoadGameSettings*`, `TestApplyGameSettings*`. Next: Q2.
- 2026-10-15 — **Q1 queue schema**: migration `0020_queue_slots.sql` adds
  `universes.build_queue_slots` (10)/`research_queue_slots` (4) and drops the three
  single-active partial unique indexes (`uq_active_construction/_research/_shipyard`) so all
  three scopes take multi-row chains. Tests: `TestQueueSlotsMigrationIdempotent`,
  `TestQueueAllowsMultipleActive`. Next: C1.
- 2026-10-15 — **P2 perf**: session + owned celestials resolved once per request via a
  `page`-middleware `userPrincipal` in context (was 2–3 `SessionUser`/ownership queries per
  request); `shell()` reuses the principal (drops `GetCoordinates`/`HomeworldID`/`ownsCelestial`
  queries); Overview no longer double-runs `update_celestial_resources`; resbar poll 2s→5s
  (`internal/web/web.go`, `data.go`, `layout.templ`). Tests: `TestResbarPollsEveryFiveSeconds`,
  `TestUserIDReadsPrincipal`, `TestOwnsCelestialReadsPrincipal`.
- 2026-10-15 — **P1 perf**: `/static/` served outside the per-IP rate limiter (was throttling
  page loads to 40 req/s → the "56k" feel) + ETag/Cache-Control/`304`/gzip (`internal/web/static.go`)
  + server Read/Write/Idle timeouts. Tests: `TestStaticAssetsCacheable`,
  `TestStaticAssetsBypassGlobalRateLimit`.
- 2026-10-15 — **Moons (item 4)** (build/DESTROY_MOON/Jumpgate/Phalanx), **Web UI reskin I0–I5**
  (live-parity: shell, buildings/research queue+cancel, hangar/defenses/fleet, galaxy/tech/resources,
  arsenal/market/messages), **Expeditions X1–X4**, **Stardust diameter sink**. Full notes:
  `docs/BACKLOG_ARCHIVE.md`.
- 2026-10-11 and earlier — Arsenal/Black-market pages, tech-tree + Simulator, frontend balance,
  drain `bwlean-split-v7`, 7+ acc1 colonies. See `docs/BACKLOG_ARCHIVE.md`.

## Deep dives (read on demand)
`PLAYABILITY_FIXES_PLAN.md` · `ARSENAL_UPGRADES_IMPLEMENTATION.md` · `BALANCE_DATA_NEEDED.md` · `COMBAT_FINDINGS.md` /
`COMBAT_MODEL.md` · `EXPEDITIONS_CALIBRATION_PLAN.md` · `AUTO_BUILD_DESIGN.md` · `MOONS.md`.
Live-capture runbooks, the explorer client and raw balance data live in the private ops repo.

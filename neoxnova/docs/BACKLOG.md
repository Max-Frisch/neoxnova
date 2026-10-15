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
- [ ] **B2** Resource Trader (250 DM; 4/2/1 + fees) — capture rules from ops HAR.
- [ ] **B3** Planetarium page (fields/diameter/teleport UI over existing JSON endpoints).
- [ ] **B4 future** Merchant of Fleets / Buildings & Technology Market / Voucher.
- [ ] **Auto-builder Phase 3/4** — energy-gate decision, auto-colonize (`AUTO_BUILD_DESIGN.md` §12/§15).
- [ ] **TOTP 2FA** — second auth factor (`internal/auth`).
- [ ] **Full game-loop integration test** — register → … → abandon.
- [ ] **acc1 ops** — scrap surplus BB/HC via ship trader; watch cargo/recycler sizing.
- [ ] **(low) Missiles/Interceptors** — `503` + `502` (`docs/MOONS.md`).

## Done (recent only)
- 2026-10-15 — **B1 standalone `/blackmarket` hub**: `GET /blackmarket` renders the six-tile hub
  (Resource Trader / Merchant of Fleets / Planetarium / Buildings Market / Technology Market /
  Voucher System); the two backed sub-routes (`/blackmarket/trader`, `/blackmarket/planetarium`)
  render coming-soon placeholders and the rest are disabled tiles. `GET /market` now 303s to
  `/arsenal?tab=market` (Arsenal owns the drawing auctions); the `/market/{list,buy,remove}` POSTs
  stay and default back to the Arsenal market tab. Nav "Black market" → `/blackmarket`. Tests:
  `TestBlackMarketHub`, `TestBlackMarketTilesDecoupled`, `TestBlackMarketComingSoon`,
  `TestShellRendersReskin`/`TestMenuActiveAliases`, e2e hub+redirect. Next: B2.
- 2026-10-15 — **Q4 target-level UI + live cost/time + multi-row queues**: build/research cards got
  an editable named `target_level` input (default = next unqueued level) and an embedded per-level
  cost/time table (`data-cost-table`, depth 20, from `internal/game`); `static/app.js` sums it live
  into the card total. All three scopes render the **whole** queue (numbered rows, frozen cost
  `QueueEntrySummary.Cost` computed in read models, ETA, per-row cancel). Handlers parse
  `target_level`, flash "…to level N (partial)". `Busy` dropped (chaining allows re-queue). Tests:
  `Test*BodyReskin` rewritten (editable input, JSON table, 2 rows) + e2e pass `target_level`. Next: B1.
- 2026-10-15 — **Q3 cancel by id + renumber/refund**: `CancelQueue(ctx, scope, queueID, ownerID)`
  cancels one order (owner-checked, `FOR UPDATE`, race-safe) and refunds its frozen cost; the tail is
  pulled forward and later same-code orders shift down a level (difference refunded). Tests:
  `TestCancelQueueRefunds*ByID`, e2e cancel passes `queue_id`. Next: Q4.
- 2026-10-15 — **Q2 chained queues**: `EnqueueStructure`/`EnqueueResearch` take a target level and
  chain orders up to the slot cap or first unaffordable level (partial); `EnqueueShipyard` chains per
  kind (unlimited). Read models return `[]QueueEntrySummary`; engine unchanged. Tests:
  `TestEnqueue*Chain`, `TestEnqueueStructureSlotCap/PartialOnPoverty`, `TestResolveConstructionChain`.
  Next: Q3.
- 2026-10-15 — **C1 game tuning config**: `config/game.json` parsed by
  `internal/config.LoadGameSettings` and upserted onto the `universes` row on boot
  (`store.ApplyGameSettings`); `GAME_CONFIG_PATH` overrides. Tests: `TestLoadGameSettings*`,
  `TestApplyGameSettings*`. Next: Q2.
- 2026-10-15 — **Q1 queue schema**: `0020_queue_slots.sql` adds `universes.build_queue_slots`(10)/
  `research_queue_slots`(4) and drops the single-active partial unique indexes. Tests:
  `TestQueueSlotsMigrationIdempotent`, `TestQueueAllowsMultipleActive`. Next: C1.
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

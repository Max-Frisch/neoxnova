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
- [ ] **Playability: live shipyard/defenses panels** — same htmx status-poll as buildings/research.
- [ ] **Auto-builder Phase 3/4** — energy-gate decision, auto-colonize (`AUTO_BUILD_DESIGN.md` §12/§15).
- [ ] **TOTP 2FA** — second auth factor (`internal/auth`).
- [ ] **Full game-loop integration test** — register → … → abandon.
- [ ] **acc1 ops** — scrap surplus BB/HC via ship trader; watch cargo/recycler sizing.
- [ ] **(low) Missiles/Interceptors** — `503` + `502` (`docs/MOONS.md`).

## Done (recent only)
- 2026-10-16 — **Hangar/defenses live panels + solar-satellite energy**: shipyard/defenses now
  htmx-poll like buildings/research (`GET /{shipyard,defenses}/{id}/status`, `ShipyardData.Sig` → `204`
  when unchanged) so a finished batch clears its queue row and bumps the dock count in place.
  `resolveShipyard` recomputes the celestial (accrue → station → `RecomputeCelestial`) when the
  completed unit is a solar satellite (`212`), so cached `energy_max` and the resource bar update
  immediately instead of waiting for the next building completion. Tests: engine satellite-energy +
  e2e shipyard/defenses status polls.
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

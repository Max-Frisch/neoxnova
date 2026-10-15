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
- [ ] **Auto-builder Phase 3/4** — UI (no client in repo), energy-gate decision, auto-colonize
      (`docs/AUTO_BUILD_DESIGN.md` §12/§15).
- [ ] **TOTP 2FA** — second auth factor (`internal/auth`).
- [ ] **Full game-loop integration test** — register → … → abandon.
- [ ] **acc1 ops** — scrap the surplus BB/HC fleet via the ship trader for crystal; watch
      cargo/recycler sizing. Crystal remains the fleet bottleneck.
- [ ] **(low) Missiles/Interceptors** — `503 Interplanetary Missiles` + `502 Interceptor`:
      launch IPMs to knock down a target planet/moon's defenses; Interceptors shoot the IPMs
      down. Old sub-system — deferred (`docs/MOONS.md`).

## Done (recent only)
- 2026-10-15 — **Moons (item 4)**: moon buildings buildable (`EnqueueStructure` + field cap
      `1+3·Moon base`) + page; attack-on-moon verified; `DESTROY_MOON` wired (classic curve, ≥10,000 km
      immune); Jumpgate jump API+page (ships only, no deuterium, ×3/4 cooldown, L2/L6 fit); Phalanx
      scan API+page with 10-min post-teleport offline (migration `0019`). game+store+engine+web tests.
      Deferred: `ops client jump` (private repo), missiles/Interceptors (`docs/MOONS.md`).
- 2026-10-15 — **Web UI reskin I5 (arsenal/market/simulator/messages)**: Arsenal is now tabbed
      (Arsenal / Market / Sell Upgrades) over the arsenal+market store — upgrade card boxes with
      live icons, buy/withdraw with `return`-aware redirects; Black market + Simulator restyled
      onto skin panels/tables; `/reports/{id}` became a categorized **Messages** grid
      (Spy/Combat/Expedition/All backed with real counts + `?category=` filter, the rest disabled);
      favicon + responsive pass; `i5_test.go`; fixed the stale I4 e2e tech-tree assertion. Closes I0–I5.
- 2026-10-15 — **Web UI reskin I4 (galaxy/tech/resources)**: Galaxy restyled (galaxy+system
      steppers, Saved/View, slot table with moon/debris/player/action icons → fleet/spy/colonize/
      recycle dispatch links with mission preselect, footer stats + omission note); Technologies
      is now tabbed (Buildings/Research/Fleet/Planetary Defense/Requirements) with sprite cards;
      new `game.ProductionBreakdown` + `GET /resources/{id}` resources page (per-source lines,
      bonus, storage, total/daily/weekly); Resources menu enabled; pure + render tests.
- 2026-10-15 — **Web UI reskin I3 (hangar/defenses/fleet)**: `ShipyardBody`/`DefensesBody` are
      grouped sprite card grids (Recently built / Small / Medium / Large / Special — bands captured
      from the live pages, recent units pulled out of their class) with cost rows, duration, "Have"
      count, "Max" (server-computed affordable qty + app.js fill), Build, Busy and a cancel X; new
      `/defenses/{id}` page + `/defenses/{id}/build|cancel`; wired `store.CancelQueue(CancelShipyard)`;
      Fleet restyled onto panels + `/expedition` link; web render tests (catalog-completeness) +
      `TestCancelQueueRefundsShipyard`.
- 2026-10-15 — **Web UI reskin I2/I1/I0**: card grids + queue cancel for buildings/research
      (`store.CancelQueue`, `POST /buildings|/research/{id}/cancel`, Overview restyled); `skin.css`+
      `app.js` + rewritten shell `layout.templ` (topbar, htmx resource bar with DM/AM/Stardust, full
      menu) ; artwork captured from the live skin into `static/niburu/**` (+ `go:embed` + serving tests).
- 2026-10-15 — **Stardust diameter sink**: Planetarium "increase in diameter" (`POST /planets/{id}/diameter`)
      spends a coords debris field (100B metal/50B crystal) + 1 Stardust → +276..414 km / +12..18 fields (planets & moons); store+API+tests.
- 2026-10-15 — **Expeditions X1–X4**: slots `floor(astro/2)` + universe bonus (`0017`), enforced in
      `Dispatch`; `/expedition/{planet}` panel (`exp_time`→0.25–2.5 h / `holding_minutes`, `exp_speed`,
      auto `[g:s:21]`, htmx status); rare `stardust` outcome + `users.stardust` (`0018`); mix
      recalibrated (combat 13.5 %, stardust 1.9 %, fatal-BH 1.68 %, n=2062). Pure + store tests.
- 2026-10-11 and earlier — Arsenal/Black-market pages, tech-tree + Simulator, frontend balance
  (no lab cap, storage drives limits), drain `bwlean-split-v7`, 7+ acc1 colonies. See
  `docs/BACKLOG_ARCHIVE.md`.

## Deep dives (read on demand)
`ARSENAL_UPGRADES_IMPLEMENTATION.md` · `BALANCE_DATA_NEEDED.md` · `COMBAT_FINDINGS.md` /
`COMBAT_MODEL.md` · `EXPEDITIONS_CALIBRATION_PLAN.md` · `AUTO_BUILD_DESIGN.md` · `MOONS.md`.
Live-capture runbooks, the explorer client and raw balance data live in the private ops repo.

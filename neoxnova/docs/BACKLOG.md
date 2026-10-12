# BACKLOG — kanban (todo / in-progress / done)

Read first; keep < 80 lines. One item per session. Full session history (pre-slim) lives in
`docs/BACKLOG_ARCHIVE.md`. Deeper context in the per-topic docs (see "Deep dives" below).

## Live state (operational cheat-sheet)
- acc1 **Bratwurst** `3:125:12` runs entirely on VM `azure-bot` (tmux `drain-acc1` + `bonus-acc1`),
  no local processes. acc2 TheBob retired (manual-only fallback).
- Burn-down = lean **BW 228** (`drain.mjs` v7 `bwlean-split-v7`, config `plans/farm-sites.json >
  acc1.drain`), 11 live slots, `per = floor((home+inflight)·0.97/slots)`.
- Rules: keep 109/110/111 at the techtree floor; resources are never cron-updated; live automation
  builds **ships only** (mines + ship-trader scrap are manual).
- Deploy = local commit → push → VM `git -C ~/neoxnova reset --hard origin/main`.

## In progress
- _(none)_

## Todo (ordered; pick one per session)
- [ ] **Web UI menu coverage** — remaining in-game menus missing from the frontend: Premium
      Account, Antimatter Store, Senate, Academy, Achievements, Alliance, Referals, Tournaments,
      Promote Server. Now covered too: Arsenal, Black market, Technologies (tech tree),
      Simulator (page + `POST /api/v1/sim`). Already covered: Research, Buildings, Hangar,
      Defenses (Shipyard), Fleet, Resources, Galaxy.
- [ ] **Expedition live calibration** — harvest outcomes, refine the non-locked mix weights
      (`docs/EXPEDITIONS_LIVE_2026-10-06.md`; resolver + BH done).
- [ ] **Stardust rare currency** — `game` constant + occasional expo find + debris-based planet/moon
      diameter/fields sink (`docs/EXPEDITIONS…` §11e).
- [ ] **Moons (item 4)** — `httpbot` jump command, moon build/overview API, `DESTROY_MOON`
      (`docs/MOONS.md`).
- [ ] **Auto-builder Phase 3/4** — UI (no client in repo), energy-gate decision, auto-colonize
      (`docs/AUTO_BUILD_DESIGN.md` §12/§15).
- [ ] **TOTP 2FA** — second auth factor (`internal/auth`).
- [ ] **Full game-loop integration test** — register → … → abandon.
- [ ] **acc1 ops** — scrap the surplus BB/HC fleet via the ship trader for crystal; watch
      cargo/recycler sizing. Crystal remains the fleet bottleneck.

## Done (recent only)
- 2026-10-12 — expedicion full deep-scan analysis (BW night): outcome mix n=652, loot, 53
  fights (24/24 overnight wins), fatal-BH 2.30 % window / 1.95 % since relaunch
  (`docs/EXPEDITIONS_LIVE_2026-10-06.md` §14; no code changed).
- 2026-10-11 — Web UI clusters A+D: Arsenal + Black-market pages over the existing store; new
  Technologies tech-tree page and battle Simulator (page + `POST /api/v1/sim`); nav + tests.
- 2026-10-11 — frontend balance fixes: removed the erroneous research-lab level cap (the lab only
  *unlocks* techs via their prerequisites, it does not cap level); techtree graph re-verified against
  the live server (114 items, 0 diffs); Mineral/Semi-Crystals/Fuel Research +5%/level account-wide
  production; storage levels drive capacity
  (`5000·floor(2.5·e^(20L/33))·resource_speed`, matches niburu's 53.55B @ L10).
- 2026-10-11 — drain per-fleet sizing fix (`bwlean-split-v7`) + BH instrumentation; local dev
  bootstrap scripts (`dev-up.sh`/`.ps1`); acc1 colony metal-50 round.
- 2026-10-10 — lean BW switch (BW 228); moonshot rounds 1–3; 7 colonies done; expo dashboard.
- 2026-10-09 — expedition calibration + positive BH (item 8); playable web UI + onboarding;
  research-speed model.
- 2026-10-08 — expedition resolver (item 1); auto-builder Phase 2; outcome surfacing; moons;
  acc1 migrated to the VM.
- 2026-10-07 and earlier — see `docs/BACKLOG_ARCHIVE.md`.

## Deep dives (read on demand)
`ROLLING_FARM.md` · `EXPLORER.md` · `ARSENAL_UPGRADES_IMPLEMENTATION.md` · `BALANCE_DATA_NEEDED.md` ·
`COMBAT_FINDINGS.md` / `COMBAT_MODEL.md` / `COMBAT_TEST_PLAN.md` · `ESPIONAGE_LIVE_2026-10-06.md` ·
`EXPEDITIONS_LIVE_2026-10-06.md` · `ARSENAL_LIVE_2026-10-06.md` · `AUTO_BUILD_DESIGN.md` · `MOONS.md`.

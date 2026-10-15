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
- [ ] **Web UI reskin (I1–I5)** — I0 artwork capture done (below). Mirror the live Niburuspace
      OGame-Redesign UI; plan + phases in `docs/WEB_UI_RESKIN_PLAN.md` (I1 shell/skin →
      I2 build/research/cancel → I3 hangar/fleet → I4 galaxy/tech/resources → I5 account/messages).
      Supersedes the old "Web UI menu coverage" item.
- [ ] **Moons (item 4)** — `ops client` jump command, moon build/overview API, `DESTROY_MOON`
      (`docs/MOONS.md`).
- [ ] **Auto-builder Phase 3/4** — UI (no client in repo), energy-gate decision, auto-colonize
      (`docs/AUTO_BUILD_DESIGN.md` §12/§15).
- [ ] **TOTP 2FA** — second auth factor (`internal/auth`).
- [ ] **Full game-loop integration test** — register → … → abandon.
- [ ] **acc1 ops** — scrap the surplus BB/HC fleet via the ship trader for crystal; watch
      cargo/recycler sizing. Crystal remains the fleet bottleneck.

## Done (recent only)
- 2026-10-15 — **Web UI reskin I0 (artwork capture)**: ops `tools/explorer/assets.mjs` captures the
      live skin read-only (acc1 session, page `<img>`/CSS `url()`) → curated `internal/web/static/
      niburu/**` (`buildings/research/ships/defenses/icons/bg` + `README.md` id→file map); `go:embed
      static` + serving and catalog-completeness tests.
- 2026-10-15 — **Stardust diameter sink**: Planetarium "increase in diameter" (`POST /planets/{id}/diameter`)
      spends a coords debris field (100B metal/50B crystal) + 1 Stardust → +276..414 km / +12..18 fields (planets & moons); store+API+tests.
- 2026-10-15 — **Expedition mix recalibration (X4)**: combat 13.5 % (owner direction, vs the
      suppressed live 10.2 %) + stardust 1.9 %, fatal BH 1.68 %/positive BH-loot 1.7 % pinned, six
      unlocked buckets normalized to the measured n=2062 split; point/cargo-scaled finds kept (+test).
- 2026-10-15 — **Stardust find (X3)**: rare `stardust` expedition outcome (~1.9 %) + live flavours;
      `users.stardust` (migration `0018`) credited/persisted, shown via `/auth/me` + resource state + web bar.
- 2026-10-15 — **Expedition command panel (X2)**: dedicated `/expedition/{planet}` — ship picker,
      `exp_time` 1–10 → 0.25–2.5 h (new `holding_minutes`), `exp_speed` 1–10 → 10–100 %, "send N"
      (stops at slot limit), auto `[g:s:21]`, htmx-polled slots + active expeditions + outcomes; `/fleet` untouched.
- 2026-10-15 — **Expedition slots (X1)**: `floor(astro/2)` + universe `expedition_slots_bonus`
  (optional `_cap`; migration `0017`), enforced in `FleetStore.Dispatch`; expeditions pinned to
  `[g:s:21]`; pure + store integration tests.
- 2026-10-12 — 2 more acc1 colonies (cp1917 `3:137:9`, cp1918 `3:139:9`); bootstrap + `gradual`
  bumps to mines 48/47/44, Robot 22 / Nanite 11, conveyors 9/8/7 (`run-one-colony.sh`).
- 2026-10-12 — expedicion full deep-scan analysis (BW night): outcome mix n=652, loot, 53
  fights (24/24 overnight wins), fatal-BH 2.30 % window / 1.95 % since relaunch
  (`the private ops repo` §14; no code changed).
- 2026-10-11 — Web UI clusters A+D: Arsenal + Black-market pages over the existing store; new
  Technologies tech-tree page and battle Simulator (page + `POST /api/v1/sim`); nav + tests.
- 2026-10-11 — frontend balance fixes: removed the erroneous research-lab level cap (the lab only
  *unlocks* techs via their prerequisites, it does not cap level); techtree graph re-verified against
  the live server (114 items, 0 diffs); Mineral/Semi-Crystals/Fuel Research +5%/level account-wide
  production; storage levels drive capacity
  (`5000·floor(2.5·e^(20L/33))·resource_speed`, matches the reference server's 53.55B @ L10).
- 2026-10-11 — drain per-fleet sizing fix (`bwlean-split-v7`) + BH instrumentation; local dev
  bootstrap scripts (`dev-up.sh`/`.ps1`); acc1 colony metal-50 round.
- 2026-10-10 — lean BW switch (BW 228); moonshot rounds 1–3; 7 colonies done; expo dashboard.
- 2026-10-09 — expedition calibration + positive BH (item 8); playable web UI + onboarding;
  research-speed model.
- 2026-10-08 — expedition resolver (item 1); auto-builder Phase 2; outcome surfacing; moons;
  acc1 migrated to the VM.
- 2026-10-07 and earlier — see `docs/BACKLOG_ARCHIVE.md`.

## Deep dives (read on demand)
`ARSENAL_UPGRADES_IMPLEMENTATION.md` · `BALANCE_DATA_NEEDED.md` · `COMBAT_FINDINGS.md` /
`COMBAT_MODEL.md` · `EXPEDITIONS_CALIBRATION_PLAN.md` · `AUTO_BUILD_DESIGN.md` · `MOONS.md`.
Live-capture runbooks, the explorer client and raw balance data live in the private ops repo.

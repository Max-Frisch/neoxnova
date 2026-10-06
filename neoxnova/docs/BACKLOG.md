# BACKLOG / status — read first; update at task end; keep < 80 lines

## Live state
- Stack: `docker compose` (`neoxnova_postgres`, `neoxnova_redis`); run `go`/`make` from `neoxnova/`.
- acc1 (local, Bratwurst `3:125:12`): espionage L21; colony builders **stopped** (all 6 hit caps);
  **moon present** (diameter <10 000). Next session: parallel acc1+acc2 expedition testing.
- acc2 (VM `azure-bot`, TheBob `2:188:16`): colony builders `colo-1687/88/89/96` **running**, `1697/98` done.
- VM sync rule: local commit -> `push` -> `ssh azure-bot 'git -C ~/neoxnova reset --hard origin/main'`.
- **Expo matrix running (2026-10-06):** both accounts, `cmd=1`, from main planets, same 7 arms
  (time=1, speed=10): cargo `217:20` · fodder `204:200` · combat `207:20` · `217:20+204:200` ·
  `217:20+207:20` · `217:50+204:500` · `217:20+226:100`. acc1 cap is *unenforced*: counter `13/7`
  (6 legacy "shadow slots" from the prior `cmd=2` hanging-fleet bug + our 7). Harvest daemons
  (`run-expharvest.sh`: tmux `expharv-acc2` on VM, nohup loop locally) write
  `data/expeditions.json`, `data/expedition-reports.json`, sends in `data/expedition-runs.json`.
  **Next: analyze when fleets return; then run a second round to raise sample counts.**

## Open backlog (ordered; one item per session)
1. **Expedition resolver (Go)** — capture done (see below); model `MissionExpedition` in
   `internal/engine`: outcome roll, loot, and points-scaled enemy. BLOCKERS: enemy formula unknown;
   `cmd=2` leaks ghost fleets (avoid). Notes: `docs/EXPEDITIONS_LIVE_2026-10-06.md`.
1b. **Expedition enemy formula** — collect composition-controlled samples (pure vs escort vs
    **meatshield/"Schussfang"**); stop poking `--pve`. Matrix live on BOTH accounts (see Live state).
    Key metric is now **fight reports** (`exp-report`, messcat=3): per-unit start counts + total
    losses, so cargo `217` survival can be diffed across arms. Early hint: enemy size is *not*
    proportional to sent fleet points (pure `217:10` drew a bigger template than `217:100`).
2. **Incoming-fleet view** (transport/attack/espionage) — mission text+colour per planet so online
   defenders see/react before arrival (currently only espionage is visible via its reports endpoint).
3. **Auto-builder base (Go)** — blueprint per planet + account research; design in `docs/AUTO_BUILD_DESIGN.md`.
4. **Moons** — acc1 `3:125:12` now HAS a moon (live, spawn confirmed; diameter <10 000).
   Rules to model: **≥10 000 km = indestructible**; below that destroyable by Deathstar.
   Scope: moonbase, moon buildings (sensor phalanx/jump gate), creation (debris) + destruction.
5. **TOTP 2FA** (auth 2nd factor on `internal/auth`).
6. **Full game-loop integration test** — register→login→colonize→build→research→shipyard→dispatch→
   battle→report→recycle→abandon.

## Done (newest first)
- expedition live capture + tooling (`httpbot` `expedition`/`exp-state`/`exp-log`); docs/EXPEDITIONS_LIVE_2026-10-06
- espionage: full report + counter-espionage + reports routes — `9074c93`
- espionage live matrix doc; explorer fleet step-2 mission fix — `2c19386`, `602a324`
- colony caps robot 21 / nanite 10 / mines 44/42/39 — `b69fdd2`
- relocation (teleport) + DM field expansion + attack lockout — `2d15674`, `97ca89f`
- combat engine, missions, auth, planets, combat reports — `9fa5787`
- (older history: `git log --oneline`)

## Deep dives (read on demand)
`docs/EXPLORER.md` · `docs/BALANCE_DATA_NEEDED.md` · `docs/COMBAT_FINDINGS.md` / `COMBAT_MODEL.md` /
`COMBAT_TEST_PLAN.md` · `docs/ESPIONAGE_LIVE_2026-10-06.md` · `docs/EXPEDITIONS_LIVE_2026-10-06.md` ·
`docs/AUTO_BUILD_DESIGN.md` · `docs/BACKUP.md`

## Open questions / blockers
- Counter-espionage exact formula unknown; using approximation + ships-only detection.
- Moon spawn chance: owner testing manually (deferred).
- Defense-vs-ships detection not isolated live (targets always had ships).

# BACKLOG / status — read first; update at task end; keep < 80 lines

## Live state
- Stack: `docker compose` (`neoxnova_postgres`, `neoxnova_redis`); run `go`/`make` from `neoxnova/`.
- acc1 (local, Bratwurst `3:125:12`): espionage L21; colony builders **stopped** (all 6 hit caps).
- acc2 (VM `azure-bot`, TheBob `2:188:16`): colony builders `colo-1687/88/89/96` **running**, `1697/98` done.
- VM sync rule: local commit -> `push` -> `ssh azure-bot 'git -C ~/neoxnova reset --hard origin/main'`.

## Open backlog (ordered; one item per session)
1. **Expeditions live capture (acc1)** — send expos, log every outcome (nothing/delay/early-return/
   black hole/resources/DM/ships/pirates/aliens + losses), estimate black-hole rate; then model resolver.
   Tooling: `tools/explorer`; notes in `docs/EXPLORER.md`.
2. **Incoming-fleet view** (transport/attack/espionage) — mission text+colour per planet so online
   defenders see/react before arrival (currently only espionage is visible via its reports endpoint).
3. **Auto-builder base (Go)** — blueprint per planet + account research; design in `docs/AUTO_BUILD_DESIGN.md`.
4. **Moons** (deferred; owner testing spawn chance) — moonbase, creation/destruction.
5. **TOTP 2FA** (auth 2nd factor on `internal/auth`).
6. **Full game-loop integration test** — register→login→colonize→build→research→shipyard→dispatch→
   battle→report→recycle→abandon.

## Done (newest first)
- espionage: full report + counter-espionage + reports routes — `9074c93`
- espionage live matrix doc; explorer fleet step-2 mission fix — `2c19386`, `602a324`
- colony caps robot 21 / nanite 10 / mines 44/42/39 — `b69fdd2`
- relocation (teleport) + DM field expansion + attack lockout — `2d15674`, `97ca89f`
- combat engine, missions, auth, planets, combat reports — `9fa5787`
- (older history: `git log --oneline`)

## Deep dives (read on demand)
`docs/EXPLORER.md` · `docs/BALANCE_DATA_NEEDED.md` · `docs/COMBAT_FINDINGS.md` / `COMBAT_MODEL.md` /
`COMBAT_TEST_PLAN.md` · `docs/ESPIONAGE_LIVE_2026-10-06.md` · `docs/AUTO_BUILD_DESIGN.md` · `docs/BACKUP.md`

## Open questions / blockers
- Counter-espionage exact formula unknown; using approximation + ships-only detection.
- Moon spawn chance: owner testing manually (deferred).
- Defense-vs-ships detection not isolated live (targets always had ships).

# Handoff — state & next steps (2026-10-05)

Durable memory for the next session (on the PC). Read this first, then
`docs/COMBAT_SESSION_2026-10-05.md` (live data) and `COMBAT_FINDINGS.md` (model).
`AGENTS.md` has the environment/layout rules.

## Environment recap
- Local dev host = **acc1 Bratwurst** (`neoxnova/secrets/explorer.env`).
- Azure VM `azure-bot` (`neoxnova/secrets/ssh/config`) = **acc2 TheBob**; its repo
  carries the newest explorer tooling + combat plans (`30–61`, `80`).
- Postgres/Redis via `docker compose` (`neoxnova_postgres`, `neoxnova_redis`).
- Run Go/make from `neoxnova/`. Go 1.24 (stdlib only — no new modules).

## How to run / verify
```
docker compose up -d            # or `make up`
make migrate && make seed       # migrations 0001..0007 now
make run                        # :8080
go test ./...                   # hermetic (DB integration tests skip)
$env:DATABASE_URL="postgres://postgres:password@localhost:5432/neoxnova?sslmode=disable"
go test ./...                   # runs DB integration tests too
```
Migrations: `0004_academy`, `0005_combat_reports`, `0006_auth`, `0007_planets`.
Seeded commander password: **`commander-dev-pass`** (regenerate: `go run ./cmd/hashpw '<pw>'`).

## Implemented this session (all build/vet/test green)
- **Combat engine** `internal/game/combat.go` + generated `unit_stats.go`:
  quadratic techs `round(L(L+2)/4)`, 8 rounds, shield-first (no bounce),
  deterministic RF (r1 `floor(0.70·N·RF)`, then `N·RF`), 50% ships-only debris,
  loot 50% M→C→D, attacker academy procs `1103/1109`. **~91% winner match** on
  `testdata/niburus_combat.json`.
- **Trust seed**: `game.CombatSeed(attacker, defender)` — simulator and resolver
  share it, so same fleets reproduce the exact battle.
- **ATTACK / RECYCLE / HOLD / COLONIZE** resolved in
  `internal/engine/event_engine.go` (battle persistence, 61% defense repair,
  debris field, loot, colonise roll, hold window).
- **Noob-protection** (`store.PlayerPoints`, ~4:1) in `fleet_store.go`.
- **Combat reports**: `0005` table + `GET /api/v1/combat/reports/{id}` and
  `GET /api/v1/planets/{id}/combat-reports`.
- **Auth**: `internal/auth` (PBKDF2 600k, hashed sessions, hardened cookies),
  `POST /auth/register|login|logout`, `GET /auth/me`, `RequireAuth` on all game
  routes, per-IP rate limiting + login lockout + security headers.
- **Planets**: `internal/game/planets.go` (niburu per-slot fields/temp ranges,
  `SatelliteEnergy = round((tempMax+160)/6)`), colonise rolls fields/temp/fields
  seeds, `POST /planets/{id}/abandon` with a **24 h global slot lock**
  (`store.SlotLockDuration`), terraformer **+7 fields/level (planets only)**.
- **Fuel/flight** recalibrated in `game_math.go` (distance piecewise,
  `fuel = max(1, round(baseFuelTotal·dist/8750·speed%))`, flight-time fit to the
  one live sample; engine techs 115/117/118 give +10%/level to speed).

## Game-design decisions locked in
- **20 planet slots + slot 21 = expedition** (`universes.planets_per_system`).
- Fields roll **uniformly** within the niburu per-slot ranges; **900+** is the
  field-farming target (slot 9 peaks at 1020).
- Abandoned/destroyed planets lock the slot **24 h** (global). Homeworld not
  abandonable; can't abandon your last planet or while fleets are in transit.
- Combat: **no per-shot bounce**; defense repair **~61% for draw AND wipe**;
  moon generation is **off** on niburu (adopt classic `min(20, debris/100000)%`).
- Loot 50% M→C→D, cargo-capped; server cargo: **217 = 400M, 219 = 200M**.
- Never spend academy points on **1105 Engine limitation**; avoid the auto-spend
  `httpbot.mjs academy` command — use `academy-up`/`academy-map` only.

## Remaining implementation (ranked small→large)
1. **Relocate** planet (Dark Matter, distance-priced; doesn't lock the origin,
   ~1 h per-planet cooldown) — needs the DM cost fitted (capture niburu relocate page).
2. **Moons** (moonbase +3 fields, creation/destruction) — needs data (moons off on niburu).
3. **Espionage** mission (report + counter-espionage).
4. **API keys** for sanctioned (paid) auto-builder automation.
5. **TOTP 2FA**.
6. **Expedition + Arsenal scaling** — L; depends on a niburu capture of Arsenal
   upgrade effects and expedition outcome rates.

## Bigger testings (do next; in priority order)
- **(A) Full game-loop integration test** (local, no balance): register→login→
  colonize→build→research→shipyard→dispatch→battle→report→recycle→abandon.
  Highest confidence, cheap.
- **(B) Niburu combat-fidelity sweeps** (simulator, no balance): pin the
  damage-distribution rule, RF edge cases, academy isolation.
- **(C) Niburu live campaign for Expeditions/Arsenal** (needs balance top-up):
  capture Arsenal effects + expedition tables, then implement.

## Known open items
- **Damage distribution**: reference concentrates hull damage more than uniform
  count-weighted targeting (winner ~91%, loss magnitudes diverge in
  swarm-vs-capital). Kill curve recorded in `COMBAT_FINDINGS.md §8`.
- **Distance formula**: same-galaxy position anomaly (`+1000+5·dp` case) fitted
  but only 2 supporting samples; cross-galaxy from 1 sample.
- **Engine-tech** levels recorded with any new flight-time samples.
- `testdata/` + `docs/` were untracked before this commit; now tracked.

## Live side-state (niburu)
- Colony builders **stopped** on both accounts; acc2 VM still runs the `bonus`
  session (Online Bonus). acc2 colonies hit **40/38/35**; acc1/academy procs
  partially unlocked (acc1 `1103:7 1108:5 1109:4 1110:2 1111:1`; acc2 `1301:10
  1302:4 1303:5 1311:5`). Damage-distribution + expedition testing still pending.

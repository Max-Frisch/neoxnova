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
1. ~~**Relocate** planet~~ **DONE 2026-10-06**: `POST /api/v1/planets/{id}/relocate`
   (body `{galaxy,system,position}`), `internal/game/relocation.go`, migration
   `0008_relocation.sql`; origin NOT locked. Price captured from the Planetarium
   (`docs/screenshots_niburu/`), exactly linear per axis:
   `15000·Δgalaxy + 1000·Δsystem + 2500·Δposition` DM. Cooldown 1 h, but unlimited
   same-system teleports. Also done 2026-10-06: **15 min no-attack** after a
   teleport (`attack_locked_until`, enforced in `FleetStore.Dispatch`) and
   **Dark-Matter field expansion** (`POST /planets/{id}/fields`, geometric
   `round(2000·1.1^n−1)`, migration `0009_planet_fields.sql`). Phalanx-offline is
   **not** modelled; the debris+Stardust "increase diameter" option + Stardust
   currency are **scrapped for now** (owner decision 2026-10-06). See
   `BALANCE_DATA_NEEDED.md` §"Planet relocation / teleport".
2. **Moons** (moonbase +3 fields, creation/destruction) — *not* believed to be off;
   chance is likely just lower than classic. **Deferred**: owner will test the
   spawn chance manually first.
3. **Espionage** mission. **Math implemented** (`internal/game/espionage.go` +
   tests): `score = probes + (yourEsp − enemyEsp)·|yourEsp − enemyEsp|`; reveals
   Fleet ≥2, Defense ≥3, Buildings ≥5, Research ≥7 (Resources always) — reproduces
   the reference required-probe table. Counter-espionage approximated
   (`0.25%·2^(def−atk)·probes·defShips`, clamped; no ships ⇒ 0). **Live findings
   captured** in `docs/ESPIONAGE_LIVE_2026-10-06.md`: fleet-send wizard semantics,
   attacker report format, defender incoming-event text/colours, probe destruction.
   **RESOLVED (live matrix, `docs/ESPIONAGE_LIVE_2026-10-06.md`)**: niburu
   **always returns the full report** — it does **not** mask sections by tech
   difference (tested diff 0 / +1 / −1 / +2 with 1 probe all gave
   Resources·Fleet·Defense·Buildings·Research). The tech difference only shows in
   **counter-espionage** (probes destroyed whenever the target had ships — even
   satellite-only colonies; defenses not required). **Implement accordingly**:
   report store + engine `resolveEspionage` (full report; roll probe loss) + a
   generic **incoming-fleet notification** (shared by transport/attack/espionage,
   mission-specific text+colour; defender can react). No academy/senate bonus.
4. **In-game auto-builder menu** (a game page like Arsenal/Buildings) — NOT an
   external/SDK API (the old "API keys" wording was a mis-communication). Design
   captured in **`docs/AUTO_BUILD_DESIGN.md`**: persisted blueprint per planet +
   account research, fills the one normal build queue, admin/dev-gated first,
   optional DM/premium paywall later, blueprint = the proven explorer JSON schema.
   **Backlog (design only, no code yet).**
5. **TOTP 2FA** (auth — second factor on `internal/auth`).
6. **Expedition + Arsenal scaling** — L. **Next live-test topic, on acc1** (highest
   flight-speed + offensive academy bonus). Mechanics (owner): black-hole chance is
   low in classic OGame (1-2%) but likely higher here to curb 24/7 expos — still
   expected profitable. Profit rises with academy offensive/defensive bonuses and
   laser/ion/plasma/graviton research. Pirates/stronger aliens scale **only** on the
   player's Weapons/Shield/Armour research, so keep W/S/A **low** (just enough to
   unlock ships/defense) and raise the profitable bonuses instead. Needs a niburu
   capture of Arsenal effects + expedition outcome rates.

## Bigger testings (do next; in priority order)
- **(A) Full game-loop integration test** (local, no balance): register→login→
  colonize→build→research→shipyard→dispatch→battle→report→recycle→abandon.
  Highest confidence, cheap.
- **(B) Niburu combat-fidelity sweeps** (simulator, no balance): pin the
  damage-distribution rule, RF edge cases, academy isolation.
- **(C) Niburu live campaign for Expeditions/Arsenal** (needs balance top-up):
  run on **acc1**; capture Arsenal effects + expedition outcome tables (black-hole
  rate, pirate/alien scaling vs W/S/A, resource/DM/ship finds), then implement.
  - Expedition fleet notes (owner + web research). A good comp balances three
    things at once: **fleet value** (drives find size), **cargo** (haul it home)
    and **combat power** (survive pirates/aliens). Owner's rule of thumb:
    **~10 Battleships : 1 Battle Transporter** — BS has **no rapid-fire vs BT**, so
    at least one BT survives to carry loot/debris home. Web/official: finds scale
    with the **top player's points** (e.g. ~42–200 Large Cargos + 1 probe for
    resource runs). Official outcome odds: pirates ~5.8%, aliens ~2.6%, delay ~7%,
    early return ~2%, nothing ~18.6%, **black hole ~0.33%** (niburu likely higher),
    merchant ~0.7%. Expedition points = `(hull·5)/1000`; max capped by rank-1 points.
  - Arsenal/Governator "Upgrades" (Laser/Ion/Graviton/Plasma guns, armor, shields,
    engines, conveyors, production) are **unique to niburu** — not in classic OGame
    0.84. They are expedition-findable and are the main lever that scales expo
    profit while W/S/A research is kept low (pirates/aliens scale on W/S/A only).

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

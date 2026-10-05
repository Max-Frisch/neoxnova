# Auto-Build / Colony Builder — Design & Roadmap

Status: proposed (design only, no code yet)
Audience: future implementer (us)
Scope: server-side, configurable "auto-build" for neoxnova, aimed at making
accounts with 20–40 colonies manageable without a client bot.

---

## 1. Summary & goals

Add a first-class, server-authoritative **auto-build** system: a persisted,
configurable **blueprint** per planet (plus account-level research), advanced by
the existing durable scheduler, that fills each planet's normal build queue
toward a target and keeps growing economy until defined caps.

Goals:

- Build out a new colony automatically from a reusable blueprint.
- Apply a blueprint to many planets at once ("Colony Builder" page).
- Keep developing economy forever in a "gradual" mode, or just hit fixed
  targets in a "simple" mode.
- Survive restarts, races and multi-instance deploys (at-least-once, idempotent).
- Be testable as pure logic + thin DB/engine glue.

## 2. Non-goals

- Not a client-side bot. The Node explorer tooling under
  `neoxnova/tools/explorer/` stays as a live-game tool; this is the real game
  feature.
- Not a parallel/advantage build queue. Auto-build only fills the normal,
  single active build slot. It is convenience QoL, not pay-to-win (see §10).
- Not combat, fleet automation, market, or diplomacy.
- Auto-*colonize* (founding new planets) is a separate capability but is
  included here as a later phase because "auto-build new colonies" implies it.

## 3. Design decisions

1. **Server-authoritative Go.** The planner lives in the Go backend; the client
   only reads/writes blueprint config. No scripts talking to the game loop.
2. **Reuse enqueue primitives.** Never duplicate cost/duration/prereq/fields/
   deduction logic. Every action goes through
   `BuildStore.EnqueueStructure/EnqueueShipyard/EnqueueResearch`
   (`internal/store/build_store.go:85/160/236`).
3. **Queue-filler semantics.** Auto-build competes for the same single active
   build slot per scope, enforced by the partial unique indexes in
   `migrations/0003_build_queues.sql:9-16`. If busy, it waits.
4. **Admin/dev-gated first.** Gate behind `users.auth_role` + a feature flag so
   it is invisible to players during development. A small paywall/perk is an
   option later, not a requirement.
5. **Blueprint spec = proven explorer schema.** The plan JSON already validated
   in the field (`tools/explorer/httpbot.mjs:185`, `plans/colo-grow.json`) is
   promoted to the canonical persisted schema.
6. **Event-driven, not a fast polling daemon.** Advance on build completion,
   with a slow sweep for cold starts/cancels.

## 4. Domain model

New migration `migrations/0004_blueprints.sql` (add it to the `migrate` target in
the `Makefile`, per `AGENTS.md`).

```
build_blueprints
  id              BIGSERIAL PK
  universe_id     UUID  -> universes(id)
  user_id         BIGINT -> users(id)
  celestial_id    BIGINT NULL -> celestial_objects(id)   -- NULL = account template/target
  name            TEXT
  scope           TEXT CHECK IN ('planet','account')      -- 'account' carries research
  spec            JSONB                                   -- the blueprint language (§5)
  enabled         BOOLEAN DEFAULT false
  priority        INT DEFAULT 100
  paused_until    TIMESTAMPTZ NULL
  last_action_at  TIMESTAMPTZ NULL
  last_error      TEXT NULL
  created_at, updated_at
```

- Partial unique index: one enabled planet blueprint per `celestial_id`
  (`WHERE enabled AND celestial_id IS NOT NULL`).
- Account-scope blueprint carries research goals (research is empire-wide,
  `research_queues` keyed by `user_id`).
- Optional later: `blueprint_templates` (reusable named specs) and
  `blueprint_runs` (audit log) if status/history needs outgrow `last_error`.

Reuse existing per-state reads instead of new tables: `planet_structures`,
`user_technologies`, `construction_queues`, `shipyard_queues`,
`research_queues`, `celestial_objects`.

## 5. Blueprint language

Formalize the explorer plan schema (all fields optional):

```json
{
  "mode": "gradual",
  "buildings": { "metal_mine": 22, "crystal_mine": 20, "solar_plant": 21 },
  "research":  { "energy_tech": 5 },
  "ships":     { "212": 200 },
  "defenses":  { "401": 500 },
  "order":     ["robotics_factory", "shipyard", "metal_mine", "..."],
  "gradual":       ["metal_mine", "crystal_mine", "deuterium_synthesizer"],
  "caps":          { "metal_mine": 40, "crystal_mine": 38, "deuterium_synthesizer": 35 },
  "bumpBuilders":  ["robotics_factory", "nanite_factory"],
  "allowSlow":     ["energy_tech"],
  "energySats":    200,
  "builderBumpSec": 360
}
```

- `mode: "simple"` — fixed targets only; build each `buildings`/`research`/
  `ships` target in dependency + `order` sequence, then done.
- `mode: "gradual"` — bootstrap `buildings`, then raise every code in
  `gradual` by +1 each cycle up to its `caps` entry; raise `bumpBuilders`
  (Robot/Nanite) by +1 when a mine's next build time exceeds `builderBumpSec`;
  keep energy positive via `energySats`.
- `hybrid` — both present; gradual starts after fixed targets are met.

Codes: prefer stable string codes (the Go catalog uses strings like
`metal_mine`; the explorer uses numeric ids `1,2,3`, ships `212`). The persisted
spec should use Go catalog codes; the UI/API maps display names.

## 6. Planner (pure logic)

New package `internal/blueprint/` — no DB, fully unit-testable in the style of
`internal/game` tests.

Input: parsed spec + a state snapshot (structure levels, tech levels, queued
items per scope, resources, energy, fields).
Output: the next recommended action (`structure` | `research` | `ship` |
`defense`) or `idle`/`done` plus a reason.

Responsibilities:

- **Prerequisite closure.** Go's `game.Requires` (`internal/game/catalog.go:256`)
  is a flat map with no graph. Either complete the maps or port the live
  techtree graph (`tools/explorer/parse.mjs:105`) into Go data. Expansion must
  skip forcing prerequisites of already-satisfied goals (the bug we hit in
  `httpbot.mjs` `visit()`).
- **Ordering.** Honor explicit `order`, then dependency order.
- **Gradual growth.** `gradual` + `caps` + `bumpBuilders` semantics.
- **Per-queue discipline.** At most one building / one research in flight
  (mirrors the DB unique indexes); the engine may cap in-flight further.
- **Energy strategy.** The server never gates builds on energy today
  (`build_store.go`); only `game.RecomputeProduction` (`internal/game/economy.go:26`)
  scales production. The planner should detect deficit and target Solar
  Satellites (unit `212`), as the bot does.
- **Dry-run.** Same function, no side effects, for the UI preview.

## 7. Engine integration

`internal/engine/event_engine.go`:

- After a successful resolve commit
  (`resolveConstruction:317`, `resolveShipyard:367`, `resolveResearch:413`),
  call `AdvanceBlueprint(ctx, celestialID/userID)`.
- Add a **slow periodic sweep** (e.g. every 5–10 s, not the 200 ms tick) in
  `processDue` (`event_engine.go:80`) to catch manual cancels, cold starts and
  planets that were idle at boot.
- `AdvanceBlueprint` loads the blueprint + state, asks the planner, then calls
  the existing `Enqueue*`. Treat `store.ErrQueueBusy` as "wait", not an error.
- Locking/idempotency: take a per-blueprint lock (`SELECT ... FOR UPDATE SKIP
  LOCKED` on the blueprint row, or `pg_advisory_xact_lock`) so multiple
  scheduler instances cannot double-advance. `Enqueue*`'s partial unique index
  is the final backstop.
- Throttling: with 40 planets, cap blueprint advances per tick (e.g. N per
  sweep) to bound DB load.

Note: there is **no energy check** in the enqueue path; if we want manual builds
to be energy-gated too, that is a separate change (do not silently alter
player-visible build rules as part of this feature).

## 8. API

Admin-gated for Phase 1. Proposed routes (mirroring `internal/api/router.go:16-25`):

- `GET    /api/v1/planets/{id}/blueprint`
- `POST   /api/v1/planets/{id}/blueprint`           (create/replace, enable)
- `PATCH  /api/v1/planets/{id}/blueprint`           (pause/resume/targets)
- `DELETE /api/v1/planets/{id}/blueprint`
- `GET    /api/v1/blueprints/templates`
- `POST   /api/v1/blueprints/apply`                 (spec + planet ids / "all")
- `POST   /api/v1/planets/{id}/blueprint/preview`   (dry-run next N actions)

**Prerequisite:** the API currently has **no auth/ownership** — every endpoint is
keyed by raw `planet_id` and `password_hash`/`auth_role` are seeded but never
checked. Admin-first is feasible with a minimal role check; a player-facing
release requires real auth + ownership enforcement first.

## 9. UI — "Colony Builder"

A subpage in the side menu (alongside Buildings/Research/Shipyard).

Layout (text wireframe):

```
+-------------------------------------------------------------+
| COLONY BUILDER                       [x] enabled  [stop all]|
+-------------------------------------------------------------+
| Planets:  [ ] select all   filter:  galaxy [2] system [186]  |
|   [x] 2:186:9  Zojiqu    blueprint: gradual   next: Metal 23 |
|   [x] 2:186:10 Xepaku    blueprint: gradual   next: Crystal 21|
|   [ ] 2:188:16 Japoqu    (none)                              |
+-------------------------------------------------------------+
| Mode:  ( ) Simple: build each to the max level below         |
|        (x) Gradual: economy growth (acc2 model)              |
|        ( ) Hybrid                                           |
+-------------------------------------------------------------+
| Buildings                        Research (account-wide)     |
|   Metal Mine      [ 40 ]           Energy Tech      [  5 ]   |
|   Crystal Mine    [ 38 ]           Computer Tech    [ 12 ]   |
|   Deuterium       [ 35 ]           Astrophysics     [  5 ]   |
|   Solar Plant     [ 21 ]                                     |
|   Robotics        [ 18 ]         Fleet / Defense (per planet)|
|   Nanite          [  7 ]           Solar Satellites [ 200 ]  |
|   Shipyard        [ 14 ]           Missile Launcher [ 500 ]  |
|   Research Lab    [ 10 ]                                     |
|                                                              |
|  [x] advanced: order / gradual set / caps / bump builders    |
+-------------------------------------------------------------+
| Preview (first planet): Robot 11 -> Storage 8 -> Metal 23 ...|
| [apply to selected]   [dry run]   [save as template]         |
+-------------------------------------------------------------+
```

Notes for the implementer:

- Image-less list with numeric inputs; no icons needed.
- **Scopes differ:** buildings/ships/defense are per-planet; research is
  account-wide and must be a separate section (one active research per user).
- The mode radio selects a **preset** that fills the advanced fields; the
  "advanced" panel exposes `order`/`gradual`/`caps`/`bumpBuilders`.
- Per-planet status badge + "next action" and a "why idle" reason.
- Dry-run preview before applying, especially before apply-to-all.
- Per-planet pause/resume and a global kill-switch.

## 10. Scale & fairness

- **Queue-filler** means no competitive advantage; it only automates clicks a
  human could do. This keeps it QoL for all players.
- With many planets, advance work is throttled and spread across sweeps.
- Default behavior should never exceed caps or spend below a configured
  resource reserve (optional `minReserve` in the spec) — add when needed.
- If ever monetized: keep it cosmetic/convenience (e.g. unlimited blueprint
  slots), never a second build queue or faster builds.

## 11. Dependencies / gaps to close first

- **Auth & ownership** — none exists today; required before player-facing.
- **Prerequisite graph in Go** — `game.Requires` is flat; planner needs a DAG.
- **Energy modeling** — not gated on enqueue; planner needs deficit handling.
- **Colony limits & colonize** — `base_colonies`/`max_colonies_hardcap` are
  seeded but unread; `MissionType.COLONIZE` exists (`internal/models/types.go:22`)
  but `resolveFleetEvent` has no case. Needed for auto-colonize (Phase 5).
- **Custom systems unmodelled** — University (6), conveyors (71/72/73),
  custom research (125/131/132/133/199), peaceful/combat levels
  (`docs/BALANCE_DATA_NEEDED.md:59-126`). Blueprints can still target them, but
  their effects/optimization are not modeled.

## 12. Roadmap

- **Phase 1 — foundations (prereq work):** auth/ownership (minimal), Go
  prerequisite graph, energy handling in the planner.
- **Phase 1b — core auto-build (single planet):** `0004_blueprints.sql`,
  `internal/blueprint` planner + tests, `AutoBuildStore`, engine
  advance-on-completion + sweep, admin-gated API, basic UI form.
- **Phase 2 — scale:** templates, apply-to-all, per-planet status, dry-run
  preview, throttling.
- **Phase 3 — polish:** advanced gradual editor, per-planet pause, budget/
  reserve rules, audit/analytics, optional paywall gating.
- **Phase 4 — auto-colonize (later):** colony slots (Astrophysics), Colony Ship
  production, `MissionColonize` in the engine, empty-position selection, and
  auto-assigning a blueprint to newly founded planets.
- **Phase 5 — unmodelled systems:** University/conveyor/research effects in the
  economy so blueprints optimize against them.

Auto-colonize (Phase 4) is what unlocks the full "send it and forget it" story
for 20–40 colonies; Phases 1b–3 make each existing planet self-managing first.

## 13. Testing

- **Planner:** table-driven tests over committed JSON fixtures under
  `neoxnova/testdata/` (mirror `internal/game/catalog_test.go`). Cover:
  prerequisite closure, order, gradual/caps/bumpBuilders, energy deficit,
  hybrid mode, done/idle detection, and the "satisfied prerequisite" bug.
- **Engine:** integration-style test against the compose stack (move
  `end_time` into the past per `AGENTS.md`) — advance fires once, is
  idempotent, and returns `ErrQueueBusy` politely under contention.
- Keep planner pure so the majority of coverage needs no DB.

## 14. Open questions

- Persist spec as JSONB vs normalized stage rows? (start JSONB; normalize only
  if querying/reporting demands it.)
- String catalog codes vs numeric ids in the stored spec? (recommend string
  Go catalog codes; map in the UI.)
- Do we energy-gate manual builds too, or only the planner? (recommend planner
  only for now.)
- How many blueprint advances per sweep to stay safe at 40 planets?
- Do we need blueprints for moons/other celestial types?

---

## Appendix A — current-state references

- Enqueue primitives: `internal/store/build_store.go:85` (`EnqueueStructure`),
  `:160` (`EnqueueShipyard`), `:236` (`EnqueueResearch`), `:324`
  (`RecomputeCelestial`), `:357` (`GetBuildings`).
- One-active constraint: `migrations/0003_build_queues.sql:9-16`.
- Scheduler/resolvers: `internal/engine/event_engine.go:80` (`processDue`),
  `:271` (`resolveConstruction`), `:325` (`resolveShipyard`), `:375`
  (`resolveResearch`).
- Resource accrual: `update_celestial_resources` (`migrations/0001_init.sql:330`).
- Catalog/economy: `internal/game/catalog.go:97/120/144/174`, `:256`
  (`RequiresMet`), `internal/game/economy.go:26`.
- API/routes: `internal/api/router.go:16-25`, `internal/api/handlers/build.go`.
- Explorer blueprint reference implementation: `tools/explorer/httpbot.mjs:185`
  (`cmdResolve`), `tools/explorer/parse.mjs:105` (`parseTechtreeGraph`),
  `tools/explorer/plans/colo-grow.json`, `tools/explorer/run-colonies.sh`.
- Design/balance context: `docs/BALANCE_DATA_NEEDED.md`, `AGENTS.md`.

## Appendix B — worked example (gradual)

`plans/colo-grow.json`, promoted as the "Gradual economy" preset:

```json
{
  "mode": "gradual",
  "buildings": { "14": 10, "22": 7, "23": 7, "24": 7, "21": 10, "31": 10,
                 "1": 22, "2": 20, "3": 17, "4": 21 },
  "ships": { "212": 200 },
  "gradual": ["1", "2", "3", "21", "22", "23", "24"],
  "bumpBuilders": ["14", "15"],
  "caps": { "1": 40, "2": 38, "3": 35, "21": 14, "22": 12, "23": 12,
            "24": 12, "14": 18, "15": 7 },
  "order": ["14", "22", "23", "24", "21", "31", "1", "2", "3", "4"]
}
```

Behavior: bootstrap the fixed targets; then +1 on every `gradual` code per
cycle up to `caps`; raise Robot/Nanite +1 each when a mine build exceeds 6 min;
keep energy positive with Solar Satellites.

# AGENTS.md

Neo-XNova: a Go backend for a stateful space-MMO. Git root `C:\code\projects\neoxnova`;
Go module in `neoxnova/` — run every `go`/`make` command from `neoxnova/`, not the repo root.

> **Session start:** read `neoxnova/docs/BACKLOG.md` (live state + open backlog), then update it
> at task end. `AGENTS.md` is human-owned — agents do **not** edit it.

## Layout & commands

- `.gitignore` allows only `README.md`, `AGENTS.md`, `neoxnova/**`; new root files need an exception.
- Packages: `cmd/server` (wiring), `internal/api` (+`handlers`), `internal/store` (Postgres),
  `internal/engine` (durable scheduler), `internal/game` (pure math/catalog), `internal/cache`,
  `internal/models`, `migrations/`.
- `make up` (Postgres 16 + Redis 7: `neoxnova_postgres`/`neoxnova_redis`), `make migrate`,
  `make seed`, `make run` (`:8080`), `make build`, `make test`, `make lint` (go vet). Format `gofmt -w`.
- `make`/Go 1.24 may be missing on Windows hosts — run the underlying commands directly. `migrate`/
  `seed` shell into the container (`docker exec -i neoxnova_postgres psql`), so no local `psql` needed.

## Environment & testing

- `DATABASE_URL` is the only required variable (see `.env.example`); `.env` auto-loads, absence is fine.
  `UNIVERSE_ID` (default `universe_6_niburu`) must match `universes.code_name`.
- Unit tests are pure (`internal/game`). DB integration tests skip unless `DATABASE_URL` is set; with
  it, `go test ./...` runs them against the compose stack.
- Seed: homeworld id 1 `1:1:1`, outpost id 2 `1:2:3`, commander user; idempotent (does not reset levels).
- The scheduler polls Postgres (~200 ms), so events aren't real-time. Fast-forward a due row keeping
  the timeline check valid, e.g.
  `UPDATE fleets SET start_time = NOW() - interval '5 s', arrival_time = NOW() - interval '1 s' WHERE id = <id>;`

## Architecture (non-obvious)

- Durable scheduler `internal/engine/event_engine.go`: claims due rows with `FOR UPDATE SKIP LOCKED`,
  re-checks the condition, idempotent, multi-instance safe. Redis is only a wake hint (`cache.WakeKey`).
- Resources are never cron-updated: `update_celestial_resources(id)` (PL/pgSQL) accrues on demand and
  row-locks. Call it before crediting cargo or spending on a build.
- Builds: typed catalog in `internal/game/catalog.go`/`economy.go`; `store/build_store.go` deducts +
  queues atomically; the scheduler applies the level and calls `RecomputeCelestial` (rewrites
  `*_prod_hourly`/`energy_*`/`fields_used`). Research draws from the planet and needs `research_lab >= level`.
- `Dispatch` resolves `fleets.target_id` by coordinates; empty space stays NULL and never resolves —
  point transports/deploys at a real celestial.
- Fleet speed is hardcoded `15.0` in `internal/game/game_math.go` (editing `universes` won't change it).
  Build durations read `universes.game_speed`; production reads `resource_speed`.
- Coordinate domains: galaxy 1-9, system 1-499, position 1-21 (21 = deep space).

## Editing gotchas

- lib/pq rejects multiple statements in one `Exec` once parameters are present — use separate `ExecContext`.
- `fleets.target_id` is nullable → scan into `sql.NullInt64`.
- Migrations: numbered SQL applied manually, must be idempotent; add each to the `migrate` target in the Makefile.
- One active build per scope is enforced by partial unique indexes; map pq `23505` → `store.ErrQueueBusy`.
- Conventional Commits; do not commit unless asked.

## Game data & live tooling

- Balance is approximate/calibrated — read `neoxnova/docs/BALANCE_DATA_NEEDED.md` before "fixing" it;
  costs are locked to `testdata/niburus_catalog.json`.
- Raw `*.HAR` captures are gitignored (may hold session cookies) — never commit.
- Live-game automation + the fleet/combat wizard: read `neoxnova/docs/EXPLORER.md`.

## Working style & Token Constraints

- Scope: Execute exactly one item from `neoxnova/docs/BACKLOG.md` per session. Update the backlog file immediately upon completion.
- Rule: Do not modify `AGENTS.md`. If a project change requires updating these rules, ask the human to do it.
- Token Economy: Minimize tool output. Use targeted flags (e.g., `rg -c`, `tail -n 15`). Never dump entire large source files into context; read specific blocks or functions.
- Multi-Step Tasks: For long tasks, write a brief execution plan to the terminal before changing code. If topic drift occurs, stop and prompt the user to start a fresh session.

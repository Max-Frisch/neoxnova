# AGENTS.md

Neo-XNova: a Go backend for a stateful space-MMO. The git root is
`C:\code\projects\neoxnova`; the Go module lives in `neoxnova/`. Run every
`go`/`make` command from `neoxnova/`, not the repo root.

## Layout gotcha
- `.gitignore` ignores everything except `README.md`, `AGENTS.md`, and
  `neoxnova/**`. New root-level files are invisible to git unless you add an
  exception.
- Single Go module. Package boundaries: `cmd/server` (wiring), `internal/api`
  (+ `handlers`), `internal/store` (Postgres), `internal/engine` (event loop),
  `internal/game` (math), `internal/cache`, `internal/models`, `migrations/`.

## Commands (run from `neoxnova/`)
- `make up` — start Postgres 16 + Redis 7 via compose (containers
  `neoxnova_postgres`, `neoxnova_redis`).
- `make migrate`, then `make seed` — apply `migrations/0001_init.sql` /
  `0002_seed.sql`.
- `make run` — `go run ./cmd/server`, listens on `:8080`.
- `make build` → `bin/server`; `make test` → `go test ./...`; `make lint` →
  `go vet ./...`; format with `gofmt -w`.
- `migrate`/`seed` shell out to `docker exec -i neoxnova_postgres psql ...`, so
  no local `psql` is needed. `make` itself and Go 1.24 may be missing on Windows
  hosts — if so, run the underlying commands directly; older Go toolchains
  auto-download 1.24 (`GOTOOLCHAIN=auto`).

## Testing / verification
- There are **no test files**. `go test ./...` only compiles packages. Verify
  behavior by running against the compose stack and hitting the API (endpoints
  are listed in `README.md`).
- Seeded state: homeworld id `1` at `1:1:1`, outpost id `2` at `1:2:3`, commander
  with hangar ships. Schema and seed are idempotent; re-running is safe.
- `UNIVERSE_ID` (default `universe_6_niburu`) must match the seeded universe. The
  event engine uses Redis ZSET `universe:<UNIVERSE_ID>:fleet_events`.
- Real flight times are minutes-to-hours, so you can't watch an arrival. To
  exercise resolution, re-score the event to the past:
  `docker exec -i neoxnova_redis redis-cli ZADD "universe:universe_6_niburu:fleet_events" <now_ms-1000> <fleet_id>`

## Environment
- `DATABASE_URL` is the only required variable; `config.Load` fails without it.
  All others have defaults (see `.env.example` / `Makefile`).
- `.env` is auto-loaded by godotenv when present; absence is non-fatal. The
  Makefile exports defaults, so `make run` works with no `.env`.

## Architecture notes (non-obvious)
- `cmd/server/main.go` wires Postgres → Redis → event-engine goroutine → HTTP
  server, with graceful shutdown.
- Resources are never cron-updated: `update_celestial_resources(id)` (PL/pgSQL)
  accrues on demand, takes a row lock, and returns the fresh state. Call it
  before crediting cargo.
- Fleet state is split: phase/timeline in Postgres (`fleets`), arrival score in
  Redis. A 10 Hz loop Lua-pops due fleet ids and resolves
  `OUTBOUND`/`HOLDING`/`RETURNING` in one transaction.
- `Dispatch` resolves `fleets.target_id` by coordinates. Empty-space targets stay
  `NULL`, and the engine logs an error — the fleet never resolves. Point
  transports/deploys at an existing celestial.
- Universe fleet speed is hardcoded `15.0` in `internal/game/game_math.go`;
  editing the `universes` table does not change it.
- Coordinate domains: galaxy 1-9, system 1-499, position 1-21 (21 = deep space).

## Editing gotchas
- lib/pq uses the extended query protocol once parameters are present, which
  **rejects multiple statements in one `Exec`**. Use separate `ExecContext`
  calls (see `accrueResources`/`creditCargo`/`stationShips` in
  `internal/engine/event_engine.go`).
- `fleets.target_id` is nullable. Scan into `sql.NullInt64`; scanning into
  `&x.Int64` leaves `Valid=false` and writes NULL.
- No migration framework: schema changes are numbered SQL files applied manually
  via `psql`. Keep them idempotent (`CREATE TABLE/INDEX IF NOT EXISTS`,
  `DO $$ ... EXCEPTION WHEN duplicate_object`, `CREATE OR REPLACE`).
- Commit messages follow Conventional Commits (`feat:`, `docs:`, `refactor:`).
  Do not commit unless asked.

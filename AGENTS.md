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
- `make migrate`, then `make seed` — apply `migrations/0001_init.sql` +
  `0003_build_queues.sql`, then `0002_seed.sql`.
- `make run` — `go run ./cmd/server`, listens on `:8080`.
- `make build` → `bin/server`; `make test` → `go test ./...`; `make lint` →
  `go vet ./...`; format with `gofmt -w`.
- `migrate`/`seed` shell out to `docker exec -i neoxnova_postgres psql ...`, so
  no local `psql` is needed. `make` itself and Go 1.24 may be missing on Windows
  hosts — if so, run the underlying commands directly; older Go toolchains
  auto-download 1.24 (`GOTOOLCHAIN=auto`).

## Testing / verification
- Unit tests exist only for the pure formulas in `internal/game`
  (`go test ./internal/game/`). There are no integration tests; verify runtime
  behavior against the compose stack and the API (endpoints in `README.md`).
- Seeded state: homeworld id `1` at `1:1:1` (with structures incl. a
  `research_lab` and hangar ships), outpost id `2` at `1:2:3` as a fleet target,
  commander user. Schema and seed are idempotent; re-running is safe. The seed
  does **not** reset levels you have already built — delete rows to re-seed.
- `UNIVERSE_ID` (default `universe_6_niburu`) must match the seeded universe
  (`universes.code_name`).
- The scheduler polls Postgres, so events cannot be watched in real time (flights
  are minutes and builds longer). Fast-forward by moving the row's due time into
  the past, keeping the timeline check valid:
  `UPDATE fleets SET start_time = NOW() - interval '5 seconds', arrival_time = NOW() - interval '1 second' WHERE id = <id>;`
  For `construction_queues`/`shipyard_queues`/`research_queues` set **both**
  `start_time` and `end_time` (constraint requires `start_time < end_time`).

## Environment
- `DATABASE_URL` is the only required variable; `config.Load` fails without it.
  All others have defaults (see `.env.example` / `Makefile`).
- `.env` is auto-loaded by godotenv when present; absence is non-fatal. The
  Makefile exports defaults, so `make run` works with no `.env`.

## Architecture notes (non-obvious)
- `cmd/server/main.go` wires Postgres → optional Redis → scheduler goroutine →
  HTTP server, with graceful shutdown. Redis being down is non-fatal (logged
  warning, `rdb = nil`); it is only a wake accelerator.
- **Durable scheduler** (`internal/engine/event_engine.go`): every 200 ms it
  selects due rows (`fleets.arrival_time`, `construction_queues.end_time`, etc.),
  then each resolver re-locks one row with `FOR UPDATE SKIP LOCKED` and re-checks
  its due condition. At-least-once, idempotent, multi-instance safe. Redis is
  only a wake hint (`cache.WakeKey`), not the source of truth.
- Resources are never cron-updated: `update_celestial_resources(id)` (PL/pgSQL)
  accrues on demand, takes a row lock, and returns the fresh state. Call it
  before crediting cargo or spending on a build.
- Builds: costs/durations/production live in the typed Go catalog
  (`internal/game/catalog.go`, `economy.go`). `internal/store/build_store.go`
  deducts resources and inserts a queue row atomically; the scheduler applies the
  level and calls `RecomputeCelestial` (which rewrites cached
  `*_prod_hourly`/`energy_*`/`fields_used`). Research costs draw from the supplied
  planet and require `research_lab >= target level`.
- `Dispatch` resolves `fleets.target_id` by coordinates. Empty-space targets stay
  `NULL`, and the engine logs an error — the fleet never resolves. Point
  transports/deploys at an existing celestial.
- Universe fleet speed is hardcoded `15.0` in `internal/game/game_math.go`;
  editing the `universes` table does not change it. Build durations, by contrast,
  read `universes.game_speed`.
- Coordinate domains: galaxy 1-9, system 1-499, position 1-21 (21 = deep space).

## Game data / balance provenance
- Costs in `internal/game/catalog.go` are **calibrated** from a niburuspace.com
  HAR via `cmd/harparse` → `testdata/niburus_catalog.json`; real numeric unit
  codes are used (e.g. `207` Battleship, `212` Solar Satellite). `catalog_test.go`
  locks costs to the fixture.
- Durations, unit combat stats, prerequisites and the server's custom systems
  (University, conveyors, custom research, peaceful/combat levels) are still
  approximate or unmodelled. Read `neoxnova/docs/BALANCE_DATA_NEEDED.md` before
  trusting or "fixing" balance, and read tests via `go test ./internal/game/`.
- Real server rates are seeded into `universes`: `game_speed` 4000,
  `resource_speed` 10000, `fleet_speed` 15 (player override; server advertises
  5), `debris_rate` 0.50, `base_colonies` 5. `resource_speed` scales production,
  `game_speed` divides build times, `fleet_speed` divides flight time — all read
  from `universes`, so edit that row to retune rather than hardcoding.
- Raw network captures (`*.HAR`) are gitignored and may contain session cookies;
  never commit them.

## Explorer bots (live game automation) — `neoxnova/tools/explorer/`
- Prefer the browser-less **`httpbot.mjs`** (Node `fetch` + cookie jar, ~40 MB);
  `explorer.mjs` is the Playwright version (Edge/Chromium, ~300 MB) — use it only
  for debugging/UI inspection. Run from `tools/explorer/`.
- Secrets (gitignored): `neoxnova/secrets/explorer.env` with `NIBURU_USER/PASS`
  (account #1) and `NIBURU_SECOND_USER/PASS` (account #2); SSH key + config in
  `neoxnova/secrets/ssh/`.
- Commands: `node --max-old-space-size=96 httpbot.mjs levels --out data/levels.json`;
  `... resolve --goals plans/account2-goals.json --steps 5000`; `... dump "page=research"`.
  `explorer.mjs` additionally has `scan|status|build|cancel|sats|map|officers`.
- Action POSTs: buildings `{cmd:insert,building,lvlup}`, research
  `{cmd:insert,tech,lvlup}`, shipyard `{fmenge[<code>]:N}`. Research boxes are
  `#research_<id>` (buildings `#build_<id>`).
- Requirements come from `page=techtree`, parsed into a graph; `resolve`
  recursively builds/researches prerequisites and skips locked targets.
- Account #2 runs on an Azure VM (Ubuntu, 2 vCPU/1 GB; tmux) — never run a
  browser there, only `httpbot.mjs`. Refresh `data/account2-levels.json`
  (gitignored) with `refresh-levels.ps1`.

## Session hygiene (token savings)
- Prefer a fresh OpenCode session per task; long transcripts are re-sent every
  turn. This file + `docs/` are the durable memory — put rules here, not in chat.
- Reference files by path; never paste HARs/large JSON. Captures/secrets stay gitignored.
- Run long jobs detached (tmux/nohup/Start-Process) and check with short `tail`s.

## Editing gotchas
- lib/pq uses the extended query protocol once parameters are present, which
  **rejects multiple statements in one `Exec`**. Use separate `ExecContext`
  calls (see `accrueResources`/`creditCargo`/`stationShips` in
  `internal/engine/event_engine.go`).
- `fleets.target_id` is nullable. Scan into `sql.NullInt64`; scanning into
  `&x.Int64` leaves `Valid=false` and writes NULL.
- No migration framework: schema changes are numbered SQL files applied manually
  via `psql`. Keep them idempotent (`CREATE TABLE/INDEX IF NOT EXISTS`,
  `DO $$ ... EXCEPTION WHEN duplicate_object`, `CREATE OR REPLACE`). When you add
  a schema migration, add it to the `migrate` target in the Makefile too.
- One active build per scope is enforced by partial unique indexes
  (`0003_build_queues.sql`). Build code catches pq `23505` and maps it to
  `store.ErrQueueBusy`; don't rely on read-then-write checks alone.
- Commit messages follow Conventional Commits (`feat:`, `docs:`, `refactor:`).
  Do not commit unless asked.

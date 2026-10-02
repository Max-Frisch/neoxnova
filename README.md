# Neo-XNova Game Engine

Neo-XNova is a high-performance, concurrent, and stateful space-MMO backend engine designed to modernize legacy PHP-based strategy game architectures (such as 2Moons, XNova, and OGame clones). Written in Go, it uses PostgreSQL as the transactional source of truth (including the durable event scheduler) and Redis as an optional low-latency wake-up accelerator.

## Architectural Improvements

This engine resolves critical flaws common in legacy PHP strategy game implementations:

1. **Zero-Cron Continuous Resource Accumulation**: Legacy systems relied on database-wide periodic cron loops or page-load triggers to increment player resources. Neo-XNova calculates resources deterministically on-the-fly when requested using the formula:
   `current_resources = min(capacity, stored + (hourly_rate / 3600) * elapsed_seconds)`.
   Database updates are deferred until a state-changing transaction occurs (e.g., building, queue completion, or fleet landing).

2. **Durable Event Scheduler**: Instead of processing fleet arrivals and build completions synchronously within user web requests, a background scheduler polls PostgreSQL for due rows (`arrival_time`/`end_time <= now()`) and claims them with `SELECT ... FOR UPDATE SKIP LOCKED`. This is at-least-once and safe for multiple instances; every resolver is idempotent. Redis is used only as an optional wake-up nudge — correctness never depends on it.

3. **Atomic State Transitions**: All fleet transactions (such as outbound, holding, returning, and recall actions) are protected via PostgreSQL row-level locks (`SELECT ... FOR UPDATE`). This completely eliminates race conditions, duplicate ship exploits, and "hanging fleets."

4. **Strict Schema Constraints**: Fleet ship configurations, planet hangars, defenses, and coordinates are normalized and validated through custom PostgreSQL domains and strict check constraints.

5. **Data-Driven Economy & Build Queues**: Structure, technology and ship definitions plus their cost/duration/production curves live in a typed Go catalog (`internal/game`). Construction, shipyard and research queues are persisted in PostgreSQL and completed by the same durable scheduler, which recomputes hourly production, energy and fields from structure levels.

---

## Balance Data & Provenance

Server-wide rates are seeded into the `universes` row and are the source of truth: `game_speed` (4000), `resource_speed` (10000), `fleet_speed` (5 advertised; 15 with the player override), `debris_rate` (0.50), `base_colonies` (5). `resource_speed` scales production, `game_speed` divides build durations, and `fleet_speed` divides flight time.

Structure, technology, ship and defense **costs** are calibrated from a real niburuspace.com HAR capture (XNova "GOW" theme): the extractor `cmd/harparse` parses the capture into `testdata/niburus_catalog.json`, and `internal/game/catalog_test.go` asserts the Go catalog reproduces every captured cost. Costs follow `base * factor^(level-1)`, with server specifics encoded (e.g. Crystal Mine factor 1.5, storages base 2000) and real numeric unit codes (`207` = Battleship, `212` = Solar Satellite).

Durations, unit combat stats, prerequisites and the server's custom systems are still approximate or unmodelled. See [neoxnova/docs/BALANCE_DATA_NEEDED.md](neoxnova/docs/BALANCE_DATA_NEEDED.md) for exactly what data is confirmed, what is placeholder, and which captures are needed next.

> Raw HAR captures are gitignored (`*.HAR`) because they contain session cookies — never commit them. Only the sanitized `testdata/niburus_catalog.json` fixture is committed.

---

## Directory Structure

```
/neoxnova/
├── cmd/
│   └── server/
│       └── main.go            # Thin entrypoint: config -> wiring -> graceful shutdown
├── internal/
│   ├── api/
│   │   ├── router.go          # Route registration and middleware chain
│   │   ├── middleware.go      # Request logging and panic recovery
│   │   └── handlers/
│   │       ├── handler.go     # Shared handler dependencies and JSON helpers
│   │       ├── health.go      # GET /api/v1/health
│   │       ├── planet.go      # Planet resources and overview endpoints
│   │       ├── fleet.go       # Fleet dispatch and recall endpoints
│   │       ├── build.go       # Construction / shipyard / research endpoints
│   │       └── dashboard.go   # Developer dashboard
│   ├── cache/
│   │   └── redis.go           # Redis client and scheduler wake-key helper
│   ├── config/
│   │   └── config.go          # Environment-driven configuration
│   ├── engine/
│   │   └── event_engine.go    # Durable scheduler: due-row poll + fleet/queue resolvers
│   ├── game/
│   │   ├── game_math.go       # Topology distance, flight duration, and fuel formulas
│   │   ├── catalog.go         # Static structure/tech/ship catalog and cost/duration curves
│   │   └── economy.go         # Production, energy and storage derivations
│   ├── models/
│   │   └── types.go           # Strong Go types, REST API payload structures, and enums
│   └── store/
│       ├── postgres.go        # PostgreSQL pool setup
│       ├── errors.go          # Repository sentinel and typed errors
│       ├── planet_store.go    # Planet resource/overview queries
│       ├── fleet_store.go     # Atomic fleet dispatch/recall transactions
│       └── build_store.go     # Atomic enqueue + production recompute
├── migrations/
│   ├── 0001_init.sql          # PostgreSQL 16+ DDL (schemas, constraints, functions)
│   ├── 0002_seed.sql          # Idempotent local dev seed (universe/user/planets/ships)
│   └── 0003_build_queues.sql  # Single-active-item queue indexes
├── .env.example               # Runtime environment template
├── docker-compose.yml         # Local services definition (PostgreSQL and Redis)
├── Makefile                   # run / build / test / lint / migrate / seed / up / down
├── go.mod                     # Go module definition
├── go.sum                     # Dependency checksums
└── README.md
```

---

## Getting Started

### Prerequisites
* Go 1.24 or higher
* Docker and Docker Compose

### 1. Spin up Infrastructure
Start the pre-configured PostgreSQL and Redis instances in the background:
```bash
cd neoxnova
docker-compose up -d
```

### 2. Initialize Database Schema
Apply the schema and the continuous resource accumulator function located in `migrations/0001_init.sql` to your active PostgreSQL instance. The migration is idempotent and safe to re-run:
```bash
make migrate
```

### 3. Seed Development Data
Provision a ready-to-play test universe (`universe_6_niburu`), a `commander`, a
resource-rich homeworld (`id = 1`, at `1:1:1`), a secondary colony used as a
fleet target (`id = 2`, at `1:2:3`), hangar ships, and base structures. The seed
is idempotent, so it is safe to re-run:
```bash
make seed
```
No compile step or manual `psql` queries are required — after seeding, the
homeworld is always planet id `1` for deterministic demo URLs.

### 4. Configure Environment
Copy `.env.example` to `.env`; it is auto-loaded on startup (via `godotenv`) during local development:
```bash
cp .env.example .env
```
The `Makefile` also provides sane defaults if `.env` is absent.

### 5. Run the Engine
Run the server to start both the REST API gateway and the background time-wheel event engine:
```bash
make run
# or: go run ./cmd/server
```

The web server will listen on port `8080` (override with `HTTP_ADDR`).

### Out-of-the-Box Smoke Test
With the stack up, migrated, and seeded, the seeded homeworld is always id `1`:
```bash
curl http://localhost:8080/api/v1/health
curl http://localhost:8080/api/v1/planets/1/resources
curl http://localhost:8080/api/v1/planets/1/overview
# Live auto-refreshing dashboard:
#   http://localhost:8080/dashboard/1

# Dispatch a transport from the homeworld (1) to the seeded outpost (1:2:3):
curl -X POST http://localhost:8080/api/v1/fleets/dispatch \
  -H "Content-Type: application/json" \
  -d '{"origin_planet_id":1,"target":{"galaxy":1,"system":2,"position":3,"type":"PLANET"},"mission":"TRANSPORT","ships":{"202":10},"speed_percent":100,"cargo":{"metal":100,"crystal":50,"deuterium":0}}'
```

Quick end-to-end copy/paste (Docker + migrate + seed + run):
```bash
cd neoxnova
make up && make migrate && make seed && make run
```

> `make migrate` / `make seed` apply the SQL through the `neoxnova_postgres`
> compose container, so only Docker is required (no local `psql`). To target an
> external database instead, override the container/user/name or run `psql`
> directly:
> ```bash
> psql "$DATABASE_URL" -f migrations/0001_init.sql
> psql "$DATABASE_URL" -f migrations/0003_build_queues.sql
> psql "$DATABASE_URL" -f migrations/0002_seed.sql
> ```

---

## API Endpoints Spec

The API gateway implements strict input validation and returns fully structured JSON payloads:

* **`GET /api/v1/health`**
  Returns the current system operational status.

* **`GET /api/v1/planets/{id}/overview`**
  Returns the complete current planet overview, including real-time interpolated resources, fleet event countdowns, structures, and technology levels.

* **`GET /api/v1/planets/{id}/resources`**
  Returns the real-time resource state calculated on-the-fly using the continuous resource accumulator.

* **`GET /api/v1/planets/{id}/buildings`**
  Returns structure levels, the active construction item, and next-level costs.

* **`POST /api/v1/planets/{id}/build`** `{ "structure_code": "metal_mine" }`
  Validates prerequisites, fields and energy, atomically deducts the cost, and queues the next structure level.

* **`POST /api/v1/planets/{id}/shipyard`** `{ "unit_code": "202", "quantity": 10 }`
  Atomically deducts the ship cost and queues a shipyard batch.

* **`POST /api/v1/planets/{id}/research`** `{ "tech_code": "energy_tech" }`
  Atomically deducts the tech cost and queues an empire-wide research item; the planet's research lab gates the target level.

* **`POST /api/v1/fleets/dispatch`**
  Atomically checks ship counts on the planet hangar, computes fuel costs and arrival times, and deducts the payload and ships.

* **`POST /api/v1/fleets/{id}/recall`**
  Locks the fleet, calculates the exact elapsed flight time, and reverses the trajectory.

* **`GET /dashboard/{id}`**
  A developer dashboard that serves as a visual, real-time feedback loop. It outputs the planet stats with an auto-refresh script to demonstrate the resource accumulation and event-loop engine running live.

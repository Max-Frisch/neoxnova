# Neo-XNova Game Engine

Neo-XNova is a high-performance, concurrent, and stateful space-MMO backend engine designed to modernize legacy PHP-based strategy game architectures (such as 2Moons, XNova, and OGame clones). Written in Go, it leverages PostgreSQL for strict relational data integrity and Redis for active event-wheel task execution.

## Architectural Improvements

This engine resolves critical flaws common in legacy PHP strategy game implementations:

1. **Zero-Cron Continuous Resource Accumulation**: Legacy systems relied on database-wide periodic cron loops or page-load triggers to increment player resources. Neo-XNova calculates resources deterministically on-the-fly when requested using the formula:
   `current_resources = min(capacity, stored + (hourly_rate / 3600) * elapsed_seconds)`.
   Database updates are deferred until a state-changing transaction occurs (e.g., building, queue completion, or fleet landing).

2. **Stateful Event Loop (Time-Wheel)**: Instead of processing fleet arrivals and construction events synchronously within user web requests, a dedicated background daemon polls a Redis Sorted Set (`ZSET`) where the score represents the arrival timestamp in milliseconds. An atomic Lua script pops matured event IDs, dispatching them to Go worker pools.

3. **Atomic State Transitions**: All fleet transactions (such as outbound, holding, returning, and recall actions) are protected via PostgreSQL row-level locks (`SELECT ... FOR UPDATE`). This completely eliminates race conditions, duplicate ship exploits, and "hanging fleets."

4. **Strict Schema Constraints**: Fleet ship configurations, planet hangars, defenses, and coordinates are normalized and validated through custom PostgreSQL domains and strict check constraints.

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
│   │       └── dashboard.go   # Developer dashboard
│   ├── cache/
│   │   └── redis.go           # Redis client and event-wheel key helpers
│   ├── config/
│   │   └── config.go          # Environment-driven configuration
│   ├── engine/
│   │   └── event_engine.go    # Background event loop, Redis Lua pop, and mission resolver
│   ├── game/
│   │   └── game_math.go       # Topology distance, flight duration, and fuel formulas
│   ├── models/
│   │   └── types.go           # Strong Go types, REST API payload structures, and enums
│   └── store/
│       ├── postgres.go        # PostgreSQL pool setup
│       ├── errors.go          # Repository sentinel and typed errors
│       ├── planet_store.go    # Planet resource/overview queries
│       └── fleet_store.go     # Atomic fleet dispatch/recall transactions
├── migrations/
│   ├── 0001_init.sql          # PostgreSQL 16+ DDL (schemas, constraints, functions)
│   └── 0002_seed.sql          # Idempotent local dev seed (universe/user/planet/ships)
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

* **`POST /api/v1/fleets/dispatch`**
  Atomically checks ship counts on the planet hangar, computes fuel costs and arrival times, deducts the payload and ships, and inserts the mission into the Redis event wheel.

* **`POST /api/v1/fleets/{id}/recall`**
  Locks the fleet, calculates the exact elapsed flight time, reverses the trajectory, and updates the arrival time inside the Redis ZSET.

* **`GET /dashboard/{id}`**
  A developer dashboard that serves as a visual, real-time feedback loop. It outputs the planet stats with an auto-refresh script to demonstrate the resource accumulation and event-loop engine running live.

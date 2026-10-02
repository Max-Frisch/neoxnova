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
├── engine/
│   └── event_engine.go    # Background event loop, Redis Lua pop, and mission resolver
├── models/
│   └── types.go           # Strong Go types, REST API payload structures, and enums
├── utils/
│   └── game_math.go       # Topology distance, flight duration, and deuterium fuel formulas
├── databasefile.sql       # PostgreSQL 16+ DDL (Schemas, constraints, and triggers)
├── docker-compose.yml     # Local services definition (PostgreSQL and Redis)
├── go.mod                 # Go module definition
├── go.sum                 # Dependency checksums
└── main.go                # API Gateway, route handlers, and graceful shutdown coordinator
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
Apply the schema and the continuous resource accumulator function located in `databasefile.sql` to your active PostgreSQL instance.

### 3. Run the Engine
Run the main server to start both the REST API gateway and the background time-wheel event engine:
```bash
go run main.go
```

The web server will listen on port `8080`.

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

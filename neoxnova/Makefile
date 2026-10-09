APP_ENV      ?= development
HTTP_ADDR    ?= :8080
UNIVERSE_ID  ?= universe_6_niburu
DATABASE_URL ?= postgres://postgres:password@localhost:5432/neoxnova?sslmode=disable
REDIS_ADDR   ?= localhost:6379
REDIS_PASSWORD ?=
REDIS_DB     ?= 0

# Migration targets apply SQL through the docker-compose Postgres container,
# so only Docker is required (no local psql client). Override to point at an
# external database, e.g. `make seed DB_CONTAINER=my_pg`.
DB_CONTAINER ?= neoxnova_postgres
DB_USER      ?= postgres
DB_NAME      ?= neoxnova

export APP_ENV
export HTTP_ADDR
export UNIVERSE_ID
export DATABASE_URL
export REDIS_ADDR
export REDIS_PASSWORD
export REDIS_DB

.PHONY: run build test lint migrate seed up down web web-gen web-css

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

# Server-rendered UI assets: Templ generates *_templ.go from *.templ, Tailwind
# compiles static/app.css. Both outputs are committed; run after editing views.
web: web-gen web-css

web-gen:
	"$$(go env GOPATH)/bin/templ" generate

web-css:
	npx --yes tailwindcss@3 -c tailwind.config.js -i internal/web/static/input.css -o internal/web/static/app.css --minify

test:
	go test ./...

lint:
	go vet ./...

migrate:
	docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) -v ON_ERROR_STOP=1 < migrations/0001_init.sql
	docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) -v ON_ERROR_STOP=1 < migrations/0003_build_queues.sql
	docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) -v ON_ERROR_STOP=1 < migrations/0004_academy.sql
	docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) -v ON_ERROR_STOP=1 < migrations/0005_combat_reports.sql
	docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) -v ON_ERROR_STOP=1 < migrations/0006_auth.sql
	docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) -v ON_ERROR_STOP=1 < migrations/0007_planets.sql
	docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) -v ON_ERROR_STOP=1 < migrations/0008_relocation.sql
	docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) -v ON_ERROR_STOP=1 < migrations/0009_planet_fields.sql
	docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) -v ON_ERROR_STOP=1 < migrations/0010_espionage_reports.sql
	docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) -v ON_ERROR_STOP=1 < migrations/0011_arsenal.sql
	docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) -v ON_ERROR_STOP=1 < migrations/0013_expedition_reports.sql
	docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) -v ON_ERROR_STOP=1 < migrations/0014_blueprints.sql
	docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) -v ON_ERROR_STOP=1 < migrations/0015_universes_two_galaxies.sql

seed:
	docker exec -i $(DB_CONTAINER) psql -U $(DB_USER) -d $(DB_NAME) -v ON_ERROR_STOP=1 < migrations/0002_seed.sql

up:
	docker compose up -d

down:
	docker compose down

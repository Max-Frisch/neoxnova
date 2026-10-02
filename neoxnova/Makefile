APP_ENV      ?= development
HTTP_ADDR    ?= :8080
UNIVERSE_ID  ?= universe_6_niburu
DATABASE_URL ?= postgres://postgres:password@localhost:5432/neoxnova?sslmode=disable
REDIS_ADDR   ?= localhost:6379
REDIS_PASSWORD ?=
REDIS_DB     ?= 0

export APP_ENV
export HTTP_ADDR
export UNIVERSE_ID
export DATABASE_URL
export REDIS_ADDR
export REDIS_PASSWORD
export REDIS_DB

.PHONY: run build test lint migrate up down

run:
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./...

lint:
	go vet ./...

migrate:
	psql "$(DATABASE_URL)" -f migrations/0001_init.sql

up:
	docker compose up -d

down:
	docker compose down

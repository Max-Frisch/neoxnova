#!/usr/bin/env bash
# dev-up.sh - bring up the local Neo-XNova dev stack and run the server WITHOUT
# requiring `make` (often absent on Windows hosts). Mirrors the Makefile:
#
#   1. docker compose up -d              (Postgres 16 + Redis 7)
#   2. wait for Postgres to accept connections
#   3. apply every migrations/*.sql except 0002_seed.sql, in order (idempotent)
#   4. optionally seed (--seed)
#   5. go run ./cmd/server
#
# Usage (from neoxnova/, in Git Bash or any POSIX shell):
#   bash dev-up.sh                 # migrate + run
#   bash dev-up.sh --seed          # also apply 0002_seed.sql first
#   bash dev-up.sh --migrate-only  # docker + migrations only, no server
#   bash dev-up.sh --seed-only     # docker + migrations + seed, no server
#
# Env overrides: DB_CONTAINER (neoxnova_postgres), DB_USER (postgres), DB_NAME (neoxnova).

set -euo pipefail
cd "$(dirname "$0")"

DB_CONTAINER="${DB_CONTAINER:-neoxnova_postgres}"
DB_USER="${DB_USER:-postgres}"
DB_NAME="${DB_NAME:-neoxnova}"

SEED=0
RUN_SERVER=1
for arg in "$@"; do
  case "$arg" in
    --seed) SEED=1 ;;
    --seed-only) SEED=1; RUN_SERVER=0 ;;
    --migrate-only) RUN_SERVER=0 ;;
    -h|--help)
      echo "usage: bash dev-up.sh [--seed] [--migrate-only] [--seed-only]"
      exit 0 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

command -v docker >/dev/null || { echo "docker not found on PATH" >&2; exit 1; }
command -v go >/dev/null || { echo "go not found on PATH" >&2; exit 1; }

# .env is auto-loaded by the server (godotenv) and supplies the required
# DATABASE_URL; create it from the example if missing.
if [ ! -f .env ] && [ -f .env.example ]; then
  cp .env.example .env
  echo "[i] created .env from .env.example"
fi

psql_file() {
  docker exec -i "$DB_CONTAINER" psql -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 < "$1"
}

echo "[1/4] docker compose up -d"
docker compose up -d

echo "[2/4] waiting for Postgres ($DB_CONTAINER) ..."
for i in $(seq 1 60); do
  if docker exec "$DB_CONTAINER" pg_isready -U "$DB_USER" -d "$DB_NAME" >/dev/null 2>&1; then
    echo "      ready"
    break
  fi
  if [ "$i" -eq 60 ]; then
    echo "      Postgres did not become ready in time" >&2
    exit 1
  fi
  sleep 1
done

echo "[3/4] applying migrations"
for f in migrations/*.sql; do
  case "$(basename "$f")" in
    0002_seed.sql) continue ;;
  esac
  echo "      -> $f"
  psql_file "$f"
done

if [ "$SEED" -eq 1 ]; then
  echo "[i] seeding 0002_seed.sql"
  psql_file migrations/0002_seed.sql
fi

if [ "$RUN_SERVER" -eq 1 ]; then
  echo "[4/4] go run ./cmd/server  (Ctrl-C to stop)"
  exec go run ./cmd/server
fi

echo "[done]"

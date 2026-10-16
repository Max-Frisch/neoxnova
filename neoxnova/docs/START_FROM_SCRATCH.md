# Start from scratch — fresh DB / new universe

Run everything from `neoxnova/`. `game.json` is the tuning source: on **every boot**
the server parses `config/game.json` and `UPDATE`s the `universes` row whose
`code_name == UNIVERSE_ID` (`store.ApplyGameSettings`). It does **not** create the
row — the row must exist (seed, or your own INSERT).

## A. Reset everything (nuke the DB volume)

```bash
docker compose down -v      # drops pgdata + redisdata
```

## B. Fresh standard universe (the normal path)

```bash
cp .env.example .env        # only needed once; DATABASE_URL is the required var
make up                     # Postgres 16 + Redis 7
make migrate                # schema + universe rows (0015/0016), no seed
make seed                   # universe_1_alpha + commander + planets id 1 & 2
make run                    # boot applies config/game.json onto the row
```

Playable: `http://localhost:8080/dashboard/1` (login `commander` / `commander-dev-pass`).

No `make`/Go on the host? Use the script (mirrors the Makefile):

```bash
bash dev-up.sh --seed      # docker + migrate + seed + run
```

Windows: `pwsh -File dev-up.ps1 -Seed`.

## C. New universe with a custom name

`game.json` only updates an existing row, so create it first:

```bash
# 1. create the universes row with placeholder speeds (game.json overwrites them)
docker exec -i neoxnova_postgres psql -U postgres -d neoxnova -v ON_ERROR_STOP=1 <<'SQL'
INSERT INTO universes (code_name, display_name, game_speed, resource_speed, fleet_speed,
                       debris_rate, max_galaxies, base_colonies)
VALUES ('universe_2_beta', 'Beta Universe', 1, 1, 15, 0.5, 2, 5)
ON CONFLICT (code_name) DO NOTHING;
SQL

# 2. point the server at it
#    set UNIVERSE_ID=universe_2_beta in .env
make run                   # game.json is now applied to the new row
```

`UNIVERSE_ID` must equal the row's `code_name`. Registration/homeworld spawning
reads that row; players get random slots in galaxy `1..max_galaxies`.

## D. Applying game.json without a full boot

`make run` is the normal way. To force it standalone, either start the server
once, or update the row directly (note: any later boot overwrites it):

```bash
docker exec -i neoxnova_postgres psql -U postgres -d neoxnova -v ON_ERROR_STOP=1 <<'SQL'
UPDATE universes SET game_speed=4000, resource_speed=12000, fleet_speed=15,
       build_queue_slots=10, research_queue_slots=5, merchant_rate=50, dm_market_divisor=1000
WHERE code_name='universe_1_alpha';
SQL
```

## Notes

- `0002_seed.sql` (the `seed` target) hardcodes `universe_1_alpha` and its own
  speeds; `game.json` wins on the next boot. Seed is idempotent and does not
  reset player levels.
- `make migrate` runs an explicit file list — a new migration must be added to
  the Makefile `migrate` target. `dev-up.sh` instead globs all
  `migrations/*.sql` except `0002_seed.sql`.
- `universe_test` (migration `0016`) is a fast sandbox for the web e2e test;
  switch to it with `UNIVERSE_ID=universe_test`.
- `fleet_speed` in `game.json` is stored but flight duration uses the hardcoded
  `15.0` in `internal/game/game_math.go`; production/build read the DB values.

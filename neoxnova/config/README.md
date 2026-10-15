# `config/game.json` — game tuning

A single JSON file drives the `universes` row for `UNIVERSE_ID`. It is parsed at
startup and **upserted onto the universe row on every boot** (mirrors the dev
seed / gwx "applied on every start").

## Precedence

1. **File wins on boot.** Whatever `game.json` says is written to `universes`
   when the server starts.
2. **The DB is the runtime source.** All game code reads the values from
   `universes` (e.g. `fleet_store.go`), never from the file, so editing live
   values by hand in Postgres is possible but will be overwritten on next boot.
3. **Absent/invalid file ⇒ built-in defaults**, which are `game_speed 1.0`,
   `resource_speed 1.0`, `fleet_speed 15.0`, `build_queue_slots 10`,
   `research_queue_slots 4`.
4. Unknown keys are rejected (typo guard). Missing keys keep their default.

Override the path with `GAME_CONFIG_PATH` (default `config/game.json`, relative
to the working directory — run the server from `neoxnova/`).

## Fields

| key | meaning |
| --- | --- |
| `game_speed` | build/research duration divisor (`universes.game_speed`) |
| `resource_speed` | hourly production multiplier (`universes.resource_speed`) |
| `fleet_speed` | flight-time divisor (`universes.fleet_speed`) |
| `build_queue_slots` | building chain length cap (`universes.build_queue_slots`) |
| `research_queue_slots` | research chain length cap (`universes.research_queue_slots`) |

Ships and defenses have **no slot cap** (unlimited back-to-back chains), so they
have no key here.

## Speeds

The shipped values match the calibrated reference server (Niburuspace:
game 4000x, resource 10000x). Flight time scales as `1/fleet_speed`: the
reference advertised **5x** fleet speed, but the in-game player modifier is
**15x**, so flights here are ~3x faster than a 5x server. This applies to all
ship flights *and* the expedition round trip. The expedition **hold window** is
player-chosen (1–24 h) and is intentionally **not** scaled by fleet speed.
Covered by `internal/game/game_math_test.go`
(`TestFlightDurationScalesWithFleetSpeed`).

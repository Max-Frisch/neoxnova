# Balance Data — What Is Calibrated and What Is Still Needed

This file tracks the provenance of the game-balance numbers in
`internal/game/catalog.go` / `economy.go` and lists the additional captures
required to finish calibration.

## Source of calibrated data

- Capture: `niburuspace.com_Archive [26-10-02 20-16-23].har` (local only, gitignored).
- Server: **XNova, "GOW" theme** — `niburuspace.com`, universe `universe_6_niburu`.
- Account/planet captured: `BobbyVerse`, planet **Fogigy [1:45:15]**, 24,633 km, 331/706 fields.
- Server rates: Game **4000x**, Resource **10000x**, Fleet **5x** (player override **15x**), debris **50%**, 5 free colonies, max 35.
- Extractor: `cmd/harparse` → `testdata/niburus_catalog.json` (sanitized, no cookies).
- Regression test: `internal/game/catalog_test.go` asserts every captured next-level
  cost and (roughly) structure durations.

Costs for structures/techs/ships/defenses are **exact** for the captured level,
because unit costs are derived as `base = observed_cost / factor^level` with the
standard factor for each item. Durations are **approximate** (see below).

## Confirmed data

- Structure/tech costs follow the standard `base * factor^(level-1)` curve.
- Metal/Crystal/Deuterium mine and Solar plant use factor **1.5**; most other
  buildings and techs use factor **2.0**.
- Crystal Mine uses factor **1.5** (not the usual 1.6).
- Storages use base **2000** (not 1000).
- Production formula confirmed: `30/20/10 * level * 1.1^level * resource_speed`
  (matches the resources page for Metal Mine L48 and Crystal Mine L46).
- Real unit codes captured: e.g. `202` Light Cargo, `207` Battleship,
  `212` Solar Satellite, `219` Battle Recycler, `401` Missile Launcher.

## Needs more recordings (currently approximated / dummy)

1. **Build duration formula.** We only have one Robotics (22) / Nanite (11) /
   University (6) snapshot, so `buildingTimeCalibration` (2.25) is an empirical
   fudge. Needed: captures at several Robotics/Nanite/University levels to solve
   the real exponents. Nanite Factory's own build time does not fit the curve.
2. **Research duration.** Off by large, inconsistent factors across techs
   (`researchTimeCalibration` = 0.0156 is a rough median). Likely depends on
   Research Lab and/or University per tech. Needed: research page at several lab
   levels, plus University levels.
3. **Ship/defense build times.** Not present in the capture (duration field empty
   for shipyard rows). Needed: a shipyard page while affordable so the duration
   renders, or a started-then-cancelled build.
4. **Unit combat & flight stats.** Only `information&id=1/219/401` were captured.
   We still need per-unit **Attack, Shield, Hull, Base speed, Cargo capacity,
   Fuel consumption, Rapid fire** for all ships/defenses. Needed: open the
   `information&id=<code>` dialog for every unit, and a few `battleSimulator`
   runs. This also lets us replace the hardcoded `baseSpeed := 10000` in
   `internal/store/fleet_store.go`.
5. **Prerequisites / requirements.** The build pages used don't expose the
   requirement tree. We currently use standard OGame prerequisites. Needed:
   the building/tech detail dialogs (`Dialog.info`) or the techtree page data.
6. **Shipyard/tech code coverage.** Custom units with zero cost in the capture
   (`218` Avatar, `221`, `222`, `227`, `228`, several defenses) may require
   special resources/events. Needed: a capture when they are buildable.

## Server-custom systems that affect base systems (not yet modelled)

These appear in the capture but their effects are unknown:

- **University** (id 6): likely a research booster.
- **Deuterium Power Plant** (id 12): energy source.
- **Light/Average/Heavy Conveyor** (ids 71/72/73): the overview mentions
  "small/medium/large production factory" +N units/second — possibly these.
- **Brother Hood** (tech 125), **Mineral / Semi-Crystals / Fuel Research**
  (techs 131/132/133), **Graviton Research** (199).
- **Peaceful / Combat levels** with bonuses (+% energy, +units/second, combat %).
- **"Basic Production"** flat bonus (30,000,000 / 20,000,000 / 10,000,000 per hour).
- **Energy Technology / Plasma Technology** production bonuses.
- **Storage capacities** and **field** growth (Terraformer).
- **Debris field** generation/recycling at 50%.

## How to capture more data

1. Browser DevTools (F12) → **Network** → enable **Preserve log** and
   **Disable cache**.
2. Log in as your account, then visit each page in turn:
   `/game/game.php?page=buildings`, `...?page=research`, `...?page=shipyard&mode=fleet`,
   `...?page=shipyard&mode=defense`, `...?page=resources`, `...?page=techtree`.
3. Open the info dialog (`?`) for each unit/building/tech you want stats for.
4. Right-click the request list → **Save all as HAR with content**.
5. Drop the file in `neoxnova/` (gitignored) and tell the assistant the filename.

Re-run the extractor:

```
go run ./cmd/harparse -in "<path>.har" -out testdata/niburus_catalog.json
```

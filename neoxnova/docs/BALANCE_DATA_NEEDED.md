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

## Confirmed mechanics (owner-provided + captured)

- **Building ratios** the owner targets: metal ~2-3 levels above crystal, deuterium
  ~3-4 below crystal; solar plant just high enough to cover energy.
- **Queue locks**: ships/defenses lock the shipyard + nanite factory (and vice
  versa); research and University are mutually exclusive; robot factory is
  independent.
- **Cancel** a queued building and 100% of resources are refunded.
- **Solar satellites**: once build time <= 1s, queue 100-200 at a time (shipyard
  `fmenge[<code>]` form).
- **Debris**: 50% of the *base* resource cost of destroyed ships piles up at the
  coordinates; recyclers/battle recyclers collect it; expedition survivors return
  it (a single surviving transporter can carry it all).
- **Do NOT spend Dark Matter to instantly finish buildings/research/ships** — it's
  a waste for our purposes.
- **Premium account (24h new-account bonus)**: +1000% resource production,
  +100% research speed, +5 construction/research queue slots, +100% experience.
  The premium page exposes a `pblist` of purchasable bonuses with costs/factors.
- **Bonus systems**: University +16% research speed; Deuterium Power Plant (energy
  for deuterium); Light/Average/Heavy conveyor batch-build their unit class;
  Brotherhood = alliance bank deposit/withdraw limit; Mineral/Semi-Crystals/Fuel
  Research +5% metal/crystal/deuterium production per level; Energy Technology
  +10% energy per level (account-wide); Plasma Technology +2% damage (plasma
  weapons); Graviton Research +4% damage and costs the planet's max energy;
  Terraformer +7 fields/level (also moons); "Basic Production" is a static
  per-planet baseline.
- **Unit stats**: every ship/defense info page carries weapon type(s)+attack,
  structural armor, shields, engine+base speed, fuel, cargo and rapid-fire. A
  `tools/explorer` scan captures these for all 26 ships and 21 defenses.
- **Senate**: parent page linking Officers and Governators.
  - **Officers** (page=officier), recruited with Dark Matter, empire-wide effects:
    Geologist 0/30 (production), Admiral 0/20 (combat), Engineer 0/10 (energy of
    all colonies), Technocrat 0/10 (research), Constructor 0/3 (construction
    speed), Scientologist 0/3 (tech), Minister of Defence 0/2 (defence build).
    Tooltip descriptions captured in `testdata/niburus_senate_officers.txt`.
  - **Governators** (page=gubernators), cost **Dark Matter + Achievement Points**
    and a number of **days**, each level +10%: Weapons (atk+armor, 0/65, 40k DM),
    Shield (0/65, 40k), Building (-10% build time, 0/50, 7.5k), Resource (+10%
    extraction, 0/250, 30k), Energy (+10%, 0/100, 10k), Research (0/40, 25k),
    Fleet (+10% flight time, 0/20, 50k). Strategy: activate at the default % for
    as many days as possible first, raise the % later — cheaper in Dark Matter.
- **Peaceful level** (sidebar): grants ~+1%/level to mine extraction, research
  speed and energy production, plus fleet slots (e.g. L14 = +14% each, +1 slot).
  Combat level tracks combat XP. Achievements award Antimatter (not Dark Matter).
- **Do not spend Dark Matter** on officers/governators or instant finishes during
  data collection; effects must be measured against the resources tab instead.
- **Achievements** (see `testdata/niburus_achievements.json`): 47 definitions in
  groups General/Daily/Buildings/Research/Fleet/Defense/Misc; each grants
  **Antimatter + Achievement Points**, with an increasing "tier". Examples:
  "Metal Miner" at Metal Mine 51 → 1217 AM / 122 pts; "Geologist" needs all three
  extraction researches at 22. Note: achievements award Antimatter, not Dark
  Matter directly.

## Duration calibration (updated)

Base build-time constant fitted from the **fresh Bratwurst** account (Robotics 15,
Nanite 0) over 6 samples: `buildingTimeCalibration = 1.20` in
`internal/game/catalog.go`. The older officer/peaceful-buffed Fogigy capture runs
**~1.88x faster** than this base — so its durations are not directly comparable.

Still unconfirmed: the exact Nanite-Factory speedup exponent (we only have
Nanite 0 and Nanite 11 samples, and the latter is confounded by officer bonuses).
To separate them we need a fresh-account capture after building a Nanite Factory,
or durations at a known Nanite level with no officers.

## Arsenal / Academy / endgame

- **Arsenal** (`page=arsenal`, raw text in `testdata/niburus_arsenal.txt`):
  expedition-found **upgrades** give flat bonuses per category, e.g. weapon types
  (Laser +0.75, Ion +0.75, Gravitational +0.75, Plasma +0.75), armor (Light +0.6,
  Medium +0.5, Heavy +0.4), shields (Light +0.6, ...), engines (Jet +0.6, ...).
  Upgrades become findable at specific fleet-point thresholds (~5k / 25k / 150k).
- **Academy**: not captured (only appears as a nav link) — needs a `page=academy`
  capture. Academy points likely come from Peaceful levels and/or Achievements and
  give additive (or multiplicative) combat bonuses; vital for beating pirates and
  aliens. TBD.
- **Online Bonus** button (above "Premium account"): appears periodically and
  grants Dark Matter + Peaceful-level experience.
- **Premium** options affect: peaceful XP gain, how often the Online Bonus button
  appears, expedition count/speed, expedition resource/fleet/upgrade finds, and
  combat XP. See the premium page's `pblist` (captured in scan output).

## Academy / Online Bonus / Officers (mapped live)

Raw page text saved under `testdata/niburus_map/`.

- **Academy** (`page=academy`): a skill tree with per-branch tiers (I/II/III).
  Skills cost **Academy points** and are upgraded via GET
  `?page=academy&mode=up&skil=<id>`; branches reset for 5000 ?? / Antimatter /
  Dark Matter (50% refund). Confirmed nodes: `1101 Weaponry` +1% Attack/level,
  `1201 Production speed` +5% mine extraction/level, `1301 Defensive strategy`
  +1% shields+armour/level; deeper nodes (e.g. Academy of Sciences +1 IRN,
  Expanding Empire +1 planet) are gated by other nodes. **Academy points can be
  obtained by donation** (`page=academy&mode=donation`, POST) and from Peaceful
  levels/achievements (17 observed on one account).
- **Online Bonus** (`page=bonus`): clicking/visiting grants **Dark Matter +
  Peaceful XP** (observed: 8,838 DM + 11% / 4 XP). Premium options can raise the
  XP gained and how often the button appears.
- **Officers** (`page=officier`): recruit via POST `game.php?page=officer` with
  `id=601..607` (Geologist/Admiral/Engineer/Technocrat/Constructor/
  Scientologist/Minister of Defence), each level ~1,000 DM at level 1. Only
  Geologist + Admiral were unlocked on the test account. **Measured**: Geologist
  L1→L2 gave ~**+0.17%** to all three resource productions; Admiral L1→L2 had no
  economic effect on the resources tab (combat only). See
  `testdata/niburus_officers_experiment.json`.
- **Energy matters**: after a bad build order the planet hit a 100% energy
  deficit, which zeroes effective production (and masked the officer delta) until
  Solar Plants were raised — solar must cover energy before measuring bonuses.

## Planet relocation / "teleport" (captured — exact linear price)

- **Implemented** 2026-10-06 (`POST /api/v1/planets/{id}/relocate`,
  `internal/game/relocation.go`, migration `0008_relocation.sql`). Moves a planet
  (with its structures/ships/defenses/stored resources) to new coordinates for a
  Dark Matter fee; it does **not** lock the origin slot.
- **Source**: the niburuspace.com **Planetarium** (Black market → "Planetarium":
  *"increase field count and teleport your planet"*), captured on 2026-10-06 into
  `docs/screenshots_niburu/` (14 PNGs; planet `3:125:12`). The galaxy view's
  `mode=savecord`/`delcord` are unrelated bookmarks.
- **Price is exactly linear and per-axis** (no distance formula):
  ```
  cost = 15000·|Δgalaxy| + 1000·|Δsystem| + 2500·|Δposition|
  ```
  Captured samples: `3:125:12→3:125:13` = 2500, `→3:126:12` = 1000,
  `→3:127:14` = 7000, `→2:125:12` = 15000, `→2:127:14` = 22000. The constants are
  `RelocationDMperGalaxy/System/Position` in `internal/game/relocation.go`;
  `relocation_test.go` locks every sample.
- **Rules** (from the Planetarium page):
  - Teleport a planet **once per hour**, but **unlimited teleports within the same
    system** (cooldown only applies on a system/galaxy change) — enforced in
    `store.RelocatePlanet` via `game.RelocationLeavesSystem`.
  - After teleport: **no attacking for 15 min** (modelled: `attack_locked_until`
    set on the planet for `store.AttackLockoutAfterRelocation`; `FleetStore.Dispatch`
    rejects ATTACK missions with `ErrAttackLocked`), **phalanx sensor offline
    10 min** (*not modelled* — no phalanx subsystem yet).

### Dark-Matter field expansion (Planetarium, captured)

- Implemented 2026-10-06: `POST /api/v1/planets/{id}/fields` (body `{"fields":N}`),
  `internal/game/fields.go`, migration `0009_planet_fields.sql`. Works on planets
  and moons (the reference allows both); moons are not implemented in the engine yet.
- The Planetarium's *"Increase the amount of fields on the current planet"* box
  charges a **geometric** price, independent of the planet's base size. Captured
  cumulative costs: +1 **200**, +2 **420**, +3 **662**, +4 **928**, +5 **1221**,
  +6 **1543** DM → marginal 200·1.1^(n-1). Closed form:
  ```
  cost(purchased, additional) = round(2000 · 1.1^purchased · (1.1^additional − 1))
  ```
  `fields_bought` tracks prior DM purchases so buying many fields in one go equals
  buying them one at a time (no arbitrage). A purchase folds into `base_fields_max`
  (and `fields_max`) so `RecomputeCelestial` preserves it across builds.
- The same page also has **"Increase the diameter"** — a *different* system that
  grants +diameter/+fields using **Debris field (planets or moons) + Stardust**
  (capture: `Diameter +276..+414`, `Fields +12..+18` for 100.0B M + 50.0B C debris
  + 1 Stardust). Stardust sources (trade/buy, rare expedition finds) unknown.
  **Not implemented** — needs a debris-spend path and a Stardust currency.

## Explorer tool

`neoxnova/tools/explorer/` (Node + playwright-core, drives installed Edge):

```
node explorer.mjs scan                 # crawl all info + build pages (read-only)
node explorer.mjs status               # current building/research/shipyard levels
node explorer.mjs build --steps 150    # economy-first build plan (mutates account)
node explorer.mjs cancel               # clear the construction queue (100% refund)
node explorer.mjs sats 150             # queue N solar satellites (shipyard fmenge[212])
node achievements.mjs <file.har> [out] # offline achievement extraction
```

Credentials live in the gitignored `neoxnova/secrets/explorer.env`.

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

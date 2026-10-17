# Playbook — Moon Collider + Empire Overview (capture triage + session split)

Source captures (Oct 2026): `neoxnova/info/{Moons,Empire view,Academy,Bonus Button Peaceful}`.
This is a **planning doc**; implement one session at a time, each as its own `docs/BACKLOG.md`
item. Next free migration number: **0025**.

---

## 0. Capture triage — is the new data useful?

### Moons folder — **NEW + highly useful**
`MoonCollider.har` + 4 screenshots. First-ever capture of the **Collider** (`page=kollaider`,
building sprite `gebaeude/69.gif`). Confirms it is a moon building bought with **Antimatter**
that outputs **Dark Matter/hour**, independent of moon size. L0/L1/L2 all captured. Verdict:
**enough to model S1–S3; production curve needs 2–3 more levels** (cheap ATM).

### Empire view folder — **NEW + highly useful**
`Empire_view.har` + 2 screenshots of `page=imperium`. Full per-celestial matrix (resources,
production incl. **Dark Matter/h**, 25 buildings, 29 ships, 24 defenses) with a **Σ Total**
column and all owned celestials in **creation order**. Not previously captured in this repo
(no `imperium`/empire references in `docs/`). Verdict: **spec-complete for S4–S5**; only the
exact CSS/asset styling needs to be copied from the ops repo.

### Arsenal (folder is labelled "Academy") — **useful**
The folder is named `Academy` but the intended feature is the **Arsenal**; the HAR/screenshots are
`page=arsenal` (upgrades) + `page=shipyard&mode=fleet|defense` + `page=resources`. Useful because it:
- re-confirms the 19-upgrade catalog + per-level increments (matches `internal/game/arsenal.go`);
- adds two missing `greid` keys: **`h_conveyer`** (Heavy conveyor) and **`m_production`** (Metal
  production); activation = `POST game.php?page=arsenal` `mode=send&greid=<key>` → info page
  "Upgrade successfully activated!";
- shows Arsenal levels carry a **pre-existing base bonus** at owned level 0 (e.g. Heavy conveyor
  +2.8% = 7×0.4, Metal production +4% = 8×0.5) → the "Bonus" shown is `(activations)×PerLevel`, not
  `PerLevel`.
No Academy capture is needed for this run — the target was the Arsenal.

### Bonus Button Peaceful folder — **useful, but not complete**
7 click HARs + screenshots + premium page. New/valuable:
- **actual payout results** at peaceful levels 3 / 4 / 76 / 77:
  L3 → 4666 & 9683 DM (3–4% XP, 1 pt); L4 → 7990 & 9267 DM (10% & 6%, 1 pt);
  L76 → 20318 DM (16%, 17 pt) & 66284 DM (16%, 34 pt); L77 → 58779 & 69905 DM (20%, 34 pt).
- the **premium feature list** with live multipliers: `Button "Bonus" ×4`, `Update of "Bonus"
  button +100%`, `Amount of earn experience +100%`, `Construction and research speed +49%`,
  `Amount of "Expeditions" +5`, `"Expeditions" speed +155%`, `Power of light conveyor +200%`,
  `Getting combat experience +55%`, `Generation collider`, `Moon creation`, etc.
- online-time curve points: L3→4 ≈ 4210 s, L4→5 ≈ 4460 s, L76→77 ≈ 210 600 s (no premium) /
  105 300 s (with premium), L77→78 ≈ 222 000 s. Confirms the current linear estimate at high
  levels but **not** at low levels — and the premium state is **inconsistent across captures**
  (some are exactly half), so the curve cannot be fit cleanly from this set.
Verdict: **good enough to re-fit the payout and add the premium multipliers; still missing
mid-levels (5/10/15/20/25/50) and a premium-state-tagged time series.**

> Net: Moons + Empire are actionable now. Arsenal is mostly confirmation + 2 `greid` keys.
> Peaceful is better but still needs a deliberately premium-tagged capture run.

---

## 1. Collider — live spec (from `page=kollaider`, 2026-10-17)

- Moon-only passive building, numeric id **69**, internal page name **`kollaider`**.
- Bought/levelled with **Antimatter** (account-wide `users.antimatter`).
- Produces **Dark Matter per hour**, independent of moon size/fields (Empire overview lists it
  under "Production per hour → Dark Matter" while the **Buildings** matrix has **no** Collider row).
- Description (verbatim): *"After years of study of antimatter, scientists have finally invented
  a way to create dark matter. It consists in breaking up antimatter collider. But the work of the
  collider prevent the planet's atmosphere, so it was decided to build it on the moon, where the
  atmosphere does not exist."*
- Page shows **the next level's** production and the **next level's** ATM cost.

### Captured levels (page at level L shows level L+1)

| current level | next-level production DM/h | next-level cost ATM |
|---:|---:|---:|
| 0 | 4 752 | 850 |
| 1 | 10 454 | 1 148 |
| 2 | 17 250 | 1 549 |

- **Cost curve is clean:** `cost(L) = round(850 · 1.35^(L-1))` (1148/850=1.3506,
  1549/1148=1.3493). Treat ×1.35 as authoritative.
- **Production curve is under-determined.** 3 points fit a quadratic through origin
  `≈ 998·L² + 3754·L` (L2 off by ~10%) or a 3-parameter `≈ 547·L² + 4061·L + 144` (exact but
  arbitrary). **Do NOT lock a formula** — capture L3–L6 first, then fit. Ship a small lookup
  table (`[4752, 10454, 17250, …]`) + tunable, same style as `peaceful.go`.
- Multipliers (hooks only, verify later): combat-level tooltip advertises **"+3% to the
  development of the collider"**, and the premium list has **"Generation collider"**.
- Production is real-time (DM stock ticks continuously, like a resource).

### Open questions
1. Production formula (need L3+ captures).
2. Exact effect % of the combat-level "development of the collider" bonus and the
   "Generation collider" premium multiplier.
3. Does the cost curve cap / does production continue past L0 cap like fields?
4. Is the Collider shown anywhere on the moon page nav, or only via `?page=kollaider`?

---

## 2. Empire Overview — live spec (`page=imperium`)

A single matrix of **all owned celestials in creation order** (same order as the planet selector),
preceded by a **Σ Total** column. Columns = celestial; rows = stat groups:

- Header: `Name` / `Coordinates` / `Fields` (`used / max`, e.g. `515 / 825`) / `Go` (per-column links).
- **Resources**: Metal / Crystal / Deuterium / Energy (current).
- **Production per hour**: Metal / Crystal / Deuterium / **Dark Matter**.
- **Buildings** (25 cols): Metal Mine, Crystal Mine, Deuterium Refinery, Solar Power plant,
  University, Deuterium Power Plant, Robot Factory, Nanite Factory, Shipyard, Metal/Crystal/
  Deuterium Storage, Research Lab, Terraformer, Alliance Depot, Moon base, Phalanx Sensor,
  Jumpgate, Missile Silo, Planetarium, Sensor module, Research Center, Light/Average/Heavy conveyor.
- **Ships** (29 cols): Spy Probe, Solar Satellite, Light/Heavy Cargo, Light/Heavy Fighter, Recycler,
  Cruiser, M7, Battleship, Colony Ship, Battle Transporter, Battle Cruiser, Star Fighter, Planet
  Bomber, M19, Dark Matter Collector, Battle Recycler, Scrappy, Galleon, destroyer, Battle Fortress,
  Black Moon, M32, Frigate, Black Wanderer, Flying death, Avatar, Battleship class ONill.
- **Defenses** (24 cols): Missile Launcher, Light Laser Turret, Iron Mehador, Heavy Laser Turret,
  Ion Cannon, Gauss Cannon, Plasma Cannon, Hydrogen Gun, Grand Mehador, Dora Gun, Photon Cannon,
  Lepton Gun, Graviton Cannon, Proton Gun, Particle Emitter, Mehador Slim, Canyon, Quantum Gun,
  Small/Large Shield Dome, Atmospheric Shield, Orbital Defence Platform, Interceptor,
  Interplanetary missiles.
- Nav: left/right paging + **filters** `Все`(All) / Building / Fleet / Defense (client-side, `empire.js`).
- Two bulk links: `game.php?page=reduceresources` ("Take away from the planets: resources") and
  `game.php?page=reducefleet` ("…: Fleets").

Notes for us: our catalog already covers most buildings/ships/defenses; new names to check
(`Sensor module`, `Research Center`, `Planetarium`) and the DM/h column ties into S3.

---

## 3. Session split (one BACKLOG item each)

### S1 — Collider catalog + pure math *(no DB)*
- New `internal/game/collider.go`: `Collider` def (id 69, moon-only), `ColliderCost(level)` =
  `round(850·1.35^(level-1))`, `ColliderProduction(level)` lookup table + tunable, plus multiplier
  hooks (`CombatLevel`, `PremiumGenerationCollider`).
- Extend `game.MoonOnlyStructures`/`MoonStructureDefs` so the Collider is moon-legal (but flag it
  as **not field-occupying**, unlike 41/42/43).
- Tests: cost series 850/1148/1549; production table; moon-only enforcement.

### S2 — Collider persistence + store
- Migration `0025_collider.sql`: `celestial_objects.collider_level INT NOT NULL DEFAULT 0`
  (idempotent; add to Makefile `migrate`).
- `store.MoonStore` (or new `ColliderStore`): `UpgradeCollider(celestialID)` row-locks the user,
  checks/spends `users.antimatter` (`ErrInsufficientAntimatter`), increments level.
- `RecomputeCelestial` / moon overview carries `collider_level` + `dm_per_hour`.
- Tests: insufficient ATM rejected; level increments; production read.

### S3 — Collider production accrual + page/UI
- DM accrual: add `collider_dm_prod_hourly` (or compute from `collider_level`) into the resource
  state / `update_celestial_resources` path so `dark_matter` ticks account-wide at the collider's
  DM/h; surface `DarkMatter`/`Antimatter` already exist in `RealtimeResourceState`.
- New page `GET /collider/{moon}` mirroring `page=kollaider` (title, sprite 69, next-level
  production + ATM cost, Build button), plus a moon-page nav link. `POST /collider/{moon}/build`.
- Tests: build requires moon + ATM; production shows in resbar/DM ticker; render test.

### S4 — Empire overview v1 (core matrix)
- `GET /empire`: load all owned celestials **ordered by creation**, compute
  resources + production (incl. DM/h) + fields; render the Σ column + Resources/Production/Fields
  rows. Reuse `RealtimeResourceState`/`RecomputeCelestial` outputs — no new math.
- Tests: ordering, Σ totals, DM/h aggregation.

### S5 — Empire overview v2 (full + filters + perf)
- Add the Buildings / Ships / Defenses matrices, the All/Building/Fleet/Defense filters
  (pure CSS/JS toggle), the per-column `Go` links, and the two bulk-action links
  (`reduceresources`/`reducefleet`) as either stubs or real routes.
- Perf: gzip/ETag on the (large) response; avoid N+1 (one query joining
  `planet_buildings`/`planet_ships`/`planet_defenses` per celestial).
- Tests: section toggles render; full-matrix render on the sandbox.

### S6 — Peaceful calibration (follow-up, from Bonus captures)
- Re-fit `bonusButtonDarkMatterBase/Slope` + `ProgressPct` against the L3/L4/L76/L77 payouts;
  add premium multipliers (`Button Bonus ×4`, `Update of Bonus button +100%`, XP +100%).
- Refit the online-time curve with the new low-level points (L3≈4210 s, L4≈4460 s) while keeping
  the high-level linear fit; model premium time-reduction explicitly (captures differ by exactly 2×).
- **Blocked on** a premium-tagged capture run at 5/10/15/20/25/50.

### S7 — Arsenal `greid` keys (small, optional)
- Add `h_conveyer` → Heavy conveyor and `m_production` → Metal production to `UpgradeByKey`
  (`internal/game/arsenal.go:64`), and document the "base bonus already present at level 0" rule.

### Capture requests (ops repo, not this repo)
- Collider at levels 3–6 (cheap ATM) to pin the production curve.
- Peaceful tooltips + bonus clicks with the premium panel visible / premium off, at
  5/10/15/20/25/50.
- Raw `imperium` HTML/CSS assets if the visual reskin matters.

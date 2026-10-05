# Combat Model — Scout/Test Notes for neoxnova

Status: **data collection in progress**. Goal: capture enough of niburuspace.com's
(server `universe_6_niburu`, XNova "GOW" theme) combat mechanics to design a fast,
correct, bug-free Go combat engine for neoxnova. neoxnova currently has **no combat
engine** (`MissionAttack` exists in `internal/models/types.go` but
`resolveFleetEvent` in `internal/engine/event_engine.go` has no ATTACK case).

Everything here is empirical, reproduced via the in-game battle simulator and real
attacks between **acc1 Bratwurst planet `2:188:9`** and **acc2 TheBob planet
`2:188:16`** (distance `1035`, same galaxy/system).

## Tooling

- `tools/explorer/httpbot.mjs simsuite <scenarios.json> [--out dir]`
  — runs an array of `{id, notes?, slots?, attacker:{code:count}, defender:{code:count}}`
  scenarios through `page=battleSimulator&mode=send`, fetches
  `CombatReport.php?raport=<hash>`, and writes per-scenario
  `<id>.input.json`, `<id>.report.html`, `<id>.report.json` plus an `index.json`.
- `parseCombatReport(html)` in `tools/explorer/parse.mjs` extracts:
  per-round attacker/defender unit snapshots (`count`, `lost`, derived
  `firepower`/`shield`/`armour`), per-round damage summary
  (attacker firepower, defender shield absorbed, defender firepower, attacker
  shield absorbed), final result, loss totals, debris (metal/crystal), moon chance.
- Scenario files live in `tools/explorer/plans/combat/`.
- Raw captures are gitignored under `tools/explorer/data/combat/`.

Simulator input codes: `1xx` = technologies (`109` weapons, `110` shields,
`111` armour, `120` laser, `121` ion, `122` plasma, `199` graviton), academy
skills `1103/1108/1109/1110/1111/1303/1308/1311`; `2xx` = ships; `4xx` = defenses.
On the **defender** slot, codes `1/2/3` are lootable Metal/Crystal/Deuterium.
`slots` selects multi-party (ACS) battles.

## Findings (confirmed)

### F1 — Server PHP bug: simulator crashes on attacker win with incomplete loot
The simulator POST returns the generic "Error / Administration notified" page
(no report) whenever the **attacker wins** and the defender's resource vector is
**not all three non-zero**. Evidence:
- `204:600` attacker vs `204:100` defender, no resources → crash.
- defender `{1:1, 204:100}` (metal only) → crash; `{2:1}`/`{3:1}` → crash.
- defender `{1:1,2:1,3:1, 204:100}` → works (attacker win).
- defender `{1:1e6,2:1e6,3:1e6, 204:100}` → works.
- defender wins / draws with zero resources → works.
This is almost certainly a divide-by-zero while formatting the
"Needed to capture the resources" loot line (tests single-resource / zero paths).
**Go must not inherit this**: guard all loot/cargo maths against zero totals.

Workaround for testing: always give the defender `{1:1,2:1,3:1}` (or more) except
in the dedicated loot tests.

### F2 — Deterministic
Identical simulator inputs produce byte-identical outcomes (3× repeats of both a
draw and an attacker win gave identical rounds/losses/debris). So a Go engine can
reproduce results exactly, and we do **not** need to model RNG distributions —
only the same deterministic target/rapid-fire resolution.

### F3 — Tech bonus is QUADRATIC (server-custom, not +10%/level)
Weapons (`109`), Shields (`110`) and Armour (`111`) all use the **same** curve.
Measured bonus on a Light Fighter, tech levels 1..20:

```
L:   1  2  3  4  5  6  7  8  9 10 11 12 13 14 15 16 17 18 19 20
%:   1  2  4  6  9 12 16 20 25 30 36 42 49 56 64 72 81 90 100 110
```

Fits exactly: `bonusPercent = round(L * (L + 2) / 4)`
(= round(L²/4 + L/2); increments ≈ (L+1)/2 %/level).
At L15 this is +64%, which matches the live accounts' displayed `+64%`. The three
techs are **additive/independent** (109=110=111=10 → fp+30, sh+30, ar+30).

### F4 — Derived per-unit stats & rounding
For a unit with base attack A, base shield S, base hull/armour H at tech levels
`w,s,a`:
- firepower = round(A * (1 + wbonus/100))
- shield    = round(S * (1 + sbonus/100))
- armour    = round(H * (1 + abonus/100))
Base values are exposed at tech 0 (e.g. Light Fighter 204 → 50 / 35 / 400).
Rounding observed: 35*1.3 = 45.5 → 46 (half up); 35*1.64 = 57.4 → 57.

### F5 — Academy skills DO affect combat (not shown in the card header)
The single-unit probes (`acad-*`) looked identical because the effect is below
one unit's displayed rounding, but on a large battle it is real. Removing/adding
academy codes to the big-battle sim changes defender losses:
baseline sats 1336 → `1103:5` 1337 → `1109:5` 1364 → `1111:5` 1378 →
**all 8 skills at 5: 1445 sats** (and Gauss cannons went from 0 to some losses).
So the simulator's academy inputs
(`1103/1108/1109/1110/1111/1303/1308/1311`) are genuine damage modifiers.
**Gotcha:** they are *not* reflected in the per-unit `Firepower/Shield/Armour`
header (which only reflects `109/110/111`), so they must be modelled separately.

### F18 — Weapon-type techs (120/121/122/199) DO affect damage
Same story: they don't change the card header, but removing them from the big
battle changed resolution (sats lost 1398 → 1336, heavy lasers 17 → 24, BS 0 → 1
when added). Laser/Ion/Plasma/Graviton techs modify the per-weapon-type damage
amount. **Go must apply them per weapon type**, even though the info card /
report header doesn't display it.

### F20 — Large real battle matches the simulator
Real acc1 strike `300 LF + 200 HF + 150 Cruiser + 80 BS + 40 BC` → acc2:
- **Real**: defender wins, 5 rounds, attacker wiped; debris `M5,336,500 / C5,424,500`.
- **Sim** (same comps + techs): defender wins, 5 rounds, attacker wiped; debris
  `M5,329,000 / C5,445,750`. Per-unit defender losses within 1–2 units (differences
  explained by the exact live sats/defence counts). Strong end-to-end validation,
  including academy + weapon-type techs.

### F6 — Debris = 50% of base Metal+Crystal of destroyed ships
- 1000 Solar Satellites (`212`, base M0 C4000) destroyed → debris C 2,000,000
  (= 1000 * 4000 * 0.5). Same for 2000 sats → C 4,000,000.
- 1 Battleship (`207`, M41000 C17000) destroyed + 1 Light Fighter lost →
  debris M 22,000 / C 9,000 (= BS 20500/8500 + LF 1500/500).
So both sides' losses contribute; defenses TBD (see next tests).

### F7 — Report round structure
Each round block shows unit counts at **start of round** and losses **during** that
round. Max observed rounds: **8** in the live sample, and the current tests resolve
well before a cap; "draw" happens when neither side is wiped within the cap
(observed draws ending at round 9 in one sample). Exact cap TBD (sweep).

### F8 — Base combat table (tech 0), from simulator
Captured for all buildable ships/defenses (`base-*` scenarios). Ships
(attack/shield/hull):

```
202 LightCargo 0/20/400     203 HeavyCargo 0/50/1200    204 LightFighter 50/35/400
205 HeavyFighter 150/100/1100 206 Cruiser 400/105/2450    207 Battleship 700/190/5800
208 Colony 0/100/45000       209 Recycler 0/10/1600      210 SpyProbe 0/0/750
211 PlanetBomber 1400/480/11500 212 SolarSat 0/0/400     213 StarFighter 2000/500/11000
214 BattleFortress 150000/50000/950000                   215 BattleCruiser 1400/430/9000
216 BlackMoon 135000/60000/1200000                       217 BattleTransporter 40/120/5500
219 BattleRecycler 0/1000/160000                         220 DMCollector 0/50000/13000000
225 Galleon 20000/8000/160000 226 Destroyer 65000/23000/500000
227 Frigate 470000/190000/4000000                        228 BlackWanderer 1500000/500000/12000000
```
Defenses (attack/shield/hull):
```
401 MissileLauncher 80/200/200   402 LightLaser 100/250/200    403 HeavyLaser 250/1000/800
404 Gauss 1100/2000/3500         405 Ion 150/5000/800          406 Plasma 3000/3000/10000
407 SmallShieldDome 0/2000000/200000  408 LargeShieldDome 0/10000000/1000000
409 AtmosphericShield 0/1000000000/150000000  410 GravitonsCannon 500000/800000/3000000
411 OrbitalDefencePlatform 1e9/5e9/7e8  412 LeptonGun 400000/500000/1500000
413 ProtonGun 900000/1000000/4300000  414 Canyon 2500000/5000000/14000000
415 QuantumGun 8000000/15000000/43000000  416 HydrogenGun 9000/7000/35000
417 DoraGun 12000/7000/50000     418 PhotonCannon 70000/60000/375000
419 ParticleEmitter 1510000/1450000/7500000
```
Cargo/utility ships have **attack 0** on this server (Light Cargo, Heavy Cargo,
Colony, Recycler, Spy, Solar Sat, Battle Recycler, DM Collector). `502` Interceptor
and `503` IP Missiles crash the simulator (`base-def-502/503` → PHP_ERROR); treat
as special. This matches `data/scan-*.json` (`stats.weapons[].attack`, `shield`,
`armor`) — the scan's `atk` column was just parsed from `weapons[]`.

### F9 — Defenses produce NO debris
`rf-211-401` (Bomber wins vs 500 Missile Launchers) → debris `0/0`.
`def-204-401` (1000 LF lose 16 LF, kill 100 ML) → debris `M24000/C8000` = exactly
the 16 LF (16×1500 / 16×500); the 100 destroyed Missile Launchers contributed
**zero**. Debris is ships-only, at 50% of base M+C.

### F10 — Bounce confirmed
A unit whose shot can't beat a target's shield does nothing:
`bounce-204-408` (100 LF vs Large Shield Dome 10M shield) → draw, `0/0` debris;
`bounce-402-408` (100 Laser Turrets vs LSD) → defender wins 1 round, LSD intact.
So the classic "damage < 1% of shield → bounce" style rule is in effect (exact
threshold still to pin down).

### F11 — Rapid fire works (extra shots)
`rf-206-204-all1`: 100 Cruisers vs 600 LF → attacker wins in 3 rounds; losses
600 LF + 4 Cruisers (debris M930000/C319000). Control `ctrl-204-204`
(600 vs 600 LF) draws for 9 rounds. So Cruiser's RF-vs-LF (6) is applied.
RF data per unit is already in `data/scan-*.json`.

### F12 — Real attack baseline (acc1 2:188:9 → acc2 2:188:16)
Fleet `50 Light Fighter + 10 Cruiser`, mission Attack, speed 100%:
`Distance 1.035`, `Consumption of deuterium 398`, displayed `Fleet speed 56.198`.
Target(Arrival) `04 Oct 2026 07:24:08`, Return(Back) `07:29:11` (one-way ≈ 5 min;
sent ≈ 07:19). neoxnova's current `CalculateDeuteriumConsumption` predicts ≈119
for this (fuel base 50×20 + 10×300 = 4000 → ×1035/35000 → +1) — **off (~3.3×)**;
and `CalculateFlightDuration` with fleet speed 15 is far too slow. These need
recalibration from several distance/speed/fleet-size samples.

**Fuel samples** (all `2:188:9 → 2:188:16`, distance **1035**, speed 100%):
| fleet | Σ base fuel | reported fuel | ratio |
|---|---|---|---|
| 1 Battle Recycler (219 ×1, fuel 300) | 300 | 36 | 0.120 |
| 100 Light Fighter (204, fuel 20) | 2000 | 238 | 0.119 |
| 10 Battleship (207, fuel 250) | 2500 | 297 | 0.119 |
| 50 LF + 10 Cruiser (206, fuel 300) | 4000 | 398 | 0.0995 |
Single-type fleets give `fuel ≈ 0.119 × Σ base` at this distance; the mixed fleet
is lower (0.0995), suggesting the per-type fuel or a fleet term needs more samples.
One-way time ≈ **5 min** for these (LF/BS/BC), so `distance 1035` flight ≈ 300 s
with the live speed. **Need**: multiple distances (same system, different system,
different galaxy), several speed settings, and larger fleets to solve both
formulas; record engine techs (115/117/118) with every sample.

### F13 — ACS / multi-slot: unresolved
The public simulator page exposes `slots` + an "Add ACS-Slot" button, but posting
`battleinput[0][2][...]` with `slots=3` did **not** add a second attacker (report
still showed only Attacker Nr.1 / Defender Nr.1). The exact multi-slot POST format
must be captured from the browser (`action=moreslots`). Deferred — not on the
critical path for a first Go engine.

### F14 — Real attack vs simulator validated (end-to-end)
Real attack `50 LF + 10 Cruiser` (acc1, real techs) → acc2 `2:188:16`:
- Attacker message: "Your fleet was destroyed. Contact with fleet is lost." — fleet
  wiped, **no return**, no loot.
- Simulator (`real-acc1-on-acc2`, real techs) also says `defender` wins in 2 rounds.
- **Real debris field appeared at `2:188:16`**: `Metal 150,000 / Crystal 106,500`.
  That equals the attacker's full loss (50 LF → 75k/25k + 10 Cruiser → 75k/47.5k =
  150k/72.5k) plus **17 Solar Satellites** destroyed on the defender
  (17×4000×0.5 = 34k crystal). Sim with techs predicted 16 sats (104,500 C) — a
  1-sat rounding difference; tech-0 sim matched exactly (106,500 C). Strong
  validation that the simulator reproduces real battles.
- Note: `CombatReport.php?raport=<msgID>` is **not** the real report; the real
  attack's message body carried no report link (destroyed attacker). Attacker
  combat messages load via
  `game.php?page=messages&mode=view&messcat=3&site=1&ajax=1`.

## Still to test (planned)

- Per-unit base table for all buildable ships/defenses (attack/shield/hull) via
  tech-0 single-unit sims, cross-checked against `data/scan-*.json`
  (`stats.weapons[].attack`, `shield`, `armor`, `rapidfire`).
- Round-resolution details: target selection, damage assignment, shield regen,
  bounce threshold (damage < 1% of target shield?), overkill carry vs discard.
- Rapid fire: extra-shot counts, chains, vs mixed targets.
- Defenses: do they fire, do they make debris, do they repair after battle?
- Loot: fraction & capacity; which resource first.
- Moon chance formula (from debris).
- Multi-slot (ACS) battles (`slots=3`).
- Real attacks: flight time / fuel vs `CalculateFlightDuration` /
  `CalculateDeuteriumConsumption` (`internal/game/game_math.go`), debris field
  spawn + recycle with `219`, defense repair across two attacks, recall.

### F16 — Loot = 50% of each resource, capped by attacker cargo
The report's "Needed to capture the resources: N Light Cargo" reveals the loot.
- defender `1M/1M/1M`, attacker cargo `1,050,000` (200 LC + 1000 LF) → 210 LC
  (= 1,050,000) — cargo-capped.
- defender `1M/1M/1M`, attacker cargo `10,050,000` → 300 LC (= 1,500,000 = 50%
  of 3M) — resource-capped.
- defender `10M/10M/10M` (30M) with small cargo → still 1,050,000.
So `loot_r = min(floor(0.5 * stored_r), remaining_cargo)`, taken metal→crystal→deut.
(Test only via attacker-wins; remember the all-three-resource workaround.)

### F17 — Moon chance always 0% in simulator
Even with 60M debris (`moon-f`), `Moon Chance: 0 %`. Moons appear disabled/not
modelled in the public simulator (no moon observed in the galaxy either). Do not
rely on simulator moon data; treat moon generation as server-off / TBD.

### F15 — Recycle mission flow (RESOLVED)
Recycling **must target the debris field (type 2)**, not the planet (type 1).
`fleet 2:188:16 8 219:1` failed with the planet type; after sending
`planet_type=2`/`type=2` in checkTarget + step2 it succeeded:
`Mission Recycle, Distance 1.035, Fleet speed 58.445, Consumption 36`,
Target `07:33:44`, Return `07:38:41`. Use the **Battle Recycler (219)**, not the
small Recycler (209). (httpbot `cmdFleet` now auto-selects type 2 for mission 8.)
**Result: debris field at `2:188:16` is gone after the recycler arrived** (collected,
recycler returning) — end-to-end debris→recycle loop confirmed.

### Note — engine tech does NOT affect combat
The simulator takes no engine-tech inputs (`115/117/118`), and combat resolution
is speed-independent. Engine research only affects flight time/fuel, so it is safe
to keep researching engines while capturing combat data (just record engine levels
alongside any flight-time calibration samples).

## Status snapshot
- Batches run: `01-scaling`, `02-diag`, `03-workaround`, `04-workaround2`,
  `05-techsweep` (60), `06-basestats` (43), `07-resolution` (30), `08-acs`,
  `09-realbattle`. Raw captures in `tools/explorer/data/combat/`.
- Folded dataset: **`testdata/niburus_combat.json`** (183 scenarios, 41 base
  units, 30 recorded PHP-error scenarios). Regenerate with
  `node tools/explorer/combat-dataset.mjs`.
- Validated: determinism; tech curve; base table; debris = 50% ships-only;
  bounce; rapid fire; loot = 50% capped by cargo; real battle == simulator;
  debris spawn + recycle at target; moon chance always 0.
- Open: defense repair across two real attacks; exact bounce threshold operator;
  rapid-fire extra-shot rule (deterministic form) + chain/cap; overkill carry;
  ACS POST format (`action=moreslots`); academy-skill effect (F5); flight-time
  & fuel recalibration (record engine levels).

## Go engine design (concrete)

Data model (extend `internal/game/catalog.go`):
```go
type UnitDef struct {
    Code, Name  string
    BaseCost    Cost
    Requires    map[string]int
    Attack      int            // F8 table
    Shield      int
    Hull        int
    Speed       int
    Fuel        int
    Cargo       int
    RapidFire   map[string]int // keyed by target numeric code
}
```

Pure resolver (`internal/game/combat.go`):
```
func Resolve(atk, def Fleet, atkTech, defTech Techs, opts) Report
```
Algorithm (matches observed behaviour):
1. Derive each side's per-unit `attack/shield/hull` via F3/F4 rounding.
2. Repeat up to `maxRounds` (observed draws end ~round 9; sweep to confirm cap):
   a. Reset shields to full for all surviving units (F-rounds).
   b. Attacker fires: each unit picks a random target type weighted by live count.
      For each shot: damage = attackerAttack; apply to target shield then hull;
      if shield>0 and damage < bounceThreshold*shield → bounce (no damage) (F10);
      on hull<=0 destroy the unit (overkill does not carry);
      rapid fire: repeat with probability `RF[targetCode]/(RF+1)` or as extra shots
      until a non-triggered roll (confirm exact rule) — bounded loop + shot cap.
   c. Defender fires identically.
   d. Stop if either side has no units.
3. Outcome: attacker / defender / draw (max rounds).
4. Debris = floor/round(0.5 * base M+C) summed over destroyed **ships only**
   (F6/F9); defenses contribute 0.
5. Loot (TBD), moon chance (TBD), defense repair (TBD).

Guardrails so we never inherit PHP bugs (F1 etc.):
- Never divide by a resource total / cargo / count / shield without a `> 0` check;
  handle "no loot", "no cargo", "zero shield/hull", "single resource type".
- All maths integer, explicit rounding; bounded rapid-fire loop (no recursion
  blow-up); explicit round cap.
- `502`/`503` special units: model them explicitly (they crash the reference sim).

### F21 — Per-round firing model (core)
Each surviving unit fires **once per round at a single enemy unit**:
- damage is applied to that unit's shield (regenerated to full each round), then
  its persistent hull; if a shot kills it the excess is **discarded** (overkill lost).
- Evidence: `rfrule-destroyer` (5 Destroyers, 65,000 attack, no RF vs LF) destroyed
  only **38 of 2000 LF over 9 rounds** (~4/round) — one target per ship per round.
  `overkill-214-hf10` (1 BF, 150,000 attack) killed 1 HF/round. Same in every test.
- Therefore a **meat shield works**: a big pool of cheap ships soaks one shot per
  enemy capital per round; capitals only clear a screen efficiently via rapid fire.

### F22 — Rapid fire drives screen-clearing, not raw damage
`rfrule-hf` (300 HF, **no** RF vs LF) still beat 600 LF — 150 dmg × 300 = 45k/round
vs LF 435 HP ≈ 6 rounds. `rfrule-cru` (100 Cruiser, RF6) killed 600 LF in 3 rounds;
`rf-bc-lf` (100 BC, RF4) cleared 2000 LF in 7 rounds; `rf-bm-lf` (2 Black Moon,
RF180) cleared 2000 sats/LF instantly. Extra-shot counts are **deterministic per
run** here (fixed seed), but the effective shots are lower than the nominal RF
(6 → ≈2 effective kills/round vs LF because an LF needs 2 hits). Model RF as extra
shots with the OSS probability and validate by replay.

### Meatshield ratio (quantified)
Attacker `100 BS` + screen `LF` vs defender `50 BS + 1000 LF`, BS losses:
| screen LF | attacker BS lost | result |
|---|---|---|
| 0 | 100 | defender |
| 250 | 100 | defender |
| 500 | 33 | **attacker wins** |
| 1000 | 9 | attacker |
| 2000 | 2 | attacker |
| 4000 | 0 | attacker |
Reverse direction is **identical** (defender screen protects its BS the same) →
no defender bonus, and the crossover is ≈ **5 screen ships per capital**.

## Comparison with open-source OGame / 2Moons / XNova
| mechanic | OSS (OGame/2Moons) | this server | take |
|---|---|---|---|
| max rounds | 6 | **8** | 8 |
| tech bonus | +10%/level linear | **quadratic round(L(L+2)/4)%** | server formula (configurable) |
| round order | all attackers then all defenders | same (see per-round blocks) | OSS |
| shot model | 1 shot/round/target, overkill lost | **same** | OSS |
| shield regen | full each round | **same** | OSS |
| bounce | damage < ~1% target shield | **same** | OSS |
| rapid fire | probabilistic (rf-1)/rf extra shots | deterministic here (fixed seed); nominal RF | OSS probabilistic + seedable RNG |
| debris | 50% of M+C (ships) | **same, ships only** | OSS |
| defenses rubble | none | **none** (confirmed) | OSS |
| loot | 50% per resource, cargo-capped | **same** | OSS |
| moon | ~1% per 100k debris (cap 20%) | always 0% observed | OSS (server appears off) |
| weapon-type techs | modify per-weapon damage | **same (hidden in card)** | server |
| academy skills | buffs | **real damage modifiers (hidden)** | server |
| earth-to-moon/ACS | supported | simulator ACS POST unknown | defer |

**Recommended Go model:** standard OSS round engine (1 shot/round, shield regen,
overkill lost, bounce, probabilistic rapid fire on a seedable RNG) but with this
server's constants — **8 rounds**, the quadratic `109/110/111` bonus, per-weapon
damage from `120/121/122/199`, academy modifiers, 50% ships-only debris, 50%
cargo-capped loot, no moon. All integer, all zero-guarded, plus a rapid-fire
shot cap and round cap. Replay `testdata/niburus_combat.json` as the acceptance
test (RNG-sensitive scenarios tagged so they can be compared statistically).

## Go engine design notes (older, to fold in)

- Extend `UnitDef` (`internal/game/catalog.go`) with
  `Attack, Shield, Hull int; Speed, Fuel, Cargo int; RapidFire map[string]int`.
- New `internal/game/combat.go`: pure `Resolve(attacker, defender, techs) Report`
  implementing the round loop; **all integer maths with explicit zero-guards**
  (never divide by cargo/shield/hull/count without a guard), a **bounded**
  rapid-fire loop, and an explicit round cap. Mirror F3/F4 rounding exactly.
- `internal/game/combat_test.go` replays `testdata/niburus_combat.json`
  (parsed dataset, committed) — the same pattern as `catalog_test.go` locks costs.
- Wire `MissionAttack` into `resolveFleetEvent`, plus debris-field creation,
  recycling, loot and defense repair in the relevant stores.
- Explicitly document each PHP quirk (F1, and any found later) and how Go avoids
  it, so bugs are not inherited.

## Open questions for the owner
- Are academy/graviton combat bonuses supposed to matter? (F5 showed no effect.)

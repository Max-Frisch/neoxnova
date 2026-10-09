# Expeditions — Go calibration playbook (next session)

Owner-directed port of the 2026-10-09 live findings (`EXPEDITIONS_LIVE_2026-10-06.md`
§11). Pure resolver lives in `internal/game/expedition.go`; persistence in
`internal/engine/event_engine.go` (`resolveExpedition` ~898). **Nothing here is
implemented yet** — this is the work order.

Baseline numbers from §11 (acc1, n=772 outcomes / 36 expo fights, gen≈82 =
`MirroredResearchBonus`):

- outcome mix live: ships 29.1, resources 20.5, DM 14.8, delay 11.1, combat 8.9,
  nothing 7.9, fast 5.1, fatal BH 1.68, positive BH-loot 1.7, stardust 0.1 %.
- mirror def/atk: **pirates 0.60–0.69 (med 0.64)**, **aliens 0.88–0.94 (med ~0.90)**.
- enemy strength roll (×gen): pirates 12–148 % (med 64 → ~0.8×), aliens 90–202 %
  (med ~1.1×, tail ~2.5×).
- template counts (36 fights): 203 6–191, 204 1–101, 206 1–92, **207 4–97**, 213 1–46;
  215 1–21 / 216 1–3 only on aliens.

## Resolved — fatal BH rate (owner, 2026-10-09)

Owner picked the measured **1.68 %** (13/772) over the old 0.5 % lock ("0.5 % is not
dangerous enough"). Item 1's table already applies it; **no blocker remains**.

---

## 1. Outcome weights — `expeditionOutcomeWeights` (`expedition.go:83`)

Owner: nothing → 7.9 %, fast → 4.0 %, adjust ships/resources/DM to fit; keep
combat 15 %; add the new positive BH-loot (item 6); leave delay at the current 6 %.

Ship this table (sums to ~1.000; normalize after edit), live ships/resources/DM
proportions preserved and the fatal BH at the newly chosen **1.68 %**:

| outcome | old | **new** |
|---|---|---|
| resources | .310 | **.2028** |
| ships | .210 | **.2879** |
| combat | .150 | .150 |
| nothing | .160 | **.079** |
| darkmatter | .085 | **.1465** |
| delay | .060 | .060 |
| fast | .020 | **.040** |
| blackhole-loot (new) | — | **.017** |
| blackhole (fatal) | .005 | **.0168** |

- Sums to exactly 1.0000 (ships/resources/DM carry the remainder = live 29.1:20.5:14.8
  proportions). Update the comment block (`:77`) and keep the sum-to-1 assertion.
- Optional: use live delay ~10.4 % instead of 6 % (then ships/resources/DM become
  .268/.189/.136). Flagged, not assumed.

**Test:** extend/keep the existing weight-sum test; add a distribution test that a
seeded sample lands within a few % of each target.

## 2. NPC mix — `npcWeights` (`expedition.go:99`)

Live combat split 58 pirates : 11 aliens = **84 : 16** (was 70 : 30). Set
`{NPCPirates, .84}, {NPCAliens, .16}` and update the comment.

**Test:** seeded pirate/alien share.

## 3. Enemy mirror + strength — `expeditionEnemy` (`expedition.go:377`)

Split the single `factor` by NPC and retune both rolls against `gen` (owner: pirates
weaker overall; aliens rarely weaker, usually ~same, sometimes stronger, rarely much
stronger).

- **Mirror factor** (per fleet):
  - pirates: `0.60 + rng.Float64()*0.09` → 0.60–0.69.
  - aliens: `0.85 + rng.Float64()*0.09` → 0.85–0.94.
- **Strength roll** (`roll`, multiplied by `gen=MirroredResearchBonus`):
  - pirates (skewed low): `0.10 + 1.70*math.Pow(rng.Float64(), 1.5)` →
    p10≈0.15 / med≈0.70 / p90≈1.55 / max 1.8 (matches live).
  - aliens (centred just above 1, rare tail): `0.70 + 1.80*math.Pow(rng.Float64(), 2)`
    → min 0.70 / med≈1.15 / max 2.5 (2 samples only; tune after more data).
- Keep `MirroredResearchBonus` (109/110/111 only) unchanged.

**Test:** replay the §11 report set (attacker 227+hull) through `Resolve` and assert
winner/rough loss; add a seeded percentile test for each roll.

## 4. Widen the template bands — `expeditionTemplate` (`expedition.go:110`)

Owner: widen for more randomness. Replace with (inclusive min–max):

| code | old | **new** |
|---|---|---|
| 203 | 40–120 | **20–180** |
| 204 | 10–40 | **5–90** |
| 206 | 20–60 | **5–85** |
| 207 | 4–15 | **4–90** |
| 213 | 8–20 | **5–45** |

Optional: alien-only extra `{"215",0–20}` and `{"216",0–3}` (215/216 otherwise only
arrive via the mirror of our 1-each comp; 216 is no longer in the drain set).
Keep the template independent of fleet size (matches live).

**Test:** bounded-range assertions.

## 5. Dark matter scales with fleet points — `RollExpedition` (`expedition.go:143`)

Owner: DM should scale on sent fleet size/points like resource finds, not be a flat
`100 + rng.Int63n(4900)`. Model it on `expeditionLoot` but keyed on `FleetPoints`:

- `DarkMatter = round(FleetPoints(fleet) * (lo + rng.Float64()*(hi-lo)))`, clamped
  to a sane floor, plus a small flat base if the live floor is non-zero.
- Calibrate lo/hi so the acc1 expo fleet (~10–13k Frigates) yields the §11 window
  **1,553–7,044, median ≈ 3,642** (compute `FleetPoints` of that fleet to derive the
  constant; today's flat floor 100 is probably too low).

**Test:** seeded sample for two fleet sizes shows a proportional median.

## 6. Rare *positive* black hole — new outcome `ExpeditionBlackHoleLoot`

Owner: **implement** — a rare positive BH that multiplies resources ("drawn into the
black hole … the resources became much more", 13/772 ≈ 1.7 %).

- Add `ExpeditionBlackHoleLoot ExpeditionOutcome = "blackhole-loot"` (`:31`) and the
  weight from item 1 (~.017).
- `RollExpedition`: new case computing `res.Loot` as a **boosted** resource find —
  e.g. `expeditionLoot(capacity, rng)` scaled by `1.5–3×` (or a point-scaled lump);
  carrier must still cap at cargo.
- **Engine:** no structural change needed — it does not match `ExpeditionBlackHole`,
  so it hits the `default` branch (`event_engine.go:975`) and credits cargo normally.
  Verify the log/report line and that `persistExpeditionReport` stores it.
- `ExpeditionMessage`: add the "multiplied resources" text (item 7 catalogue).
- The 2 "close-ups of an opening black hole" msgs are a **separate cosmetic**
  observation flavour (treat as nothing/resources with a flavour string), not loot.

**Test:** forced-seed roll yields loot > a normal resources find; engine integration
credits cargo and does not wipe the fleet.

## 7. Cosmetic flavour catalogue — `ExpeditionMessage` (`expedition.go:290`)

Owner: add (players like cosmetics). Today each outcome returns one fixed string;
pick a **random flavour** per outcome from the live `sys_expe_*` set:

- ships: predecessor / armada / war-wrecks / starbase / lost-fleet
- resources: asteroid belt / supply depot / bacterium / virus / (non-fatal) BH-loot…
- delay: particle-storm / red-giant / collision / navigator / navigation-module / missed-target
- nothing: life-form / yellow-fever / supernova / red-anomaly / emptiness / still-nothing
- fast: relay / wormhole / solar-wind
- combat: roleplay intro ("heavy fights with unidentified pirate ships") + win/loss
- blackhole: the 4 fatal texts (hyperspacejump / nuclear breach / Zzrrt radio / encountered)
- blackhole-loot: "resources became much more"

`ExpeditionMessage` is pure over `ExpeditionResult`; add a `Flavor int` (or `uint16`)
field chosen inside `RollExpedition` so the message is deterministic and the engine
persists it verbatim. Harvest the exact strings from `data/_lang_FLEETphp` +
`data/messages.json` (§10/§11).

**Test:** each outcome maps to a non-empty title/body; flavour index bounded.

## 8. Stardust — deferred

Tracked separately as BACKLOG item 7 (rare currency + debris-based diameter/fields
sink). Not part of this pass.

---

## Verification checklist

- `go test ./internal/game/... ./internal/engine/...` (DB tests need `DATABASE_URL`).
- Update any test that asserted the old weights/mirror/template ranges.
- `cmd/exposim` sweep: confirm expected pirate loss ≈ live (median ~24 %) and alien
  tail can still wipe; sample the new outcome distribution.
- Commit split: (a) weights+npc+enemy+template+DM+positive BH, (b) flavour catalogue.

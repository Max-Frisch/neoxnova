# Moons — live findings + formulas (item 4)

Status: **partial.** Formulas + the moon building catalog are captured; creation,
destruction, jumpgate and phalanx are not wired into the engine yet. acc1 now has
**two** live moons (`3:125:12` + `2:188:9`), so jumpgate/phalanx testing is
unblocked for a later session.

## Live facts (acc1)

- Moon #1 `cp=1725`, planet `3:125:12` (same coordinates), **diameter 8,426 km**
  (captured 2026-10-07).
- Created by combat at a **20 %** chance (owner flew 5 attacks of **2500 Battle
  Recyclers** `219`; the moon appeared on the 3rd/4th lost fight). 2500 is not a
  threshold — the 20 % cap needs only ~2,000,000 debris (~3 Battle Recyclers).
- Moon #2 `cp=1772` `2:188:9` (Xusyty), **diameter 8,544 km** (owner-observed
  2026-10-08). It coexists with the planet `cp=1648` (also Xusyty, 2:188:9).
  Created by 4 failed + 1 successful attempt, each **5000 Battle Recyclers**
  (double #1); the moon `cp` is not listed by `httpbot planets` (planets only) —
  grab it from the planet overview's moon link. Diameter is *not* proportional to
  the recyclers sent: chance is capped at 20 % either way, and the classic
  diameter `floor(√(x+3p)·1000)` only varies with the random `x` (10..20) →
  8,366–8,944 km. 8,544 ⇒ `x=13` (vs `x=11` for #1).
- **Moon #2 snapshot (2026-10-08, nothing built):** all moon structures 0
  (`14/15/21/34/41/42/43/71/72/73 = 0`), 0 ships/defenses/resources, and
  **fields `0 used / 3 max` (Free: 3)** — enough for the first Moon base.
  Captures: `data/moon2-acc1-{levels.json,overview.html,buildings.html}`.
- **Moon #1 fields: 62 used / 63 max** (`Fields occupied: 62 from 63, Free: 1`).
- Buildings: `14 Robot Factory 10`, `15 Nanite 13`, `21 Shipyard 10`,
  `34 Alliance Depot 0`, `41 Moon base 20`, `42 Phalanx 0`, `43 Jumpgate 0`,
  `71 Light conveyor 9`, `72/73 = 0`.
- Defenses on the moon: `401 Missile Launcher` ×43,661,592, `402 Light Laser`
  ×38,739,864. Research shown on the moon page is the **account-wide** research
  (moons do not research separately). New unit names seen: `502 Interceptor`,
  `503 Interplanetary missiles` (not in our catalog yet).
- Demolition exists on moon buildings (50 % refund).

## Creation (standard OGame, matches the live moon)

Chance from the debris field (`internal/game/combat.go:MoonChance`):
```
moon chance % = min( floor( (metal + crystal) / 100000 ), 20 )
```
2,000,000 debris = 20 %. **Server nuance (observed 2026-10-08): only the debris
created by the moon-forming battle itself counts.** acc1's moon #2 had a debris
field already on the planet from the 4 prior failed attempts, yet the chance used
only the successful attack's new debris — the accumulated field did NOT push the
chance higher (it was capped at 20 % regardless). The current engine already does
this (`resolveAttack` feeds only `res.DebrisMetal/Crystal` into `MoonChance`).

Diameter (`game/moon.go:MoonDiameterKm`):
```
diameter = floor( sqrt(x + 3*p) * 1000 ) km
x = uniform integer 10..20, p = creation chance %
```
p=20 → 8,366..8,944 km (our 8,426 = x=11). p=1 → 3,605 km. **Verified.**

## Fields (`game/moon.go:MoonFieldsMax`)

Live Moon base info card: *"each level increases the free fields on the moon by
3"*, *"one field occupies itself Moon Base"*. A fresh moon starts at **1** base
field (2026-10-08: the fresh moon #2 shows `0 used / 3 max` — 1 base + the
account's `+2` premium), so its first Moon base can be built immediately.
```
capacity = 1 + 3 * Moon base_level + external bonuses   (premium "+N fields on
                                                          the moon", Planetarium DM)
occupied = sum of every building level on the moon
```
Live check: moon #2 `1 + 0 + 2 = 3` max, occupied `0`, free `3`; moon #1
`1 + 3*20 + 2 = 63` max, occupied `62`, free `1`. The classic `(diameter/1000)^2`
(70 here) is **not** used by this server — kept as `game.MoonFields` for
reference only. Model: `game.MoonFieldsMax(L) = 1 + 3L` (premium external).

## Moon building catalog (`game/moon.go`)

All costs are base values; level-N cost = base · 2^(N-1) (verified against the
live next-level prices). Requirements are standard OGame (not all re-measured).

| id | building | metal / crystal / deuterium | requires |
|---:|---|---|---|
| 41 | Moon base | 20,000 / 40,000 / 20,000 | — |
| 42 | Phalanx Sensor | 20,000 / 40,000 / 20,000 | Moon base 1 |
| 43 | Jumpgate | 2,000,000 / 4,000,000 / 2,000,000 | Moon base 1, Shipyard 1 |

Moon-legal **planet** buildings (reuse their planet costs): `14 Robot Factory`,
`15 Nanite Factory`, `21 Shipyard`, `34 Alliance Depot`, `71/72/73 conveyors`.
**Not** moon-legal: mines, power plants, Terraformer, Missile Silo (44) — the live
moon page omits them entirely.

## Jumpgate (`game/moon.go:JumpgateCooldown`) — needs a 2nd moon

**acc1 now has a 2nd moon (`2:188:9`) → cooldown/eligibility measurable next
session.** Live info card: instant transfer between your moons; **requires >= 2
jumpgates**;
**resources cannot be transported**; base recharge **>= 1 h**, *"with each level,
cooldown [is] reduced by 2 times"* → modelled as `3600s >> level`. PROVISIONAL;
the deuterium mention in the card is dubious (standard OGame is fuel-free).

## Phalanx Sensor (`game/moon.go:PhalanxRange`)

Live info card range formula: **`level^2 - 1`** systems. Costs deuterium per scan;
does not see recalled fleets. The documented *phalanx offline 10 min after
teleport* (`docs/BALANCE_DATA_NEEDED.md`) is still unmodelled.

## Destruction (PROVISIONAL)

Standard OGame by Battle Fortress (`214`):
```
destroy moon %      = (100 - sqrt(S)) * sqrt(D)
Death Stars lost %  = sqrt(S) / 2          (one roll for the whole fleet)
```
S = diameter km, D = Battle Fortresses. At S=8,426: 8.21 %/BF, ~150 for 100 %,
~45.9 % fleet-loss. Attacker must first win the battle; no debris is created.
Server rule: a moon **>= 10,000 km cannot be destroyed**. The live Moon base card
adds *"each 2 [levels] reduce [destruction] by 3 %"* →
`game.MoonBaseDestructionReduction(L)` (multiplicative, provisional); Moon base 20
= −30 %.

## Open questions

1. **Destruction measurement**: Battle Fortress count vs diameter, additive vs
   multiplicative Moon-base reduction, and the >= 10,000 km rule.
2. **Jumpgate** exact cooldown curve + eligible ships + the deuterium question
   (needs a 2nd moon on the account).
3. ~~Whether the *accumulated* debris or only the creating battle's new debris
   drives chance + diameter.~~ **RESOLVED 2026-10-08** (moon #2): only the
   creating battle's new debris; a pre-existing field does not add to the chance
   (and diameter ignores recycler count once the 20 % cap is reached).
4. Phalanx deuterium cost per scan and the post-teleport offline window.
5. Engine: resolve `ATTACK` on a `MOON` target, moon production safety, overview/
   galaxy/dashboard, and the fleet wizard's moon targeting.

## Implementation

- `internal/game/moon.go`: `MoonDiameterKm`, `MoonCreation`, `MoonFieldsMax`,
  `MoonFields`, `PhalanxRange`, `JumpgateCooldown`, `MoonDestruction`,
  `MoonBaseDestructionReduction`, `MoonStructure`/`MoonOnlyStructureByID`.
  Creation chance is `combat.go:MoonChance`. All tested.
- **Creation wired** (`engine/event_engine.go:resolveAttack`): an ATTACK on a
  player-owned `PLANET` rolls `game.MoonCreation(res.MoonChance, seed)` after the
  battle; on success it inserts a `MOON` at the same coordinates (0 fields, no
  production, cold/inherited temp) with `ON CONFLICT DO NOTHING` (one moon per
  planet). The result is recorded on `CombatResult.MoonCreated`/`MoonDiameterKm`
  (in the report JSON).
- Remaining: `DESTROY_MOON` mission, Jumpgate, Phalanx, moon build/overview API,
  and `resolveAttack` against a `MOON` target. Tracked in `docs/BACKLOG.md` item 4.

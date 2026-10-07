# Moons — live findings + formulas (item 4)

Status: **partial.** Formulas below are captured; the moon building catalog and
creation/destruction wiring are not implemented yet. acc1 has the only live moon.

## Live facts (acc1)

- Moon `cp=1725`, same coordinates as its planet (`3:125:12`).
- **Created via combat** at a **20 %** moon chance. Owner flew 5 attacks of
  **2500 Battle Recyclers** (`219`); the moon appeared on the 3rd/4th lost fight.
  2500 is *not* the exact threshold for 20 % (the cap needs only ~2,000,000 debris).
- **Diameter 8,426 km** — exactly the standard formula for `x=11, p=20` (below).
- The server's combat reports otherwise show `Moon Chance: 0 %` and no moon from
  non-moon-shot tests up to 13.5 M debris — i.e. the chance is real but the older
  probes never reached it / used the wrong ships.
- Achievements confirm the feature set: `Moon base N lvl`, `Creater/Destroyed
  N moons`, `Moon Coloniser`, `Moon destroyer`.

## Standard OGame formulas (verified against the 8,426 km moon)

Creation chance, from the debris field:
```
moon chance % = min( floor( (metal + crystal) / 100000 ), 20 )
```
`internal/game/combat.go:MoonChance(m, c)` already implements this. 2,000,000
debris = 20 %. Server nuance: the owner says the field may be **accumulated**
(not only the debris from the single creating battle).

Diameter:
```
diameter = floor( sqrt(x + 3*p) * 1000 ) km
x = uniform integer 10..20   (11 equally likely values), p = creation chance %
```
- p=20: 8,366 (x=10) .. 8,426 (x=11, our moon) .. 8,944 (x=20) km.
- p=1: 3,605 km. Overall range 3,605–8,944 km.

Classic field count (reference only — this server differs, see below):
```
fields = floor( (diameter/1000)^2 )        # e.g. 8,426 km -> 70
```

Destruction by **Battle Fortress** (this server's Death Star; deployed unit
`214`), standard formula:
```
destroy moon %      = (100 - sqrt(S)) * sqrt(D)
Death Stars lost %  = sqrt(S) / 2          (one roll for the whole fleet)
```
S = diameter km, D = number of Battle Fortresses. At S=8,426: 8.21 %/BF; ~150
for 100 %. Attacker must first win the battle against the moon's ships/defenses;
there is **no debris** on a moon-destruction mission. Server rule: a moon
**>= 10,000 km cannot be destroyed** (standard moons cap at 8,944, so normally
moot).

## Live/server-specific rules (owner-confirmed 2026-10-07)

- **Fields:** a moon starts at **0 fields**, *independent of diameter*, but the
  first `Moon base` (41) can still be built. Each `Moon base` level consumes 1
  field and grants 2 (owner: "3 total" — exact accounting to be re-measured). This
  is a per-level-grant model, **not** the classic `(diameter/1000)^2`.
- **No** resource buildings and **no** power plants on a moon. Buildable scope
  includes fleet/defense production (Robot Factory, Nanite Factory, Shipyard),
  conveyors, `Phalanx Sensor` (42), `Jumpgate` (43) and `Alliance Depot`.
- Live building ids (`_lang_TECHphp`): `41 Moon base`, `42 Phalax Sensor`,
  `43 Jumpgate`, `44 Missile Silo`.

## Open questions

1. **Moon building catalog**: base costs / factors / requirements / field costs
   for 41/42/43 and which existing buildings (14 Robot, 15 Nanite, 21 Shipyard,
   22/23/24 storages, 31 Lab, 44 Silo) are buildable on a moon. Needs a live
   capture of `page=buildings&cp=1725` (+ research/shipyard).
2. **Fields accounting**: exact per-level `Moon base` grant and the "0/1" start.
3. **Creation source**: confirm moons only come from combat debris (no separate
   "Moon Coloniser" object), and whether the accumulated DF or only the creating
   battle's new debris drives chance + diameter here.
4. **Destruction formula** on this server (Battle Fortress count vs diameter,
   success/loss split) and the >= 10,000 km immunity — unmeasured.
5. **Jumpgate** (needs a **second** moon): cooldown (`GateCoolTime`), eligible
   ships, resources lost/kept, shared cooldown across gates.
6. **Phalanx Sensor** (42): range formula, deuterium cost per scan, output, the
   post-relocation 10-min offline window (documented but unmodelled).
7. Engine integration: `resolveAttack`/`accrueResources` against a `MOON` target,
   production safety, overview/galaxy/dashboard, fleet-wizard moon targeting.

## Implementation

- `internal/game/moon.go`: `MoonDiameterKm`, `MoonFields`, `MoonDestruction`,
  `MoonIndestructibleKm` (pure, tested). `MoonChance` already lives in `combat.go`.
- Remaining: moon catalog, creation on attack resolve, destruction mission,
  jumpgate, phalanx, and the moon view. Tracked in `docs/BACKLOG.md` item 4.

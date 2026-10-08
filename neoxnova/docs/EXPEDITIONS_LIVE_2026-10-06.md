# Expeditions — live findings (acc1 Bratwurst `3:125:12`, 2026-10-06)

Live capture of niburuspace.com expeditions to drive the Go resolver (backlog
item 1). Raw log: `tools/explorer/data/expeditions.json`. Tooling: `httpbot.mjs`
(`expedition`, `exp-state`, `exp-log`). Server code reference:
`tools/explorer/data/_MissionCaseExpedition.php` (vanilla 2Moons),
`data/_lang_FLEETphp` (outcome strings).

## 1. How expeditions are sent (this server is custom)

The normal fleet wizard has **no Expedition mission** for occupied or empty
targets. Expeditions go through the custom **"Automatically send expedition"**
panel on `page=fleetTable` (form `#expfleet`):

```
POST game.php?page=fleetTable
  ship2<code> = count      (note the `ship2` prefix, e.g. ship2217)
  exp_num     = how many fleets to send now
  exp_time    = 1..10  ->  0.25 .. 2.5 h expedition time
  exp_speed   = 1..10  ->  10 .. 100 %
  cmd         = 1  (random "Deep area of galaxy")
  cmd=2, pve  = 1 Barbarians | 2 Pirates | 3 Aliens   // owner 2026-10-08: only Pirates/Aliens exist; the pve=1 label is a mislabel
```

Targets land at `[g:s:21]` (position 21 = deep space per `AGENTS.md`).
`httpbot.mjs expedition <code:count,...> [num] [time] [speed] [--pve N]`.

Messages arrive in **Expedition messages** (`page=messages&mode=view&messcat=15`):
one outcome message at arrival, plus a per-fleet return ack carrying the loot
(`httpbot.mjs exp-log`).

## 2. Outcome distribution (live, 35 outcome messages)

| outcome | count | notes |
|---|---:|---|
| resources | 9 | incl. a "virus → pure deuterium" custom string |
| ships | 8 | predecessor wrecks / deserted pirate base, lists recovered ships |
| combat | 7 | Pirates / Aliens (see §3) |
| delay (`time_slow`) | 4 | incl. "collision with a strange ship" |
| darkmatter | 4 | e.g. "asteroid core … dark matter", DM containers |
| fast-return (`time_fast`) | 2 | |
| nothing | 1 | |
| **black hole** | **0** | |

Loot from 23 return acks: **Metal 2.51 B, Deuterium 0.73 B, Dark Matter 6 528**;
only 7/23 returns actually carried loot.

**Black-hole estimate:** 0/35 → naive rate 0 %, 95 % upper bound ≈ 8 % (rule of
three). Vanilla 2Moons rolls `mt_rand(1,9)` with a black hole on 1/9 ≈ 11 % (and
"nothing" on 3/9); this server clearly does **not** follow that (no black hole in
35, "nothing" almost absent). Do **not** import the 1/9 figure.

## 3. The expedition enemy is not a vanilla mirror

Vanilla 2Moons clones your own composition (`round(count × 0.3–0.9)`). On this
server the combat report instead shows **fractional unit counts** (points ÷ cost
without rounding, visible on hover), i.e. a **points-scaled enemy template**.
Typical defenders: Heavy Cargo / Light Fighter / Cruiser / Battleship /
Star Fighter / Battle Transporter — including types the attacker never sent.

Evidence (CombatReport hashes in `data/_msgs3.html`):

- Mixed `204:120,206:20,207:20,217:16` vs **Pirates** → **entire fleet
  destroyed** (`Battle Transporter 0 -16`), attacker losses 3.01 M, defender
  survived (42 Battleships + …). `raport=dfab98d8…`
- Pure `217:100` vs **Pirates** → all 100 Battle Transporters destroyed.
  `raport=87c2065b…`
- Pure `217:10` vs Pirates/Aliens → often lost 5/10, sometimes all.
- Smaller fleets are **not** proportionally safer, and escorting does not
  protect the cargo — the escort inflates the enemy.

So the "10 battle ships : 1 transporter" rule is **not** supported here, and
neither is "cargo-only = guaranteed draw". The only reliable conclusion so far:
**composition and fleet size both strongly scale the enemy.** More controlled
samples are needed before modelling the resolver.

## 4. `cmd=2` (pve / "Hostail sector") is buggy — ghost fleets

`--pve` fleets get stuck: they count in `fleetTable` ("6 / 7 expedition") but the
overview shows **zero** fleet movements; they never resolve, and recalling them
turns them to `(R)` but they still never land (counter stayed 6/7 for 30+ min).
The auto-send cap is **not enforced** either (counter reached "13 / 7").
Avoid `cmd=2`; if used, the slots are effectively leaked. (One such batch
eventually resolved with ~4 B of attacker losses and 0 defender losses.)

## 5. Go resolver — open questions

1. Enemy generation formula: what drives the template size (expedition fleet
   points? account points?), and how the fractional counts arise.
2. Black-hole / danger rates are unknown and must stay out of any model until
   more live data exists.
3. `nothing`/`delay`/`fast` implementation is cosmetic (message only).

Next session: collect a larger, composition-controlled sample (pure vs escort)
and diff planet ship counts around each batch to measure survival.

## 6. Fight-report findings — rounds 2–3 (2026-10-06)

`exp-report` finally captured combat reports for the campaign arms (harvest holds
~48). Server clock is **UTC+3** (a 10:06Z send appears ~13:06 server). Rows
below are `atkPts/defPts` = (metal+crystal)/1e6 of the start comps.

| when (server) | arm | attacker | enemy | result |
|---|---|---|---|---|
| 12:42:56 | r2 `217:20+207:20` | 2.3 pts | 13.3 pts | defender (atk 40 lost) |
| 12:44:54 (acc2) | r2 `217:20+207:20` | 2.3 pts | 18.0 pts | defender |
| 12:43:50 | r2 `217:50+204:500` | 4.8 pts | 8.4 pts | defender (atk 550 lost) |
| 13:22:30 | r3 `226:100` | 500 pts | 342 pts | **attacker** (atk −22, enemy −228) |
| 13:24:13 (acc2) | r3 `217:50+226:100` | 503 pts | 344 pts | **attacker** (atk −54, enemy −218) |

Takeaways:
- **The enemy IS points-scaled, roughly ~0.7× the sent fleet points at large
  sizes** (500 → 342; 503 → 344). The earlier "not proportional" impression came
  from small fleets, which sit on a **fixed minimum template + high variance**:
  pure `217:10` (0.6 pts) drew 2.4–23 pts across samples.
- The enemy is a **fixed composition** the attacker often never sent — Heavy
  Cargo, Light Fighter, Cruiser, Battleship, Star Fighter, Battle Transporter,
  Destroyer. Both Destroyer arms drew **Destroyer x68** regardless of escort.
- Composition matters for **losses**, not enemy size: pure `226:100` lost 22/100;
  `217:50+226:100` lost 54 (the cargo dragged the escort down). Cargo `217` is
  *not* protected by escorts.
- acc1 fleet diff across round 3: only `226` dropped (−22), matching the report;
  `217`/`204`/`215` came back net-positive (ship-loot outcomes), so no global loss.

### Enemy-points ratio (enemyPts ÷ sentPts)

| sent arm | atkPts | enemyPts | ratio |
|---|---:|---:|---:|
| BT×10 | 0.55 | 2.4 / 2.9 / 23 | 4.3 / 5.3 / 41.8 |
| BB20+BT20 | 2.3 | 13 / 18 | 5.9 / 8.0 |
| BT×50 | 2.8 | 40 | 14.7 |
| LF120+Cr20+BB20+BT16 | 3.2 | 7.1 | 2.2 |
| LF500+BT50 | 4.8 | 8.4 | 1.8 |
| BT×100 | 5.5 | 7.6 / 10.0 | 1.4 / 1.8 |
| DD×100 | 500 | 342 | **0.68** |
| BT50+DD100 | 503 | 344 | **0.68** |

The ratio collapses from ~5–40 (a minimum-template floor with big variance) and
**converges to ~0.68 by 500 pts**; both Destroyer arms drew the same enemy
`DD×68`. Note only ~11 of the 33 harvested reports are real expedition enemies —
the rest are vs planets (Solar Satellites + defense lines) and are irrelevant.

### Rounds 4–5 rolled **0 combats** (both accounts)

Rounds 4 (small) and 5 (big: `226:1000/2000`, `225:2000`, `219:1000`, …) produced
**no enemy encounters** — the ~13.3 % pirate roll missed on all 28 expeditions.
Fleets came home with loot, not fights, so **the ratio above 503 pts (the whole
5k/50k tier question) is still unmeasured**. Fleet deltas before-r5→end were
net-positive for every class on both accounts, except acc2's `219:1000`
Battle-Recycler arm which was **lost to a black hole** (msg id 219935).

### Black-hole rate
- acc1: **0 / 70** outcomes (95 % upper bound ≈ 4 %).
- acc2: **1 / 35** (2.86 %) — the `219:1000` arm vanished.
Small sample; still nothing like vanilla 1/9.

Still open: does the ~0.7 ratio hold / shift at ≥5,000 pts (Arsenal gate), and
what sets the small-fleet floor.

## 7. Rolling-farm campaign snapshot (2026-10-07, both accounts)

Two accounts run the fixed expedition set continuously (acc1 local, acc2 VM).
Command: `httpbot.mjs expedition "207:S,203:5S,219:round(S/250),202:1,204:1,205:1,206:1"`.
At the snapshot S = 87 331 (acc1, ≈10.9k pts) and S = 61 764 (acc2, ≈7.7k pts) —
i.e. **well above the 500-pt arms in §6** and still climbing.

| | acc1 | acc2 |
|---|---:|---:|
| outcome messages | 338 | 370 |
| return / nothing | 165 / 9 | 182 / 12 |
| ships found | 44 | 51 |
| resources found | 38 | 36 |
| dark matter found | 28 | 30 (incl. 13 "alien" DM) |
| delay / fast-return | 18 / 6 | 18 / 6 |
| combat | 19 | 16 |
| **black hole** | **0** | **5** |

Fight reports (`exp-report`): acc1 **14 W / 11 L / 10 D** (35), acc2 **22 W / 8 L / 7 D**
(37); debris collected ≈ 46.5 B (acc1) and 51.3 B (acc2) metal+crystal.

Black-hole rate now: acc1 0/338 & acc2 5/370 ⇒ combined **5 / 708 ≈ 0.7 %**. Still
nothing like vanilla `mt_rand(1,9)` (11 %); keep the 1/9 figure out of any model.

Arsenal: **4 upgrade drawings found** at these sub-75k fleet sizes — see
`docs/ARSENAL_LIVE_2026-10-06.md` "Live correction (2026-10-07)". This is strong live
evidence the documented 75 000-pt gate is not what delivers our finds.

Open for the resolver (backlog 1b/1): the harvested reports are now large-fleet
(~8k–11k pts) and can extend the §6 enemy-points-ratio table past 503 pts; the enemy
template/composition still needs a controlled diff. Our set is fixed, so enemy size
is the only variable left.

## 8. Overnight campaign — enemy formula resolved (2026-10-08)

Both accounts ran the fixed set all night. The enemy generation model is now
**measured and closed** (backlog 1b).

### Enemy = mirror × ~0.66 + small template
Across Oct 7–8 fights the defender is the attacker's own composition scaled by a
single per-fleet roll, plus a small random template (Light Fighter / Cruiser /
Star Fighter, tens–hundreds of units). Verified far past the old 503-pt ceiling:

| S (BB) | def207/atk207 | result |
|---|---:|---|
| 623,318 | 0.62–0.64 | attacker |
| 751,803 | 0.66–0.93 | attacker / draw |
| 814,397 | 0.62–0.68 | attacker |

Recent 40 acc1 fights: median **0.66**, range **0.60–0.94** (acc2 shows the same
0.67–0.87). The ratio is uniform across every shared type in a report (e.g. 203
and 207 both 0.660), so it is **one roll per fleet**, not per unit. Small fleets
are dominated by the template, which is why the old §6 ratio table reads 2–40×
below ~500 pts.

Model: `enemy = round(fleet × roll[~0.6..0.9]) + template(HC/LF/Cruiser/BB/SF)`.

### Enemy W/S/A research is a single rolled value (aliens skew ~2×)
The report header exposes the bonuses actually applied (per-unit
`Firepower/Shield/Armour` share it). The player's three are distinct (acc1
**+112/+85/+94**, acc2 **+95/+81/+100**, = 109/110/111 + arsenal), but the NPC
shows **one rolled value on all three**. Harvested acc1 headers (46 fights,
06–08 Oct):

| NPC | observed Firepower (=Shield=Armour) | median |
|---|---|---:|
| Pirates | 11,12,14,18,19,23,23,25,27,28,29,30,31,33,36,37,44,46,49,49,50,51,52,65,68,80,82,82,82,85,86,91,97,102,103,112,114,115,121,121,124,134,137,138,139,152 | ~51 |
| Aliens | 51,53,56,74,81,84,90,96,96,113,118,133,168,**200**,**202** | ~90 |

acc2 matched (Pirates +10…+125, Aliens +30…+159). Aliens sit clearly higher.
The owner-confirmed rule: the NPC's value is the **mirror of our general
Weapons/Shield/Armour research (109/110/111)** scaled by the fight roll — the
two total losses below are the high rolls (200/202 ≈ **2.2×** acc1's `109`
quadratic bonus of 90). Only 109/110/111 is mirrored; the specific weapon techs
(120/121/122/199), arsenal, academy and governors are **attacker-only**.

Standing rule: **keep 109/110/111 at the techtree floor** (Frigate `227` needs
16/16/17; Battle Recycler `219` 15/15/15; `226` 14/13/13) and put research into
the non-mirrored bonuses. Every extra general-tech level raises the alien as
much as us, and a high alien roll wipes a full fleet — 2026-10-08, two "contact
with unknown ships" encounters ~1 min apart:

| account | report | our fleet | result | enemy W/S/A |
|---|---|---|---:|---:|
| acc1 | `1fc3647d` (msg 243839) | 20,228 (13,478 Frig + 6,739 BR) | lost **100 %** | +202 % |
| acc2 | `ebd41e22` (msg 243848) | 5,188 (3,451 Frig + 1,726 BR) | lost **100 %** | +159 % |

Header captured as `attackerInfo`/`defenderInfo` by `parseCombatReport` and
stored in `data/expedition-reports.json` (harvest 2026-10-08). Go model:
`game.Combatant.FlatBonusPct` (a single additive W/S/A percent), used by
`cmd/exposim` (pirate ~0.6×, hard alien ~2.2× the 109 bonus).

### The next alien fight was a weak roll — and we won (10:48)
`e219ad37` (msg 244369) is the mirror-image of the wipe 43 min later: same comp,
near-identical enemy fleet, **half the enemy research**.

| report | enemy Frig | enemy BR | enemy W/S/A | our Frig | our BR | result |
|---|---:|---:|---:|---:|---:|---|
| `1fc3647d` 10:05 (wipe) | 11,861 | 5,930 | **+202 %** | 13,478 | 6,739 | lost **100 %** |
| `e219ad37` 10:48 (win) | 12,231 | 6,116 | **+90 %** | 13,012 | 6,506 | **attacker wins** |

The weak-alien enemy was actually *larger* (12,231 vs 11,861 Frigates) yet we
won: effective power `count·(1+research)` flips from `11,861·3.02 = 35.8k` vs
our `13,478·2.12 = 28.6k` (enemy ~1.25× → wipe) to `12,231·1.90 = 23.2k` vs
`13,012·2.12 = 27.6k` (us ~1.19× → win). Losses: 4,963 Frig + 6,474 BR (the BRs
are glass), enemy annihilated. 6 rounds, debris **M 264.2 B / C 89.8 B**
(50 % of the 708 B total). Rebuild at Academy Standardisation −13 % ≈ **183 B**,
so the fight is **~+171 B net (1.94×)**. Conclusion: the enemy fleet roll is
secondary — the **research roll is the whole fight**; the same hull wins or wipes
on it alone.

### Win record flipped at scale
- acc1 Oct 7–8: **29 W / 11 D / 0 L** (was 14 W / 11 L / 10 D at ~9k pts).
- acc2 Oct 7–8: **37 W / 5 D / 0 L**.
At 450k–815k pts the player essentially cannot lose **to pirates** — but the wins are bloody:
40 acc1 fights cost **~40.9 M HC + 2.96 M BB**, yielding **627 B M / 443 B C**
debris. HC take ~25–50 % losses per fight — this is the crystal drain in
`docs/ROLLING_FARM.md`.
**Exception (2026-10-08):** the `Pb+recycler` Frigate comp *can* lose — see the
alien wipe table above (two full-fleet losses ~1 min apart). The `0 L` record
predates the Frigate flip and only holds at low enemy W/S/A rolls.

### Black-hole rate is account-asymmetric
- acc1: **2 / 461 (0.43 %)**.
- acc2: **19 / 493 (3.85 %)** — 19 fleets lost, clustered on Oct 7.
Combined 21/954 ≈ 2.2 %. Still below vanilla 1/9 (11 %), but acc2 is ~9× acc1;
whether that is luck or a size/speed/account effect is unresolved.

### Growth, loot, new outcomes
- acc1 S grew 83,204 → **814,397** (~10× in a day); acc2 → **445,401**.
- acc1 loot from 198 return acks: **800.8 B M, 511.2 B C, 50.1 B D, 391,187 DM**.
  "ships" finds are large (one dropped LC 787,657 + HC 22,120 + HF 1,673 +
  Cruiser 262).
- Both accounts are now **build-capacity-limited** ("not enough ships for 1
  fleet"): acc1 fires 5/9 slots, acc2 8/9. The S ratchet sits above the current
  cap, so combat losses cannot refill fast enough to keep slots full.
- New outcome strings logged: 34× "Moa Tikarr demands unconditional surrender"
  (the whole `unknown` bucket; no separate combat report harvested — check
  whether these are dropped combats), plus bacterium / red-giant /
  particle-storm / life-form / disconnect flavour.
- 1 genuine arsenal drop: *"drawing for an upgrade Jet engine"* (Oct 7 05:07),
  again below the documented 75,000-pt gate.

## 9. Go resolver (implemented 2026-10-08)

`internal/game/expedition.go:RollExpedition` is the pure model: a weighted
outcome mix, cargo-capped resource finds, ship recovery, dark matter, delay/fast
ETA shifts, black holes (2 %) and the §8 enemy (mirror × U(0.60..0.90) +
template, ONE rolled W/S/A carried on `Combatant.FlatBonusPct`; Aliens have the
~2.2× tail that wipes fleets).

**Outcome mix (PROVISIONAL).** Based on the open-codebase defaults (OGame,
inherited by 2Moons/XNova): resources 32.5 %, ships 22 %, dark matter 9 %,
combat 8.4 % (pirates 5.8 / aliens 2.6 ⇒ 70/30), delay 7 %, early return 2 %,
nothing 18.6 %, black hole 0.33 %, merchant 0.7 %. This server differs (combat
~15 %, black hole ~2 %), so the mix is rescaled to: resources 30 %, ships 20 %,
**combat 15 %** (pirates 70 / aliens 30), nothing 17 %, dark matter 8 %, delay
6 %, fast 2 %, **black hole 2 %**. Owner-locked 2026-10-08: BH 2 %, combat 15 %,
only Pirates + Aliens (no "barbarians" — that is a pirate flavour string).

**Enemy scaling.** Pirate/alien fights use the regular combat engine but scale
ONLY on the general Weapons/Shield/Armour research (109/110/111) — the strongest
of the three feeds the single rolled value (`game.MirroredResearchBonus`). The
specific weapon techs (120/121/122/199), Arsenal upgrades, Academy and governors
are never mirrored.

`internal/engine/event_engine.go:resolveExpedition` runs at the
HOLDING→RETURNING transition and persists the outcome: a `combat_reports` row +
attacker losses + deep-space debris for fights, the Arsenal draw
(`store.AddUpgradeItems`, ~10 % on a win / ship find), dark-matter credit, the
cargo manifest and early/late return, or `RESOLVED` on a wipe / black hole.
Expeditions are dispatched through the normal `EXPEDITION` mission (a holding
time is required); the buggy live `cmd=2` path is not needed server-side.

Tests: `internal/game/expedition_test.go` (pure; determinism, mix, enemy
mirror/ranges, loot cap, drop rate) and
`internal/engine/expedition_integration_test.go` (DB; forced outcomes for
resources/combat/black-hole persistence).

Remaining: surface non-combat outcomes as messages/reports and calibrate the
outcome mix from a larger harvest.

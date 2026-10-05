# Combat Findings — niburuspace.com → neoxnova

**Read this first.** Sleek summary of the combat scouting. Raw evidence, every
scenario and the step-by-step log live in `docs/COMBAT_MODEL.md`; the machine
readable replay set is `testdata/niburus_combat.json`.

- Server: XNova "GOW", `universe_6_niburu`, game 4000x / resource 10000x.
- Accounts: attacker **Bratwurst `2:188:9`**, defender **TheBob `2:188:16`**
  (distance **1035**).
- Method: in-game battle simulator (hundreds of controlled runs, deterministic)
  plus real fleet strikes, cross-checked against each other.

---

## 1. The combat model (confirmed)

| Rule | Value / behaviour | Confidence |
|---|---|---|
| Max rounds | **8** (draw after round 8) | high |
| Firing | each surviving unit fires **once per round** at **one** enemy unit | high |
| Overkill | damage past the target's death is **discarded** | high |
| Shield | absorbs first, **fully regenerates every round** | high |
| Hull | damage **persists** across rounds | high |
| Bounce | **none** — shield-first + full regen only; a side whose round damage < shield does 0 | high (2026-10-05, S2) |
| Rapid fire | extra shots vs specific targets; drives screen-clearing | high |
| Debris | **50%** of base M+C of destroyed **ships only** (defenses = 0) | high |
| Loot | **50%** of each resource, M→C→D, capped by cargo (real: 3.3B looted) | high (2026-10-05) |
| Moon | **off**: real report prints `Moon Chance: 0 %`; no spawn ≤13.5M debris | high (2026-10-05) |
| Defender bonus | **none** (perfect side-swap symmetry) | high |
| Repair | **~61%** of destroyed defenses, **same for draw and total wipe**; domes prob. | high (2026-10-05) |
| Cargo (server) | scaled: Battle Transporter `217`=**400M**, Battle Recycler `219`=**200M**; LC/HC are meatshields | high (owner) |

### Stat formula (important — server-custom)

```
bonusPercent(L) = round(L * (L + 2) / 4)      # NOT +10%/level
attack  = round(baseAttack  * (1 + bonus(W)/100))   # W = Weapons Tech  (109)
shield  = round(baseShield  * (1 + bonus(S)/100))   # S = Shield Tech   (110)
hull    = round(baseHull    * (1 + bonus(A)/100))   # A = Armour Tech   (111)
```
`bonus`: L1=1, L5=9, L10=30, **L15=64**, L20=110. Identical for all three techs,
additive.

**Extra modifiers (real, but NOT shown on the card header):**
- Weapon-type techs `120 Laser / 121 Ion / 122 Plasma / 199 Graviton` add damage.
- Academy skills `1103/1108/1109/1110/1111/1303/1308/1311` add damage
  (all-at-5 measurably increased defender losses in a big battle). **Real
  confirmation 2026-10-05:** with acc1 at `1103:7 1108:5 1109:4 1110:2 1111:1`,
  a real strike matched the *with-procs* sim (PB lost 38 vs 39; debris M3.330M vs
  M3.365M) and diverged from the no-academy sim (PB 47; M3.645M). Defender-side
  `1303/1311/1308` showed no sim effect at L5 — keep parametric.

---

## 2. The meatshield doctrine (confirmed)

Because each enemy capital can only remove **one** of your ships per round
(unless it has rapid fire vs it), a large pool of cheap ships absorbs fire while
your capitals do the killing. Raw damage doesn't matter — **overkill is lost**.

Attacker `100 Battleships` + `N Light Fighters` vs `50 BS + 1000 LF`:

| screen LF | your BS lost | result |
|---:|---:|:--|
| 0 | 100 | defender wins |
| 250 | 100 | defender wins |
| 500 | 33 | **attacker wins** |
| 1000 | 9 | attacker wins |
| 2000 | 2 | attacker wins |
| 4000 | 0 | attacker wins |

**Rule of thumb: ≥ ~5 cheap ships per capital ship.** The reverse (defender
screen) is *identical* → no defender bonus.

---

## 3. Rapid fire & anti-defense specialists

- Rapid fire is the only efficient way for capitals to chew through screens.
  No-RF capital ships kill only ~1 screen ship/round regardless of attack
  (a 65,000-attack Destroyer killed ~4 LF/round over 9 rounds — one per ship).
- **Planet Bomber (211)** has huge RF vs defenses (Missile Launcher 100,
  Light Laser 80, Ion 70, Gauss 60, Plasma 50, …).
- **Destroyer (226)** has RF vs high-tier defenses (Gravitons 25, Proton 20,
  Particle 15, …).
- Tested: a handful of **Black Moon (RF 180 vs LF)** cleared 2000 LF/sats in
  seconds; StarFighter/Cruiser/BattleCruiser RF behaved consistently.

### Anti-defense battery (all vs the "wall": 990 ML / 600 LL / 400 HL /
### 200 Gauss / 200 Ion / 100 Plasma / 1 Small + 1 Large Shield Dome)
| attacker | result |
|---|---|
| 50 Planet Bomber vs 500 ML/300 LL/200 HL | **attacker wins, 2 rounds, only 2 PB lost** |
| 50 PB vs 200 Gauss/100 Ion/50 Plasma | attacker wins, 3 rounds, 47 PB lost |
| 100 PB vs full wall | defender wins (too few) |
| 2000 LF + 1000 PB vs full wall | **attacker wins, 4 rounds** |
| 5000 LF + 1000 PB + 1000 Destroyer | **attacker wins, 3 rounds** |
| 5000 LF + 1000 Galleon | attacker wins, 5 rounds |
| 5000 LF + 500 Destroyer | attacker wins, 7 rounds |
| 200 Battleship / 200 Cruiser vs full wall | **all lost** (no screen, no RF vs the wall) |
| 50 Destroyer vs 10 Gravitons/Lepton/Proton | all Destroyers lost (those turrets hit for 500k–1.5M) |

**Takeaway:** Planet Bombers are the anti-defense workhorse (impossible without
huge RF); Destroyers excel vs the high-tier turrets but those turrets are
brutally strong, so always bring a **screen** — no screen = your capitals melt.

---

### Endgame units (Black Moon 216, Frigate 227)
- **Black Moon**: RF 180 vs LF, 350 vs sats. 3 BM killed 4165/5000 LF in 9 rounds;
  2 BM wiped 5000 sats; 2 BM killed 184/200 BS in 9 rounds (draw).
- **Frigate**: 10 Frigates wiped 200 BS in 2 rounds and 5000 LF in 3; vs the full
  tower wall they survived 9 rounds killing 30 ML/15 LL/15 HL/7 Gauss/7 Ion (draw).
- **5 Black Moon + 20 Frigates** destroyed 500 Cruisers + 500 BS in 3 rounds with
  **zero losses** — the strongest observed composition.

## 4. Real-battle validation

| run | result | real | simulator |
|---|---|---|---|
| `50 LF + 10 Cruiser` → acc2 | attacker wiped | debris 150,000 M / 106,500 C | 150,000 / 106,500 (tech-0) |
| `300 LF+200 HF+150 Cru+80 BS+40 BC` → acc2 | defender wins, 5 rounds, attacker wiped | debris 5,336,500 / 5,424,500 | 5,329,000 / 5,445,750 |
| **reverse**: acc2 `100 LF+60 HF+35 Cru+15 BS` → acc1 ships-only planet | defender (acc1, **no towers**) wins 4 rounds, attacker wiped | acc1 lost 51 LC/12 LF/11 HF/2 Cru/261 sats; debris 1,088,000 / 1,038,750 | — |
| recycle of the debris with Battle Recycler (219) | field emptied | ✔ | — |

Real combat results reproduce the simulator (per-unit losses within 1–2 units).

---

## 5. Base stats (tech 0)

Ships (attack/shield/hull) — full table in `COMBAT_MODEL.md` §F8. Highlights:
`LF 50/35/400`, `HF 150/100/1100`, `Cruiser 400/105/2450`, `BS 700/190/5800`,
`Bomber 1400/480/11500`, `BC 1400/430/9000`, `Destroyer 65000/23000/500000`,
`Galleon 20000/8000/160000`. Cargo/utility ships have **attack 0**.

---

## 6. Reference quirks (do NOT inherit in Go)

1. **Simulator crashes** (PHP notice, no report) when the attacker wins and the
   defender's resources are not all-three non-zero. Workaround: seed `{1:1,2:1,3:1}`.
2. Putting a **defense unit on the attacker side** yields a malformed report.
3. Spy-probe (`210`) defenders give inconsistent numbers.
4. Weapon-type techs and academy skills are invisible in the card header but
   change resolution — model them explicitly.
5. Rapid fire here is deterministic per run (fixed seed); OSS uses probabilistic
   `(rf-1)/rf`. Model probabilistically on a **seedable** RNG.
6. `CombatReport.php?raport=<id>` is not the real report; the real report hash is
   linked inside the combat message
   (`page=messages&mode=view&messcat=3&site=1&ajax=1`).

---

## 7. Recommended Go engine

Standard OSS OGame/2Moons round engine, with this server's constants:

```
Resolve(attacker, defender, techs, academy, rng) Report
  derive per-unit atk/shield/hull  (formula above + weapon-tech + academy)
  for round in 1..8:
    reset shields
    for side in [attacker, defender]:
      for each unit: fire at a random live enemy (weighted by count)
        apply damage: shield -> hull; overkill discarded
        rapid fire: extra shots with (rf-1)/rf, bounded by a shot cap
    if a side is empty: break
  result / debris(50%, ships only) / loot(50%, cargo-capped) / repair
```

Guardrails: all integer maths, every division zero-guarded, bounded RF and round
loops, deterministic seeding for replay tests.
Acceptance test: replay `testdata/niburus_combat.json`; RNG-sensitive scenarios
compared statistically.

---

## 8. Open items

- Defense repair % across repeated real attacks (partial reduction seen: ML
  500→420, LL 300→259, HL 200→190 after one strike — repair rule unconfirmed).
- Exact bounce threshold operator and rapid-fire shot cap.
- ACS / multi-slot POST format (`action=moreslots`).
- Moon generation (appears disabled).
- Flight-time & fuel formulae (samples collected; neoxnova's current formulas are
  off by ~3×). Record engine levels with each sample.
- **Target selection = random, weighted by unit count (confirmed 2026-10-05).**
  Each shot picks one random enemy **unit**, so loss *fractions* are equal across
  equally-durable types (evidence: `pb-ships` LF/HF/Cruiser = 15/15/12 %;
  `big-pb-ships` = 86/86/85 %). There is **no type preference / weakest-first** —
  this is why the meatshield doctrine works. The earlier "sequential damage"
  note was wrong.
- **Rapid-fire shots (confirmed 2026-10-05, `plans/combat/rf-shots.json`).**
  Round 1 total shots = `floor(0.70 × N_units × RF)`; rounds 2+ = exactly
  `N_units × RF`. Measured with Solar-Sat targets (attack 0, 1-shottable, kills
  == shots): BS(RF25)→17,25,25…; BM(RF350)→244,350…; Galleon(RF125)→87,125…;
  BC(RF50)→35,50….
- **Residual (open).** Targeting + RF now match exactly, yet swarm-vs-capital
  **loss magnitudes** diverge: `1000 LF vs 100 BS` (tech 0) kills **~15 BS** on
  the reference but **0** in our engine (it kills ~0 because per-round per-BS
  hull damage averages ~150–250 vs 5800 hull, while ~114k of the 156k damage is
  re-absorbed by shields each round). The reference concentrates ~5× more hull
  damage than uniform per-shot distributing allows, so it looks like a hidden
  RF-or-damage factor in many-target fights. Winner prediction stays ~91 % on the
  committed dataset. Kill curve (tech 0, `plans/combat/damage-curve.json`):
  `500 LF→0 BS`, `1000→15`, `2000→100`, `4000→100` (vs 100 BS); `1000 LF→1 BS`
  kills it r1, `1000 LF→10 BS` kills 8 in r1 — a sharp nonlinear threshold that
  per-unit shield absorption cannot explain (1000 LF vs 10 BS would need ~960
  hull hits but only fires ~700 shots). Looks like pooled/per-side shield
  handling with concentrated hull damage; the exact rule is still unmodelled.

**Trust note (implemented).** For players to trust the simulator, the battle
seed is derived deterministically from the stable inputs via
`game.CombatSeed(attacker, defender)` (unit counts + techs + academy), and the
scheduler uses it too. So simulating the same fleets reproduces the real
resolution exactly — independent of how faithfully we clone the reference's
opaque damage distribution.

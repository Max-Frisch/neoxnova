# Combat Test Campaign — Session 2026-10-05 (live results)

Durable record for the **final live combat tests** (acc1 Bratwurst × acc2 TheBob)
on niburuspace.com `universe_6_niburu`. Read `COMBAT_FINDINGS.md` (model) and
`COMBAT_MODEL.md` (evidence) first; this file records **what was actually done
and measured on 2026-10-05**, including the T1/T2 real battles, loot, repair and
moon conclusions. The Go implementation is a **separate follow-up step**.

Environment: local dev host Windows (acc1) + Azure VM `azureuser@70.153.144.215`
(acc2). Tooling `neoxnova/tools/explorer/httpbot.mjs`.

---

## 0. Gate / readiness (C0)

- **Points balance passes**: acc1 Gotijy `16,648,567` vs acc2 Japoqu
  `7,456,339` = **2.23 : 1** (well inside the ~4:1 both-ways noob-protection gate).
- **Moonless targets confirmed**: only moon in the strike systems is `2:186:20`
  (not ours); every acc1/acc2 colony in `2:185/186/187/188/191` is moonless.
- acc1 strike hub = cp1648 `Xusyty 2:188:9`; fleet at send time:
  `202:2000 204:15499 205:1000 206:1500 207:1500/1200 211:900 212:11965 215:900
  216:6 217:15 219:500 225:603 226:700 227:30`.
- acc1 research: `106:19 108:21 109:16 110:16 111:17 120:22 121:20 122:18 123:10
  124:10 131:22 132:22 133:22 199:5` (+engine 113:20 114:16 115:21 117:18 118:19).
- acc2 research: `106:19 108:21 109:16 110:16 111:17 120:20 121:19 122:17
  123:11 124:8 131:20 132:20 133:20 199:5`.

### Target prep (acc2 colonies)

Built on `1689` (`2:186:11`, moonless, verify MET):
`401:500 402:300 403:200 404:100 405:100 406:100 407:1 408:1` + `202:2000`,
stored ≈ `3.74B M / 1.91B C / 0.99B D` at battle time.

Staged LC moon targets: `1688` 2500 LC, `1696` 6172 LC (each also has 600 sats).

---

## 1. T1 — Full-wipe battery (one strike → many answers)

**Strike**: acc1 cp1648 → `2:186:11 (1689)`, mission Attack, speed 100%.

Fleet: `226:400, 211:600, 216:6, 227:30, 217:15, 202:300`
(report `aea7b462506f77330f22e0e33b5001dd`, real `CombatReport.php`).

Defender: the wall above + `202:2000` + `212:600`.

| metric | real result |
|---|---|
| outcome | **attacker wins, 6 rounds** (both domes killed r5, empty r6) |
| attacker losses | LC 119, Planet Bomber 89, Battle Transporter 2 (≈11.54M value) |
| defender losses | entire wall + 2000 LC + 600 sats (28.27M value) |
| **debris** | **M 5,455,000 / C 5,515,000** (ships only) |
| **loot** | **M 1,869,021,360 / C 954,035,646 / D 492,122,212** |
| moon | **none**; real report prints **`Moon Chance: 0 %`** |

### 1a. Loot = exactly 50 %, no cargo cap here
Loot = 50 % of each stored resource, taken **M → C → D**, and it was **not**
cargo-limited: pre-battle stored (production since snapshot) ≈ `3.74B/1.91B/0.99B`,
post-battle storage was `1.878B/0.959B/0.494B`, i.e. `stored_after = stored_before
- loot` exactly. The cargo was supplied by **15 Battle Transporters (217)** —
hence the owner's server fact below.

### 1b. Wipe repair ≈ 61.6 % (NOT ~37 %)
Destroyed = whole wall + both domes (`500/300/200/100/100/100/1/1 = 1302`).
Post-battle (repaired counts): `401:350 402:165 403:112 404:61 405:57 406:56
407:0 408:1`.

```
repaired = 350+165+112+61+57+56+0+1 = 802  ->  802/1302 = 61.6 %
```

So a **total wipe repairs at the same ~61 %** as a normal battle (see T2). The
earlier "~37 % wipe rate" (COMBAT_REAL_TESTS §4) is **not reproduced** and should
be dropped. Dome behaviour is noisy: Small Shield Dome `407` was lost, Large
Shield Dome `408` was restored (single units → probabilistic).

### 1c. Real ≈ simulator (with techs)
Sim (`plans/combat/t1-acad.json`, real techs, no academy): attacker wins 6 rounds,
debris `M5,420,000 / C5,492,500`. Real debris `M5,455,000 / C5,515,000`
(≈0.6 % apart). Per-unit defender losses identical (full wipe). Small attacker
divergence (real LC lost 119 vs sim ~270) — within the known target/RNG spread.

---

## 2. T2 — Normal-battle defense repair (draw)

**Strike**: acc1 cp1648 → `2:187:9 (1674)`, `226:80`, speed 100%
(report `5827af03a96d2aef9a406d144d09d828`).

Defender: wall `428/239/171/79/83/82` + `407`/`408` + 218 sats.

| metric | real result |
|---|---|
| outcome | **draw, 9 rounds** |
| defender losses | ML 200, LL 114, HL 80, Gauss 38, Ion 38, Plasma 38, sats 101 (no domes) |
| debris | `M 0 / C 202,000` (101 sats ×4000×0.5) |
| post defenses | `401:362 402:203 403:134 404:62 405:68 406:70 (+domes)` |

**Repaired** = post − before + destroyed:

```
ML 134/200, LL 78/114, HL 43/80, Gauss 21/38, Ion 23/38, Plasma 26/38
pooled = 325 / 508 = 64.0 %
```

### 2a. T3 — `1304 Mechanics` delta
acc2 had academy `1304` at **L4** (+4 % defense recover) for **both** T1 and T2.
Observed: T1 wipe 61.6 %, T2 normal 64.0 % — consistent with the historical
~61.5 % base **plus** a small `1304` boost. No clean no-`1304` acc2 baseline
exists (all acc2 planets share the account-wide skill), so the delta is
**indicative, not isolated**. (Simulator/base rate ~61.5 %.)

---

## 3. T4 — Moon series (real server decides)

All on **moonless** planets; pure ship debris (LC `202`, attack 0 → no attacker
losses; sats `212`).

| stage | planet | debris | moon? |
|---|---|---|---|
| 600 sats | `2:186:9` (1687) | `C 1,200,000` | no |
| 2500 LC + 600 sats | `2:186:10` (1688) | `M 2,500,000 / C 3,700,000` | no |
| 6172 LC + 600 sats | `2:185:9` (1696) | `M 6,172,000 / C 7,372,000` (13.5M) | no |
| T1 (wall+LC+sats) | `2:186:11` (1689) | `M 5,455,000 / C 5,515,000` (11M) | no |

**Conclusion: real-server moon generation is effectively disabled.** Not only did
no moon appear at up to 13.5M debris, the **real combat report itself prints
`Moon Chance: 0 %`** (e.g. the T1 report). This matches the simulator (which also
always prints 0 %). Classic OGame would give `min(20 %, floor(debris/100000)%)`
(e.g. 13.5M → 20 % cap; 1000 LF ≈ 1M debris → ~10 %).

**Decision for the Go target:** adopt the classic
`moonChance = min(20 %, floor(debris / 100000))` formula (debris = M+C), because
the reference server does not exercise it. No further live moon attempts — a
≤20 % roll needs many strikes per bash-limit/planet to observe, for no code benefit.

---

## 4. Ungated simulator results (fills the open items)

### S2 — Bounce: **there is NO per-shot bounce rule**
- `s2b-lf50k-407`: 50,000 LF (…20k; per-shot 50 = **0.0025 %** of the 2M Small
  Dome shield) → **attacker wins, dome destroyed** (r2).
- `s2b-gal600-408`: 600 Galleon (per-shot 20,000 = **0.2 %** of the 10M LSD
  shield) → **attacker wins, LSD destroyed** (r2).
- `s2b-lf5k-401`: 5,000 LF → ML destroyed.
- `s2b-lf300-407`: 300 LF total 15k damage ≪ 2M shield → **draw**, dome intact.
- `s2-gal500-408`: exactly 10M vs 10M shield → draw.

Resolution is simply **shield-first, shields fully regenerate each round, overkill
discarded**: a side whose *cumulative round damage* cannot beat the target's
shield does nothing. Any "damage < ~1 % of shield → bounce" rule is **not**
present and must not be inherited.

### S1 — Rapid fire (from the committed dataset + raw reports)
Deterministic. Steady shots/round = nominal RF (info table); **round 1 ≈
`floor(0.70 × N × RF)`**, rounds 2+ are steady. Exceptions: Frigate `227` shows
factor ≈1.0 (its info values may be ~1.43× low), and target-saturated cases
(50 BS/BC vs 1000–2000 sats) wipe all in R1 (lower bound only). Cruiser `206`
one-shots LF in the dataset — confounded, do not use for RF. Details in
`niburus_rapidfire.json` (`validated`: `216->212 = 350`, `228->212 = 500`).

### S3 — RF shot cap
No cap observed; max measured steady = **350/round** (`216 -> 212`), equals the
nominal table value. Higher-RF units (`218/221/222/224`) have no testable battles.

### S6 — Base-stat completeness
`baseStats` complete for ships `202–217, 219, 220, 225–228` and defenses
`401–419`. **Missing**: `218, 221, 222, 224` (no data anywhere) and `502, 503`
(crash the simulator). `223` unreferenced.

---

## 5. Academy A/B (`docs/COMBAT_TEST_PLAN.md` T7)

Owner's ask: measure combat outcome **with** skilled academy abilities vs
**before** spending points. Status:

- acc1 (attacker): `1102 Weapons Class A` L7 → unlocked `1103 Double attack` L5
  and `1108 Accurate shots` L1; **4 points left**.
- acc2 (defender): `1301 Defensive strategy` L8 → unlocked `1304 Mechanics` L4;
  **3 points left**.
- **Simulator A/B** (`plans/combat/t1-acad.json`, T1 comp + real techs):
  adding acc1 `1103:5 + 1108:1` produced **zero** change (identical losses /
  debris). These proc skills are too low to move the outcome.
- **Needed to make T7 measurable** (estimated from the observed point costs):
  raise the attack branch to a level the sim actually honours — e.g. acc1
  `1103 → 7`, `1108 → 5`, `1111 → 5` (or `1109/1110 → 7`). Rough cost
  **≈ 200–250 more acc1 academy points**. acc2 defence proc (`1308/1311`) needs
  the `Double shields` branch unlocked (more points). Owner supplies points on
  request.
- The real before/after strike must be **identical comps on identical targets**
  (rebuild defender mass between runs), with the same techs, comparing per-unit
  losses to the simulator with/without the same academy codes.

### 5a. Simulator isolation — attacker procs work, defender procs (as coded) do not
Spent to combat-relevant levels: acc1 **`1103:7 1108:5 1109:4 1110:2 1111:1`**;
acc2 **`1301:10 1302:4 1303:5 1311:5`** (never `1105 Engine limitation`).

Sim results (`plans/combat/t1-acad3.json`, `def-acad.json`):

| scenario | attacker losses | defender losses | debris |
|---|---|---|---|
| T1 comp, no academy | PB 47 | full wipe | M3,645,000/C4,257,500 |
| T1 comp, **+ acc1 procs** | PB **39** | full wipe | **M3,365,000/C4,077,500** |
| T1 comp, + acc2 procs | PB 47 | full wipe | M3,645,000/C4,257,500 |
| 2000 LF vs 100 BS, no acad | LF 721 | BS 100 | M3,131,500/C1,210,500 |
| 2000 LF vs 100 BS, **+ acc1 procs** | LF **509** | BS 100 | M2,813,500/C1,104,500 |
| 2000 LF vs 100 BS, + acc2 `1303:5 1311:5 1308:5` | LF 721 | BS 100 | unchanged |

**Conclusion (sim):** the **attacker-side proc branch** (`1103/1108/1109/1110/1111`)
measurably changes combat damage. The **defender-side `1303 Double shields`
(+2 %/lvl proc) / `1311 Sealing double shields` / `1308 Heavy Armour`** had **zero
effect** in these tests at L5 — mirroring finding F5 that the simulator honours
the shot/proc skills unevenly. The real server's defender-side behaviour still
needs a real A/B (acc2 already owns `1303:5/1311:5`, so there is no clean
no-skill baseline on that account).

### 5b. Real attacker-side A/B (2026-10-05)
Rebuilt an **identical** T1 target on acc2 `2:186:9` (1687) — wall
`500/300/200/100/100/100 + 407/408` + `202:2000` + `212:600` (verify MET) — and
struck it with `226:300,211:300,225:200` (all available on cp1648; no capitals
needed). **Result (real, `raport=8bd1c59086eeca395ce794e51b73cc00`):** attacker
wins 6 rounds; attacker lost **Planet Bomber 38** (Galleon/Destroyer 0); debris
**M3,330,000 / C4,055,000**; full defender wipe; `Moon Chance: 0 %`.

```
                        attacker PB lost      debris M / C
sim, no academy               47             3,645,000 / 4,257,500
sim, + acc1 procs             39             3,365,000 / 4,077,500
REAL (acc1 procs active)      38             3,330,000 / 4,055,000   <-- matches +procs
```

**Conclusion: the real server DOES apply the attacker-side academy proc branch**
(`1103 Double attack`, `1108 Accurate shots`, `1109 Chain reaction`,
`1110 Strengthening explosion`, `1111 Focusing`). Real losses/debris sit on the
"+procs" prediction (~1 % off) and clearly away from the no-academy one (~9 %).
(Contrast the earlier T1 real strike, run with only `1103:5/1108:1`, which matched
the no-academy sim.) The **defender-side** `1303/1311/1308` still show no effect
in the simulator and have no clean real baseline; leave them parametric in Go.

---

## 6. S5 — Fuel & flight samples (ungated transports, cp1648 source)

| to | coords | ships | dist | speed field | fuel |
|---|---|---|---|---|---|
| 2:191:9 | | 1 LC | 2.985 | 10 | 7 |
| 2:191:9 | | 100 LC | 2.985 | 5 | 362 |
| 2:191:10 | | 10 BS | 3.990 | 10 | 1076 |
| 3:125:9 | | 1 LC | 28.685 | 10 | 63 |
| 2:186:11 | T1 mixed | 400D+600PB+6BM+30F+15BT+300LC | 3.900 | 10 | 243.292 |
| 2:186:9 | | 300 BS | 2.890 | 10 | 23.313 |
| 2:186:10 | | 100 Galleon | 3.895 | 10 | 14.665 |
| 2:185:9 | | 200 Galleon | 2.985 | 10 | 22.475 |

Observed: fuel ∝ count and ∝ distance (1 LC: 7 @2890 → 63 @28685). "Fleet
speed" display varies by slowest ship type (LC 43.355, BS 126.295,
Galleon/Destroyer 18.944). Note the numbers are German thousands-formatted.

**Recalibrated 2026-10-05 (`internal/game/game_math.go`):**
- distance fitted piecewise to the samples (same-system `1000+5·dp`;
  same-galaxy `2700+95·ds`, plus `1000+5·dp` when positions differ;
  cross-galaxy `28000+100·dg`).
- fuel `= max(1, round(baseFuel·distance / 8750 · speedPercent/100))` — fits all
  eight samples within ~10% (down from ~3×). `baseFuel`: LC 20, BS 250,
  Galleon 320 (solved); others defaulted. Fuel scales *down* with speed here.
- flight time `= round(4275 · sqrt(distance·10/maxSpeed) / fleetSpeed)`, fitted to
  the single live time sample (distance 1035 @15× ≈ 300 s one-way). Needs more
  time samples to be precise. Regression tests lock the fuel samples and the
  ~300 s figure.

---

## 7. Server facts confirmed this session

- **Loot = 50 %** of each resource, **M → C → D**, not cargo-capped when enough
  freight capacity exists (verified: 3.3B looted).
- **Cargo capacities are server-scaled**: Battle Transporter `217` = **400M**,
  Battle Recycler `219` = **200M** (owner). Light/Heavy Cargo are effectively only
  **meatshields** at these rates, not freighters.
- **Debris = 50 % of base M+C, ships only** (defenses give 0) — confirmed again
  (T2: 101 sats → 202k crystal, 0 metal).
- **Defense repair ≈ 61 %** for both normal battles and total wipes; domes
  probabilistic per single unit.
- **Moon generation off / 0 %** (real report + no spawns).

---

## 8. Artefacts

- Plans: `tools/explorer/plans/combat/t1-wipe*.json`, `t1-acad.json`,
  `t2-repair.json`, `s2-bounce*.json`, `moon-chance-probe.json`,
  `acc2-target-wipe.json`, `moon-2500lc.json`, `moon-5000lc.json`.
- Real reports: `tools/explorer/data/t1-real-report.html`
  (`raport=aea7b462506f77330f22e0e33b5001dd`), plus hashes in
  `data/msg2.html` (`0ce292…` 1688, `5827af…` T2/1674, `0b64ed…` 1696,
  `2b2605…` 1687).
- Snapshots: `data/post1689.json`, `data/post1674.json`, `data/t1-real-report.html`.
- Sim reports folded under `tools/explorer/data/combat/`.

---

## 9. Still open after this session

1. **T7 academy A/B** — blocked on more academy points (est. ~200–250 for acc1).
2. **T3 `1304` isolation** — needs a no-`1304` baseline (separate account/planet);
   current delta is indicative only.
3. **S5 fuel/flight formula** — samples collected, fit not yet done.
4. **Bash limit** per destination — not exhausted this session (≤1 strike/target).
5. **Defense repair** — refine probabilistic model with more samples (61 % ±
   noise; domes special).

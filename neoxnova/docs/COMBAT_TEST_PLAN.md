# Combat Test Playbook — acc1 × acc2 on niburuspace.com

Status: **ARMED — waiting for the owner's start signal.** Do not start real
attacks until both accounts' colonies are sufficiently built out (points gate).

Owner intent: finish the remaining **live** combat tests (no Go implementation
yet), combining attacks where one strike can answer several unknowns, and
parallelising across planets so the agent loop runs as briefly as possible
(loop time = tokens = money).

This file is written to be **self-contained for catch-up**: if a session dies,
read only this file (plus the referenced docs) and continue.

Read order for full context:
1. This file.
2. `docs/COMBAT_FINDINGS.md` (sleek confirmed model).
3. `docs/COMBAT_MODEL.md` (evidence + open items).
4. `docs/COMBAT_REAL_TESTS.md` (real-attack campaign + this-server rules).
5. `docs/COMBAT_SESSION_2026-10-04.md` (tooling cheat sheet, snapshots).

---

## 0. STATUS BOARD (update in place as work proceeds)

Legend: `[ ]` todo · `[~]` in progress · `[x]` done · `[!]` blocked

Gating

- [x] **C0** Gate check: points ratio 2.23:1 (passes), targets chosen + moonless
      confirmed, bash limit not exhausted (≤1 strike per destination so far).

Ungated (can run in parallel with the colony build-out; do NOT need the gate)

- [x] **S1** Rapid-fire extra-shot rule + chain — deterministic, steady = nominal
      RF, round 1 ≈ `floor(0.70 × N × RF)`; Frigate `227` and saturated cases are
      exceptions. See `docs/COMBAT_SESSION_2026-10-05.md` §4.
- [x] **S2** Bounce operator — **no per-shot bounce**; shield-first + full regen.
- [x] **S3** Rapid-fire shot cap — none ≤350/round (equals nominal).
- [ ] **S4** ACS / multi-slot POST capture (`action=moreslots`, local browser).
- [~] **S5** Fuel/flight samples collected (8 rows, session doc §6); formula not
      yet fitted.
- [x] **S6** Base-stat completeness — complete for modelled roster; `218/221/222`
      `224/502/503` missing.

Gated on C0 (real attacks)

- [x] **T1** Full-wipe battery — attacker win r6; loot 50 % (M→C→D), debris
      M5,455,000/C5,515,000, wipe repair 61.6 %, domes 407 lost / 408 restored,
      no moon (real report `Moon Chance: 0 %`). See session doc §1.
- [x] **T2** Defense repair normal (draw) — pooled 325/508 = **64.0 %**
      (acc2 `1304` L4 active). See session doc §2.
- [~] **T3** `1304` delta — acc2 L4 active for T1+T2 (61.6 %/64.0 %); no
      no-`1304` baseline, so indicative only.
- [x] **T4** Moon series — 1.2M/6.2M/13.5M/11M debris, **no moon**; real report
      prints 0 %. Concluded: server moons off; adopt classic formula in Go.
- [x] **T5** Real mixed-fleet debris — folded into T1 (matches sim ≈0.6 %).
- [x] **T6** Real RF validation — T1 real report per-unit == sim (BM/Frigate/Dest
      behaviour reproduced).
- [x] **T7** Academy A/B — acc1 attacker procs (`1103:7 1108:5 1109:4 1110:2
      1111:1`) **confirmed on the real server**: real PB lost 38 / debris
      M3,330,000 vs sim +procs 39 / M3,365,000 and no-academy 47 / M3,645,000.
      Defender-side `1303/1311/1308` unchanged in sim. See session doc §5.

Already DONE — do not redo (see COMBAT_FINDINGS/MODEL/REAL_TESTS)

- [x] Determinism; round structure; 8-round cap; overkill discarded.
- [x] Quadratic tech bonus `round(L(L+2)/4)%` for 109/110/111.
- [x] Base stats table (tech 0) for 41 units.
- [x] Debris = 50% base M+C, **ships only** (defenses = 0).
- [x] Bounce exists (~<1% of shield).
- [x] Rapid fire exists and drives screen-clearing; nominal RF matrix captured.
- [x] Loot = `min(floor(0.5*stored), cargo)`, M→C→D (**simulator only**).
- [x] Real battle == simulator (multiple end-to-end validations).
- [x] Debris spawn at target + recycle with Battle Recycler 219 (type 2).
- [x] Simulator moon chance always 0% even at 60M debris (F17).
- [x] Moon-shot sacrifice: **Light Cargo `202`** (>0-attack LF `204`) for attacker
      losses — 2000 LC = 0 attacker losses vs 2000 LF = 11 BS (≈638k), same debris
      (sim, 2026-10-05). See T4.

---

## 1. GROUND TRUTH

### Accounts

- **acc1 attacker: Bratwurst** — runs **locally on Windows** (this host).
  Creds in `neoxnova/secrets/explorer.env` (`NIBURU_USER`/`NIBURU_PASS`).
- **acc2 defender: TheBob** — runs on the **Azure VM**
  (`azureuser@70.153.144.215`, key `neoxnova/secrets/ssh/VM-GW-Automation_key.pem`).
  Creds are the VM's `secrets/explorer.env` (second account).

### Planet roster (re-verify with `planets` at start; moonless noted)

acc1 Bratwurst (`cp` ids):

| cp | coords | note |
|---|---|---|
| 1593 | 3:125:12 | Gotijy, main hub |
| 1648 | 2:188:9 | **Xusyty, fleet hub** (closest to acc2) |
| 1655 | 3:125:9 | Fazif |
| 1656 | 3:125:10 | Nemyvon |
| 1657 | 3:125:11 | Votoru |
| 1690/1692/1693 | 3:124:9/10/11 | new colonies |
| 1691/1694/1695 | 2:191:9/10/11 | new colonies |

acc2 TheBob (`cp` ids):

| cp | coords | note |
|---|---|---|
| 1598 | 2:188:16 | Japoqu, home |
| 1672/1673 | 2:188:10/11 | Mybeti, Hufumyd |
| 1674/1675/1676 | 2:187:9/10/11 | Sysezum, Quitamo, Tumebi |
| 1687/1688/1689 | 2:186:9/10/11 | Zojiqu, Xepaku, Cuxiw (new) |
| 1696/1697/1698 | 2:185:9/10/11 | Cukywi, Chepola, Wutoxel (new) |

All acc2 **new** colonies are moonless and are the prime strike targets.

### Server access rules (learned the hard way — COMBAT_REAL_TESTS §0)

- **Points balance gate ~4:1, both directions.** Outside it: attacker sees
  "Noob Protection", defender sees "Player is too strong". This is why the
  colony build-out (raising acc2 points) gates all real attacks.
- **Bash limit** = "maximum number of attacks at this destination today",
  **per destination**. Multiple planets = multiple fresh targets per day.
  Determine the exact per-day cap empirically at C0.
- Attacks only on non-alliance; acc1/acc2 are not allied.

### Distance reference

`2:188:9 <-> 2:188:16` = distance **1035**, one-way ≈ 5 min (observed).
Other pairs: compute with `CalculateCoordinateDistance` or the send dialog.

---

## 2. TOOLING QUICK REFERENCE

Run from `neoxnova/tools/explorer/` (acc1 local) or the same path on the VM
(acc2). `httpbot.mjs` is the browser-less client.

```
# roster / snapshot
node --max-old-space-size=96 httpbot.mjs planets
node --max-old-space-size=96 httpbot.mjs levels --out data/x.json --cp 1648

# simulator batch (reads [{id,attacker,defender,slots?}])
node --max-old-space-size=96 httpbot.mjs simsuite plans/combat/<file>.json

# real attack (mission 1) from cp1648's ships; speed 10 = 100%
node --max-old-space-size=96 httpbot.mjs fleet 2:188:16 1 "204:100,206:20" 10 --cp 1648

# recall newest outgoing fleet
node --max-old-space-size=96 httpbot.mjs fleetback <fleetID>

# recycle debris (mission 8 auto-targets type 2)
node --max-old-space-size=96 httpbot.mjs fleet 2:188:16 8 "219:1" 10 --cp 1648

# read real combat messages (the real report hash lives in the message)
node --max-old-space-size=96 httpbot.mjs dump "page=messages&mode=view&messcat=3&site=1&ajax=1"
node --max-old-space-size=96 httpbot.mjs get "CombatReport.php?raport=<hash>"

# fold new captures into the committed replay set
node combat-dataset.mjs
```

Daemon controls (leave running; use dedicated planets for combat):

- acc1 local: `pwsh -File run-colonies.ps1 {start|status|logs|stop}`
- acc2 VM: `bash run-colonies.sh {start|status|logs|stop}`

Notes:
- Sending `metal=0` is rejected — use empty strings for cargo on non-loot
  missions; `httpbot cmdFleet` already handles this.
- Success detection: look for `Fleet sent` text, not the URL.
- The real report is fetched by the hash linked in the combat **message**
  (`CombatReport.php?raport=<hash>`).

---

## 3. COMBINED-ATTACK MATRIX (one strike → many answers)

The goal is to not waste strikes. Map each unknown to the conditions required,
then design a **single** attack that satisfies several at once.

| Unknown | Needs | Measurement |
|---|---|---|
| Real loot fraction/cap/order | defender has resources; attacker **wins** with surviving cargo | resource delta on both planets + cargo on return + report "Needed to capture" |
| Debris 50% ships-only (real, mixed) | mixed ships destroyed | in-game debris field M/C vs sim |
| Defense repair (wipe) | all defenses destroyed | before/after defense snapshot (planets) |
| LSD permanence | Large Shield Dome `408` present, wiped | is `408` gone and not restored? |
| Moon spawn @ high debris | ≥ ~2M debris, moonless planet | galaxy view shows moon? message? |
| Repair normal (~61%) | draw/defender-win, partial defense loss | before/after snapshot |
| Mechanics repair delta | defender has `1304` at known level | same measurement, compare ratio |

**T1 (full-wipe battery)** is therefore: a prepared **moonless acc2 target** with
known resources + a full defense wall incl. `407`/`408` + **cheap mass ships**
(LF `204` and/or sats `212`), struck by an acc1 fleet that **wipes everything and
survives**, carrying cargo ships (`202` LC and/or `217` Battle Transporter).
Size the defender's cheap mass so the resulting debris lands in the ≥20% band
(see T4). One strike then yields loot + debris + wipe-repair + LSD + moon.

`407/408` domes and a target with no moon: pick/confirm at C0.

---

## 4. UNGATED TESTS (run now, parallel to build-out)

### S1 — Rapid-fire extra-shot rule + chain

- **Goal:** pin the deterministic form (extra shots, chains) and whether counts
  are nominal `rf` or probabilistic; resolve conflict between F11 (works) and
  F22 (effective shots lower).
- **Method:** `simsuite` sweeps. Single-type attacker vs single-type defender at
  known RF pairs (Cruiser↔LF rf6, BC↔LF rf4, Black Moon↔LF rf180/sats rf350,
  Galleon↔…, Bomber↔defenses). Sweep attacker count (1,10,50,100) and defender
  count (10,100,1000,2000). Record kills/round and compare against:
  (a) `rf` extra shots deterministically, (b) `(rf-1)/rf` probabilistic mean,
  (c) capped variants.
- **Output:** a new `plans/combat/rf-*.json` + a short note appended here.
- **Acceptance:** a rule + cap that reproduces observed kills; note per-pair
  constants; hand to T6 for one real validation.

### S2 — Bounce-threshold operator

- **Goal:** exact condition (`damage < k * shield`?) and `k`.
- **Method:** `simsuite` with a fixed low-damage attacker vs a high-shield target
  (`408` LSD). Sweep attacker attack vs shield ratio across the suspected
  threshold; find the switch from 0 damage to >0.
- **Acceptance:** the operator and constant.

### S3 — Rapid-fire shot cap

- **Goal:** the cap on shots/round (if any) for extreme RF.
- **Method:** Black Moon (rf 180 vs LF, 350 vs sats) / Black Wanderer (500) vs
  large screens; compare to nominal `rf` extrapolation.
- **Acceptance:** bounded shot rule.

### S4 — ACS / multi-slot capture

- **Goal:** obtain the real POST format for `slots=3` (`action=moreslots`).
- **Method:** run the **Playwright** explorer locally (`explorer.mjs`) on acc1
  (browser allowed on Windows host), open `page=battleSimulator`, add an ACS
  slot, and capture the network/form. Save the sanitized form field names here.
- **Why local:** the VM must not run a browser (per AGENTS); local acc1 can.
- **Acceptance:** working `slots=3` post that yields two attacker rows.

### S5 — Fuel & flight-time recalibration samples

- **Goal:** real formula for `CalculateFlightDuration` /
  `CalculateDeuteriumConsumption` (currently ~3× off, F12).
- **Method:** send cheap fleets (transports/deploy to own planets = ungated) at
  several **distances** (same system, adjacent system, cross-system,
  cross-galaxy), several **speeds** (10/9/…/1), several **fleet sizes**, using
  distinct types (LF `204`, BS `207`, Cruiser `206`, Battle Recycler `219`).
  **Record engine techs 115/117/118 with every sample** (they change with
  research — F "engine tech does not affect combat, only flight").
- **Output:** append rows to a samples table in this file or
  `tools/explorer/data/fuel-samples.json`.
- **Acceptance:** a formula fitting the new samples better than the current one.

### S6 — Base-stat completeness

- **Goal:** ensure every buildable ship/defense has attack/shield/hull in the
  dataset; fill gaps for rare units.
- **Method:** tech-0 single-unit `simsuite`; cross-check `data/scan-*.json`.
- **Acceptance:** no missing units in `testdata/niburus_combat.json` baseStats.

---

## 5. GATED TESTS (need C0 passed)

### C0 — Gate check & target selection (do first when signalled)

1. `planets` on both accounts; snapshot `levels` for candidate targets.
2. Read points (`page=statistics` or overview rank) and compute the acc1:acc2
   ratio. Confirm it is inside ~4:1 in **both** directions. If not, keep
   building acc2 and stop.
3. Confirm the per-destination bash limit (send repeated probes / read the
   message after the limit hits). Record the cap here.
4. Choose targets (record cp + coords):
   - **T1/T4 target:** acc2 **moonless** colony; will hold resources + defenses
     (incl. `407`,`408`) + cheap mass ships.
   - **T2 target:** acc2 colony with a stable defense wall for repeated strikes.
   - **T3 target:** acc2 colony where `1304` (Mechanics) is researched to a known
     level.
   - Prefer **different coord systems** per test to avoid bash-limit collisions.
5. Confirm the galaxy view shows **no moon** at the chosen positions
   (`dump` the galaxy page or use the Playwright `scan`/`map`).

### Target setup (build before striking)

Use `resolve` plans (see `tools/explorer/plans/combat/`) to place, on the acc2
target planets:

- Resources: send from acc2 home/colonies via `fleet … 3` (transport) to reach a
  **known** stored amount (record it).
- Defenses: wall `401`–`406` + `407`+`408` (plan `defenses` map). Note: defense
  repair tests need accurate before/after **defense** snapshots (`levels`).
- Cheap mass ships for debris/moon: **Light Cargo `202`** (preferred for pure
  moon-shots — attack 0, so the attacker is unharmed; see T4) or sats `212`
  (ships → debris; defenses → **0 debris**). Target ~2M–20M+ debris per T4 stage.

On acc1: assemble a **winning** fleet + **surviving cargo** on `cp1648` (Xusyty
`2:188:9`). Strongest observed comp: `5 Black Moon 216 + 20 Frigate 227` wiped
`500 Cru + 500 BS` with **0 losses**; cheaper: mass Planet Bombers vs defenses.
Record the exact fleet you send.

### T1 — Full-wipe battery (loot + debris + repair(wipe) + LSD + moon)

- **One strike design:** acc1 fleet that destroys **all** defender ships+defenses
  on the moonless target and **survives**, with cargo `202`/`217` aboard.
- **Measure:**
  1. **Loot (real):** target resources before vs after; acc1 cargo on return;
     report "Needed to capture the resources". Verify
     `min(floor(0.5*stored), cargo)`, order M→C→D.
  2. **Debris (real):** galaxy debris field M/C vs simulator on the same comp.
  3. **Repair (wipe):** defenses before vs after → `repaired = destroyed - netLoss`
     (expect ≈37%).
  4. **LSD permanence:** is `408` gone permanently?
  5. **Moon:** did a moon appear at the target coords?
- **Acceptance:** numbers within tolerance; LSD answer; moon on/off.
- **Depends:** C0; target setup; acc1 winning fleet + cargo.

### T2 — Defense repair (normal battle), 2–3 strikes

- **Goal:** confirm ≈61.5% and its variance (probabilistic vs fixed).
- **Method:** 2–3 strikes on the T2 target that end in **draw / defender-win**
  with partial defense loss (attacker has no RF vs the wall). Snapshot defenses
  before/after each; compute `repaired/destroyed`.
- **Guard:** bash limit — either space the strikes or use different target
  colonies; record the limit hit.
- **Acceptance:** pooled ratio ≈61%; per-unit spread noted.

### T3 — Mechanics `1304` repair delta

- **Goal:** quantify `1304` (+1%/lvl Defense recover) effect on repair.
- **Method:** repeat T2 on a target where acc2 has `1304` at a known level;
  compare ratio to T2 baseline.
- **Note:** acc1's earlier base rate (~59%) was account-independent (COMBAT_REAL_TESTS §4);
  measure the **defender (acc2)** holding `1304`.
- **Acceptance:** delta per level; feed back into the repair model.

### T4 — Moon chance series

- **Goal:** determine if the **real server** creates moons (sim says 0%) and, if
  yes, the chance vs debris.
- **Preconditions:** moonless planet; each attack that could moon must be the
  one creating the debris (moon chance is evaluated on the attack, not on
  recycle); once a moon exists, that planet is used up for this test.
- **Method:** generate escalating debris in stages on separate moonless planets
  (to dodge bash limit), roughly: **~2M (≈20% OSS), ~5M, ~10M, ~20M+**. Debris ≈
  50% of base M+C of destroyed ships.
- **Sacrifice choice — sim-proven 2026-10-05** (`plans/combat/80-moonshot-lc-vs-lf.json`):
  use **Light Cargo `202`** as the sacrificial defenders, **not** Light Fighters.
  `2000 LF` cost the attacker **11 Battleships (≈638k)**; `2000 LC` cost **0** — for
  the same **4.0M** defender debris (traders have attack 0). Rough mass:
  `1000 LC ≈ 2.0M` (20% cap), `2000 LC ≈ 4.0M`; scale up for larger stages.
  Use combat ships only when the strike is *also* a combat test (T1).
- **Measure:** galaxy view / moon message after each strike.
- **Acceptance:** on/off; if on, at least 2–3 points to bound the formula.
- **Note:** the OSS cap is ~20% (≈1%/100k debris up to 2M). The simulator reports
  0% even at 60M, so the real server may be off; this test settles it.

### T5 — Real mixed-fleet attacker-win debris

- Fold into T1 if the T1 attacker fleet is mixed; otherwise a small extra strike.
- Verify debris = 50% base M+C ships only against the simulator.

### T6 — Real RF-pair validation

- **Goal:** validate the S1 rule with one real attack using an RF pair (e.g.
  Cruisers vs LF, or Black Moon vs sats) with matching simulator techs.
- **Acceptance:** real kills/round ≈ S1 prediction.

---

## 6. EXECUTION ORDER & PARALLELISM (minimise loop time)

While colonies build (now): run **S1–S6** in parallel (simulator + a few local
browser/transport actions). These need no gate and cost little.

On start signal:
1. **C0** gate + targets (single short session).
2. **T1** full-wipe battery (one strike → 5 answers).
3. **T2 + T3** in parallel across two different acc2 colonies (one normal, one
   with `1304`) — separate destinations dodge the bash limit.
4. **T4** moon series across remaining moonless colonies (staged debris).
5. **T6/T5** validation strikes as needed.

Parallelisation levers: multiple destination planets; the simulator is free;
acc1 and acc2 work can be prepared independently.

---

## 7. STATE SNAPSHOT (verify at start — these go stale)

- acc1 research (COMBAT_SESSION snapshot, 2026-10-04):
  `106:18 108:19 109:15 110:15 111:15 113:19 114:15 115:20 117:16 118:14
  120:20 121:18 122:16 123:7 124:5 131:19 132:19 133:19 199:2`.
- acc1 fleet hub `cp1648` (2026-10-04): `202:2000 204:15500 205:1000 206:1500
  207:1000 211:600 212:5439 215:600 219:500 225:603 226:637` (+ `216`/`227`
  were being built). **Re-snapshot before striking.**
- acc2 research (2026-10-04): `… 109:15 110:15 111:15 … 123:3 124:2 …`.
- acc2 home defenses (2026-10-04): `401:420 402:259 403:190 404:100 405:100
  406:50 407:1 408:1`.
- acc1 also has `1301:8` (Arsenal) & Mechanics research noted; acc2 to get
  `1304` for T3.
- Local snapshots: `tools/explorer/data/acc1-p{1593,1648,1655,1656,1657}.json`,
  `account2-levels.json`, `state-snapshot.json` (gitignored).

---

## 8. OPEN QUESTIONS / UNKNOWNS

- Repair: probabilistic (~0.6) vs fixed ratio; wipe-rate mechanism (~0.37).
- LSD permanence needs a 2nd confirmation.
- RF: deterministic-per-run vs probabilistic; effective shot cap.
- Moon: real server on/off; formula if on.
- ACS multi-slot format.
- Fuel/flight formulas.
- Academy proc-skills real effect (F5) + weapon-type techs real contribution.
- Does the real server's report expose loot explicitly ("Needed to capture")?

---

## 9. RESULT LOG (append newest at top)

Template per entry:
`YYYY-MM-DD | task | target cp/coords | fleet sent | result summary | files`

- 2026-10-05 | T1 | acc2 1689 `2:186:11` | `226:400,211:600,216:6,227:30,217:15,202:300` |
  attacker win r6, wipe; loot M1.869B/C954M/D492M (=50 %, no cargo cap); debris
  M5.455M/C5.515M; repair 802/1302=**61.6 %**; 407 lost, 408 restored; no moon
  (report `Moon Chance: 0 %`) | `data/t1-real-report.html` |
- 2026-10-05 | T2 | acc2 1674 `2:187:9` | `226:80` | draw r9; defLost
  ML200/LL114/HL80/Ion38/Gauss38/Plasma38+sats101; repair 325/508=**64.0 %** |
  `raport=5827af03a96d2aef9a406d144d09d828` |
- 2026-10-05 | T4 | acc2 1687/1688/1696 | 300 BS / 100 Galleon / 200 Galleon |
  debris 1.2M / 6.2M / 13.5M, **no moons**; real report shows 0 % → moons off |
  session doc §3 |
- 2026-10-05 | S2 | simulator | LF/Galleon vs 407/408 | **no per-shot bounce**;
  shield-first + full regen; 0.0025 % and 0.2 % shots still killed the domes |
  `plans/combat/s2-bounce2.json` |
- 2026-10-05 | S7-acad | simulator | T1 comp ± `1103:5 1108:1` | no measurable
  difference → need more academy points for T7 | `plans/combat/t1-acad.json` |
- 2026-10-05 | S(moon) | simulator | 200 BS/300 BS vs 1000/2000 LF and LC | LC:
  attacker 0 losses; LF: 11 BS lost, same ~4.0M defender debris → use LC for
  moon-shots | `plans/combat/80-moonshot-lc-vs-lf.json`, `data/combat/moonshot-*.report.json`

_(Real-attack results will be appended above as the campaign proceeds.)_

---

## 10. CHANGE LOG (this file)

- Created: combat test playbook/runbook, combined-attack matrix, task tracker,
  gated/ungated split. No live actions taken (colonies still building).
- 2026-10-05: added moon-shot sacrifice finding (Light Cargo 202 preferred);
  `plans/combat/80-moonshot-lc-vs-lf.json`.

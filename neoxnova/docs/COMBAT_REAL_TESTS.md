# Combat Real-Test Findings — acc1 vs acc2 (niburuspace.com)

Live-battle campaign between **acc1 Bratwurst** (attacker) and **acc2 TheBob**
(defender) on `universe_6_niburu`, cross-checked against the in-game simulator.
Companion to `COMBAT_FINDINGS.md` (model) and `COMBAT_MODEL.md` (evidence); the
machine-readable replay set is `testdata/niburus_combat.json` and the rapid-fire
matrix is `testdata/niburus_rapidfire.json`.

Method: snapshot defender `levels` → send real `ATTACK` → read the real combat
report → re-snapshot. `repaired = reportDestroyed - (before - after)`.

## 0. Access rules of this server (learned the hard way)

- **Points balance gate:** attacks are only allowed within a **~4:1 points
  ratio, in both directions**. `7.04M` vs `1.60M` = 4.4:1 → acc1 saw
  "Player is in the Noob Protection!" and acc2 saw "Player is too strong!".
  Growing acc2 (colonies) closed the gap and unblocked both directions.
- **Bash limit:** "maximum number of attacks at this destination today". It is
  **per destination**, so a defender with multiple planets gives multiple fresh
  strike targets per day.
- Map: acc1 fleet planet **Xusyty 2:188:9**; acc2 home **Japoqu 2:188:16** plus
  colonies 2:188:10/11 and 2:187:9/10/11.

## 1. Rapid fire (RF)

- Authoritative outgoing table captured from the unit info pages
  ("He makes shots per round") → `testdata/niburus_rapidfire.json` (303 entries).
  The earlier generic scan was wrong because it **merged the two info tables**
  ("makes" = outgoing vs "gets" = incoming).
- Simulator: deterministic; **steady shots/round == the info-table value**
  (BS↔sats 25, Cruiser 10/11, Black Moon 350 vs sats & 180 vs LF, Galleon 125,
  Black Wanderer 500). **Round 1 ≈ 0.70×** the steady value; rounds 2+ are steady.
- **Real battles reproduce the simulator per-unit exactly** (see §3).

## 2. Debris & loot

- Debris = **50% of base Metal+Crystal of destroyed ships only**; defenses
  contribute 0. Confirmed twice against sim predictions:
  - S1: real `M28,989,500 / C34,125,000` vs sim `M28,957,500 / C34,108,250`.
  - S3: real `M17,500 / C212,000`. The metal came from the 1 lost Battle
    Transporter (`M35,000→17,500`) and crystal from 101 sats (`C4,000→2,000`),
    matching exactly.
- (No loot in these attacks — attacker was wiped/drew.)

## 3. Real report == simulator (validation)

| strike | attacker | result | sim | real | note |
|---|---|---|---|---|---|
| S1 | 500 BS + 300 BC + 300 PB → Japoqu | defender, 2 rds | losses/debris as above | identical (403: 12 vs 13) | matches |
| S3 | 100 Destroyer (+1 BT) → Mybeti | draw, 9 rds | defLost 254/153/101/52/52/52, atkLost 0 | **identical** | exact |
| S6 | 6 Black Moon (+1 BT) → Quitamo (3000 LF) | attacker, 5 rds | 3000 LF wiped, 0 BM lost | 3000 LF + 400 sats wiped, 0 BM lost | BM RF validated |
| S8 | 30 Frigate (+1 BT) → Mybeti | draw, 9 rds | defLost 69/43/30/15/15/15 | **identical** | Frigate RF validated |

S3 real `defLost`: 401:254, 402:153, 403:101, 404:52, 405:52, 406:52, sats 101;
`atkLost`: only the 1 Battle Transporter. This is a strong end-to-end lock.

## 4. Defense recovery (NEW — main result)

After each battle, a fraction of the **destroyed defenses is restored**.
`repaired = reportDestroyed - netLoss`.

**Normal battles (draw or defender-win): repair ≈ 61%.**

| strike | planet | result | destroyed (401/402/403/404/405/406) | repaired | % |
|---|---|---|---|---|---|
| S1 | Japoqu | defender | 61 / 37 / 13 / 0 / 0 / 0 | 31 / 21 / 8 | 57 |
| S3 | Mybeti | draw | 254 / 153 / 101 / 52 / 52 / 52 | 152 / 98 / 66 / 28 / 27 / 35 | 61 |
| S4 | Hufumyd | draw | 226 / 138 / 92 / 46 / 46 / 46 | 124 / 97 / 60 / 29 / 31 / 31 | 62 |
| S5 | Sysezum | draw | 226 / 138 / 92 / 46 / 46 / 46 | 154 / 77 / 63 / 25 / 29 / 28 | 63 |
| S8 (Frigate) | Mybeti | draw | 69 / 43 / 30 / 15 / 15 / 15 | 43 / 29 / 16 / 9 / 8 / 9 | 61 |
| **ACC1** (base) | Xusyty | draw | 100 / 61 / 38 / 23 / 23 / 23 | 63 / 30 / 25 / 13 / 15 / 12 | **59** |

**Pooled normal: 1486 repaired / 2418 destroyed = ~61.5%.** Note the last row:
acc1 defending also repairs ~59% → the base rate is **account-independent**
(acc1 had no `Mechanics`). acc1's fleet was deployed away so its 420 Destroyers
faced only the 500/300/… defenses; the Destroyers survived (draw) and returned.

**Total-wipe (attacker WIN): repair drops to ~37%, and the Large Shield Dome is
destroyed permanently.**

- S7: 300 Destroyers wiped **all** 500/300/200/100/100/100 + both domes in 8
  rounds, attacker lost 0. Afterwards: `340/183/118/67/65/50` + Small Shield
  Dome `1` — **Large Shield Dome `408` gone (never restored)**. That is
  **478/1302 = ~37%** repaired.

So "instant destruction" is real but partial: when the defender is overrun and
every defense dies, repair is roughly halved (~37%) and the Large Shield Dome is
permanently lost. In normal (non-wipe) battles repair is ~61%.

Still open:
- Is the 61% per-unit-probabilistic or a fixed ratio? Spread 49–68% over many
  units suggests probabilistic (~0.6). More samples will tighten it.
- Does the ~37% wipe rate reflect a different formula, or "destroyed in the
  final round" not being repaired? LSD permanence needs a second confirmation.
- Academy `1304 Mechanics` (+1% Defense recover/lvl): acc1 now has it unlocked
  (bought 1301→8); the next acc1-defended battle will measure the delta vs the
  **59%** base above.

Data: `tools/explorer/data/defense-recovery.json`.

## 5. Academy effects

- Full 3-branch tree captured (`tools/explorer/data/academy-map.json`).
- **Simulator honours the proc/shot skills** (1103 Double attack, 1108 Accurate
  shots, 1109 Chain reaction, 1110 Strengthening explosion, 1111 Focusing) and
  **ignores flat stat skills** (1101 Weaponry, 1102 Weapons Class A, 1211 Empire,
  1301/1302/1305/1306) even at level 50.
- acc1 currently owns only flat skills (1101:6, 1102:3, 1301:3, 1304? no) plus
  economy/utility ones; the real S1/S3 battles match the simulator (which ignores
  flat skills) **exactly** → flat-stat academy skills had **no measurable combat
  effect** in these battles. Proc-skill real effect still to be measured.

## 6. Tooling used

- `httpbot.mjs fleet <g:s:p> <mission> <code:count,...> <speed> --cp <id>` —
  `speed` is the form value (10 = 100%, 1 = 10%); add a Battle Transporter for
  cargo/fuel. Missions: 1 attack, 3 transport, 7 colonize, 8 recycle.
- `httpbot.mjs simsuite <file>` — batch simulator (use exact techs).
- `httpbot.mjs academy-map` / `academy-up <code> <target>`.
- `rapidfire-scan.mjs` → `testdata/niburus_rapidfire.json`.
- `node httpbot.mjs get "game.php?page=bonus"` — Online Bonus (grants Dark
  Matter + peaceful XP; the button only appears on cooldown-ready).

## 7. Final live campaign — 2026-10-05 (updates §4)

Full write-up: `docs/COMBAT_SESSION_2026-10-05.md`.

### T1 — total wipe on acc2 `2:186:11` (real, one strike)
Attacker (acc1 cp1648) `226:400,211:600,216:6,227:30,217:15,202:300`; defender
wall `500/300/200/100/100/100 + 407/408` + `202:2000` + `212:600`.
- **Attacker wins, 6 rounds**; both domes killed r5, field empty r6.
- Debris **M5,455,000 / C5,515,000** (matches sim within ~0.6 %).
- Loot **M1,869,021,360 / C954,035,646 / D492,122,212** = exactly **50 %**,
  M→C→D, **no cargo cap** (freight = 15 Battle Transporters × 400M).
- **Wipe repair = 802/1302 = 61.6 %** — i.e. the same ~61 % as a normal battle.
  This **contradicts §4's "~37 % wipe"** and the "LSD permanently lost" claim:
  here `407` (Small Dome) was lost and `408` (Large Dome) was restored.
- Real report prints **`Moon Chance: 0 %`**; no moon.

### T2 — normal repair on acc2 `2:187:9`
`226:80` → draw 9 rounds; defLost ML200/LL114/HL80/Ion38/Gauss38/Plasma38 +
101 sats; debris C202,000. Post `362/203/134/62/68/70` → **repair 325/508 =
64.0 %** (acc2 had academy `1304` L4 active).

### T4 — moons
1.2M / 6.2M / 13.5M / 11M ship debris on moonless planets → **no moons**, and the
real report shows 0 %. Concluded: real moons disabled; adopt classic OGame
`min(20 %, floor(debris/100000)%)` for the Go model.

### S2 — no per-shot bounce
50,000 LF (0.0025 %/shot of a 2M shield) and 600 Galleon (0.2 %/shot of 10M)
both destroyed the shield domes. Resolution is shield-first + full regen; a side
whose total round damage cannot beat the shield simply does nothing.

### Repair model (updated)
Treat repair as **~61 % of destroyed defenses**, applied the same whether the
battle was a draw or a total wipe. Shield domes are single units → their restore
is effectively a 61 % coin flip. `1304 Mechanics` adds ~1 %/level (acc2 L4;
indicative only, no baseline).

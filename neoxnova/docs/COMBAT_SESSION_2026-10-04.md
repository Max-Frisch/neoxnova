# Combat Scouting — Session Log 2026-10-04

Durable memory for the combat-data scouting on niburuspace.com leading to the
neoxnova Go combat engine. **Read `docs/COMBAT_FINDINGS.md` first** (sleek), then
`docs/COMBAT_MODEL.md` (detailed evidence), then this log for tooling/state.

Environment: Windows dev host + PowerShell; Go module `neoxnova/`; all explorer
commands run from `neoxnova/tools/explorer/`.

Primary deliverable: **`testdata/niburus_combat.json`** — 299 replay scenarios,
41 base units, 30 recorded PHP-error cases. Regenerate with
`node tools/explorer/combat-dataset.mjs`.

---

## 1. What was done

1. Stopped all economy daemons (acc1 colony resolvers + acc2 resolver on the VM).
2. Built combat tooling: report parser, batch simulator runner, real-fleet cp/type
   support, dataset generator, analyzer.
3. Ran ~23 scenario batches (hundreds of simulator runs) + 4 real fleet strikes.
4. Captured base stats, the quadratic tech curve, debris/loot/moon, bounce, round
   cap, rapid fire, overkill, defender-bonus, meatshield, anti-defense, endgame.
5. Wrote the findings docs and the committed replay dataset.

## 2. Confirmed findings (details in COMBAT_FINDINGS.md)

- 8 rounds max; 1 shot/round at one target; overkill discarded; shields regen each
  round; hull persists; bounce below ~1% shield.
- Tech bonus `round(L(L+2)/4)%` for 109/110/111 (L15 = +64%); weapon-type techs
  `120/121/122/199` and academy skills modify damage but are hidden in the card.
- Debris 50% base M+C, **ships only**; loot 50% cargo-capped; moon always 0%.
- **No defender bonus** (side-swap identical).
- **Meatshield confirmed**: 100 BS losses 100→33→9→2→0 as LF screen grows
  0→500→1000→2000→4000 (≈5 screen ships per capital).
- Planet Bomber/Destroyer strongly counter defenses; endgame `5 Black Moon +
  20 Frigate` wiped 500 Cru + 500 BS with zero losses.
- Real battles reproduce the simulator (big strike debris within ~0.2%).

Real battles captured (`data/`):
1. `50 LF+10 Cru` → acc2: attacker wiped, debris 150,000/106,500 (matches sim).
2. `300 LF+200 HF+150 Cru+80 BS+40 BC` → acc2: defender wins 5 rounds, attacker
   wiped, debris 5,336,500/5,424,500 (sim 5,329,000/5,445,750).
3. **reverse** acc2 `100 LF+60 HF+35 Cru+15 BS` → acc1 `2:188:9` (ships-only, no
   towers): defender wins 4 rounds, attacker wiped, debris 1,088,000/1,038,750;
   acc1 lost 51 LC/12 LF/11 HF/2 Cru/261 sats. File: `data/real-reverse2.html`.
4. Recycle with Battle Recycler (219) emptied the debris field.

## 3. Tooling changes (uncommitted)

| file | change |
|---|---|
| `tools/explorer/parse.mjs` | added `parseCombatReport(html)` |
| `tools/explorer/httpbot.mjs` | `planets`, `levels --cp`, `simsuite`, `get`, `fleet --cp`, recycle target type 2 for mission 8, PHP-error detection, `EXPLORER_*` env overrides, accurate ship/defense counts |
| `tools/explorer/combat-dataset.mjs` | folds `data/combat/` into `testdata/niburus_combat.json` |
| `tools/explorer/analyze-combat.mjs` | prints per-unit loss tables |
| `tools/explorer/plans/combat/*.json` | 22 scenario batches + build plans (tracked) |
| `docs/COMBAT_FINDINGS.md`, `docs/COMBAT_MODEL.md` | new |

Raw captures live in `tools/explorer/data/combat/` (gitignored) — do not commit
(large; some contain account coords).

## 4. Snapshots as of shutdown

Acc1 (Bratwurst) research (all planets share it):
`106:18 108:19 109:15 110:15 111:15 113:19 114:15 115:20 117:16 118:14 120:20 121:18 122:16 123:7 124:5 131:19 132:19 133:19 199:2`

| planet | coords | key buildings | fleet |
|---|---|---|---|
| Gotijy cp1593 | 3:125:12 | 1:51 2:48 3:45 4:37 6:7 14:22 15:11 21:18 31:24 71:10 | 208:7, 212:70003 |
| Xusyty cp1648 | 2:188:9 | 1:45 2:43 3:40 4:28 14:22 15:9 21:16 31:23 | 202:2000 204:15500 205:1000 206:1500 207:1000 211:600 212:5439 215:600 219:500 225:603 226:637 |
| Fazif cp1655 | 3:125:9 | 1:42 2:40 3:36 4:32 14:20 15:9 21:16 31:22 | 212:1000 |
| Nemyvon cp1656 | 3:125:10 | 1:41 2:39 3:35 4:32 14:20 15:9 21:16 31:22 | 212:1000 |
| Votoru cp1657 | 3:125:11 | 1:41 2:39 3:35 4:32 14:20 15:9 21:16 31:22 | 212:1000 |

Acc2 (TheBob) Japoqu `2:188:16`, research:
`106:10 108:12 109:15 110:15 111:15 113:14 114:10 115:15 117:11 118:9 120:14
121:16 122:7 123:3 124:2 131:13 132:13 133:12`; defenses `401:420 402:259
403:190 404:100 405:100 406:50 407:1 408:1`; ships `204:24 205:12 206:5 207:4
211:300 212:2239 225:274` (was building Planet Bombers → 300 and Galleons → 300).

Local JSON snapshots: `data/acc1-p{1593,1648,1655,1656,1657}.json`,
`data/account2-levels.json`, `data/state-snapshot.json`.

## 5. Jobs that were running when the PC shut down (NOT preserved)

Local jobs are killed by the PC shutdown; the **acc2 job runs on the Azure VM and
keeps going** (tmux `acc2unlock`). Game-queue state persists, so re-run any local
resolver to resume it:

- **acc1 `2:188:9` build** — `plans/combat/acc1-fleet2.json` (sats 25k + big fleet)
  then `plans/combat/acc1-research.json` (109:16,110:16,111:17,121:19,199:5, and
  build Black Moon 216/Frigate 227). Launcher:
  `C:\Users\max-f\AppData\Local\Temp\opencode\run-acc1unlock.cmd`.
  Note: **Graviton 199 to 5 needs ~2.7M+ energy/level** — likely needs far more
  solar satellites (tens–hundreds of k) than were built; may not be worth it.
- **acc2 unlock** — `plans/combat/acc2-unlock.json` (118:10, 121:17 → Galleon/
  Destroyer). Was run in tmux `acc2unlock` on the Azure VM.

Resume a resolver:
```
node httpbot.mjs resolve --goals plans/combat/<file>.json --cp <cp> --steps 100000
```
(acc2 has a single planet: omit `--cp`; run on the VM.)

## 6. Known resolver gotcha

The resolver can **stall** when a queued ship batch is rejected (e.g. while the
shipyard/nanite upgrades, or on energy): its in-process `pendB`/`pendU` never
clears. Symptom: repeated `queues busy; waiting` while the shipyard is idle.
Fix: restart the resolver. Use `EXPLORER_MAX_BUILD_QUEUE=1` and a modest
`EXPLORER_UNIT_BATCH` (500) to reduce it.

## 7. Commands cheat sheet

```
# list planets, levels for one planet
node httpbot.mjs planets
node httpbot.mjs levels --out data/x.json --cp 1648

# batch battle-sim (reads [{id,attacker,defender,acs?,slots?}])
node httpbot.mjs simsuite plans/combat/<file>.json

# real strike / recall (from the cp planet's ships)
node httpbot.mjs fleet 2:188:16 1 "204:100,206:20" 10 --cp 1648
node httpbot.mjs fleetback <fleetID>

# recycle needs debris target (auto for mission 8)
node httpbot.mjs fleet 2:188:16 8 "219:1" 10 --cp 1648

# read a real combat report (hash is inside the combat message)
node httpbot.mjs dump "page=messages&mode=view&messcat=3&site=1&ajax=1"
node httpbot.mjs get "CombatReport.php?raport=<hash>"

# fold captures into the committed dataset
node combat-dataset.mjs
```

## 8. Open items / next steps

1. Defense repair % across repeated real attacks (after one strike ML 500→420,
   LL 300→259, HL 200→190 — repair rule unconfirmed).
2. Exact bounce-threshold operator; rapid-fire extra-shot rule + shot cap
   (reference is deterministic here; model OSS probabilistic on a seedable RNG).
3. ACS / multi-slot POST format (`/tmp` note: `action=moreslots`, index 2 was not
   accepted — needs a browser capture).
4. Flight-time & fuel formula recalibration; current `game_math.go` formulas are
   off ~3×. Samples (distance 1035): 1 Battle Recycler → 36 fuel; 100 LF → 238;
   10 BS → 297; 50 LF+10 Cru → 398. Flight one-way ≈ 5 min.
5. Build the Go engine: extend `UnitDef` with
   `Attack/Shield/Hull/Speed/Fuel/Cargo/RapidFire`; add
   `internal/game/combat.go` (pure resolver) + `combat_test.go` replaying
   `testdata/niburus_combat.json`; wire `MissionAttack` into `resolveFleetEvent`.
   Guardrails: integer maths, every division zero-guarded, bounded RF/round loops,
   seedable RNG, do not inherit the PHP quirks listed in COMBAT_FINDINGS.md §6.

## 9. PHP reference quirks (do NOT inherit)

1. Simulator crashes (no report) when the attacker wins and the defender's
   resources aren't all-three non-zero (seed `{1:1,2:1,3:1}` to dodge).
2. A defense unit on the **attacker** side yields a malformed report.
3. Spy-probe (`210`) defenders give inconsistent numbers.
4. Weapon-type/academy damage is invisible in the card header.
5. `CombatReport.php?raport=<id>` is not the real report; the real hash is linked
   in the combat message.

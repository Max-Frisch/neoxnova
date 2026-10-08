# Arsenal (upgrades) — live findings, 2026-10-06

The user's "dusty memory" was right: the upgrade system is documented **on the game
website**, behind the left-menu link **Arsenal** (`game.php?page=arsenal`) → the `?`
icon → **`game.php?page=manualinfo&id=10`**. The manual is in Russian; raw text is
saved locally at `tools/explorer/data/arsenal-manual.txt` (gitignored). Entry on the
Arsenal page also exposes **Market** (`game.php?page=market`) and **Sell Upgrades**
(`Dialog.CreateLotUpgrade()`).

## Upgrade catalog (Arsenal page + manual)

| group | items | per-activation effect | light/medium/heavy pierce |
|---|---|---|---|
| Weapons (Fleet+Defense) | Laser, Ion, Gravitational, Plasma | dmg +2.6% / 2.4% / 2.5% / 2.3% | Laser 125/100/50 · Ion 115/110/100 · Grav 50/80/125 · Plasma 80/100/115 |
| Armor (class-locked) | Light, Medium, Heavy | hull +2.5% / 2.0% / 1.5% | Light avg 94% · Medium 93% · Heavy 88% (of /500%) |
| Shields (class-locked) | Light, Medium, Heavy | shield +4% / 3% / 2% | Medium doubles, Heavy triples the Academy/Officer/Tech bonus |
| Engines | Jet (light), Impulse (light+medium), Hyperspace (heavy) | speed +5% / 4% / 3% | — |
| Conveyors | Light / Average / Heavy (buildings 71/72/73) | — | — |
| Production | Metal / Crystal / Deuterium | — | — |

Armor/shield class lock = fleet **and** defense of that class (light/medium/heavy).

## Activation rules (manual "Важно")
- **First 10 levels: 100 % success.**
- Each *successful* activation **above level 10** lowers the next success chance by **2 %**.
- Floor: **75 %** minimum success chance.
- A **failed** activation drops the value by 10 % of the last successful activation value.
- Max sale price: **5,000,000 Dark Matter** or **50,000 Antimatter** per unit.
- A listed upgrade stays on sale **72 h**, then returns to the owner.

## Where upgrades are found (the "unlock")
1. **Expedition "Бесконечные дали" (Infinite distances)** = the normal panel (`cmd=1`):
   can find **any** upgrade. Gate: **minimum 75,000 fleet points** on the expedition
   (`1 point = 1,000,000 resources`, deuterium excluded ⇒ 75,000 pts ≈ 75 B metal+crystal).
   *The manual states this, but live finds happen far below it — see the live
   correction at the bottom of this file.*
   Pirates encounter chance 13.3 %, and 10 % chance to find an upgrade after winning.
2. **Hostail sector** (`cmd=2`, `pve=`) — race-specific loot:

   | pve | chance | upgrades |
   |---|---:|---|
   | 1 Barbarians | 8 % | Laser weapon, Ion cannon, Jet engine, Light armor, Light shields |
   | 2 Pirates | 11 % | Ion, Plasma, Impulse engine, Medium armor, Medium shields |
   | 3 Aliens | 14 % | Plasma, Gravitational, Hyperspace engine, Heavy armor, Heavy shields |

   **+1 %** find chance per **10 combat levels**.
3. **Buying**: you buy the cheapest upgrade matching the search criteria from a random seller.

## Mapping to the user's memory
- "stages (light/medium/heavy)" ⇒ the **Barbarian/Pirate/Alien** Hostail tiers (which
  drop light/medium/heavy gear classes) — *not* separate point thresholds.
- "~5k / 50k / 250k fleet points" ⇒ the only hard gate is a single **75,000 fleet
  points** for the regular expedition (Hostail has no point gate, only race chances).
- Consequences: our current test fleets (≈5–500 pts) are **far** below 75,000, so they
  cannot find upgrades — matrices remain valid only for the enemy-formula question.
- Hostail (`cmd=2`) is exactly the ghost-fleet-bugging path (`docs/EXPEDITIONS_LIVE_2026-10-06.md` §4),
  and it is the only route to tier-targeted upgrades; a future test must weigh that bug.

## Live correction (2026-10-07) — the 75,000-pt gate does **not** gate these finds

The rolling farm has produced **4 upgrade drawings while flying fleets far under the
"75,000 fleet points" the manual states** (fleet points ≈ `0.1244 · S`, where
`pts = 0.058·S + 0.012·5S + 1.6·S/250`):

| acc | fleet S | ≈ fleet points | upgrade found | source encounter |
|---|---:|---:|---|---|
| acc1 | 87 331 | ≈ 10 900 | **Jet engine** (light) | "ancient battlefield" |
| acc2 | 61 764 | ≈ 7 700 | **Light armor** (light) | pirate trap (combat) |
| acc2 | 61 764 | ≈ 7 700 | **Laser weapons** | Moa Tikarr duel (combat) |
| acc2 | 61 764 | ≈ 7 700 | **Jet engine** (light) | pirate trap (combat) |

- **Drop rate ≈ 10 % of combat wins** (4 upgrades / 36 wins, both accounts) — matches
  the manual's "10 % after winning" figure.
- Drops come from the **combat encounters** (pirate trap / Moa Tikarr / ancient
  battlefield), not from a 75 000-pt exploration roll. At ~7.7k–10.9k pts we are at
  ~1/7 of the alleged gate, so **75 000 must not be modelled as a hard requirement**;
  the user's lower-tier (≈5k) intuition fits better. Still unknown: whether the tier
  of the drop scales with fleet points.
- All four finds are **entry-tier** (Jet engine ×2, Light armor, Laser weapons) — no
  medium/heavy yet.

Implication for backlog 1c: the *find* path is already exercised; the open work is the
activation/catalog rules and the tier-vs-points relationship, not reaching 75k.

## Upgrade-find harvest (2026-10-08) — events, source outcomes, fleet sizes

Find events live in the **expedition message category (`messcat=15`)** — they are
appended to a normal outcome body: *"… the engineers were able to create a drawing
for an upgrade **<NAME> (2 pc)**."* **Every find awards 2 pc.** (The harvest daemon
only polls `messcat=15`+`3`; the upgrade text was not previously extracted.)

| acc | msg | server time (UTC+3) | upgrade | source outcome | fleet that resolved |
|---|---|---|---|---|---|
| acc1 | 240268 | 01:14:15 | Heavy armor | Moa Tikarr combat (report 240267) | 207:814397 + 203:4071985 + 219:3258 |
| acc1 | 240634 | 01:56:54 | Gravitational gun | Moa Tikarr combat (report 240633) | same giant BB fleet |
| acc1 | 240636 | 01:56:54 | Ion cannon | Moa Tikarr combat (report 240633) | same giant BB fleet |
| acc1 | 240880 | 02:32:46 | **Hyperspace engine** | "ancient battlefield" (ship find) | BB-era fleet (~814k BB) |
| acc1 | 242699 | 08:24:05 | Plasma gun | "ancient battlefield" | (post-flip) |
| acc2 | 241074 | 02:59:36 | Ion cannon | "ancient battlefield" | Frigate |
| acc2 | 241076 | 02:59:36 | Light armor | Moa Tikarr combat | Frigate |
| acc2 | 241449 | 04:04:10 | Laser weapons | Moa Tikarr combat | Frigate |
| acc2 | 241669 | 06:22:03 | **Hyperspace engine** | "ancient battlefield" | 227:~3,900 + 219:~3,900 |

Key observations:
- **Drops are not only from combat wins** — two of the five acc1 finds and both
  Hyperspace finds came from the **"ancient battlefield" ship-recovery** outcome.
- **Fleet points, two formulas** (the model's unit is ambiguous):
  - manual "1 pt = 1M res" ⇒ model `(M+C)/1e6`: acc1 BB fleet ≈ **101k**;
    acc2 Frigate fleet ≈ **162k**.
  - live PHP `data/_MissionCaseExpedition.php:42`
    (`expeditionPoints = (M+C)*5/1000`, i.e. 1 pt ≈ 200 res): acc1 ≈ **507M**;
    acc2 ≈ **811M**.
- **Consequence for the tiers:** every observed **heavy** drop came from a fleet
  that is far **under 250k** under the model's `/1e6` points but **far over** it
  under the PHP formula. So either the Arsenal tier uses the PHP-style points (then
  250k is trivially met), or the real heavy threshold is **≈100k, not 250k/150k**
  (the manual's 75k gate stays suspicious but closer). **Do not trust the
  `ArsenalTier` 5k/50k/250k constants until the live drop code is captured.**
- Raw captures for re-parse: `data/upg/c15_*.html` (acc1), `data/upg2/*.html`
  (acc2); the acc1/acc2 find messages were also matched 1:1 to combat reports in
  `expedition-reports.json`.

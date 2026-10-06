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

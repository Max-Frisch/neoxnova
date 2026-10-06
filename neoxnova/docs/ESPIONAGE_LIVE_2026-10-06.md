# Espionage — live findings (acc1 × acc2, 2026-10-06)

Live capture of the niburuspace.com espionage mission, to drive the Go
implementation. Accounts: **acc1 Bratwurst** (`3:125:12` home, strike hub
`2:188:9 Xusyty`) and **acc2 TheBob** (`2:188:16` home, colonies incl.
`2:188:10 Mybeti`). Tooling: `tools/explorer/httpbot.mjs`.

## 1. Fleet-send wizard semantics (corrected)

The wrapper's step names differ from the game. The real flow is:

- **Step 1** — select ships **and the target**; **speed** lives here and defaults
  to **100%**. Speed values are `1..10` (10 = 100%). *A probe on its own cannot
  proceed if no target is set.*
- **Step 2** — choose the **mission/mode** (Transport, Station/Deploy, Attack,
  Spying, Recycle, …) + resources. **The mission must be selected here** (posting
  `mission=0` fails and bounces back to `fleetTable`).
- **Step 3** — confirm; returns the "Fleet sent" page (Mission / Distance / Fleet
  speed / Consumption of deuterium / From / Destination / Target time / Time of
  Return / Fleet list).

`httpbot.mjs cmdFleet` was fixed to send `mission` in step 2 (it previously sent
`mission=0`) — the mission-specific wrappers now work.

Sample send (acc1 probe → acc2 colony, cross-galaxy):
`Mission Spying | Distance 29.695 | Fleet speed 584.350.000 | Consumption 4 |
From 3:125:12 | Destination 2:188:10 | Target time 07:04:31 | Return 07:04:49`.

## 2. Attacker report (the "Spy Report")

Delivered to the **attacker** as a message in category **Spy messages**
(`page=messages`, AJAX `page=messages&mode=view&messcat=0&site=1&ajax=1`).

Report header: `Intelligence report — Spy Report {targetName} [{coords}] on {date}`.
Sections (all present in the capture): **Resources, Fleet, Planetary Defense,
Buildings, Research**, each a row of counts (ships/defenses grouped by type).
Footer offers `Attack` / `Simulate` (prefills the battle simulator with the exact
seen counts) and the delete controls.

Counter-espionage: when the defender shoots probes down, the report footer shows
**"Your spy probes were destroyed!"** (the report itself is still delivered —
confirmed live).

### Section gating — RESOLVED: niburu does **not** mask sections

Matrix run 2026-10-06, **one probe** per strike, sections observed in the report:

| attacker → defender | esp diff | classic score | sections returned |
|---|---|---|---|
| acc1 (20) → acc2 home/colony (19) | +1 | 2 | Resources·Fleet·Defense·Buildings·Research |
| acc2 (19) → acc1 home/colony (20) | −1 | 0 | Resources·Fleet·Defense·Buildings·Research |
| acc1 (19) → acc2 (19) | 0 | 1 | Resources·Fleet·Defense·Buildings·Research |

Classic OGame would have shown Fleet-only at +1 and Resources-only at −1. Niburu
returns the **full report regardless** — the espionage tech difference does **not**
mask sections here. **Conclusion for the Go target: always return the full report;
use the tech difference (and probes/fleet) only for counter-espionage.**
Confirmed again at acc1 Esp21 vs acc2 Esp19 (diff +2) — still full.

## 3. Defender-side visibility (incoming fleets)

Incoming hostile missions show in the **defender's `page=overview` "Fleet" event
list** (same place as attacks), with a countdown and a mission-specific tooltip /
CSS class. Observed text templates (paraphrase-accurate):

| mission | class / colour | text (defender overview) |
|---|---|---|
| Spying | espionage (orange/yellow) | `A hostile Fleets from player {player} [PM] from Planet {originName} [{o}] reached the Planet {targetName} [{t}]. Mission: Spying` |
| Transport | `flight transport` (green) | `A hostile … from Planet {originName} [{o}] reached the Planet {targetName} [{t}]` |
| Attack | `tooltip attack` (deep red) | `A hostile … Fleets from player {player} [PM] from Planet {originName} [{o}] reached the Planet {targetName} [{t}]` |

- Overview status indicators exist: `#attack` (`Your empire is not being
  attacked`) and `#espionage` (`No one is spying you`). The espionage indicator is
  only on **while a spy is en route** (probes are ~instant, so it is brief).
- The event string says "reached" even while the fleet is still inbound; the real
  state is the countdown.
- Owner: colours should be differentiated on our front end too (green
  transport/station, orange espionage, deep red attack).

Live checks performed: espionage (arrived, captured), transport (slow LC, captured
inbound), attack (1 LF, captured inbound, then **recalled** via
`page=fleetTable&action=sendfleetback&fleetID=…`).

## 4. Counter-espionage

Confirmed live: probes **were destroyed** on **every** strike, including
satellite-only colonies (e.g. `Fleet 872` / `1.050` and **zero** Planetary
Defense) and the big homeworlds — i.e. the detection appears driven by **ships**
(probes/sats/combat), not defense towers. Both directions and both tech signs
(+1/−1) lost the probe. No planet with **zero** ships was available to confirm the
"no ships ⇒ 0% chance" rule.
Published approximation (o-tools): `chance ≈ 2^(defenderEsp − attackerEsp) ·
probes · defenderShips · 0.25%`, 0 when the defender has no ships. Owner prefers
probes as a replaceable commodity; more probes should raise success chance. Exact
formula is undocumented/randomised.

## 5. Reports endpoint (for the Go API)

`GET /game/game.php?page=messages&mode=view&messcat=0&site=1&ajax=1` returns the
spy-message rows (HTML fragment). Message categories are keyed by `messcat`
(0 = Spy, 1 = Player, 3 = Combat, 6 = Transport, …).

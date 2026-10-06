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

### Open question — section gating
Classic OGame reveals sections by
`score = probes + (yourEsp − enemyEsp)·|yourEsp − enemyEsp|` (Resources always,
Fleet ≥2, Defense ≥3, Buildings ≥5, Research ≥7). **Live niburu did NOT do this:**
with **both accounts at Espionage 19** and a **single probe** (classic score = 1 →
resources only), the report revealed **every section with exact counts**. So
niburu appears to always return the full report and to use the tech difference
only for **counter-espionage**, not for section masking. *Needs an owner ruling
before we wire the resolver* — see `docs/HANDOFF.md` item 3.

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

Confirmed live: probes **were destroyed** ("Your spy probes were destroyed!").
Published approximation (o-tools): `chance ≈ 2^(defenderEsp − attackerEsp) ·
probes · defenderShips · 0.25%`, 0 when the defender has no ships. Owner prefers
probes as a replaceable commodity; more probes should raise success chance. Exact
formula is undocumented/randomised.

## 5. Reports endpoint (for the Go API)

`GET /game/game.php?page=messages&mode=view&messcat=0&site=1&ajax=1` returns the
spy-message rows (HTML fragment). Message categories are keyed by `messcat`
(0 = Spy, 1 = Player, 3 = Combat, 6 = Transport, …).

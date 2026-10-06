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
  cmd=2, pve  = 1 Barbarians | 2 Pirates | 3 Aliens
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

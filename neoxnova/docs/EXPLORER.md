# Explorer bots & live-game runbook

Live automation + the fleet/combat wizard for niburuspace.com. Referenced from
`AGENTS.md`. Run everything from `neoxnova/tools/explorer/`.

## Bots
- Prefer the browser-less **`httpbot.mjs`** (Node `fetch` + cookie jar, ~40 MB);
  `explorer.mjs` is the Playwright version (Edge/Chromium, ~300 MB) — use only
  for debugging/UI inspection.
- Secrets (gitignored): `neoxnova/secrets/explorer.env` with `NIBURU_USER/PASS`
  (acc1) and `NIBURU_SECOND_USER/PASS` (acc2); SSH key + config in `neoxnova/secrets/ssh/`.
- acc1 runs on an Azure VM (Ubuntu, 2 vCPU/1 GB; tmux) — never run a browser there,
  only `httpbot.mjs`. SSH alias `azure-bot` (`-F neoxnova/secrets/ssh/config`).
  acc2 TheBob is retired from scope (manual-only fallback).
- Commands: `node --max-old-space-size=96 httpbot.mjs levels --out data/levels.json`;
  `... resolve --goals plans/account2-goals.json --steps 5000`; `... dump "page=research"`.
  `explorer.mjs` adds `scan|status|build|cancel|sats|map|officers`.
- Action POSTs: buildings `{cmd:insert,building,lvlup}`, research `{cmd:insert,tech,lvlup}`,
  shipyard `{fmenge[<code>]:N}`. Research boxes `#research_<id>`, buildings `#build_<id>`.

## Resolver (`resolve`) semantics
- Requirements come from `page=techtree` -> graph; `resolve` recursively builds/researches
  prerequisites and skips locked targets.
- **Fast server**: once Nanite is balanced against Robot, every building finishes in ~1-3 min;
  if a build shows hours, Nanite/Robot are too low (rule of thumb Robot ~15 <=> Nanite ~5).
- **Per-queue scheduler**: buildings and research use separate queues; submits up to one
  building *and* one research per loop, waits only when both busy. In-flight caps default
  2 buildings / 1 research (`EXPLORER_MAX_BUILD_QUEUE`, `EXPLORER_MAX_RESEARCH_QUEUE`).
- **Energy comes from Solar Satellites, not Solar Plant levels.** On `Lack of energy`, queue a
  batch of `EXPLORER_ENERGY_SATS` (default 200) code `212` via `page=shipyard&mode=fleet`.
- **Only auto-research instant techs** (cards with no `Duration`); slow cards are skipped
  (`EXPLORER_MAX_RESEARCH_SEC`, default 1).
- **University (6)** needs Robot 20, Research Lab 22, Nanite 4, Computer 12, IRN (123) 3.
  Its −16 %/level research-time bonus is **local to the planet where research is started**;
  it does **not** stack account-wide (IRN links Research Labs only). One per research
  planet is enough — extras are wasted.
- Plan JSON: `buildings` fixed targets, `gradual` codes bumped +1 until the plan is satisfied,
  `caps` per-code ceiling for gradual (the "done" state; also checked by `verify`),
  `bumpBuilders` (Robot/Nanite: Nanite trails Robot by 11, floor 1), `order` priority,
  `ships`/`defenses` unit targets. `verify` exits 0 when all targets+caps are met (daemons stop).
- `httpbot.mjs cancel` clears the whole building queue; `trim page=research` removes queued rows.
- Extra: `academy` (Weaponry 1101->5, then Engine limitation 1105), `redeem <code>`,
  `academy-up`, `academy-map`. Never spend academy points on 1105 automatically — use `academy-up`/`academy-map`.

## Fleet send (WORKS — `fleet_movement.har`; wizard corrected 2026-10-06)
Real flow: **step 1** = select ships **and target** (speed lives here, 1..10, 100% default);
**step 2** = choose the **mission** (not `mission=0` — that is rejected); **step 3** = confirm.
`httpbot.mjs fleet <g:s:p> <mission> <code:count,...> [speed]` (speed 1..10). Recall:
`httpbot.mjs fleetback [fleetID]` (`page=fleetTable&action=sendfleetback&fleetID=<n>`).
- Missions: 1 attack, 3 transport, 4 deploy, 5 hold, **6 espionage**.
- Success returns a `Fleet sent` page (Mission/Distance/Fleet speed/Consumption); detect by that
  text (success *also* navigates to fleetTable).
- The cross-galaxy distance (acc1 3:125:12 -> acc2 2:188:16) is not a fuel blocker.

## Expeditions (custom server "auto" panel)
Expeditions are **not** in the fleet wizard; use the `#expfleet` panel on
`page=fleetTable`. Wrapper: `httpbot.mjs expedition <code:count,...> [num] [time] [speed] [--pve N]`
(`ship2<code>` fields; `time` 1..10 = 0.25..2.5 h; `pve` 1 Barbarians / 2 Pirates / 3 Aliens).
`exp-state` lists outgoing fleets; `exp-log` appends messcat=15 outcomes to
`data/expeditions.json` and prints the distribution. **Do not use `--pve`** — it
leaks ghost fleets (counted in fleetTable, never resolve). Findings + black-hole
status: `docs/EXPEDITIONS_LIVE_2026-10-06.md`.

## Messages (`msg-scan` / `msg-stats`)
The inbox has 12 categories (sidebar `Message.getMessages(id)`): 0 spy, 1 player,
2 alliance, 3 combat, 4 system, 5 transport, 15 expedition, 50 game,
99 construction, 100 all, 199 archive, 999 outbox. `exp-log`/`exp-report` only
cover 15/3; `msg-scan` covers **every** category in one pass.
- `node --max-old-space-size=192 httpbot.mjs msg-scan [--cats 0,3,15] [--max-sites N]`
  pages each category by `site`, parses + classifies every row, and merges into
  `data/messages.json` (additive: deleted/archived rows stay local). Defaults to a
  **deep backfill**; the server repeats the final page instead of returning an
  empty one, so paging stops when two consecutive pages carry the same ids.
  A `--max-sites 6` scan is enough for forward capture (`run-msgharvest.sh`).
- `node httpbot.mjs msg-stats` reprints the statistics from `data/messages.json`
  without fetching.
- Rows are classified by an ordered taxonomy: expedition bodies against the live
  `data/_lang_FLEETphp` `sys_expe_*` strings + the custom flavours this server
  added (bacterium, virus, stardust, non-fatal "blackhole-loot", "ancient
  battlefield" Arsenal drops); fight reports also yield `profit`/`rubblefield`/
  `combatXp`; system rows yield the achievement name/level/reward; spy rows yield
  the sighting owner/coords. Live mix + flavour table:
  `docs/EXPEDITIONS_LIVE_2026-10-06.md` §10.
- Continuous: `bash run-msgharvest.sh start [tag]` (tmux `msgharv-<tag>`, every
  `EXPLORER_MSG_EVERY_S`=900 s, `EXPLORER_MSG_MAX_SITES`=6).

## Battle simulator (WORKS)
POST `page=battleSimulator&mode=send` with `slots=2` and `battleinput[0][0][code]` (attacker) /
`battleinput[0][1][code]` (defender); `1xx` = techs/skills, `2xx`/`4xx` = ships/defenses. Response is
a report hash -> `${BASE}/game/CombatReport.php?raport=<hash>`. Wrapped by `httpbot.mjs sim <file.json>`.

## Fleet/defense building via resolve
`resolve` with a `ships`/`defense` goal map (counts from `id="val_<code>"`, POSTed as `fmenge[<code>]=N`
to `page=shipyard&mode=fleet` / `&mode=defense`, batch-capped). Plans: `plans/acc1-fleet.json`,
`plans/acc2-fleet.json`.

## Live accounts
- **acc1 Bratwurst** `3:125:12` (harvest hub `2:188:9` Xusyty) — Azure VM `azure-bot`, tmux
  (`drain-acc1` + `bonus-acc1`).
- **acc2 TheBob** `2:188:16` — RETIRED from scope (manual-only fallback).
- Fleet-speed recovery: engine techs 115/117/118 researched only while instant; Academy branch I
  Weaponry 5 -> Engine limitation.

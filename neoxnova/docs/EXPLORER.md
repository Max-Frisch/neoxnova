# Explorer bots & live-game runbook

Live automation + the fleet/combat wizard for niburuspace.com. Referenced from
`AGENTS.md`. Run everything from `neoxnova/tools/explorer/`.

## Bots
- Prefer the browser-less **`httpbot.mjs`** (Node `fetch` + cookie jar, ~40 MB);
  `explorer.mjs` is the Playwright version (Edge/Chromium, ~300 MB) — use only
  for debugging/UI inspection.
- Secrets (gitignored): `neoxnova/secrets/explorer.env` with `NIBURU_USER/PASS`
  (acc1) and `NIBURU_SECOND_USER/PASS` (acc2); SSH key + config in `neoxnova/secrets/ssh/`.
- acc2 runs on an Azure VM (Ubuntu, 2 vCPU/1 GB; tmux) — never run a browser there,
  only `httpbot.mjs`. SSH alias `azure-bot` (`-F neoxnova/secrets/ssh/config`).
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

## Battle simulator (WORKS)
POST `page=battleSimulator&mode=send` with `slots=2` and `battleinput[0][0][code]` (attacker) /
`battleinput[0][1][code]` (defender); `1xx` = techs/skills, `2xx`/`4xx` = ships/defenses. Response is
a report hash -> `${BASE}/game/CombatReport.php?raport=<hash>`. Wrapped by `httpbot.mjs sim <file.json>`.

## Fleet/defense building via resolve
`resolve` with a `ships`/`defense` goal map (counts from `id="val_<code>"`, POSTed as `fmenge[<code>]=N`
to `page=shipyard&mode=fleet` / `&mode=defense`, batch-capped). Plans: `plans/acc1-fleet.json`,
`plans/acc2-fleet.json`.

## Live accounts
- **acc1 Bratwurst** `3:125:12` (harvest hub `2:188:9` Xusyty) — local.
- **acc2 TheBob** `2:188:16` — Azure VM `azure-bot`.
- Fleet-speed recovery: engine techs 115/117/118 researched only while instant; Academy branch I
  Weaponry 5 -> Engine limitation.

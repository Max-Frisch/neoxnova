# Playability fixes — staged plan (P / Q / C / B)

Fixes from the first hands-on human test: **P**erformance, building/research/ship **Q**ueues with
target-level input, game tuning **C**onfig, and the **B**lack-market hub. Each numbered step is
**one agent session**; do them in order where noted. Run the verification block at the end of each.

> Companion docs: `WEB_UI_RESKIN_PLAN.md` (skin rules, CSP). Reference to port: sibling `gwx` repo —
> `internal/store/queue_store.go` (chained queues, slot caps, cancel renumbering),
> `migrations/0009/0012/0013`, `internal/game/builder.go`. `gwx` uses one generic `queues` table;
> this engine keeps its typed tables — port the *behaviour*, not the schema.

## Decisions (locked)
- **Queues:** buildings default **10**, research **4**, ships+defenses **unlimited** (back-to-back
  chains, gwx-style). **Target level with insufficient resources ⇒ queue the affordable partial**,
  stopping at the first unaffordable level / full slot.
- **Config:** a single JSON file drives game speed, resource speed, fleet speed and the queue
  limits, applied to the `universes` row on startup (gwx pattern: "applied/upserted on every start").
- **Dynamic cost/time:** no extra request. Generate a per-level cost/time table **server-side from
  `internal/game`** and embed it on the page as JSON; `static/app.js` sums it live as the target
  changes. (Instant, and the formula stays single-sourced in Go.)
- **Black market:** `/blackmarket` is its own hub. `/market` is the **Arsenal** upgrade market and
  lives only under the Arsenal scope — the two are **not connected**.

## Ground rules
- Stack stays Templ + htmx + Tailwind. After editing `*.templ`: `make web-gen`; commit generated
  `*_templ.go`. CSP stays `style-src 'self'` (dynamic values via committed `static/app.js` +
  `data-*`, no inline style).
- Migrations are numbered, idempotent, appended to the `Makefile` `migrate` target (dev-up scripts
  auto-glob). Conventional Commits; commit only after a step is verified. Do not edit `AGENTS.md`.

## Verification (per step)
```
cd neoxnova
"$(go env GOPATH)/bin/templ" generate      # only if *.templ changed (or: make web-gen)
gofmt -w internal
go build ./... && go vet ./... && go test ./...
# DB integration tests skip unless DATABASE_URL is set; with it they run against compose.
# manual: make up && make migrate && make seed && make run   (:8080)
```

---

# Phase P — Performance (buildings/research pages load like a 56k modem)

**Root cause #1 (the big one):** the global per-IP rate limiter
(`globalLimit := newIPLimiter(40, 80)`, `internal/api/router.go:21`) wraps the **whole mux**,
including `/static/`. A building page requests ~50–300 sprite/icon assets; at 40 req/s (burst 80)
the browser is throttled for **seconds** before the page paints. This is exactly why the
sprite-heavy buildings/research pages are worst.
**Root cause #2:** `/static/` is a bare `http.FileServer` over `embed.FS` with **no Cache-Control,
no ETag, no gzip** (`internal/web/web.go:76`) — every navigation refetches all 308 files / 3.8 MB.
**Root cause #3:** resource bar polls **every 2 s** (`layout.templ:170`), each poll ~8 queries incl.
a `SELECT … FOR UPDATE`; the session is resolved twice per request (`web.go:144-154`).

## P1 — Serve `/static/` outside rate limiting + caching + gzip + timeouts
> **Status: ✅ done (2026-10-15).** `/static/` now mounts on a root mux *before* the
> per-IP limiter (`internal/api/router.go`); `internal/web/static.go` precomputes an asset
> index with content-hash `ETag`, `Cache-Control`, `Last-Modified`, gzip for text/svg and
> `304` revalidation; `cmd/server/main.go` gained Read/Write/Idle timeouts. Tests:
> `TestStaticAssetsCacheable`, `TestStaticAssetsBypassGlobalRateLimit`.
Work:
1. **Exempt static assets from the IP limiter.** Mount the `/static/` handler on a separate
   `http.ServeMux` that is **not** wrapped by `globalLimit` (serve it before/around the limited
   mux in `router.go:74-77`), or skip the limiter for `strings.HasPrefix(path, "/static/")`.
   Keep page/API routes limited. Re-tune the limiter burst if needed for a normal page load.
2. Replace the bare `http.FileServer` with a wrapper that:
   - sets a strong `ETag` (content hash) + `Cache-Control: public, max-age=3600` (names are not
     fingerprinted — keep revalidatable, **not** `immutable`) and a fixed `Last-Modified`;
   - answers `If-None-Match` with `304`;
   - gzips text types (`css`, `js`, `svg`); leaves `gif/jpg/png` alone;
   - prefer `http.ServeContent` for correct range/ETag handling.
3. Add `ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout` (`cmd/server/main.go:57`).
4. Optional cheap wins for the card grids: `loading="lazy"` on below-the-fold sprites; replace the
   heavy background JPEGs (`bg/mond.jpg` 90 KB, `icons/sfondonuovo.jpg` 240 KB) with a CSS
   space-blue gradient (user OK'd this).
**Tests:** extend `internal/web/static_test.go` — `ETag`+`Cache-Control` present, gzip for
`.css`/`.js`, no gzip for `.gif`, `304` on `If-None-Match`; an api test that `/static/*` is **not**
rate-limited (N rapid asset requests all succeed).
**Exit:** a building page's assets load in one shot (no throttling); a second load is `304`s.

## P2 — Cut per-request/poll cost
Work:
1. Resolve the session **once** per request into `context` (middleware) so `requireUser`
   (`web.go:157`) + the handler don't both call `SessionUser`.
2. Resbar `every 2s` → `every 5s` (`layout.templ:170`); same for `expedition.templ:68`.
3. Trim `shell()` (`data.go:47-67`): reuse loaded data, fold `ownsCelestial` into the planet
   query, stop the double `update_celestial_resources` on Overview.
4. Optional: short server-side resource-snapshot cache per celestial.
**Tests:** existing web tests green; query-count/benchmark assertion if practical.
**Exit:** nav/refresh issues materially fewer queries; bar no longer locks every 2 s.

---

# Phase Q / C — Queues (build 10 / research 4 / units unlimited) + target-level + config

Reference behaviour (`gwx`): orders chain back-to-back (`start = MAX(finish)`), each order targets
`current + (#already queued for that code) + 1`, cap enforced in Go under the planet/user row lock,
slots on `universes`, all rows `IN_PROGRESS` (no engine promotion needed). Cancel renumbers the tail.

## Q1 — Schema + slot columns
Work:
1. New `migrations/0020_queue_slots.sql`:
   - `ALTER TABLE universes ADD COLUMN IF NOT EXISTS build_queue_slots SMALLINT NOT NULL DEFAULT 10;`
   - `ALTER TABLE universes ADD COLUMN IF NOT EXISTS research_queue_slots SMALLINT NOT NULL DEFAULT 4;`
   - `DROP INDEX IF EXISTS uq_active_construction;`
   - `DROP INDEX IF EXISTS uq_active_research;`
   - `DROP INDEX IF EXISTS uq_active_shipyard;`  -- units become an unlimited chain (gwx)
2. Append to the `Makefile` `migrate` target (`Makefile:65`).
3. Update `migrations/0002_seed.sql` / `0016_test_universe.sql` if they set universes columns.
**Tests:** migration idempotency; a store probe that multiple `IN_PROGRESS` rows coexist per scope.
**Exit:** schema permits multi-row queues in all three scopes.

## C1 — Game tuning config file (speeds + queue limits)
Work:
1. Add `neoxnova/config/game.json` (simple, documented):
   ```json
   { "game_speed": 1.0, "resource_speed": 1.0, "fleet_speed": 15.0,
     "build_queue_slots": 10, "research_queue_slots": 4 }
   ```
   (unit queue is unlimited — no slot) with a short `config/README.md` on precedence.
2. Loader in `cmd/server` startup (or `internal/config`): parse the file (defaults if absent),
   then **upsert** the values onto the `universes` row for `UNIVERSE_ID` (mirrors the seed / gwx
   "applied on every start"). Precedence: file wins on boot; DB is the runtime source.
3. Note: `game_speed`/`resource_speed`/`fleet_speed` are already read from `universes`
   (`fleet_store.go:49,197`); this only populates them — no game-math change.
**Tests:** loader unit test (defaults, parse, unknown field); integration upsert test.
**Exit:** editing `game.json` + restart changes speeds/slots; values visible in the `universes` row.

## Q2 — Store: chained multi-order enqueue + read-model lists
Work (port `gwx` `StartBuilding`/`StartResearch`/`StartUnits` into `internal/store/build_store.go`):
1. `EnqueueStructure(ctx, planetID, code, targetLevel int)` and
   `EnqueueResearch(ctx, planetID, code, targetLevel int)`: empty/0 ⇒ one level (`current+1`);
   otherwise loop `current+1 … targetLevel`, enqueuing until the scope's slots fill or the next
   level is unaffordable (**partial**).
2. `EnqueueShipyard`/defenses: append a batch chained behind the last of the **same kind**
   (ships and defenses are separate lines), unlimited.
3. Per order target = `current + #queued(same code) + 1`; cost = `game.StructureCost`/`TechCost`/
   `ShipCost` at that target; duration via `game.StructureDuration`/`TechDuration`/`ShipDuration`
   using the **projected** robotics/nanite (builds), lab/IRN (research) or shipyard/nanite (units).
4. Chain `start_time = GREATEST(MAX(end_time) for scope, NOW())`, `end_time = start + duration`,
   under the existing row lock (`update_celestial_resources` / `FOR UPDATE`) so enqueues serialize.
5. Read models: `GetBuildings`/`GetResearch` (`build_store.go:618,679`) and the shipyard read model
   return the **full queue list** (`[]QueueEntrySummary`); update `models/types.go:251-274`,
   `data.go:73-75`, and `blueprint_store.go:344-352` busy checks (now "queue full", not "any active").
6. Engine needs **no change** — resolvers already apply any due `IN_PROGRESS` row
   (`event_engine.go:492,547,598`); chained end-times make them run in order. Add an integration
   test proving back-to-back resolution.
**Tests:** store integration — multi-level enqueue, slot cap, partial-on-poverty, cost/time scale,
engine resolves N rows in order; update `build_research_integration_test.go`.
**Exit:** target N queues the affordable, in-cap levels for all three scopes; scheduler builds in order.

## Q3 — Cancel by id + renumber + refund
Work:
1. Replace scope-based `CancelQueue` (`build_store.go:405-489`) with **cancel by queue id**
   (verify owner/scope). Refund that row's frozen cost, mark `CANCELLED`.
2. Renumber the tail (port `gwx` `renumberBuildingsTx`/`renumberResearchTx`/`renumberUnitsTx`):
   pull remaining orders forward and shift later orders of the **same** code down one level
   (refunding the level-cost difference).
3. Update handlers/routes to pass a queue id; the UI already renders an X per row (`queueRow`).
**Tests:** cancel a middle order → assert tail renumber + refunds (rebuild from
`build_cancel_integration_test.go`); e2e cancel subtests updated.
**Exit:** cancelling any queued order leaves a consistent, correctly-priced chain.

## Q4 — Web UI: target-level input + live cost/time + multi-row queues
Work:
1. `pages.templ`: make the level input (`buildCard:244`, `researchCard:471`) an **enabled, named**
   `target_level` field (default = next level); Build/Research submit it. Empty ⇒ 1 level.
2. Embed the per-level cost/time table for the card's code as JSON (generated from `internal/game`)
   and have `static/app.js` update the shown total cost + total duration live as the input changes
   (no extra request — decision above).
3. Render the **whole queued list**: loop over the queue slice in `BuildingsBody`/`ResearchBody`/
   shipyard/defenses (`queueRow` is already numbered, only item 1 rendered today); each row shows
   its own level, frozen cost and ETA, with a per-row cancel X.
4. Submit handler: parse `target_level`, call the new store signatures, flash "Queued to level N"
   or the partial result.
**Tests:** rewrite `i2_test.go`/`i3_test.go` markup assertions (input editable/named, multiple queue
rows, JSON cost table present); update e2e building/research subtests to pass a target level.
**Exit:** typing a target shows the total cost/time live; the button queues that many levels; the
queue lists each order with its own price/ETA.

---

# Phase B — Black market hub (decoupled from Arsenal)

Today `/market` wrongly shows the Arsenal drawing auctions (`account.go:132-199`,
`account.templ:196-201`). Target: `/blackmarket` as the live six-tile hub
(`niburuspace.com/game.php?page=port`); `/market` stays Arsenal-only.

## B1 — `/blackmarket` hub + separate `/market`
Work:
1. New `GET /blackmarket` + `BlackMarketPage` hub with six tiles (**Resource Trader, Merchant of
   Fleets, Planetarium, Buildings Market, Technology Market, Voucher System**), each linking to its
   own sub-route; art from `static/niburu/` (capture more if needed).
2. Remove the Arsenal-auction listing from Black market. Repoint the nav "Black market" → `/blackmarket`.
3. `/market` is the **Arsenal** market and lives only under the Arsenal scope: redirect the
   standalone `GET /market` → `/arsenal?tab=market` (bookmark-friendly) and keep the existing
   Arsenal tab. **Do not** link `/market` to `/blackmarket`.
4. Sub-pages not yet built render disabled "coming soon" (reskin rule) — never fake pages. Built
   this pass: Resource Trader (B2), Planetarium (B3).
**Tests:** hub render test (six tiles + links); `/market` redirects to the Arsenal market tab and no
longer shows lot tables on its own.
**Exit:** Black market is a standalone hub; Arsenal auctions live only under Arsenal.

## B2 — Resource Trader
Work:
1. Capture the exact rules from ops `captures/Resource_Trader_Crys_for_Deut_example.har` + the
   screenshots: each call costs **250 Dark Matter**; pick one resource to **sell**; buy the other(s)
   at the **4/2/1** (metal:crystal:deut) ratio; per-resource **fee** (Deuterium 1, Metal 0.25,
   Crystal 0.5) on the bought side.
2. `internal/game`: pure `TradeQuote(sellCode, buyCode, amount, dm)` (ratio + fee + DM cost) + tests.
3. `internal/store`: atomic exchange — accrue via `update_celestial_resources`, debit
   `users.dark_matter`, move resources with the `LEAST(cap, …)` clamp.
4. Page + `POST /blackmarket/trader` (+ preview) + flash; `return`-aware redirects.
**Tests:** pure quote tests; store integration (DM debited, resources swapped, capped); web render +
submit tests.
**Exit:** a player can trade resources for 250 DM from the UI.

## B3 — Planetarium page
Work:
1. `GET /blackmarket/planetarium` wiring the **already-implemented** JSON endpoints into the
   captured layout: increase fields (`POST /api/v1/planets/{id}/fields`), increase diameter
   (Stardust sink, `…/diameter`), teleport/relocate (`…/relocate`). Reuse `internal/game/fields.go`,
   `relocation.go`; honour the teleport cooldown / 10-min attack-lock / phalanx-offline rules in
   `planet_store.go`.
2. Verify the cost curves against `game` before wiring labels (capture shows fields-add "0 DM").
**Tests:** render + submit tests (store logic already covered).
**Exit:** Planetarium performs fields/diameter/teleport from the UI.

## B4 (future, not this pass)
Merchant of Fleets (ship scrapping), Buildings Market (build with DM), Technology Market (research
with DM), Voucher System. Disabled until captured + designed.

---

## Remaining open question
- **Config format:** JSON (`config/game.json`) chosen for machine-parsing simplicity; swap to
  TOML/`.md` if you prefer human-first editing.

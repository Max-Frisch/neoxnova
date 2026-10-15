# Web UI reskin — live-parity plan (I0–I5)

Goal: make the server-rendered UI **look and feel like the live Niburuspace.com client**
(classic OGame "Redesign" skin) while mapping every button, icon and link to functionality
that already exists in this engine. Unbacked menus are shown but disabled.

Source of truth for the target look: `neoxnova/info/*.png` (+ `resources_view.png`).

> One phase = one agent session. Run the verification block at the end of each phase.
> Do not start a phase until the previous one's exit criteria are met.

## Ground rules
- Stack stays: Templ + htmx + Tailwind. Regenerate templ with `make web-gen` after editing
  `*.templ` and commit the generated `*_templ.go`.
- Skin styling goes in a hand-authored `internal/web/static/skin.css` (favour CSS over new
  Tailwind utilities so `make web-css` is rarely needed).
- **CSP:** `pageCSP` uses `style-src 'self'`, which blocks inline `style=""`. Dynamic bar
  widths must be set by a committed `static/app.js` (allowed by `script-src 'self'`) reading
  `data-*` attributes. Do not weaken the CSP.
- Unbacked menus render as **disabled "coming soon"** entries, never fake pages.
- Conventional Commits; do not commit until the phase is verified.

## Artwork (decided: capture from the live client)
Niburuspace is a private 2Moons-based server; artwork will be mirrored from it and is intended
**for local private use only**.

Feasibility (verified): static assets are publicly served once the path is known. The
authenticated acc1 client already lives in the sibling ops repo:

- `neoxnova-ops/tools/explorer/httpbot.mjs` (browserless) and `explorer.mjs` (Playwright)
- `neoxnova-ops/tools/explorer/session.state.json` (saved cookies), `neoxnova-ops/secrets/explorer.env`
- building sprites are `gebaeude/<id>.gif` (see `neoxnova-ops/tools/explorer/parse.mjs:270`)

Per `AGENTS.md`, the capture tool lives in `neoxnova-ops` (never committed here); only the
resulting images are copied into `neoxnova/internal/web/static/niburu/`.

---

## I0 — Artwork capture pipeline (ops repo + assets here)
**Goal:** a committed, code-addressable image set for buildings, units, defenses, resources,
and shell icons.

Work (in `neoxnova-ops`, sibling repo):
1. Add `tools/explorer/assets.mjs` — reuse the authenticated acc1 session (httpbot login or
   `session.state.json`), fetch each in-game page (`page=buildings|research|shipyard|
   defenses|galaxy|resources|techtree|arsenal`), extract every `<img src>` and CSS `url()`,
   and download the referenced static files (public) into `captures/assets/`.
2. Dedupe + rename by catalog code where derivable (`gebaeude/<id>.gif` → `<code>.<ext>`;
   keep the numeric id map for units `2xx/4xx`).
3. Copy the curated set into `neoxnova/internal/web/static/niburu/` grouped as
   `buildings/`, `research/`, `ships/`, `defenses/`, `icons/`, `bg/`.

Deliverable here: `static/niburu/**` + a short `static/niburu/README.md` listing the
id→file mapping (so templates can reference assets by catalog code).

**Exit:** `go run` serves the images; a template can render `niburu/buildings/metal_mine.gif`.
**Fallback:** if any asset can't be captured, ship an inline-SVG placeholder for that slot
(skin still renders without gaps).

## I1 — Skin foundation + game shell + resource bar
> **Status: ✅ done (2026-10-15).** `static/skin.css` + `static/app.js`; rewritten
> `layout.templ` (top bar, level-free shell, full left menu, resource bar with
> Stardust/DM/AM). `Shell` gained `ServerTime`/`ActiveCoords`. Resources* also renders
> disabled until its page lands in I4 (was unstarred in the menu spec).

- `static/skin.css` (starfield, panels, tables, `.card-grid`, `.res-bar`, `.progress`,
  buttons, `.disabled`) + `static/app.js` (clock, `data-*` bar widths).
- Rewrite `layout.templ`: top bar (server clock, planet switcher, Metal/Crystal/Deut with
  capacity bars, Energy, **Stardust, Dark Matter, Antimatter** — all already on
  `RealtimeResourceState`); left vertical menu with the full live menu
  (Overview, Buildings, Research, Hangar, Defenses, Fleet, Technologies, **Resources**,
  Senate*, Arsenal, Academy*, Achievements*, Black market, Galaxy, Alliance*, Simulator,
  Referals*, Tournaments*, Promote Server*, Online Bonus*, Premium Account*, Antimatter
  Store*). `*` = disabled "coming soon".
- `Shell` gains `ServerTime`; render DM/AM from `s.Resources`. Keep htmx `#resbar` polling.
- **Tests:** existing `internal/web` tests green + a shell-render assertion.
- **Exit:** every existing page wears the new shell; menu matches the screenshots.

## I2 — Overview + Buildings + Research (+ queue cancel)
> **Status: ✅ done (2026-10-15).** `BuildingsBody`/`ResearchBody` are sprite card grids
> (cost rows, energy note, duration, "Necessary for the construction" prereqs, "Busy",
> disabled "Buy with Dark matter.") with an active-queue row + cancel X; `store.CancelQueue`
> (multi-instance safe, refunds construction/shipyard/research) and
> `POST /buildings/{id}/cancel`, `POST /research/{id}/cancel`; Overview restyled onto the
> panel/sprite shell.

- Card grids with `niburu/` icons, cost rows, prereq "Necessary for the construction",
  energy deficit note, level input + "Build/Research to the next level", "Busy",
  "Buy with Dark matter" (disabled placeholder), and a queue list with **cancel X**.
- **Backend:** add `CancelQueue` to `store/build_store.go` + cancel handlers/routes
  (`POST /buildings/{id}/cancel`, `POST /research/{id}/cancel`); multi-instance safe
  (row lock + condition recheck), matching the engine pattern.
- **Tests:** store cancel integration test (skips w/o `DATABASE_URL`), web submit test.
- **Exit:** queue + cancel work from the new UI.

## I3 — Hangar (Shipyard) + Defenses + Fleet
> **Status: ✅ done (2026-10-15).** Hangar/Defenses render grouped sprite card
> grids (Recently built / Small / Medium / Large / Special, bands captured from
> the live shipyard+defenses pages); new `/defenses/{id}` + build/cancel routes;
> wired `store.CancelQueue(CancelShipyard)`; server-computed `Max` (app.js
> delegated fill); Fleet restyled onto panels + `/expedition` link. Class tables
> are unit-tested against the catalog for completeness.
- Shipyard sections: Recently built / Small / Medium / Large class (+ defenses group for the
  Defenses nav) with cost, qty, **Max**, Build.
- Restyle Fleet dispatch + fleet/incoming tables; keep `/expedition` link.
- **Exit:** Hangar and Defenses land on grouped card grids; dispatch unchanged functionally.

## I4 — Galaxy + Technologies (tabbed) + Resources page
- Galaxy: galaxy/system steppers + Saved/View, slot table (moon/debris/player state/alliance)
  with action icons (fleet → dispatch link, spy → `ESPIONAGE` dispatch; message/buddy =
  disabled). Footer stats where derivable (probes/recyclers from `PlanetShips`; fleet slots
  from `computer_tech`); omit stats with no source and note it.
- Technologies: tabs Buildings / Research / Fleet / Planetary Defense / Requirements.
- Resources page (new): add exported `game.ProductionBreakdown(...)` (per-line basic + each
  mine/plant/satellite, bonus, storage capacity, total, daily, weekly); new
  `GET /resources/{id}` + `ResourcesPage`/`ResourcesBody` reproducing `resources_view.png`.
- **Tests:** pure unit tests for `ProductionBreakdown`; render tests for tabs + Resources.
- **Exit:** `/resources/{id}` matches the screenshot; Galaxy footer populated.

## I5 — Account pages + categorized Messages + polish
- Arsenal tabs (Arsenal / Market / Sell Upgrades) over the existing arsenal+market store;
  restyle Black market and Simulator.
- Messages: category grid (Spy/Player/Alliance/Combat/System/Transport/Expedition/Game/
  Construction/All/Archive/Outbox) over the existing reports; backed counts real, others 0/disabled.
- Polish: disabled-nav tooltips, responsive pass, favicon/title, final nav audit vs the screenshots.
- **Tests:** render tests for tabs + message grid.
- **Exit:** every real menu maps to working UI; unbacked menus visibly disabled.

---

## Verification (per phase)
```
cd neoxnova
"$(go env GOPATH)/bin/templ" generate      # or: make web-gen
gofmt -w internal/web
go build ./... && go vet ./... && go test ./...
# manual: make up && make migrate && make seed && make run   (:8080)
```
DB integration tests run only when `DATABASE_URL` is set.

## Risks / notes
- Artwork availability depends on the acc1 session in `neoxnova-ops`; fall back to SVG
  placeholders for any gaps.
- `Resources page` needs one new exported `internal/game` helper (I4).
- `CancelQueue` must stay multi-instance safe (I2).
- This supersedes the old "Web UI menu coverage" backlog item.

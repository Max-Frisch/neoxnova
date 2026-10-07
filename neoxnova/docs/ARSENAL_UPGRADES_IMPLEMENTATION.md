# Arsenal / Upgrades — Go implementation brief (for the NEXT session)

**Goal:** model the live **Arsenal (upgrades)** system in the Go codebase: the catalog,
per-account owned levels, the fleet-point **tiers**, activation, and the **Market**
(listed lots, buy, sell). This doc is the handoff; read `docs/ARSENAL_LIVE_2026-10-06.md`
and `docs/EXPEDITIONS_LIVE_2026-10-06.md` alongside it.

> **User direction (authoritative):** ignore the manual's "75 000 fleet points" gate — it
> is **false**. Use **5 000 / 50 000 / 250 000 fleet points** as the tiers. The user's
> memory on this matches a friend's. `fleet points = (metal+crystal)/1e6` (deuterium
> excluded); the farm's per-fleet set is `≈ 0.1244·S` points (`S` = BB per fleet).

Raw captures this brief is based on (gitignored, in `tools/explorer/data/`):
`_live_page_arsenal.html`, `_live_page_market.html`, `_sellupgrade.html`, `_base.js`,
`arsenal-manual.txt`.

---

## 1. Live web surfaces (exact routes / forms — verified 2026-10-07)

All under `https://niburuspace.com/game/game.php`. Session/CP handling mirrors `httpbot.mjs`.
Routes/forms verified live on **2026-10-07**.

### 1.1 Arsenal listing — `GET game.php?page=arsenal`
- Container `<div id="build_elements" class="upgrade_list">`; one `<div class="build_box">`
  per upgrade. Each box:
  - `<div class="head">NAME</div>`
  - `Bonus: <span>+X%</span> <sup>(+Y)</sup>` — **X = current total bonus, Y = the bonus
    added by the NEXT activation** (this is the "value in brackets").
  - `Avaiable: <span>N</span>` — how many **un-activated scrolls/items of that upgrade**
    the account holds.
  - If `N == 0` and none activated: `<span style="color:#F33;">Absent</span>`, and **no form**.
  - Only when `N >= 1` the box also contains the **Activate** form:
    ```html
    <form action="game.php?page=arsenal" method="post">
      <input type="hidden" name="mode" value="send">
      <input type="hidden" name="greid" value="combustion">   <!-- internal key -->
      <input type="submit" class="input_btn" value="Activate">
    </form>
    ```
- Header strip: `<a href="game.php?page=market">Market</a>` and
  `<a href="#" onclick="return Dialog.CreateLotUpgrade();">Sell Upgrades</a>`; help =
  `Dialog.manualinfo(10)`.
- **`greid` keys** are internal names, not the 1..19 `type` ids below. Only `combustion`
  (Jet engine) has been observed so far — enumerate the rest by owning one of each.

### 1.2 Market (a.k.a. "Upgrade Auction") — `GET game.php?page=market`
- `<div id="market_conteiner">` → left `<span class="market_left_btn"
  onclick="return Dialog.CreateLotUpgrade();">Sell Upgrades</span>`; right
  `<div id="market_content">` with
  `<table class="tablesorter ally_ranks lots">`.
- Columns: **`ID | Upgrade | Amount | Total price | Buy`**. Price example:
  `50.000 Anti Matter`. Buy row form:
  ```html
  <form action="game.php?page=market" method="post">
    <input type="hidden" name="mode" value="BuyUpgrade">
    <input name="id" value="1" type="hidden">
    <button type="submit"><img src="styles/images/buy.png"></button>
  </form>
  ```
  (Shopping-cart "buy" button.) Help = `Dialog.manualinfo(12)`.

### 1.3 Sell Upgrades dialog — `GET game.php?page=market&group=sellUpgrade`
Fancybox iframe (400×154); the actual "Sell Upgrades" tab. Form:
```html
<form action="game.php?page=market" method="post">
  <input type="hidden" name="mode" value="sellUpgrades">
  <select name="type"> <!-- 1..19, label "Name [owned]" --> </select>
  Amount <input id="count" name="amount" type="number" min="1" max="25" value="1">
  Price  <input id="cost"  name="rate"   type="number" min="1" max="1000000" value="500">
  <input type="submit" value="Sell Upgrade">
</form>
```
`rate` = chosen **Antimatter** price per unit. Max sale price per the manual: **5 000 000
DM / 50 000 Antimatter**; a listed lot stays **72 h** then returns.

### 1.4 Other market dialogs (from `scripts/game/base.js` → `Dialog`)
| fn | URL | size |
|---|---|---|
| `CreateLotUpgrade()` | `game.php?page=market&group=sellUpgrade` | 400×154 |
| (post form) | `game.php?page=market&group=sellPost` | 400×200 |
| `PlanetLotInfo(lot)` | `game.php?page=market&group=LotRate&lotID=<id>` | 502×250 |
| `painfo(id)` | `game.php?page=market&group=Information&id=<id>` | 502×500 |
| `PlanetLotRate(id)` | `game.php?page=market&group=planetlotrate&id=<id>` | 502×122 |

---

## 2. Upgrade catalog (live, `page=arsenal`, per-level value = the `(+Y)` bracket)

`type` = the market/sell-form id 1..19. `greid` = activate-form key (mostly TBD).

| type | name | group | per-activation (bracket) | greid |
|---:|---|---|---:|---|
| 1 | Laser weapons | weapon | +0.75 | ? |
| 2 | Ion cannon | weapon | +0.75 | ? |
| 3 | Plasma gun | weapon | +0.75 | ? |
| 4 | Gravitational gun | weapon | +0.75 | ? |
| 5 | Light armor | armor (light) | +0.6 | ? |
| 6 | Medium armor | armor (med) | +0.5 | ? |
| 7 | Heavy armor | armor (heavy) | +0.4 | ? |
| 8 | Light shields | shield (light) | +0.6 | ? |
| 9 | Medium shields | shield (med) | +0.5 | ? |
| 10 | Heavy shields | shield (heavy) | +0.4 | ? |
| 11 | Jet engine | engine (light) | +0.6 | `combustion` |
| 12 | Impulse engine | engine (med) | +0.5 | ? |
| 13 | Hyperspace engine | engine (heavy) | +0.4 | ? |
| 14 | Light conveyor | conveyor | +0.6 | ? |
| 15 | Average conveyor | conveyor | +0.5 | ? |
| 16 | Heavy conveyor | conveyor | +0.4 | ? |
| 17 | Metal production | production | +0.5 | ? |
| 18 | Crystal production | production | +0.45 | ? |
| 19 | Deuterium production | production | +0.4 | ? |

> `ARSENAL_LIVE_2026-10-06.md` lists different weapon/armor percentages (e.g. weapons
> ~2.3–2.6 %). Those came from the Russian manual; **the live page bracket is the value to
> use** — reconcile and correct the older doc if the model needs exact numbers.

### 2.1 Activation rules (manual "Важно")
- **First 10 levels: 100 % success.**
- Each **successful** activation **above level 10** lowers the next success chance by **2 %**.
- **Floor 75 %** minimum success chance.
- A **failed** activation drops the value by **10 %** of the last successful activation.

### 2.2 How upgrades are found / tiered
- Regular expedition (`cmd=1`) combat encounters drop a drawing (live: **≈10 % of combat
  wins**; all four drops so far were **entry tier**, at ~7.7k–10.9k fleet points).
- **Tier gates (authoritative for our model): 5 000 / 50 000 / 250 000 fleet points.**
  Ignore the 75 000 figure. Decide/confirm whether the tier selects the *type pool* or the
  *drop chance* (the manual's Barbarian/Pirate/Alien Hostail split may be the race variant).
- Hostail (`cmd=2`) is buggy (ghost fleets) — **do not** implement its path yet.

---

## 3. Go implementation plan

Follow existing conventions (`internal/models`, `internal/store`, `internal/engine`,
`internal/api`+`handlers`, numbered idempotent migrations added to the Makefile `migrate`).

1. **Catalog (pure, `internal/game`)** — `arsenal.go`:
   - `type Upgrade struct{ Code int; Key, Name, Group string; PerLevel float64; ... }`
     seeded from the table in §2 (Code = the 1..19 `type`).
   - `ArsenalTier(points int64) int` → 0/<5k, 1/≥5k, 2/≥50k, 3/≥250k; plus
     `TierRange(tier)` and a `DropPool(tier)` selector.
   - `ActivationOutcome(level int, rng) (success bool, chance float64)` implementing §2.1
     (100 % for the first 10, −2 %/success, floor 75 %, −10 % value on fail).
2. **Models/store:**
   - `account_upgrades(account_id, upgrade_code, level, value)` (one row per owned upgrade).
   - `upgrade_items(account_id, upgrade_code, qty)` — un-activated drawings held.
   - `market_lots(id, seller_account_id, upgrade_code, amount, price_atm, created_at,
     expires_at)` (72 h), `market_purchases(...)` for audit.
   - Store methods: `OwnedUpgrades`, `ActivateUpgrade` (transactional: consume 1 item,
     roll, update level/value), `ListItem`, `ListMarket`, `BuyLot` (atomic buyer-charge →
     seller-credit → delete/transfer lot; row-lock), `ExpireLots`.
3. **Engine (`internal/engine`)** — scheduled `ExpireLots` and any delayed lot return.
4. **API (`internal/api/handlers`)** — mirror the live routes/semantics:
   - `GET /arsenal` → list w/ `bonus`, `nextBonus` (the bracket), `available`, `level`.
   - `POST /arsenal {mode:send, greid}` → activate.
   - `GET /market` → lots (`id, upgrade, amount, totalPriceAtm, buy`).
   - `POST /market {mode:BuyUpgrade, id}` → buy.
   - `POST /market {mode:sellUpgrades, type, amount, rate}` → list a lot.
   - `GET /market/lots/:id` / `planetlotrate` if needed.
5. **Migrations** — one numbered SQL file; idempotent; add to the `migrate` target.

---

## 4. Tooling to add (verify against live)
- `httpbot.mjs arsenal` → parse §1.1 (name, current %, next-value bracket, available) as JSON.
- `httpbot.mjs market` → parse the §1.2 table (id, name, amount, price) as JSON.
- `httpbot.mjs activate <greid>` and `httpbot.mjs sell <type> <amount> <rate>` (dry + `--go`).
- Enumerate every `greid` by owning one of each upgrade, then fill §2's `greid` column.

## 5. Open questions / risks
- Confirm **5k/50k/250k** semantics (type pool vs chance vs both) with a controlled sample.
- Reconcile the per-level numbers with `ARSENAL_LIVE_2026-10-06.md` (manual vs live bracket).
- `greid` keys for all 19 upgrades (only `combustion` known).
- Failure roll result shape (does the item get consumed on failure? manual implies the value
  drops — verify whether the scroll is lost).
- Anti-cheat / rate limits on activation and market posts.

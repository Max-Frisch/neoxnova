-- 0021_blackmarket.sql — Black market economy knobs on the universes row.
--
-- B4 of docs/PLAYABILITY_FIXES_PLAN.md. Two runtime-tunable values back the new
-- Black market sub-pages and are upserted from config/game.json on boot (C1):
--   * merchant_rate     — Merchant of Fleets refund, percent of a unit's base
--                         cost the trader pays back (default 50%, the live cut).
--   * dm_market_divisor — Buildings/Technology Market: resources-per-Dark-Matter
--                         divisor; DM price = ceil(resource cost / divisor).
--                         Default 1000 (1000 resources => 1 Dark Matter).
--
-- Idempotent: safe to re-run.

ALTER TABLE universes
    ADD COLUMN IF NOT EXISTS merchant_rate SMALLINT NOT NULL DEFAULT 50;

ALTER TABLE universes
    ADD COLUMN IF NOT EXISTS dm_market_divisor INTEGER NOT NULL DEFAULT 1000;

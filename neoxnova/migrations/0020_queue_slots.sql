-- 0020_queue_slots.sql — multi-order queues: slot caps + drop single-active indexes.
--
-- Q1 of docs/PLAYABILITY_FIXES_PLAN.md: buildings and research become *chained*
-- multi-order queues (default 10 / 4 slots), shipyard/defense units become an
-- unlimited back-to-back chain. The old one-active-row-per-scope unique indexes
-- therefore go away; the slot caps live on the universes row and are enforced in
-- Go under the planet/user lock (Q2).
--
-- Idempotent: safe to re-run.

ALTER TABLE universes
    ADD COLUMN IF NOT EXISTS build_queue_slots SMALLINT NOT NULL DEFAULT 10;

ALTER TABLE universes
    ADD COLUMN IF NOT EXISTS research_queue_slots SMALLINT NOT NULL DEFAULT 4;

DROP INDEX IF EXISTS uq_active_construction;
DROP INDEX IF EXISTS uq_active_research;
DROP INDEX IF EXISTS uq_active_shipyard;

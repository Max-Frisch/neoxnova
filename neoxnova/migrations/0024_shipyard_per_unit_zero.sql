-- 0024_shipyard_per_unit_zero.sql — allow sub-0.01s shipyard per-unit build times.
--
-- The scheduler resolves a whole shipyard batch at end_time; build_time_per_unit
-- is informational only. A very fast, large batch (e.g. 500 solar satellites at a
-- high game_speed) has its total duration floored to 1s, so
-- build_time_per_unit = 1/500 rounds below the NUMERIC(10,2) minimum and the old
-- `> 0` check rejected the insert. Relax the check to `>= 0`. Idempotent.

ALTER TABLE shipyard_queues
    DROP CONSTRAINT IF EXISTS shipyard_queues_build_time_per_unit_check;

ALTER TABLE shipyard_queues
    ADD CONSTRAINT shipyard_queues_build_time_per_unit_check CHECK (build_time_per_unit >= 0);

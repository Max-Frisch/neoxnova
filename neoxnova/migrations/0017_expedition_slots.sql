-- 0017_expedition_slots.sql — expedition slot budget (Astrophysics-gated).
--
-- The reference server grants one expedition slot per two Astrophysics levels
-- and sells extras in its premium menu. `expedition_slots_bonus` models those
-- extras; a non-zero `expedition_slots_cap` clamps the total (0 = uncapped).
-- Both are per-universe so a round can tune the number of concurrent expos.

ALTER TABLE universes
    ADD COLUMN IF NOT EXISTS expedition_slots_bonus SMALLINT NOT NULL DEFAULT 0
        CHECK (expedition_slots_bonus >= 0);

ALTER TABLE universes
    ADD COLUMN IF NOT EXISTS expedition_slots_cap SMALLINT NOT NULL DEFAULT 0
        CHECK (expedition_slots_cap >= 0);

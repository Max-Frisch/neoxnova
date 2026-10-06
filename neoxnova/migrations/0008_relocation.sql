-- 0008_relocation.sql — planet relocation.
--
-- Relocating moves a planet (with its buildings, ships, defenses and stored
-- resources) to new coordinates for a distance-priced Dark Matter fee. Unlike
-- abandonment it does NOT lock the origin slot: the planet simply leaves, so the
-- old position becomes freely colonisable. Instead the planet itself is put on a
-- per-planet cooldown (stored here) before it can be relocated again.

ALTER TABLE celestial_objects
    ADD COLUMN IF NOT EXISTS relocation_next_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_celestials_relocation_next
    ON celestial_objects (relocation_next_at)
    WHERE relocation_next_at IS NOT NULL;

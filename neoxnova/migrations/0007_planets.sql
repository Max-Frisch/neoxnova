-- 0007_planets.sql — planet slots, field farming and occupancy locks.
--
-- Planets per system is configurable (niburu uses 20; the slot after the last
-- is the expedition/deep-space slot). base_fields_max remembers the colonisation
-- roll so Terraformer bonuses can be recomputed without losing the base.

ALTER TABLE universes
    ADD COLUMN IF NOT EXISTS planets_per_system INT NOT NULL DEFAULT 20
        CHECK (planets_per_system BETWEEN 1 AND 20);

ALTER TABLE celestial_objects
    ADD COLUMN IF NOT EXISTS base_fields_max BIGINT NOT NULL DEFAULT 163
        CHECK (base_fields_max >= 0);

-- A slot stays locked for a while after a planet is abandoned/destroyed so the
-- same position cannot be instantly re-rolled. Locks are global (per slot).
CREATE TABLE IF NOT EXISTS coordinate_locks (
    universe_id  UUID NOT NULL REFERENCES universes(id) ON DELETE CASCADE,
    galaxy       coord_galaxy NOT NULL,
    system       coord_system NOT NULL,
    position     coord_position NOT NULL,
    locked_until TIMESTAMPTZ NOT NULL,
    reason       TEXT,
    PRIMARY KEY (universe_id, galaxy, system, position)
);

CREATE INDEX IF NOT EXISTS idx_coordinate_locks_until ON coordinate_locks (locked_until);

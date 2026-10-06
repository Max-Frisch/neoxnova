-- 0009_planet_fields.sql — Dark-Matter field expansion + post-teleport lock.
--
-- fields_bought counts extra fields purchased with Dark Matter (used only to
-- price the escalating curve). A purchase increments base_fields_max, fields_max
-- and fields_bought together, so RecomputeCelestial (fields_max =
-- base_fields_max + 7*terraformer) preserves the purchase across builds.
--
-- attack_locked_until is set on a planet for 15 minutes after a teleport, during
-- which it may not launch ATTACK missions (Planetarium rule).

ALTER TABLE celestial_objects
    ADD COLUMN IF NOT EXISTS fields_bought BIGINT NOT NULL DEFAULT 0
        CHECK (fields_bought >= 0);

ALTER TABLE celestial_objects
    ADD COLUMN IF NOT EXISTS attack_locked_until TIMESTAMPTZ;

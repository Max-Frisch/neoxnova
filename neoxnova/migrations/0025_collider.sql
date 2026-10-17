-- 0025_collider.sql — Moon Collider (page=kollaider) + account Dark Matter accrual.
--
-- The Collider is a moon-only building (numeric id 69) bought with Antimatter
-- that produces Dark Matter per hour. Its level lives on the moon
-- (celestial_objects.collider_level); the cached DM/h output is written by
-- RecomputeCelestial. Because Dark Matter is an **account-wide** currency
-- (users.dark_matter), the output of every owned moon's collider is summed and
-- banked continuously by update_user_dark_matter(). Idempotent.

ALTER TABLE celestial_objects
    ADD COLUMN IF NOT EXISTS collider_level INT NOT NULL DEFAULT 0;

ALTER TABLE celestial_objects
    ADD COLUMN IF NOT EXISTS collider_dm_prod_hourly NUMERIC(16, 4) NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'celestial_objects_collider_level_check'
    ) THEN
        ALTER TABLE celestial_objects
            ADD CONSTRAINT celestial_objects_collider_level_check CHECK (collider_level >= 0);
    END IF;
END $$;

-- Account-wide Dark Matter accumulator. dark_matter_calc_at is the timestamp the
-- banked integer was last advanced; dark_matter_acc keeps the sub-unit
-- remainder so flooring each accrual cannot leak fractions; dark_matter_prod_hourly
-- caches the summed collider output for the account.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS dark_matter_calc_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS dark_matter_acc NUMERIC(24, 4) NOT NULL DEFAULT 0;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS dark_matter_prod_hourly NUMERIC(16, 4) NOT NULL DEFAULT 0;

-- update_user_dark_matter banks the Dark Matter generated since the last call at
-- the account's cached rate. It row-locks the user (safe under concurrent reads)
-- and retains the fractional remainder. Returns the new banked balance and rate.
CREATE OR REPLACE FUNCTION update_user_dark_matter(p_user_id BIGINT)
RETURNS TABLE (
    out_dark_matter BIGINT,
    out_prod_hourly NUMERIC(16, 4)
)
LANGUAGE plpgsql AS $$
DECLARE
    v_now TIMESTAMPTZ := NOW();
    v_last TIMESTAMPTZ;
    v_dm BIGINT;
    v_acc NUMERIC(24, 4);
    v_rate NUMERIC(16, 4);
    v_elapsed NUMERIC;
    v_total NUMERIC(24, 4);
    v_whole BIGINT;
BEGIN
    SELECT dark_matter, dark_matter_acc, dark_matter_calc_at, dark_matter_prod_hourly
    INTO v_dm, v_acc, v_last, v_rate
    FROM users
    WHERE id = p_user_id
    FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'user % not found', p_user_id;
    END IF;

    v_elapsed := EXTRACT(EPOCH FROM (v_now - v_last));
    IF v_elapsed > 0 THEN
        v_total := v_acc + (v_rate / 3600.0) * v_elapsed;
        v_whole := FLOOR(v_total)::bigint;
        v_acc := v_total - v_whole;

        UPDATE users
        SET dark_matter = dark_matter + v_whole,
            dark_matter_acc = v_acc,
            dark_matter_calc_at = v_now
        WHERE id = p_user_id
        RETURNING dark_matter INTO v_dm;
    END IF;

    RETURN QUERY SELECT v_dm, v_rate;
END;
$$;

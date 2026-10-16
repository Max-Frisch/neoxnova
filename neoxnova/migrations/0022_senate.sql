-- 0022_senate.sql — Senate: Governators (and the Achievement Points currency).
--
-- Governators are a two-phase, account-wide bonus system captured from the live
-- server (info/Senate). Each governator has:
--   * a permanent, irreversibly-bought level (paid in Achievement Points), which
--     locks in the bonus percent (base +10% plus +1% per level, capped per
--     governator); and
--   * an activation ("hire") that makes that accumulated bonus live for a number
--     of days, priced in Dark Matter per day by the current level.
-- Level persists; expires_at is nulled when an activation lapses (see the
-- engine's expiry sweep), so the bought bonus is kept but goes inactive.
--
-- Achievement Points are granted by the (not-yet-implemented) Achievements
-- system; for now they can be seeded directly.

ALTER TABLE users ADD COLUMN IF NOT EXISTS achievement_points BIGINT NOT NULL DEFAULT 0
    CHECK (achievement_points >= 0);

CREATE TABLE IF NOT EXISTS user_governators (
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    governator_code INT NOT NULL CHECK (governator_code BETWEEN 701 AND 707),
    level           INT NOT NULL DEFAULT 0 CHECK (level >= 0),
    activated_at    TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ,
    PRIMARY KEY (user_id, governator_code)
);

-- The expiry sweep scans for lapsed activations.
CREATE INDEX IF NOT EXISTS idx_user_governators_expiry
    ON user_governators(expires_at) WHERE expires_at IS NOT NULL;

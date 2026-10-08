-- 0014_blueprints.sql — persisted auto-build blueprints (backlog item 3).
--
-- One enabled planet blueprint per celestial; an optional account-scope
-- blueprint (celestial_id NULL) carries empire-wide research goals. The spec is
-- JSONB in the language of docs/AUTO_BUILD_DESIGN.md §5. See
-- internal/blueprint for the pure planner.
CREATE TABLE IF NOT EXISTS build_blueprints (
    id             BIGSERIAL PRIMARY KEY,
    universe_id    UUID NOT NULL REFERENCES universes(id) ON DELETE CASCADE,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    celestial_id   BIGINT REFERENCES celestial_objects(id) ON DELETE CASCADE,
    name           TEXT NOT NULL DEFAULT '',
    scope          TEXT NOT NULL DEFAULT 'planet' CHECK (scope IN ('planet','account')),
    spec           JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled        BOOLEAN NOT NULL DEFAULT false,
    priority       INT NOT NULL DEFAULT 100,
    paused_until   TIMESTAMPTZ,
    last_action_at TIMESTAMPTZ,
    last_error     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- At most one enabled planet blueprint per celestial.
CREATE UNIQUE INDEX IF NOT EXISTS uq_blueprint_planet_enabled
    ON build_blueprints (celestial_id)
    WHERE enabled AND celestial_id IS NOT NULL;

-- At most one enabled account blueprint per user.
CREATE UNIQUE INDEX IF NOT EXISTS uq_blueprint_account_enabled
    ON build_blueprints (user_id)
    WHERE enabled AND celestial_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_blueprints_enabled ON build_blueprints (enabled, id);
CREATE INDEX IF NOT EXISTS idx_blueprints_user ON build_blueprints (user_id, id);

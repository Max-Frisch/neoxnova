-- 0013_expedition_reports.sql — persisted expedition outcome messages.
--
-- Every resolved expedition (resources / ships / combat / dark matter / delay /
-- early return / nothing / black hole) gets a row so the player can read the
-- outcome in their message inbox. Combat fights additionally reference the
-- combat_reports row; the recovered ships and full result are stored as JSONB.
CREATE TABLE IF NOT EXISTS expedition_reports (
    id                 BIGSERIAL PRIMARY KEY,
    universe_id        UUID NOT NULL REFERENCES universes(id) ON DELETE CASCADE,
    fleet_id           BIGINT,
    user_id            BIGINT,                      -- the expedition's owner
    target_galaxy      coord_galaxy NOT NULL,
    target_system      coord_system NOT NULL,
    target_position    coord_position NOT NULL,
    outcome            VARCHAR(24) NOT NULL,        -- resources|ships|combat|darkmatter|delay|fast|nothing|blackhole
    npc                VARCHAR(16),                 -- pirates|aliens for combat, else NULL
    combat_report_id   BIGINT,                      -- set when outcome = combat
    cargo_metal        BIGINT NOT NULL DEFAULT 0 CHECK (cargo_metal >= 0),
    cargo_crystal      BIGINT NOT NULL DEFAULT 0 CHECK (cargo_crystal >= 0),
    cargo_deuterium    BIGINT NOT NULL DEFAULT 0 CHECK (cargo_deuterium >= 0),
    dark_matter        BIGINT NOT NULL DEFAULT 0 CHECK (dark_matter >= 0),
    ships              JSONB,                       -- recovered ships, code -> count
    upgrade_code       INT NOT NULL DEFAULT 0,      -- Arsenal drawing (1..19) or 0
    return_adjust_secs BIGINT NOT NULL DEFAULT 0,   -- positive = delayed, negative = early
    title              TEXT NOT NULL DEFAULT '',
    message            TEXT NOT NULL DEFAULT '',
    detail             JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_expedition_reports_user ON expedition_reports (user_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_expedition_reports_fleet ON expedition_reports (fleet_id, id DESC);

-- 0005_combat_reports.sql — persisted battle reports for players.
--
-- The full resolution (per-round losses + outcome) is stored as JSONB so the
-- API can serve a rich report without a schema per field. See
-- internal/game/combat.go (CombatResult) for the report payload shape.
CREATE TABLE IF NOT EXISTS combat_reports (
    id             BIGSERIAL PRIMARY KEY,
    universe_id    UUID NOT NULL REFERENCES universes(id) ON DELETE CASCADE,
    fleet_id       BIGINT,                      -- attacking fleet (kept after RESOLVED for history)
    attacker_id    BIGINT,
    defender_id    BIGINT,
    target_id      BIGINT,
    galaxy         coord_galaxy NOT NULL,
    system         coord_system NOT NULL,
    position       coord_position NOT NULL,
    result         VARCHAR(16) NOT NULL,        -- 'attacker' | 'defender' | 'draw'
    rounds         INT NOT NULL,
    debris_metal   BIGINT NOT NULL DEFAULT 0 CHECK (debris_metal >= 0),
    debris_crystal BIGINT NOT NULL DEFAULT 0 CHECK (debris_crystal >= 0),
    moon_chance    INT NOT NULL DEFAULT 0 CHECK (moon_chance BETWEEN 0 AND 20),
    report         JSONB NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_combat_reports_attacker ON combat_reports (attacker_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_combat_reports_defender ON combat_reports (defender_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_combat_reports_target   ON combat_reports (target_id, id DESC);

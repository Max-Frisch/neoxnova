-- 0010_espionage_reports.sql — persisted spy reports.
--
-- Niburu returns the FULL report regardless of espionage-tech difference (see
-- docs/ESPIONAGE_LIVE_2026-10-06.md); the tech difference only drives
-- counter-espionage. The full intel payload is stored as JSONB so the API can
-- serve it without a schema per section.
CREATE TABLE IF NOT EXISTS espionage_reports (
    id           BIGSERIAL PRIMARY KEY,
    universe_id  UUID NOT NULL REFERENCES universes(id) ON DELETE CASCADE,
    fleet_id     BIGINT,
    attacker_id  BIGINT,                      -- the spying player
    defender_id  BIGINT,                      -- the target's owner (incoming event for them)
    target_id    BIGINT,
    galaxy       coord_galaxy NOT NULL,
    system       coord_system NOT NULL,
    position     coord_position NOT NULL,
    probes_sent  INT NOT NULL DEFAULT 0 CHECK (probes_sent >= 0),
    probes_lost  INT NOT NULL DEFAULT 0 CHECK (probes_lost >= 0),
    score        INT NOT NULL DEFAULT 0,
    report       JSONB NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_espionage_reports_attacker ON espionage_reports (attacker_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_espionage_reports_defender ON espionage_reports (defender_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_espionage_reports_target   ON espionage_reports (target_id, id DESC);

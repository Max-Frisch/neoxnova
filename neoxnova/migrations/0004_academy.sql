-- 0004_academy.sql — per-user academy skill levels.
--
-- The reference server's academy tree (codes 11xx attack branch, 13xx defense
-- branch) grants combat procs that measurably affect battles (see
-- docs/COMBAT_SESSION_2026-10-05.md). Only the skill level is persisted; the
-- combat interpretation lives in internal/game/combat.go.
CREATE TABLE IF NOT EXISTS user_academy_skills (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    skill_code VARCHAR(16) NOT NULL, -- e.g. '1103' (Double attack), '1109' (Chain reaction)
    level      INT NOT NULL DEFAULT 0 CHECK (level >= 0),
    PRIMARY KEY (user_id, skill_code)
);

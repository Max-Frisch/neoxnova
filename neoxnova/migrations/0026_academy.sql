-- 0026_academy.sql — Academy points currency (the skill tree itself is 0004).
--
-- Academy points are the "Experience points" granted by the Online Bonus button
-- (see 0023_peaceful.sql / game.BonusButtonReward); `user_academy_skills` (0004)
-- stores the bought skill levels. Idempotent.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS academy_points BIGINT NOT NULL DEFAULT 0
        CHECK (academy_points >= 0);

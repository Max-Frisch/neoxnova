-- 0023_peaceful.sql — peaceful (online) level tracking + the Online Bonus button.
--
-- Peaceful level accrues from time spent online: the web layer records activity
-- gaps (capped) into users.peaceful_seconds, and the level/bonuses are derived
-- from that total. The Online Bonus button grants Dark Matter (+ peaceful XP +
-- combat XP) on a cooldown. Idempotent.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS peaceful_seconds BIGINT NOT NULL DEFAULT 0;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS peaceful_seen_at TIMESTAMPTZ;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS bonus_button_ready_at TIMESTAMPTZ;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS bonus_last_click_at TIMESTAMPTZ;

-- Make the button available immediately for existing accounts.
UPDATE users SET bonus_button_ready_at = NOW() WHERE bonus_button_ready_at IS NULL;

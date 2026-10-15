-- 0018_stardust.sql — Stardust, the rare expedition currency (backlog X3).
--
-- Stardust is found rarely on expeditions (a distinct `stardust` outcome) and
-- is intended to be spent on the debris-based planet/moon diameter + fields
-- increase function. Only the find + balance is modelled for now; the sink is
-- deferred. `users.stardust` is the empire-wide balance (like dark_matter);
-- `expedition_reports.stardust` records the find on the outcome message.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS stardust BIGINT NOT NULL DEFAULT 0 CHECK (stardust >= 0);

ALTER TABLE expedition_reports
    ADD COLUMN IF NOT EXISTS stardust BIGINT NOT NULL DEFAULT 0 CHECK (stardust >= 0);

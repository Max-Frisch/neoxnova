-- 0027_messages.sql — generic player inbox messages.
--
-- The three report categories (spy / combat / expedition) keep their own
-- structured tables; every other category the live client shows (player,
-- alliance, system, transport, game, construction) is a free-form message.
-- `category` mirrors the live client's message `cat` ids (see game.MessageCategory*).
CREATE TABLE IF NOT EXISTS messages (
    id          BIGSERIAL PRIMARY KEY,
    universe_id UUID NOT NULL REFERENCES universes(id) ON DELETE CASCADE,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category    SMALLINT NOT NULL,
    class       VARCHAR(32) NOT NULL DEFAULT '',
    sender      VARCHAR(96) NOT NULL DEFAULT '',
    subject     VARCHAR(255) NOT NULL DEFAULT '',
    body        TEXT NOT NULL DEFAULT '',
    ref_kind    VARCHAR(24),                  -- optional engine link, e.g. 'fleet'
    ref_id      BIGINT,
    read_at     TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_messages_user ON messages (user_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_messages_user_cat ON messages (user_id, category, id DESC);

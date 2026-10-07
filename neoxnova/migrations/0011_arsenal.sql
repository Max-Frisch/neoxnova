-- 0011_arsenal.sql — Arsenal upgrades + auction market.
--
-- Upgrade drawings drop from expedition combat wins (see
-- docs/ARSENAL_UPGRADES_IMPLEMENTATION.md). An owned upgrade has a level (number
-- of successful activations) and a value (accumulated bonus). Un-activated
-- drawings are held in upgrade_items and can be activated or listed on the
-- market. Lots expire after 72h and are returned by the scheduler, so
-- market_lots only ever holds live offers.

CREATE TABLE IF NOT EXISTS account_upgrades (
    account_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    upgrade_code INT NOT NULL CHECK (upgrade_code BETWEEN 1 AND 19),
    level        INT NOT NULL DEFAULT 0 CHECK (level >= 0),
    value        NUMERIC(12, 4) NOT NULL DEFAULT 0 CHECK (value >= 0),
    PRIMARY KEY (account_id, upgrade_code)
);

-- Un-activated drawings held by an account.
CREATE TABLE IF NOT EXISTS upgrade_items (
    account_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    upgrade_code INT NOT NULL CHECK (upgrade_code BETWEEN 1 AND 19),
    qty          INT NOT NULL DEFAULT 0 CHECK (qty >= 0),
    PRIMARY KEY (account_id, upgrade_code)
);

-- Live auction lots. price_atm is the buyer-facing TOTAL in Antimatter; the
-- live sell form's `rate` is per-unit, and the store multiplies by amount.
CREATE TABLE IF NOT EXISTS market_lots (
    id                BIGSERIAL PRIMARY KEY,
    seller_account_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    upgrade_code      INT NOT NULL CHECK (upgrade_code BETWEEN 1 AND 19),
    amount            INT NOT NULL CHECK (amount BETWEEN 1 AND 25),
    price_atm         BIGINT NOT NULL CHECK (price_atm >= 1),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at        TIMESTAMPTZ NOT NULL DEFAULT NOW() + interval '72 hours'
);

CREATE INDEX IF NOT EXISTS idx_market_lots_expiry ON market_lots(expires_at);

-- Completed sales, for audit. Expired (unsold) lots are simply returned to the
-- seller and are not recorded here.
CREATE TABLE IF NOT EXISTS market_purchases (
    id                BIGSERIAL PRIMARY KEY,
    lot_id            BIGINT NOT NULL,
    buyer_account_id  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    seller_account_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    upgrade_code      INT NOT NULL,
    amount            INT NOT NULL,
    price_atm         BIGINT NOT NULL,
    purchased_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

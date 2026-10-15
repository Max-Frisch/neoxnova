-- 0019_moon_ops.sql — moon operations: Jumpgate cooldown + Phalanx offline.
--
-- jumpgate_next_at is a moon's Jumpgate recharge timestamp: a jump is refused
-- until NOW() passes it, then set to NOW() + game.JumpgateCooldown(level).
--
-- phalanx_offline_until bars a Phalanx Sensor from scanning for a short window
-- after its celestial is teleported (Planetarium rule; 10 minutes).

ALTER TABLE celestial_objects
    ADD COLUMN IF NOT EXISTS jumpgate_next_at TIMESTAMPTZ;

ALTER TABLE celestial_objects
    ADD COLUMN IF NOT EXISTS phalanx_offline_until TIMESTAMPTZ;

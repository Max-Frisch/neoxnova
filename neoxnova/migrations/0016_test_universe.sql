-- ============================================================================
-- 0016_test_universe.sql — fast, self-contained universe for the web e2e test.
--
-- The production model universe ('universe_6_niburu') is calibrated to the live
-- server. This migration adds a SEPARATE sandbox universe ('universe_test') with
-- cranked rates so the browser test can play a full loop in seconds:
--   game speed 50000x, resource 50000x, fleet speed 500000x (flights ~1s).
-- It seeds two ordinary players (webtest_a / webtest_b) next door to each other
-- with starter fleets, defenses, research and resources so transport, espionage,
-- colonization and combat are all exercisable through the UI.
--
-- Idempotent: safe to re-run (INSERT ... ON CONFLICT DO NOTHING / DO UPDATE).
-- Both users share the seeded dev password ('commander-dev-pass').
-- ============================================================================

-- 1. Universe ---------------------------------------------------------------
INSERT INTO universes (
    code_name, display_name, game_speed, resource_speed, fleet_speed,
    debris_rate, max_galaxies, max_systems, base_colonies, planets_per_system
) VALUES (
    'universe_test', 'Test Sandbox (fast)', 50000.00, 50000.00, 500000.00,
    0.50, 2, 499, 5, 20
)
ON CONFLICT (code_name) DO UPDATE SET
    display_name       = EXCLUDED.display_name,
    game_speed         = EXCLUDED.game_speed,
    resource_speed     = EXCLUDED.resource_speed,
    fleet_speed        = EXCLUDED.fleet_speed,
    debris_rate        = EXCLUDED.debris_rate,
    max_galaxies       = EXCLUDED.max_galaxies,
    max_systems        = EXCLUDED.max_systems,
    base_colonies      = EXCLUDED.base_colonies,
    planets_per_system = EXCLUDED.planets_per_system;

-- 2. Two players (shared dev password: commander-dev-pass) -------------------
INSERT INTO users (universe_id, username, email, password_hash, dark_matter, antimatter)
SELECT id, v.username, v.email,
       'pbkdf2_sha256$600000$L5s8bx0R/Xpb+Cl/DRhVqg$IKf0HjYPPk/HN4p7JsX5uJB8iKFb+oR2CVy79bY94jo',
       v.dm, v.am
FROM universes, (VALUES
    ('webtest_a', 'webtest_a@neoxnova.local', 100000::BIGINT, 100000::BIGINT),
    ('webtest_b', 'webtest_b@neoxnova.local',  10000::BIGINT,  10000::BIGINT)
) AS v(username, email, dm, am)
WHERE universes.code_name = 'universe_test'
ON CONFLICT (universe_id, username) DO UPDATE SET
    password_hash = EXCLUDED.password_hash,
    email         = EXCLUDED.email,
    dark_matter   = EXCLUDED.dark_matter,
    antimatter    = EXCLUDED.antimatter;

-- 3. Homeworlds: A at 1:1:1, B at 1:1:5 (same system, one hop apart) ---------
INSERT INTO celestial_objects (
    universe_id, user_id, name, object_type, galaxy, system, position,
    diameter_km, fields_used, fields_max, base_fields_max, temp_min, temp_max,
    metal, crystal, deuterium,
    metal_capacity, crystal_capacity, deuterium_capacity,
    metal_prod_hourly, crystal_prod_hourly, deuterium_prod_hourly,
    energy_used, energy_max
)
SELECT u.id, usr.id, v.name, 'PLANET', 1, 1, v.pos,
       12800, 0, 700, 700, -20, 40,
       v.metal, v.crystal, v.deut,
       1000000000000, 1000000000000, 1000000000000,
       1000000000, 1000000000, 1000000000, 0, 1000000000
FROM (VALUES
    ('webtest_a', 'Test Alpha', 1,  1000000000000::NUMERIC, 500000000000::NUMERIC, 100000000000::NUMERIC),
    ('webtest_b', 'Test Beta',  5,  2000000000000::NUMERIC, 1000000000000::NUMERIC, 200000000000::NUMERIC)
) AS v(username, name, pos, metal, crystal, deut)
JOIN universes u ON u.code_name = 'universe_test'
JOIN users usr ON usr.universe_id = u.id AND usr.username = v.username
ON CONFLICT (universe_id, galaxy, system, position, object_type) DO NOTHING;

-- 4. Structures -------------------------------------------------------------
INSERT INTO planet_structures (celestial_id, structure_code, level)
SELECT c.id, s.code, s.lvl
FROM celestial_objects c
JOIN universes u ON u.id = c.universe_id AND u.code_name = 'universe_test'
JOIN users usr ON usr.id = c.user_id AND usr.username IN ('webtest_a', 'webtest_b')
CROSS JOIN (VALUES
    ('metal_mine', 20), ('crystal_mine', 20), ('deuterium_synthesizer', 20),
    ('solar_plant', 25), ('robotics_factory', 10), ('shipyard', 12),
    ('research_lab', 10), ('nanite_factory', 2), ('university', 1)
) AS s(code, lvl)
WHERE c.object_type = 'PLANET' AND c.galaxy = 1 AND c.system = 1
ON CONFLICT (celestial_id, structure_code) DO NOTHING;

-- 5. Empire-wide research ---------------------------------------------------
INSERT INTO user_technologies (user_id, tech_code, level)
SELECT usr.id, t.code, t.lvl
FROM users usr
JOIN universes u ON u.id = usr.universe_id AND u.code_name = 'universe_test'
CROSS JOIN (VALUES
    ('espionage_tech', 6), ('energy_tech', 6), ('computer_tech', 5),
    ('combustion_drive', 10), ('impulse_drive', 6), ('hyperspace_drive', 6),
    ('astrophysics', 3), ('weapons_tech', 6), ('shielding_tech', 6),
    ('armour_tech', 6), ('laser_tech', 6), ('ion_tech', 5), ('plasma_tech', 4),
    ('hyperspace_tech', 5)
) AS t(code, lvl)
WHERE usr.username IN ('webtest_a', 'webtest_b')
ON CONFLICT (user_id, tech_code) DO NOTHING;

-- 6. Fleets: A is the attacker, B fields a mixed defense --------------------
INSERT INTO planet_ships (celestial_id, ship_code, quantity)
SELECT c.id, s.code, s.qty
FROM (VALUES
    ('webtest_a', 500::BIGINT, 100::BIGINT, 2000::BIGINT, 300::BIGINT, 200::BIGINT,
     5::BIGINT, 300::BIGINT, 200::BIGINT, 100::BIGINT, 200::BIGINT, 200::BIGINT,
     300::BIGINT, 200::BIGINT, 100::BIGINT),
    ('webtest_b', 300::BIGINT, 100::BIGINT, 1500::BIGINT, 200::BIGINT, 150::BIGINT,
     0::BIGINT, 100::BIGINT, 50::BIGINT, 50::BIGINT, 100::BIGINT, 100::BIGINT,
     200::BIGINT, 100::BIGINT, 50::BIGINT)
) AS v(username, cargo, hcargo, lf, cruiser, bs, colony, recycler, probe, bomber, sf, bc, bt, brec, frig)
JOIN universes u ON u.code_name = 'universe_test'
JOIN users usr ON usr.universe_id = u.id AND usr.username = v.username
JOIN celestial_objects c ON c.user_id = usr.id AND c.object_type = 'PLANET'
     AND c.galaxy = 1 AND c.system = 1
CROSS JOIN LATERAL (VALUES
    ('202', v.cargo), ('203', v.hcargo), ('204', v.lf), ('206', v.cruiser),
    ('207', v.bs), ('208', v.colony), ('209', v.recycler), ('210', v.probe),
    ('211', v.bomber), ('213', v.sf), ('215', v.bc), ('217', v.bt),
    ('219', v.brec), ('227', v.frig)
) AS s(code, qty)
WHERE s.qty > 0
ON CONFLICT (celestial_id, ship_code) DO NOTHING;

-- 7. Defense on Beta so an attack meets real resistance ---------------------
INSERT INTO planet_defenses (celestial_id, defense_code, quantity)
SELECT c.id, d.code, d.qty
FROM celestial_objects c
JOIN universes u ON u.id = c.universe_id AND u.code_name = 'universe_test'
JOIN users usr ON usr.id = c.user_id AND usr.username = 'webtest_b'
CROSS JOIN (VALUES
    ('401', 800::BIGINT), ('402', 400::BIGINT), ('403', 200::BIGINT),
    ('404', 100::BIGINT), ('405', 100::BIGINT), ('406', 20::BIGINT),
    ('407', 2::BIGINT)
) AS d(code, qty)
WHERE c.object_type = 'PLANET' AND c.galaxy = 1 AND c.system = 1
ON CONFLICT (celestial_id, defense_code) DO NOTHING;

-- 8. Arsenal: A owns an upgrade + un-activated drawings, B lists a lot ------
INSERT INTO account_upgrades (account_id, upgrade_code, level, value)
SELECT usr.id, a.code, a.lvl, a.val
FROM users usr
JOIN universes u ON u.id = usr.universe_id AND u.code_name = 'universe_test'
CROSS JOIN (VALUES (1, 2, 2.0::NUMERIC)) AS a(code, lvl, val)
WHERE usr.username = 'webtest_a'
ON CONFLICT (account_id, upgrade_code) DO NOTHING;

INSERT INTO upgrade_items (account_id, upgrade_code, qty)
SELECT usr.id, i.code, i.qty
FROM users usr
JOIN universes u ON u.id = usr.universe_id AND u.code_name = 'universe_test'
CROSS JOIN (VALUES (2, 3), (3, 2), (11, 1)) AS i(code, qty)
WHERE usr.username = 'webtest_a'
ON CONFLICT (account_id, upgrade_code) DO NOTHING;

INSERT INTO market_lots (seller_account_id, upgrade_code, amount, price_atm)
SELECT usr.id, 5, 2, 5000
FROM users usr
JOIN universes u ON u.id = usr.universe_id AND u.code_name = 'universe_test'
WHERE usr.username = 'webtest_b'
  AND NOT EXISTS (
      SELECT 1 FROM market_lots ml WHERE ml.seller_account_id = usr.id AND ml.upgrade_code = 5
  );

-- 9. Keep the celestial id sequence ahead of any explicit inserts -----------
SELECT setval(
    pg_get_serial_sequence('celestial_objects', 'id'),
    GREATEST((SELECT COALESCE(MAX(id), 1) FROM celestial_objects), 1)
);

-- 10. Report the seeded test homeworlds -------------------------------------
SELECT c.name, c.galaxy, c.system, c.position
FROM celestial_objects c
JOIN universes u ON u.id = c.universe_id AND u.code_name = 'universe_test'
ORDER BY c.position;

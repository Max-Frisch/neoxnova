-- ============================================================================
-- NEO-XNOVA LOCAL DEVELOPMENT SEED DATA
-- Idempotent: safe to re-run (all inserts use ON CONFLICT DO NOTHING).
--
-- Provisions a ready-to-play test universe matching the default UNIVERSE_ID
-- ('universe_6_niburu'), one commander, a resource-rich homeworld with hangar
-- ships, and basic structures. The homeworld is created with a fixed id of 1
-- so the standard demo URL works out of the box:
--     curl http://localhost:8080/api/v1/planets/1/overview
--     http://localhost:8080/dashboard/1
-- ============================================================================

-- 1. Universe (code_name must match UNIVERSE_ID in .env / Makefile) ----------
INSERT INTO universes (
    code_name, display_name, game_speed, resource_speed, fleet_speed,
    debris_rate, max_galaxies, base_colonies
) VALUES (
    'universe_6_niburu', 'Niburu Universe 6', 4.00, 10.00, 15.00,
    0.50, 1, 5
)
ON CONFLICT (code_name) DO NOTHING;

-- 2. Test commander ----------------------------------------------------------
-- Repair the users serial sequence first: prior ad-hoc test data may have been
-- inserted with explicit ids, leaving the sequence stale and causing pkey clashes.
SELECT setval(
    pg_get_serial_sequence('users', 'id'),
    GREATEST((SELECT COALESCE(MAX(id), 0) FROM users), 1)
);

INSERT INTO users (universe_id, username, email, password_hash, dark_matter)
SELECT id, 'commander', 'commander@neoxnova.local', 'seed-not-a-real-hash', 25000
FROM universes
WHERE code_name = 'universe_6_niburu'
ON CONFLICT (universe_id, username) DO NOTHING;

-- 3. Homeworld with starting resources (id = 1 for deterministic URLs) -------
INSERT INTO celestial_objects (
    id, universe_id, user_id, name, object_type,
    galaxy, system, position, diameter_km, fields_used, fields_max,
    temp_min, temp_max,
    metal, crystal, deuterium,
    metal_capacity, crystal_capacity, deuterium_capacity,
    metal_prod_hourly, crystal_prod_hourly, deuterium_prod_hourly,
    energy_used, energy_max
)
SELECT
    1, u.id, usr.id, 'Niburu Prime', 'PLANET',
    1, 1, 1, 12800, 42, 163,
    -20, 40,
    50000.0000, 50000.0000, 500000.0000,
    100000000, 100000000, 100000000,
    1800.0000, 1200.0000, 600.0000,
    0, 5000
FROM universes u
JOIN users usr ON usr.universe_id = u.id AND usr.username = 'commander'
WHERE u.code_name = 'universe_6_niburu'
ON CONFLICT DO NOTHING;

-- 3b. Secondary colony used as a fleet dispatch/transport target -------------
INSERT INTO celestial_objects (
    id, universe_id, user_id, name, object_type,
    galaxy, system, position, diameter_km, fields_used, fields_max,
    temp_min, temp_max,
    metal, crystal, deuterium,
    metal_capacity, crystal_capacity, deuterium_capacity,
    metal_prod_hourly, crystal_prod_hourly, deuterium_prod_hourly,
    energy_used, energy_max
)
SELECT
    2, u.id, usr.id, 'Niburu Outpost', 'PLANET',
    1, 2, 3, 9600, 5, 120,
    -15, 35,
    1000.0000, 1000.0000, 1000.0000,
    10000000, 10000000, 10000000,
    300.0000, 200.0000, 0.0000,
    0, 500
FROM universes u
JOIN users usr ON usr.universe_id = u.id AND usr.username = 'commander'
WHERE u.code_name = 'universe_6_niburu'
ON CONFLICT DO NOTHING;

-- 4. Hangar ships so fleet dispatch works immediately ------------------------
--    Codes per schema comments: 202 small cargo, 212 battleship,
--    217 battle transporter, 219 battle recycler.
INSERT INTO planet_ships (celestial_id, ship_code, quantity)
SELECT c.id, s.ship_code, s.quantity
FROM celestial_objects c
JOIN universes u ON u.id = c.universe_id AND u.code_name = 'universe_6_niburu'
CROSS JOIN (VALUES
    ('202', 500),   -- small cargo
    ('212', 200),   -- battleship
    ('217', 100),   -- battle transporter
    ('219', 50)     -- battle recycler
) AS s(ship_code, quantity)
WHERE c.galaxy = 1 AND c.system = 1 AND c.position = 1 AND c.object_type = 'PLANET'
ON CONFLICT (celestial_id, ship_code) DO NOTHING;

-- 5. Basic structures (optional flavor for the overview/dashboard) -----------
INSERT INTO planet_structures (celestial_id, structure_code, level)
SELECT c.id, s.structure_code, s.lvl
FROM celestial_objects c
JOIN universes u ON u.id = c.universe_id AND u.code_name = 'universe_6_niburu'
CROSS JOIN (VALUES
    ('metal_mine', 10),
    ('crystal_mine', 8),
    ('deuterium_synthesizer', 6),
    ('solar_plant', 12),
    ('robotics_factory', 2),
    ('shipyard', 4)
) AS s(structure_code, lvl)
WHERE c.galaxy = 1 AND c.system = 1 AND c.position = 1 AND c.object_type = 'PLANET'
ON CONFLICT (celestial_id, structure_code) DO NOTHING;

-- 6. Base technologies -------------------------------------------------------
INSERT INTO user_technologies (user_id, tech_code, level)
SELECT usr.id, t.tech_code, t.lvl
FROM users usr
JOIN universes u ON u.id = usr.universe_id AND u.code_name = 'universe_6_niburu'
CROSS JOIN (VALUES
    ('espionage_tech', 2),
    ('energy_tech', 3),
    ('combustion_drive', 4),
    ('astrophysics', 1)
) AS t(tech_code, lvl)
WHERE usr.username = 'commander'
ON CONFLICT (user_id, tech_code) DO NOTHING;

-- 7. Keep the BIGSERIAL sequence ahead of the explicit id = 1 insert ---------
SELECT setval(
    pg_get_serial_sequence('celestial_objects', 'id'),
    GREATEST((SELECT COALESCE(MAX(id), 1) FROM celestial_objects), 1)
);

-- 8. Report the seeded homeworld id for convenience --------------------------
SELECT id AS seeded_planet_id, name, galaxy, system, position
FROM celestial_objects
WHERE universe_id = (SELECT id FROM universes WHERE code_name = 'universe_6_niburu')
  AND galaxy = 1 AND system = 1 AND position = 1 AND object_type = 'PLANET';

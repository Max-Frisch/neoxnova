-- ============================================================================
-- OGAME / 2MOONS LEGACY ENGINE MODERNIZATION: POSTGRESQL DDL SPECIFICATION
-- Target RDBMS: PostgreSQL 16+
-- Architecture: Multi-Universe, Event-Driven, Atomic State Transitions
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "btree_gist";

-- ============================================================================
-- 1. ENUMS AND CUSTOM DOMAINS
-- ============================================================================

DO $$ BEGIN
    CREATE TYPE celestial_type AS ENUM ('PLANET', 'MOON', 'DEBRIS_FIELD', 'DEEP_SPACE');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE mission_type AS ENUM (
        'ATTACK',
        'ACS_ATTACK',
        'TRANSPORT',
        'DEPLOY',
        'HOLD',
        'ESPIONAGE',
        'COLONIZE',
        'RECYCLE',
        'DESTROY_MOON',
        'MISSILE_ATTACK',
        'EXPEDITION'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE fleet_phase AS ENUM (
        'OUTBOUND',     -- Traveling to target
        'HOLDING',      -- Stationed / Expedition duration / ACS wait
        'RETURNING',    -- Traveling back to origin
        'RESOLVED',     -- Completed & resources/ships deposited
        'CANCELLED'     -- Recalled early by player
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE queue_status AS ENUM ('PENDING', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Strict coordinate bounds (Galaxy 1-9, System 1-499, Position 1-21)
-- Position 21 is reserved for Deep Space Expedition slots observed in HAR
DO $$ BEGIN
    CREATE DOMAIN coord_galaxy AS SMALLINT CHECK (VALUE >= 1 AND VALUE <= 9);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE DOMAIN coord_system AS SMALLINT CHECK (VALUE >= 1 AND VALUE <= 499);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE DOMAIN coord_position AS SMALLINT CHECK (VALUE >= 1 AND VALUE <= 21);
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- ============================================================================
-- 2. UNIVERSES & GAME TICK RATE CONFIGURATION
-- ============================================================================

CREATE TABLE IF NOT EXISTS universes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code_name VARCHAR(64) UNIQUE NOT NULL,               -- e.g. 'universe_6_niburu'
    display_name VARCHAR(128) NOT NULL,
    game_speed NUMERIC(8,2) NOT NULL DEFAULT 1.00,        -- HAR observed: 4000.00
    resource_speed NUMERIC(8,2) NOT NULL DEFAULT 1.00,    -- HAR observed: 10000.00
    fleet_speed NUMERIC(8,2) NOT NULL DEFAULT 1.00,       -- HAR observed: 5.00
    debris_rate NUMERIC(4,2) NOT NULL DEFAULT 0.30,       -- HAR observed: 0.50 (50%)
    max_galaxies SMALLINT NOT NULL DEFAULT 1,
    max_systems SMALLINT NOT NULL DEFAULT 499,
    base_colonies SMALLINT NOT NULL DEFAULT 5,            -- HAR: 5 free colonies
    max_colonies_hardcap SMALLINT NOT NULL DEFAULT 35,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- 3. USERS, AUTHENTICATION & EMPIRE PROGRESSION
-- ============================================================================

CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    universe_id UUID NOT NULL REFERENCES universes(id) ON DELETE CASCADE,
    username VARCHAR(64) NOT NULL,
    email VARCHAR(255) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,                 -- Argon2id / bcrypt
    auth_role VARCHAR(32) NOT NULL DEFAULT 'PLAYER',     -- 'PLAYER', 'MODERATOR', 'ADMIN'
    
    -- Dual Level Progression Observed in HAR
    peaceful_level INT NOT NULL DEFAULT 0,
    peaceful_progress_pct NUMERIC(5,2) NOT NULL DEFAULT 0.00,
    peaceful_next_level_at TIMESTAMPTZ,
    
    combat_level INT NOT NULL DEFAULT 0,
    combat_xp BIGINT NOT NULL DEFAULT 0,
    combat_xp_needed BIGINT NOT NULL DEFAULT 10700,
    
    -- Premium Currencies (Global Empire-wide, Not tied to individual planets)
    dark_matter BIGINT NOT NULL DEFAULT 0 CHECK (dark_matter >= 0),
    antimatter BIGINT NOT NULL DEFAULT 0 CHECK (antimatter >= 0),
    
    vacation_until TIMESTAMPTZ,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    CONSTRAINT uq_universe_username UNIQUE (universe_id, username),
    CONSTRAINT uq_universe_email UNIQUE (universe_id, email)
);

CREATE INDEX IF NOT EXISTS idx_users_universe_login ON users(universe_id, last_login_at);

-- ============================================================================
-- 4. CELESTIAL OBJECTS (PLANETS, MOONS, DEBRIS)
-- ============================================================================

CREATE TABLE IF NOT EXISTS celestial_objects (
    id BIGSERIAL PRIMARY KEY,
    universe_id UUID NOT NULL REFERENCES universes(id) ON DELETE CASCADE,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    name VARCHAR(64) NOT NULL DEFAULT 'Colony',
    object_type celestial_type NOT NULL DEFAULT 'PLANET',
    
    -- Absolute Coordinate Vector
    galaxy coord_galaxy NOT NULL,
    system coord_system NOT NULL,
    position coord_position NOT NULL,
    
    diameter_km INT NOT NULL DEFAULT 12800 CHECK (diameter_km > 0),
    fields_used INT NOT NULL DEFAULT 0 CHECK (fields_used >= 0),
    fields_max INT NOT NULL DEFAULT 163 CHECK (fields_max >= fields_used),
    temp_min SMALLINT NOT NULL DEFAULT -20,
    temp_max SMALLINT NOT NULL DEFAULT 40,
    image_asset VARCHAR(128) NOT NULL DEFAULT 'normal_planet_01.jpg',
    
    -- Deterministic Continuous Resource Accumulator (Zero-Cron Model)
    metal NUMERIC(24, 4) NOT NULL DEFAULT 500.0000 CHECK (metal >= 0),
    crystal NUMERIC(24, 4) NOT NULL DEFAULT 500.0000 CHECK (crystal >= 0),
    deuterium NUMERIC(24, 4) NOT NULL DEFAULT 0.0000 CHECK (deuterium >= 0),
    
    metal_capacity BIGINT NOT NULL DEFAULT 100000 CHECK (metal_capacity >= 0),
    crystal_capacity BIGINT NOT NULL DEFAULT 100000 CHECK (crystal_capacity >= 0),
    deuterium_capacity BIGINT NOT NULL DEFAULT 100000 CHECK (deuterium_capacity >= 0),
    
    metal_prod_hourly NUMERIC(16, 4) NOT NULL DEFAULT 30.0000,
    crystal_prod_hourly NUMERIC(16, 4) NOT NULL DEFAULT 20.0000,
    deuterium_prod_hourly NUMERIC(16, 4) NOT NULL DEFAULT 0.0000,
    
    energy_used INT NOT NULL DEFAULT 0,
    energy_max INT NOT NULL DEFAULT 0,
    
    last_resource_calc_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    CONSTRAINT uq_universe_coords_type UNIQUE (universe_id, galaxy, system, position, object_type)
);

CREATE INDEX IF NOT EXISTS idx_celestials_lookup ON celestial_objects(universe_id, galaxy, system, position);
CREATE INDEX IF NOT EXISTS idx_celestials_user ON celestial_objects(user_id) WHERE user_id IS NOT NULL;

-- ============================================================================
-- 5. PLANET STRUCTURES & DEFENSES (NORMALIZED STORE)
-- ============================================================================

CREATE TABLE IF NOT EXISTS planet_structures (
    celestial_id BIGINT NOT NULL REFERENCES celestial_objects(id) ON DELETE CASCADE,
    structure_code VARCHAR(32) NOT NULL, -- 'metal_mine', 'crystal_mine', 'deuterium_synthesizer', 'solar_plant', 'robotics_factory', 'shipyard', 'research_lab', 'nanite_factory'
    level INT NOT NULL DEFAULT 0 CHECK (level >= 0),
    efficiency_pct NUMERIC(4,2) NOT NULL DEFAULT 1.00 CHECK (efficiency_pct BETWEEN 0.00 AND 1.00),
    PRIMARY KEY (celestial_id, structure_code)
);

CREATE TABLE IF NOT EXISTS planet_defenses (
    celestial_id BIGINT NOT NULL REFERENCES celestial_objects(id) ON DELETE CASCADE,
    defense_code VARCHAR(32) NOT NULL, -- e.g., '401' (rocket_launcher), '402' (light_laser)
    quantity BIGINT NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    PRIMARY KEY (celestial_id, defense_code)
);

CREATE TABLE IF NOT EXISTS planet_ships (
    celestial_id BIGINT NOT NULL REFERENCES celestial_objects(id) ON DELETE CASCADE,
    ship_code VARCHAR(32) NOT NULL, -- numeric unit id, e.g. '202' (Light Cargo), '207' (Battleship), '212' (Solar Satellite)
    quantity BIGINT NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    PRIMARY KEY (celestial_id, ship_code)
);

-- ============================================================================
-- 6. RESEARCH & TECHNOLOGIES (EMPIRE-WIDE)
-- ============================================================================

CREATE TABLE IF NOT EXISTS user_technologies (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tech_code VARCHAR(32) NOT NULL, -- 'espionage_tech', 'astrophysics', 'intergalactic_research_network', etc.
    level INT NOT NULL DEFAULT 0 CHECK (level >= 0),
    PRIMARY KEY (user_id, tech_code)
);

-- ============================================================================
-- 7. FLEET MISSIONS, PHASES & SHIP COMPOSITIONS (CRITICAL ENGINE CORE)
-- ============================================================================

CREATE TABLE IF NOT EXISTS fleets (
    id BIGSERIAL PRIMARY KEY,
    universe_id UUID NOT NULL REFERENCES universes(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mission mission_type NOT NULL,
    phase fleet_phase NOT NULL DEFAULT 'OUTBOUND',
    
    -- Origin & Target Relations
    origin_id BIGINT NOT NULL REFERENCES celestial_objects(id) ON DELETE RESTRICT,
    target_id BIGINT REFERENCES celestial_objects(id) ON DELETE RESTRICT,
    
    -- Snapshot Coordinates in case celestial object is altered during flight
    origin_galaxy coord_galaxy NOT NULL,
    origin_system coord_system NOT NULL,
    origin_position coord_position NOT NULL,
    origin_type celestial_type NOT NULL,
    
    target_galaxy coord_galaxy NOT NULL,
    target_system coord_system NOT NULL,
    target_position coord_position NOT NULL,
    target_type celestial_type NOT NULL,
    
    -- Time Bounds
    start_time TIMESTAMPTZ NOT NULL,
    arrival_time TIMESTAMPTZ NOT NULL,
    holding_end_time TIMESTAMPTZ,
    return_time TIMESTAMPTZ NOT NULL,
    
    -- Flight Metrics
    flight_speed_pct NUMERIC(3,2) NOT NULL DEFAULT 1.00 CHECK (flight_speed_pct BETWEEN 0.10 AND 1.00),
    deuterium_consumption BIGINT NOT NULL DEFAULT 0 CHECK (deuterium_consumption >= 0),
    fleet_points BIGINT NOT NULL DEFAULT 0 CHECK (fleet_points >= 0),
    
    -- Cargo Manifest
    cargo_metal BIGINT NOT NULL DEFAULT 0 CHECK (cargo_metal >= 0),
    cargo_crystal BIGINT NOT NULL DEFAULT 0 CHECK (cargo_crystal >= 0),
    cargo_deuterium BIGINT NOT NULL DEFAULT 0 CHECK (cargo_deuterium >= 0),
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    CONSTRAINT chk_fleet_timeline CHECK (
        start_time < arrival_time AND 
        arrival_time <= return_time AND
        (holding_end_time IS NULL OR (holding_end_time >= arrival_time AND holding_end_time <= return_time))
    )
);

-- PARTIAL INDEX: Only active flights are indexed! Resolved missions are skipped for ultra-fast event queries.
CREATE INDEX IF NOT EXISTS idx_active_fleets_arrival ON fleets(arrival_time) 
    WHERE phase IN ('OUTBOUND', 'HOLDING', 'RETURNING');

CREATE INDEX IF NOT EXISTS idx_active_fleets_user ON fleets(user_id) 
    WHERE phase IN ('OUTBOUND', 'HOLDING', 'RETURNING');

CREATE TABLE IF NOT EXISTS fleet_ships (
    fleet_id BIGINT NOT NULL REFERENCES fleets(id) ON DELETE CASCADE,
    ship_code VARCHAR(32) NOT NULL, -- numeric unit id, e.g. '202' (Light Cargo), '207' (Battleship), '219' (Battle Recycler)
    count BIGINT NOT NULL CHECK (count > 0),
    PRIMARY KEY (fleet_id, ship_code)
);

-- ============================================================================
-- 8. CONSTRUCTION & RESEARCH QUEUES
-- ============================================================================

CREATE TABLE IF NOT EXISTS construction_queues (
    id BIGSERIAL PRIMARY KEY,
    celestial_id BIGINT NOT NULL REFERENCES celestial_objects(id) ON DELETE CASCADE,
    structure_code VARCHAR(32) NOT NULL,
    target_level INT NOT NULL CHECK (target_level > 0),
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    status queue_status NOT NULL DEFAULT 'IN_PROGRESS',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_construction_timeline CHECK (start_time < end_time)
);

CREATE INDEX IF NOT EXISTS idx_active_construction ON construction_queues(end_time) 
    WHERE status = 'IN_PROGRESS';

CREATE TABLE IF NOT EXISTS research_queues (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    celestial_id BIGINT NOT NULL REFERENCES celestial_objects(id) ON DELETE CASCADE,
    tech_code VARCHAR(32) NOT NULL,
    target_level INT NOT NULL CHECK (target_level > 0),
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    status queue_status NOT NULL DEFAULT 'IN_PROGRESS',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_research_timeline CHECK (start_time < end_time)
);

CREATE INDEX IF NOT EXISTS idx_active_research ON research_queues(end_time) 
    WHERE status = 'IN_PROGRESS';

CREATE TABLE IF NOT EXISTS shipyard_queues (
    id BIGSERIAL PRIMARY KEY,
    celestial_id BIGINT NOT NULL REFERENCES celestial_objects(id) ON DELETE CASCADE,
    unit_code VARCHAR(32) NOT NULL, -- ship or defense code, e.g., '212', '401'
    quantity_total BIGINT NOT NULL CHECK (quantity_total > 0),
    quantity_completed BIGINT NOT NULL DEFAULT 0 CHECK (quantity_completed >= 0 AND quantity_completed <= quantity_total),
    build_time_per_unit NUMERIC(10, 2) NOT NULL CHECK (build_time_per_unit > 0),
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    status queue_status NOT NULL DEFAULT 'IN_PROGRESS',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_shipyard_timeline CHECK (start_time < end_time)
);

CREATE INDEX IF NOT EXISTS idx_active_shipyard ON shipyard_queues(end_time) 
    WHERE status = 'IN_PROGRESS';

-- ============================================================================
-- 9. CONTINUOUS RESOURCE ACCUMULATION STORED PROCEDURE (ZERO-CRON ENGINE)
-- ============================================================================

CREATE OR REPLACE FUNCTION update_celestial_resources(p_celestial_id BIGINT)
RETURNS TABLE (
    out_metal NUMERIC(24, 4),
    out_crystal NUMERIC(24, 4),
    out_deuterium NUMERIC(24, 4),
    out_last_calc TIMESTAMPTZ
) 
LANGUAGE plpgsql AS $$
DECLARE
    v_now TIMESTAMPTZ := NOW();
    v_last_calc TIMESTAMPTZ;
    v_elapsed_seconds NUMERIC;
    v_metal_prod NUMERIC;
    v_crystal_prod NUMERIC;
    v_deut_prod NUMERIC;
    v_metal_cap BIGINT;
    v_crystal_cap BIGINT;
    v_deut_cap BIGINT;
    v_curr_metal NUMERIC;
    v_curr_crystal NUMERIC;
    v_curr_deut NUMERIC;
BEGIN
    -- Acquire exclusive row-level lock to prevent concurrent checkout race conditions
    SELECT 
        last_resource_calc_at, metal, crystal, deuterium,
        metal_capacity, crystal_capacity, deuterium_capacity,
        metal_prod_hourly, crystal_prod_hourly, deuterium_prod_hourly
    INTO 
        v_last_calc, v_curr_metal, v_curr_crystal, v_curr_deut,
        v_metal_cap, v_crystal_cap, v_deut_cap,
        v_metal_prod, v_crystal_prod, v_deut_prod
    FROM celestial_objects
    WHERE id = p_celestial_id
    FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'Celestial object % not found', p_celestial_id;
    END IF;

    v_elapsed_seconds := EXTRACT(EPOCH FROM (v_now - v_last_calc));

    IF v_elapsed_seconds > 0 THEN
        v_curr_metal := LEAST(v_metal_cap, v_curr_metal + (v_metal_prod / 3600.0) * v_elapsed_seconds);
        v_curr_crystal := LEAST(v_crystal_cap, v_curr_crystal + (v_crystal_prod / 3600.0) * v_elapsed_seconds);
        v_curr_deut := LEAST(v_deut_cap, v_curr_deut + (v_deut_prod / 3600.0) * v_elapsed_seconds);

        UPDATE celestial_objects
        SET 
            metal = v_curr_metal,
            crystal = v_curr_crystal,
            deuterium = v_curr_deut,
            last_resource_calc_at = v_now
        WHERE id = p_celestial_id;
    END IF;

    RETURN QUERY SELECT v_curr_metal, v_curr_crystal, v_curr_deut, v_now;
END;
$$;
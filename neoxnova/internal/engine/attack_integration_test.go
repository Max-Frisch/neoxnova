package engine

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// TestResolveAttackEndToEnd exercises the persistence layer of an ATTACK
// resolution against a live Postgres (the docker-compose stack). It is skipped
// unless DATABASE_URL is set, so the normal `go test` run stays hermetic.
//
//	$env:DATABASE_URL = "postgres://postgres:password@localhost:5432/neoxnova?sslmode=disable"
//	go test ./internal/engine/ -run TestResolveAttackEndToEnd -v
func TestResolveAttackEndToEnd(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping DB integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("db not reachable: %v", err)
	}

	// Seeded layout: celestial 1 = home (attacker), celestial 2 = outpost.
	var universeID string
	if err := db.QueryRowContext(ctx, `SELECT universe_id::text FROM celestial_objects WHERE id = 1`).Scan(&universeID); err != nil {
		t.Skipf("seed missing celestial 1: %v", err)
	}

	// Fresh defenders on celestial 2: a small wall + a light-fighter screen.
	if _, err := db.ExecContext(ctx, `DELETE FROM planet_defenses WHERE celestial_id = 2`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM planet_ships WHERE celestial_id = 2`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO planet_defenses (celestial_id, defense_code, quantity) VALUES (2,'401',10),(2,'402',5)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO planet_ships (celestial_id, ship_code, quantity) VALUES (2,'204',50)`); err != nil {
		t.Fatal(err)
	}

	// A due ATTACK: 100 Battleships from celestial 1 -> celestial 2.
	var fleetID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO fleets (
			universe_id, user_id, mission, phase, origin_id, target_id,
			origin_galaxy, origin_system, origin_position, origin_type,
			target_galaxy, target_system, target_position, target_type,
			start_time, arrival_time, return_time, flight_speed_pct, deuterium_consumption,
			cargo_metal, cargo_crystal, cargo_deuterium
		) VALUES (
			$1, 2, 'ATTACK', 'OUTBOUND', 1, 2,
			1,1,1,'PLANET', 1,2,3,'PLANET',
			NOW() - interval '10 minutes', NOW() - interval '1 minute', NOW() + interval '5 minutes',
			1.00, 0, 0, 0, 0
		) RETURNING id
	`, universeID).Scan(&fleetID)
	if err != nil {
		t.Fatalf("insert fleet: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM fleets WHERE id = $1`, fleetID)
	if _, err := db.ExecContext(ctx, `INSERT INTO fleet_ships (fleet_id, ship_code, count) VALUES ($1,'207',100)`, fleetID); err != nil {
		t.Fatal(err)
	}

	eng := NewEventEngine(nil, db)
	eng.resolveFleetEvent(ctx, fleetID)

	// The 100 BS should have wiped the tiny defender.
	var defLeft int64
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(SUM(quantity),0) FROM planet_defenses WHERE celestial_id = 2`).Scan(&defLeft); err != nil {
		t.Fatal(err)
	}
	var shipLeft int64
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(SUM(quantity),0) FROM planet_ships WHERE celestial_id = 2`).Scan(&shipLeft); err != nil {
		t.Fatal(err)
	}
	// Ships are permanently lost; defenses are destroyed then ~61% repaired, so
	// a positive-but-smaller count remains.
	if shipLeft != 0 {
		t.Fatalf("expected defender ships wiped, got %d", shipLeft)
	}
	if defLeft <= 0 || defLeft >= 15 {
		t.Fatalf("expected partial defense repair (~61%% of 15), got %d", defLeft)
	}

	// A debris field must exist at 1:2:3 (ships-only debris).
	var debris int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM celestial_objects
		WHERE object_type = 'DEBRIS_FIELD' AND galaxy = 1 AND system = 2 AND position = 3
	`).Scan(&debris); err != nil {
		t.Fatal(err)
	}
	if debris == 0 {
		t.Fatal("expected a debris field at 1:2:3")
	}

	// The surviving fleet must be returning home.
	var phase string
	if err := db.QueryRowContext(ctx, `SELECT phase FROM fleets WHERE id = $1`, fleetID).Scan(&phase); err != nil {
		t.Fatal(err)
	}
	if phase != "RETURNING" {
		t.Fatalf("fleet phase = %q, want RETURNING", phase)
	}

	// A player-readable combat report must have been persisted.
	var reportResult string
	var reportRounds int
	if err := db.QueryRowContext(ctx, `
		SELECT result, rounds FROM combat_reports WHERE fleet_id = $1 ORDER BY id DESC LIMIT 1
	`, fleetID).Scan(&reportResult, &reportRounds); err != nil {
		t.Fatalf("expected a persisted combat report: %v", err)
	}
	if reportResult != "attacker" || reportRounds <= 0 {
		t.Fatalf("combat report = result %q rounds %d, want attacker / >0", reportResult, reportRounds)
	}
	t.Logf("attack resolved: fleet #%d RETURNING, defender wiped, debris + report (%s, %d rounds)", fleetID, reportResult, reportRounds)
}

// TestResolveColonizeEndToEnd verifies a COLONIZE fleet founds a new planet,
// consumes its Colony Ship and stations the rest of the fleet there.
func TestResolveColonizeEndToEnd(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping DB integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("db not reachable: %v", err)
	}

	var universeID string
	if err := db.QueryRowContext(ctx, `SELECT universe_id::text FROM celestial_objects WHERE id = 1`).Scan(&universeID); err != nil {
		t.Skipf("seed missing celestial 1: %v", err)
	}
	const colG, colS, colP = 1, 9, 8
	db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE universe_id=$1 AND galaxy=$2 AND system=$3 AND position=$4`, universeID, colG, colS, colP)

	var fleetID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO fleets (
			universe_id, user_id, mission, phase, origin_id, target_id,
			origin_galaxy, origin_system, origin_position, origin_type,
			target_galaxy, target_system, target_position, target_type,
			start_time, arrival_time, return_time, flight_speed_pct, deuterium_consumption,
			cargo_metal, cargo_crystal, cargo_deuterium
		) VALUES (
			$1, 2, 'COLONIZE', 'OUTBOUND', 1, NULL,
			1,1,1,'PLANET', $2,$3,$4,'DEEP_SPACE',
			NOW() - interval '10 minutes', NOW() - interval '1 minute', NOW() + interval '5 minutes',
			1.00, 0, 0, 0, 0
		) RETURNING id
	`, universeID, colG, colS, colP).Scan(&fleetID)
	if err != nil {
		t.Fatalf("insert colonize fleet: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM fleets WHERE id = $1`, fleetID)
	if _, err := db.ExecContext(ctx, `INSERT INTO fleet_ships (fleet_id, ship_code, count) VALUES ($1,'208',1),($1,'204',5)`, fleetID); err != nil {
		t.Fatal(err)
	}

	eng := NewEventEngine(nil, db)
	eng.resolveFleetEvent(ctx, fleetID)

	var newID int64
	var owner int64
	if err := db.QueryRowContext(ctx, `
		SELECT id, user_id FROM celestial_objects
		WHERE universe_id=$1 AND galaxy=$2 AND system=$3 AND position=$4 AND object_type='PLANET'
	`, universeID, colG, colS, colP).Scan(&newID, &owner); err != nil {
		t.Fatalf("expected a new colony at %d:%d:%d: %v", colG, colS, colP, err)
	}
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, newID)
	if owner != 2 {
		t.Fatalf("colony owner = %d, want 2", owner)
	}

	var stationed int64
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(quantity,0) FROM planet_ships WHERE celestial_id=$1 AND ship_code='204'`, newID).Scan(&stationed); err != nil {
		t.Fatal(err)
	}
	if stationed != 5 {
		t.Fatalf("stationed LF = %d, want 5", stationed)
	}

	var phase string
	if err := db.QueryRowContext(ctx, `SELECT phase FROM fleets WHERE id = $1`, fleetID).Scan(&phase); err != nil {
		t.Fatal(err)
	}
	if phase != "RESOLVED" {
		t.Fatalf("fleet phase = %q, want RESOLVED", phase)
	}
	t.Logf("colonize resolved: new planet #%d with 5 LF (colony ship consumed)", newID)
}

// TestResolveRecycleEndToEnd verifies a RECYCLE fleet hoovers a debris field
// into its cargo and heads home. Same DATABASE_URL gate as above.
func TestResolveRecycleEndToEnd(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping DB integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("db not reachable: %v", err)
	}

	var universeID string
	if err := db.QueryRowContext(ctx, `SELECT universe_id::text FROM celestial_objects WHERE id = 1`).Scan(&universeID); err != nil {
		t.Skipf("seed missing celestial 1: %v", err)
	}

	// Upsert a known debris field at 1:3:3 and find its id.
	var fieldID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO celestial_objects (universe_id, name, object_type, galaxy, system, position, metal, crystal)
		VALUES ($1, 'Debris Field', 'DEBRIS_FIELD', 1, 3, 3, 100000, 200000)
		ON CONFLICT (universe_id, galaxy, system, position, object_type)
		DO UPDATE SET metal = 100000, crystal = 200000
		RETURNING id
	`, universeID).Scan(&fieldID)
	if err != nil {
		t.Fatalf("upsert debris field: %v", err)
	}

	var fleetID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO fleets (
			universe_id, user_id, mission, phase, origin_id, target_id,
			origin_galaxy, origin_system, origin_position, origin_type,
			target_galaxy, target_system, target_position, target_type,
			start_time, arrival_time, return_time, flight_speed_pct, deuterium_consumption,
			cargo_metal, cargo_crystal, cargo_deuterium
		) VALUES (
			$1, 2, 'RECYCLE', 'OUTBOUND', 1, $2,
			1,1,1,'PLANET', 1,3,3,'DEBRIS_FIELD',
			NOW() - interval '10 minutes', NOW() - interval '1 minute', NOW() + interval '5 minutes',
			1.00, 0, 0, 0, 0
		) RETURNING id
	`, universeID, fieldID).Scan(&fleetID)
	if err != nil {
		t.Fatalf("insert recycle fleet: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM fleets WHERE id = $1`, fleetID)
	if _, err := db.ExecContext(ctx, `INSERT INTO fleet_ships (fleet_id, ship_code, count) VALUES ($1,'219',1)`, fleetID); err != nil {
		t.Fatal(err)
	}

	eng := NewEventEngine(nil, db)
	eng.resolveFleetEvent(ctx, fleetID)

	var metal, crystal int64
	if err := db.QueryRowContext(ctx, `SELECT metal::bigint, crystal::bigint FROM celestial_objects WHERE id = $1`, fieldID).Scan(&metal, &crystal); err != nil {
		t.Fatal(err)
	}
	if metal != 0 || crystal != 0 {
		t.Fatalf("debris not fully collected: M%d C%d", metal, crystal)
	}

	var phase string
	var cargoM, cargoC int64
	if err := db.QueryRowContext(ctx, `SELECT phase, cargo_metal, cargo_crystal FROM fleets WHERE id = $1`, fleetID).Scan(&phase, &cargoM, &cargoC); err != nil {
		t.Fatal(err)
	}
	if phase != "RETURNING" || cargoM != 100000 || cargoC != 200000 {
		t.Fatalf("recycle fleet = phase %q cargo M%d C%d, want RETURNING 100000/200000", phase, cargoM, cargoC)
	}
	t.Logf("recycle resolved: fleet #%d returned with M%d C%d", fleetID, cargoM, cargoC)
}

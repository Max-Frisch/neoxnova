package engine

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"neoxnova/internal/game"
)

// TestDestroyMoonRollRules locks the pure roll's guard rails: no Battle
// Fortresses and an indestructible (>= 10,000 km) moon can never be destroyed.
func TestDestroyMoonRollRules(t *testing.T) {
	if destroyMoonRoll(3605, 0, 0, 1) {
		t.Fatal("no Battle Fortresses must never destroy a moon")
	}
	if destroyMoonRoll(game.MoonIndestructibleKm, 0, 100000, 1) {
		t.Fatal("a moon >= MoonIndestructibleKm must be immune")
	}
	// A tiny moon with plenty of Battle Fortresses caps at 100% and always dies.
	if !destroyMoonRoll(3605, 0, 100000, 42) {
		t.Fatal("a guaranteed (>=100%) roll must destroy the moon")
	}
}

// TestAttackMoonEndToEnd verifies that a plain ATTACK against a MOON behaves
// like one against a planet: the stationed ships/defenses are fought, a debris
// field appears at the moon's coordinates and no new moon is spawned.
func TestAttackMoonEndToEnd(t *testing.T) {
	db := moonTestDB(t)
	ctx := context.Background()
	universeID := moonTestUniverse(t, ctx, db)

	moonID := insertTestMoon(t, ctx, db, universeID, "AttackMoon", 1, 7, 7, 8426)
	if _, err := db.ExecContext(ctx, `INSERT INTO planet_ships (celestial_id, ship_code, quantity) VALUES ($1,'204',5)`, moonID); err != nil {
		t.Fatal(err)
	}

	fleetID := insertMoonFleet(t, ctx, db, universeID, "ATTACK", moonID, 1, 7, 7, map[string]int64{"207": 100})
	eng := NewEventEngine(nil, db)
	eng.resolveFleetEvent(ctx, fleetID)

	var shipsLeft int64
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(SUM(quantity),0) FROM planet_ships WHERE celestial_id=$1`, moonID).Scan(&shipsLeft); err != nil {
		t.Fatal(err)
	}
	if shipsLeft != 0 {
		t.Fatalf("moon defender ships left = %d, want 0", shipsLeft)
	}
	var stillThere int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM celestial_objects WHERE id=$1 AND object_type='MOON'`, moonID).Scan(&stillThere); err != nil {
		t.Fatal(err)
	}
	if stillThere != 1 {
		t.Fatal("an ATTACK must not destroy the moon")
	}
	var debris int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM celestial_objects
		WHERE object_type='DEBRIS_FIELD' AND galaxy=1 AND system=7 AND position=7
	`).Scan(&debris); err != nil {
		t.Fatal(err)
	}
	if debris == 0 {
		t.Fatal("expected a debris field at the moon's coordinates")
	}
}

// TestDestroyMoonEndToEnd verifies the DESTROY_MOON mission: after a won battle
// and a successful roll the moon (and everything on it) is removed.
func TestDestroyMoonEndToEnd(t *testing.T) {
	db := moonTestDB(t)
	ctx := context.Background()
	universeID := moonTestUniverse(t, ctx, db)

	orig := destroyMoonRoll
	destroyMoonRoll = func(int, int, int64, int64) bool { return true }
	defer func() { destroyMoonRoll = orig }()

	moonID := insertTestMoon(t, ctx, db, universeID, "DoomedMoon", 1, 8, 8, 3605)
	if _, err := db.ExecContext(ctx, `INSERT INTO planet_ships (celestial_id, ship_code, quantity) VALUES ($1,'204',2)`, moonID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_structures (celestial_id, structure_code, level) VALUES ($1,'moon_base',3)
	`, moonID); err != nil {
		t.Fatal(err)
	}

	fleetID := insertMoonFleet(t, ctx, db, universeID, "DESTROY_MOON", moonID, 1, 8, 8, map[string]int64{"214": 50})
	eng := NewEventEngine(nil, db)
	eng.resolveFleetEvent(ctx, fleetID)

	var left int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM celestial_objects WHERE id=$1`, moonID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatal("a successful DESTROY_MOON roll must remove the moon")
	}
	// Its structures cascade away with the celestial.
	var structs int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM planet_structures WHERE celestial_id=$1`, moonID).Scan(&structs); err != nil {
		t.Fatal(err)
	}
	if structs != 0 {
		t.Fatalf("moon structures left = %d, want 0", structs)
	}
}

// ---- helpers -------------------------------------------------------------

func moonTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping DB integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	return db
}

func moonTestUniverse(t *testing.T, ctx context.Context, db *sql.DB) string {
	t.Helper()
	var universeID string
	if err := db.QueryRowContext(ctx, `SELECT universe_id::text FROM celestial_objects WHERE id = 1`).Scan(&universeID); err != nil {
		t.Skipf("seed missing celestial 1: %v", err)
	}
	return universeID
}

func insertTestMoon(t *testing.T, ctx context.Context, db *sql.DB, universeID, name string, g, s, p, diameter int) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(ctx, `
		INSERT INTO celestial_objects
			(universe_id, name, object_type, galaxy, system, position,
			 diameter_km, fields_used, fields_max, base_fields_max, temp_min, temp_max)
		VALUES ($1, $2, 'MOON', $3, $4, $5, $6, 0, 0, 0, -20, 40)
		RETURNING id
	`, universeID, name, g, s, p, diameter).Scan(&id)
	if err != nil {
		t.Fatalf("insert moon: %v", err)
	}
	t.Cleanup(func() { db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, id) })
	return id
}

func insertMoonFleet(t *testing.T, ctx context.Context, db *sql.DB, universeID, mission string, targetID int64, g, s, p int, ships map[string]int64) int64 {
	t.Helper()
	var fleetID int64
	err := db.QueryRowContext(ctx, `
		INSERT INTO fleets (
			universe_id, user_id, mission, phase, origin_id, target_id,
			origin_galaxy, origin_system, origin_position, origin_type,
			target_galaxy, target_system, target_position, target_type,
			start_time, arrival_time, return_time, flight_speed_pct, deuterium_consumption
		) VALUES (
			$1, 2, $2, 'OUTBOUND', 1, $3,
			1,1,1,'PLANET', $4,$5,$6,'MOON',
			NOW() - interval '10 minutes', NOW() - interval '1 minute', NOW() + interval '5 minutes', 1.00, 0
		) RETURNING id
	`, universeID, mission, targetID, g, s, p).Scan(&fleetID)
	if err != nil {
		t.Fatalf("insert fleet: %v", err)
	}
	t.Cleanup(func() { db.ExecContext(ctx, `DELETE FROM fleets WHERE id = $1`, fleetID) })
	for code, count := range ships {
		if _, err := db.ExecContext(ctx, `INSERT INTO fleet_ships (fleet_id, ship_code, count) VALUES ($1,$2,$3)`, fleetID, code, count); err != nil {
			t.Fatal(err)
		}
	}
	return fleetID
}

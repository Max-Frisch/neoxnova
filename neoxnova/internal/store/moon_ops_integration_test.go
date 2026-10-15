package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"neoxnova/internal/auth"
	"neoxnova/internal/models"
)

// withTx runs fn inside a transaction and commits it, so RecomputeCelestial's
// *sql.Tx signature is exercised while its writes stay visible to later reads.
func withTx(t *testing.T, db *sql.DB, fn func(tx *sql.Tx) error) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		t.Fatalf("tx: %T %+v", err, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func coord(g, s, p int) models.Coordinates {
	return models.Coordinates{Galaxy: g, System: s, Position: p, Type: models.TypePlanet}
}

// TestMoonOpsIntegration exercises the moon-only store paths against a live
// Postgres: moon building legality + fields, the Jumpgate (ships-only) and the
// Phalanx Sensor (offline/range). Skipped unless DATABASE_URL is set.
func TestMoonOpsIntegration(t *testing.T) {
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
	var codeName, universeID string
	if err := db.QueryRowContext(ctx, `SELECT code_name, id::text FROM universes LIMIT 1`).Scan(&codeName, &universeID); err != nil {
		t.Skipf("no universe seeded: %v", err)
	}

	as := NewAuthStore(db)
	uname := fmt.Sprintf("moontest_%d", time.Now().UnixNano())
	hash, err := auth.HashPassword("s3cret-password!")
	if err != nil {
		t.Fatal(err)
	}
	userID, planetID, err := as.CreateUser(ctx, codeName, uname, uname+"@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)

	moons := NewMoonStore(db)
	bs := NewBuildStore(db)

	// Moon #1 at the planet's coordinates (as combat would create it).
	var moon1 int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO celestial_objects
			(universe_id, user_id, name, object_type, galaxy, system, position,
			 diameter_km, fields_used, fields_max, base_fields_max, temp_min, temp_max)
		VALUES ($1, $2, 'Moon 1', 'MOON', 2, 50, 5, 8426, 0, 0, 0, -20, 40)
		RETURNING id
	`, universeID, userID).Scan(&moon1)
	if err != nil {
		t.Fatalf("insert moon1: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, moon1)

	// Moon #2 elsewhere (a jump destination).
	var moon2 int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO celestial_objects
			(universe_id, user_id, name, object_type, galaxy, system, position,
			 diameter_km, fields_used, fields_max, base_fields_max, temp_min, temp_max)
		VALUES ($1, $2, 'Moon 2', 'MOON', 2, 60, 7, 8544, 0, 0, 0, -20, 40)
		RETURNING id
	`, universeID, userID).Scan(&moon2)
	if err != nil {
		t.Fatalf("insert moon2: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, moon2)

	// A fresh moon's fields come from Moon base (1 at level 0), not 163.
	withTx(t, db, func(tx *sql.Tx) error { return bs.RecomputeCelestial(ctx, tx, moon1) })
	var fieldsMax, fieldsUsed int
	if err := db.QueryRowContext(ctx, `SELECT fields_max, fields_used FROM celestial_objects WHERE id=$1`, moon1).Scan(&fieldsMax, &fieldsUsed); err != nil {
		t.Fatal(err)
	}
	if fieldsMax != 1 || fieldsUsed != 0 {
		t.Fatalf("fresh moon fields = %d/%d, want 0/1", fieldsUsed, fieldsMax)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_structures (celestial_id, structure_code, level) VALUES ($1,'moon_base',5)
		ON CONFLICT (celestial_id, structure_code) DO UPDATE SET level = EXCLUDED.level
	`, moon1); err != nil {
		t.Fatalf("insert struct: %v", err)
	}
	withTx(t, db, func(tx *sql.Tx) error { return bs.RecomputeCelestial(ctx, tx, moon1) })
	if err := db.QueryRowContext(ctx, `SELECT fields_max, fields_used FROM celestial_objects WHERE id=$1`, moon1).Scan(&fieldsMax, &fieldsUsed); err != nil {
		t.Fatal(err)
	}
	if fieldsMax != 1+3*5 || fieldsUsed != 5 {
		t.Fatalf("moon base 5 fields = %d/%d, want 5/16", fieldsUsed, fieldsMax)
	}

	// Building legality: a moon rejects planet-only buildings.
	if _, err := bs.EnqueueStructure(ctx, moon1, "metal_mine"); !errors.Is(err, ErrUnknownCode) {
		t.Fatalf("metal_mine on moon = %v, want ErrUnknownCode", err)
	}
	// ... and accepts moon-only ones once funded and unlocked.
	if _, err := db.ExecContext(ctx, `UPDATE celestial_objects SET metal=1e9, crystal=1e9, deuterium=1e9 WHERE id=$1`, moon1); err != nil {
		t.Fatal(err)
	}
	if _, err := bs.EnqueueStructure(ctx, moon1, "phalanx_sensor"); err != nil {
		t.Fatalf("phalanx_sensor on moon: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM construction_queues WHERE celestial_id=$1`, moon1); err != nil {
		t.Fatal(err)
	}

	// Jumpgate: both moons need a gate; ships only, resources never move.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_structures (celestial_id, structure_code, level) VALUES
			($1,'shipyard',1), ($1,'jumpgate',1), ($1,'phalanx_sensor',2),
			($2,'jumpgate',1)
		ON CONFLICT (celestial_id, structure_code) DO UPDATE SET level = EXCLUDED.level
	`, moon1, moon2); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_ships (celestial_id, ship_code, quantity) VALUES ($1,'204',10), ($1,'202',5)
		ON CONFLICT (celestial_id, ship_code) DO UPDATE SET quantity = EXCLUDED.quantity
	`, moon1); err != nil {
		t.Fatal(err)
	}
	// Give moon1 resources to prove they do NOT travel with the ships.
	if _, err := db.ExecContext(ctx, `UPDATE celestial_objects SET deuterium=777 WHERE id=$1`, moon1); err != nil {
		t.Fatal(err)
	}

	if err := moons.JumpFleet(ctx, userID, moon1, moon2, map[string]int64{"204": 3, "202": 5}); err != nil {
		t.Fatalf("JumpFleet: %v", err)
	}
	var m1LF, m2LF, m2LC, m1Deut, m2Deut float64
	db.QueryRowContext(ctx, `SELECT quantity FROM planet_ships WHERE celestial_id=$1 AND ship_code='204'`, moon1).Scan(&m1LF)
	db.QueryRowContext(ctx, `SELECT quantity FROM planet_ships WHERE celestial_id=$1 AND ship_code='204'`, moon2).Scan(&m2LF)
	db.QueryRowContext(ctx, `SELECT quantity FROM planet_ships WHERE celestial_id=$1 AND ship_code='202'`, moon2).Scan(&m2LC)
	db.QueryRowContext(ctx, `SELECT deuterium FROM celestial_objects WHERE id=$1`, moon1).Scan(&m1Deut)
	db.QueryRowContext(ctx, `SELECT deuterium FROM celestial_objects WHERE id=$1`, moon2).Scan(&m2Deut)
	if m1LF != 7 || m2LF != 3 || m2LC != 5 {
		t.Fatalf("after jump: moon1 LF=%v moon2 LF=%v LC=%v, want 7/3/5", m1LF, m2LF, m2LC)
	}
	if m1Deut != 777 || m2Deut != 0 {
		t.Fatalf("resources must not jump: moon1 deut=%v moon2 deut=%v", m1Deut, m2Deut)
	}
	var readyAt sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT jumpgate_next_at FROM celestial_objects WHERE id=$1`, moon1).Scan(&readyAt); err != nil {
		t.Fatal(err)
	}
	if !readyAt.Valid || !readyAt.Time.After(time.Now()) {
		t.Fatalf("source gate must be on cooldown, got %v", readyAt)
	}
	if err := moons.JumpFleet(ctx, userID, moon1, moon2, map[string]int64{"204": 1}); !errors.Is(err, ErrJumpgateCooldown) {
		t.Fatalf("second jump = %v, want ErrJumpgateCooldown", err)
	}

	// Phalanx: an inbound fleet to a position in range is revealed.
	var fleetID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO fleets (
			universe_id, user_id, mission, phase, origin_id, target_id,
			origin_galaxy, origin_system, origin_position, origin_type,
			target_galaxy, target_system, target_position, target_type,
			start_time, arrival_time, return_time, flight_speed_pct, deuterium_consumption
		) VALUES (
			$1, $2, 'ATTACK', 'OUTBOUND', $3, NULL,
			2,60,7,'MOON', 2,50,9,'PLANET',
			NOW(), NOW() + interval '10 minutes', NOW() + interval '20 minutes', 1.00, 0
		) RETURNING id
	`, universeID, userID, moon1).Scan(&fleetID)
	if err != nil {
		t.Fatalf("insert scan fleet: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM fleets WHERE id=$1`, fleetID)

	target := coord(2, 50, 9)
	got, err := moons.PhalanxScan(ctx, userID, moon1, target)
	if err != nil {
		t.Fatalf("PhalanxScan: %v", err)
	}
	if len(got) != 1 || got[0].FleetID != fleetID {
		t.Fatalf("phalanx fleets = %+v, want fleet %d", got, fleetID)
	}
	// Out of range: another galaxy.
	if _, err := moons.PhalanxScan(ctx, userID, moon1, coord(3, 50, 9)); !errors.Is(err, ErrPhalanxOutOfRange) {
		t.Fatalf("cross-galaxy scan = %v, want ErrPhalanxOutOfRange", err)
	}
	// Offline after a teleport.
	if _, err := db.ExecContext(ctx, `UPDATE celestial_objects SET phalanx_offline_until = NOW() + interval '5 minutes' WHERE id=$1`, moon1); err != nil {
		t.Fatal(err)
	}
	if _, err := moons.PhalanxScan(ctx, userID, moon1, target); !errors.Is(err, ErrPhalanxOffline) {
		t.Fatalf("offline scan = %v, want ErrPhalanxOffline", err)
	}
}

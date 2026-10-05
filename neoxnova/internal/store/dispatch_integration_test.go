package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"neoxnova/internal/models"
)

// TestDispatchNoobProtection verifies the attack gate on the Dispatch path
// against a live Postgres. Skipped unless DATABASE_URL is set.
//
//	$env:DATABASE_URL = "postgres://postgres:password@localhost:5432/neoxnova?sslmode=disable"
//	go test ./internal/store/ -run TestDispatchNoobProtection -v
func TestDispatchNoobProtection(t *testing.T) {
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

	// A brand-new, essentially scoreless defender at 1:9:9.
	var weakUser int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO users (universe_id, username, email, password_hash)
		VALUES ($1, 'noob_gate_test', 'noob_gate_test@example.com', 'x')
		ON CONFLICT (universe_id, username) DO UPDATE SET email = EXCLUDED.email
		RETURNING id
	`, universeID).Scan(&weakUser); err != nil {
		t.Fatalf("upsert weak user: %v", err)
	}
	var weakCel int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO celestial_objects (universe_id, name, object_type, galaxy, system, position, user_id)
		VALUES ($1, 'Weak Colony', 'PLANET', 1, 9, 9, $2)
		ON CONFLICT (universe_id, galaxy, system, position, object_type) DO UPDATE SET user_id = EXCLUDED.user_id
		RETURNING id
	`, universeID, weakUser).Scan(&weakCel); err != nil {
		t.Fatalf("upsert weak celestial: %v", err)
	}
	_ = weakCel

	fs := NewFleetStore(db)
	atk := models.FleetDispatchRequest{
		OriginPlanetID: 1,
		Mission:        models.MissionAttack,
		Ships:          map[string]int64{"207": 1},
		SpeedPercent:   100,
	}

	// Attacking a ~0-point player while holding assets must be blocked.
	atk.Target = models.Coordinates{Galaxy: 1, System: 9, Position: 9, Type: models.TypePlanet}
	if _, err := fs.Dispatch(ctx, atk); !errors.Is(err, ErrNoobProtection) {
		t.Fatalf("expected ErrNoobProtection vs scoreless defender, got %v", err)
	}

	// Attacking your own planet must skip the gate entirely.
	atk.Target = models.Coordinates{Galaxy: 1, System: 2, Position: 3, Type: models.TypePlanet}
	res, err := fs.Dispatch(ctx, atk)
	if errors.Is(err, ErrNoobProtection) {
		t.Fatalf("same-owner attack should skip noob protection, got %v", err)
	}
	if err == nil && res.FleetID > 0 {
		defer db.ExecContext(ctx, `DELETE FROM fleets WHERE id = $1`, res.FleetID)
		t.Logf("same-owner dispatch allowed (fleet #%d)", res.FleetID)
	} else if err != nil {
		t.Logf("same-owner dispatch returned a non-gate error (acceptable): %v", err)
	}
}

// TestAbandonPlanet verifies the abandon rules and slot locking.
func TestAbandonPlanet(t *testing.T) {
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

	// A throwaway third planet for the commander (user 2) at 1:5:5.
	var pid int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO celestial_objects (universe_id, user_id, name, object_type, galaxy, system, position, base_fields_max)
		VALUES ($1, 2, 'Temp', 'PLANET', 1, 5, 5, 300)
		ON CONFLICT (universe_id, galaxy, system, position, object_type) DO UPDATE SET user_id = 2
		RETURNING id
	`, universeID).Scan(&pid); err != nil {
		t.Fatalf("insert temp planet: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, pid)
	defer db.ExecContext(ctx, `DELETE FROM coordinate_locks WHERE universe_id=$1 AND galaxy=1 AND system=5 AND position=5`, universeID)

	ps := NewPlanetStore(db)
	if err := ps.AbandonPlanet(ctx, pid, 2); err != nil {
		t.Fatalf("abandon: %v", err)
	}
	var gone int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM celestial_objects WHERE id=$1`, pid).Scan(&gone); err != nil || gone != 0 {
		t.Fatalf("planet not deleted (count=%d, err=%v)", gone, err)
	}
	var locks int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM coordinate_locks
		WHERE universe_id=$1 AND galaxy=1 AND system=5 AND position=5 AND locked_until > NOW()
	`, universeID).Scan(&locks); err != nil || locks != 1 {
		t.Fatalf("expected a slot lock (count=%d, err=%v)", locks, err)
	}

	// The homeworld cannot be abandoned.
	if err := ps.AbandonPlanet(ctx, 1, 2); !errors.Is(err, ErrCannotAbandonHome) {
		t.Fatalf("homeworld abandon = %v, want ErrCannotAbandonHome", err)
	}
}

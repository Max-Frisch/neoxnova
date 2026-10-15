package store

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"neoxnova/internal/models"
)

// TestDispatchExpeditionHoldMinutes verifies that a sub-hour hold expressed in
// minutes (the expedition panel's exp_time slider) is honoured and takes
// precedence over HoldingHours. Skipped unless DATABASE_URL is set.
//
//	$env:DATABASE_URL = "postgres://postgres:password@localhost:5432/neoxnova?sslmode=disable"
//	go test ./internal/store/ -run TestDispatchExpeditionHoldMinutes -v
func TestDispatchExpeditionHoldMinutes(t *testing.T) {
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
	var planetsPerSystem int
	if err := db.QueryRowContext(ctx, `
		SELECT u.id::text, u.planets_per_system
		FROM universes u
		JOIN celestial_objects c ON c.universe_id = u.id
		WHERE c.id = 1
	`).Scan(&universeID, &planetsPerSystem); err != nil {
		t.Skipf("seed missing celestial 1: %v", err)
	}

	var uid int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO users (universe_id, username, email, password_hash)
		VALUES ($1, 'expo_hold_test', 'expo_hold_test@example.com', 'x')
		ON CONFLICT (universe_id, username) DO UPDATE SET email = EXCLUDED.email
		RETURNING id
	`, universeID).Scan(&uid); err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM fleets WHERE user_id = $1`, uid)
	defer db.ExecContext(ctx, `DELETE FROM user_technologies WHERE user_id = $1`, uid)
	defer db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, uid)

	var pid int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO celestial_objects (universe_id, user_id, name, object_type, galaxy, system, position, deuterium)
		VALUES ($1, $2, 'Expo Hold Colony', 'PLANET', 1, 17, 17, 1e15)
		ON CONFLICT (universe_id, galaxy, system, position, object_type)
			DO UPDATE SET user_id = EXCLUDED.user_id, deuterium = EXCLUDED.deuterium
		RETURNING id
	`, universeID, uid).Scan(&pid); err != nil {
		t.Fatalf("upsert planet: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM planet_ships WHERE celestial_id = $1`, pid)
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, pid)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO user_technologies (user_id, tech_code, level) VALUES ($1, 'astrophysics', 4)
		ON CONFLICT (user_id, tech_code) DO UPDATE SET level = EXCLUDED.level
	`, uid); err != nil {
		t.Fatalf("set astrophysics: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_ships (celestial_id, ship_code, quantity) VALUES ($1, '202', 100)
		ON CONFLICT (celestial_id, ship_code) DO UPDATE SET quantity = EXCLUDED.quantity
	`, pid); err != nil {
		t.Fatalf("seed ships: %v", err)
	}

	fs := NewFleetStore(db)
	res, err := fs.Dispatch(ctx, models.FleetDispatchRequest{
		OriginPlanetID: pid,
		Target: models.Coordinates{
			Galaxy: 1, System: 17, Position: planetsPerSystem + 1, Type: models.TypeDeepSpace,
		},
		Mission:        models.MissionExpedition,
		Ships:          map[string]int64{"202": 1},
		SpeedPercent:   100,
		HoldingHours:   2,  // ignored when minutes are set
		HoldingMinutes: 15, // 0.25 h
	})
	if err != nil {
		t.Fatalf("dispatch expedition: %v", err)
	}

	var holdMin float64
	if err := db.QueryRowContext(ctx, `
		SELECT EXTRACT(EPOCH FROM (holding_end_time - arrival_time)) / 60
		FROM fleets WHERE id = $1
	`, res.FleetID).Scan(&holdMin); err != nil {
		t.Fatalf("read holding window: %v", err)
	}
	if holdMin != 15 {
		t.Fatalf("hold = %.1f min, want 15 (minutes take precedence over hours)", holdMin)
	}
}

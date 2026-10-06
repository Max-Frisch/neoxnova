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

// TestAttackLockout verifies that a recently teleported planet may not launch
// ATTACK missions for the lockout window, while other missions still work.
// Skipped unless DATABASE_URL is set.
func TestAttackLockout(t *testing.T) {
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

	var owner int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO users (universe_id, username, email, password_hash)
		VALUES ($1, 'atklock_owner', 'atklock_owner@example.com', 'x')
		ON CONFLICT (universe_id, username) DO UPDATE SET email = EXCLUDED.email
		RETURNING id
	`, universeID).Scan(&owner); err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	insertPlanet := func(name string, g, s, p int, locked bool) int64 {
		var id int64
		lock := "NULL"
		if locked {
			lock = "NOW() + interval '15 minutes'"
		}
		q := `INSERT INTO celestial_objects (universe_id, user_id, name, object_type, galaxy, system, position, base_fields_max, fields_max, deuterium, attack_locked_until)
		      VALUES ($1, $2, $3, 'PLANET', $4, $5, $6, 200, 200, 1000000000000, ` + lock + `)
		      ON CONFLICT (universe_id, galaxy, system, position, object_type) DO UPDATE
		          SET user_id = EXCLUDED.user_id, deuterium = EXCLUDED.deuterium, attack_locked_until = EXCLUDED.attack_locked_until
		      RETURNING id`
		if err := db.QueryRowContext(ctx, q, universeID, owner, name, g, s, p).Scan(&id); err != nil {
			t.Fatalf("insert planet %s: %v", name, err)
		}
		return id
	}
	origin := insertPlanet("LockedOrigin", 9, 411, 5, true)
	target := insertPlanet("OwnTarget", 9, 411, 6, false)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_ships (celestial_id, ship_code, quantity) VALUES ($1, '207', 10)
		ON CONFLICT (celestial_id, ship_code) DO UPDATE SET quantity = 10
	`, origin); err != nil {
		t.Fatalf("insert ships: %v", err)
	}

	defer func() {
		db.ExecContext(ctx, `DELETE FROM fleets WHERE origin_id = $1 OR target_id = $1`, origin)
		db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id IN ($1, $2)`, origin, target)
		db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, owner)
	}()

	coord := models.Coordinates{Galaxy: 9, System: 411, Position: 6, Type: models.TypePlanet}
	fs := NewFleetStore(db)

	// ATTACK is blocked while the lockout is active.
	atk := models.FleetDispatchRequest{
		OriginPlanetID: origin, Target: coord, Mission: models.MissionAttack,
		Ships: map[string]int64{"207": 1}, SpeedPercent: 100,
	}
	if _, err := fs.Dispatch(ctx, atk); !errors.Is(err, ErrAttackLocked) {
		t.Fatalf("locked ATTACK = %v, want ErrAttackLocked", err)
	}

	// Non-attack missions are unaffected.
	tr := atk
	tr.Mission = models.MissionTransport
	if _, err := fs.Dispatch(ctx, tr); err != nil {
		t.Fatalf("locked TRANSPORT = %v, want success", err)
	}

	// Once the lockout expires, ATTACK is allowed again.
	if _, err := db.ExecContext(ctx, `
		UPDATE celestial_objects SET attack_locked_until = NOW() - interval '1 minute' WHERE id = $1
	`, origin); err != nil {
		t.Fatalf("expire lockout: %v", err)
	}
	if _, err := fs.Dispatch(ctx, atk); err != nil {
		t.Fatalf("post-lockout ATTACK = %v, want success", err)
	}
}

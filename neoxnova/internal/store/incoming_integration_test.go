package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"neoxnova/internal/models"
)

// TestIncomingFleets verifies the defender-facing incoming-fleet view against a
// live Postgres. Skipped unless DATABASE_URL is set.
//
//	$env:DATABASE_URL = "postgres://postgres:password@localhost:5432/neoxnova?sslmode=disable"
//	go test ./internal/store/ -run TestIncomingFleets -v
func TestIncomingFleets(t *testing.T) {
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

	mkUser := func(name string) int64 {
		var id int64
		if err := db.QueryRowContext(ctx, `
			INSERT INTO users (universe_id, username, email, password_hash)
			VALUES ($1, $2, $2 || '@example.com', 'x')
			ON CONFLICT (universe_id, username) DO UPDATE SET email = EXCLUDED.email
			RETURNING id
		`, universeID, name).Scan(&id); err != nil {
			t.Fatalf("upsert user %s: %v", name, err)
		}
		return id
	}
	mkPlanet := func(name string, user int64, g, s, p int) int64 {
		var id int64
		if err := db.QueryRowContext(ctx, `
			INSERT INTO celestial_objects (universe_id, user_id, name, object_type, galaxy, system, position)
			VALUES ($1, $2, $3, 'PLANET', $4, $5, $6)
			ON CONFLICT (universe_id, galaxy, system, position, object_type) DO UPDATE SET user_id = EXCLUDED.user_id
			RETURNING id
		`, universeID, user, name, g, s, p).Scan(&id); err != nil {
			t.Fatalf("upsert planet %s: %v", name, err)
		}
		return id
	}

	attacker := mkUser("incoming_atk")
	defender := mkUser("incoming_def")
	origin := mkPlanet("Atk Origin", attacker, 1, 7, 6)
	target := mkPlanet("Def Target", defender, 1, 7, 7)
	decoy := mkPlanet("Other Target", defender, 1, 7, 5)

	// Fleets are inserted directly so the test does not depend on ship counts or
	// the attack noob-protection gate.
	mkFleet := func(mission models.MissionType, targetID int64, tG, tS, tP int, arrival time.Time) {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO fleets (
				universe_id, user_id, mission, phase, origin_id, target_id,
				origin_galaxy, origin_system, origin_position, origin_type,
				target_galaxy, target_system, target_position, target_type,
				start_time, arrival_time, return_time
			) VALUES ($1,$2,$3,'OUTBOUND',$4,$5, 1,7,6,'PLANET', $6,$7,$8,'PLANET',
				NOW() - interval '1 min', $9, $9 + interval '1 hour')
		`, universeID, attacker, string(mission), origin, targetID, tG, tS, tP, arrival); err != nil {
			t.Fatalf("insert %s fleet: %v", mission, err)
		}
	}
	now := time.Now()
	mkFleet(models.MissionAttack, target, 1, 7, 7, now.Add(30*time.Minute))
	mkFleet(models.MissionTransport, target, 1, 7, 7, now.Add(10*time.Minute))
	mkFleet(models.MissionAttack, decoy, 1, 7, 5, now.Add(20*time.Minute))
	defer db.ExecContext(ctx, `DELETE FROM fleets WHERE user_id = $1`, attacker)
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id IN ($1,$2,$3)`, origin, target, decoy)
	defer db.ExecContext(ctx, `DELETE FROM users WHERE id IN ($1,$2)`, attacker, defender)

	fs := NewFleetStore(db)
	fleets, err := fs.IncomingFleets(ctx, target, defender)
	if err != nil {
		t.Fatalf("IncomingFleets: %v", err)
	}
	if len(fleets) != 2 {
		t.Fatalf("incoming = %d, want 2 (attack + transport): %+v", len(fleets), fleets)
	}
	// Soonest arrival first (transport at +10m before attack at +30m).
	if fleets[0].Mission != models.MissionTransport || fleets[1].Mission != models.MissionAttack {
		t.Fatalf("order = %s, %s; want TRANSPORT then ATTACK", fleets[0].Mission, fleets[1].Mission)
	}
	atk := fleets[1]
	if !atk.Hostile || atk.MissionText != "Attack" || atk.Colour == "" {
		t.Fatalf("attack display = %+v", atk)
	}
	if atk.Origin.Galaxy != 1 || atk.Origin.System != 7 || atk.Origin.Position != 6 {
		t.Fatalf("origin = %+v", atk.Origin)
	}
	if fleets[0].Hostile || fleets[0].MissionText != "Transport" {
		t.Fatalf("transport display = %+v", fleets[0])
	}

	// A non-owner must not see the target's incoming fleets (no existence leak).
	if _, err := fs.IncomingFleets(ctx, target, attacker); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-owner incoming = %v, want ErrNotFound", err)
	}
}

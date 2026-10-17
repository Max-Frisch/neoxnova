package engine

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"neoxnova/internal/game"
)

// TestResolveTransportWritesMessage guards the generic inbox wiring: resolving a
// transport must drop a "Located fleet" message into the messages table on
// unload and a "Fleet return" message when it gets home.
func TestResolveTransportWritesMessage(t *testing.T) {
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

	var (
		universeID           string
		userID               int64
		og, os_, op          int
		tg, ts, tp, targetID int64
	)
	if err := db.QueryRowContext(ctx, `
		SELECT universe_id::text, user_id, galaxy, system, position
		FROM celestial_objects WHERE id = 1
	`).Scan(&universeID, &userID, &og, &os_, &op); err != nil {
		t.Skipf("seed missing celestial 1: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT id, galaxy, system, position FROM celestial_objects WHERE id = 2
	`).Scan(&targetID, &tg, &ts, &tp); err != nil {
		t.Skipf("seed missing celestial 2: %v", err)
	}

	var fleetID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO fleets (
			universe_id, user_id, mission, phase, origin_id, target_id,
			origin_galaxy, origin_system, origin_position, origin_type,
			target_galaxy, target_system, target_position, target_type,
			start_time, arrival_time, return_time,
			flight_speed_pct, deuterium_consumption,
			cargo_metal, cargo_crystal, cargo_deuterium
		) VALUES (
			$1, $2, 'TRANSPORT', 'OUTBOUND', 1, $3,
			$4,$5,$6,'PLANET', $7,$8,$9,'PLANET',
			NOW() - interval '1 hour', NOW() - interval '1 minute', NOW() + interval '1 hour',
			1.00, 0, 100, 200, 300
		) RETURNING id
	`, universeID, userID, targetID, og, os_, op, tg, ts, tp).Scan(&fleetID); err != nil {
		t.Fatalf("insert transport fleet: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO fleet_ships (fleet_id, ship_code, count) VALUES ($1,'202',10)`, fleetID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM messages WHERE ref_kind='fleet' AND ref_id=$1`, fleetID)
		db.ExecContext(ctx, `DELETE FROM fleet_ships WHERE fleet_id = $1`, fleetID)
		db.ExecContext(ctx, `DELETE FROM fleets WHERE id = $1`, fleetID)
	})

	eng := NewEventEngine(nil, db)

	// Arrival: unload cargo -> "Located fleet" message.
	eng.resolveFleetEvent(ctx, fleetID)
	var phase string
	if err := db.QueryRowContext(ctx, `SELECT phase FROM fleets WHERE id=$1`, fleetID).Scan(&phase); err != nil {
		t.Fatal(err)
	}
	if phase != "RETURNING" {
		t.Fatalf("phase after unload = %q, want RETURNING", phase)
	}
	assertTransportMessage(t, db, fleetID, "Located fleet")

	// Return leg: fast-forward and resolve -> "Fleet return" + RESOLVED.
	if _, err := db.ExecContext(ctx, `
		UPDATE fleets SET arrival_time = NOW() - interval '1 s', return_time = NOW() - interval '1 s' WHERE id = $1
	`, fleetID); err != nil {
		t.Fatal(err)
	}
	eng.resolveFleetEvent(ctx, fleetID)
	if err := db.QueryRowContext(ctx, `SELECT phase FROM fleets WHERE id=$1`, fleetID).Scan(&phase); err != nil {
		t.Fatal(err)
	}
	if phase != "RESOLVED" {
		t.Fatalf("phase after return = %q, want RESOLVED", phase)
	}
	assertTransportMessage(t, db, fleetID, "Fleet return")
}

func assertTransportMessage(t *testing.T, db *sql.DB, fleetID int64, subject string) {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM messages
		WHERE category = $1 AND ref_kind = 'fleet' AND ref_id = $2 AND subject = $3
	`, game.MessageCategoryTransport, fleetID, subject).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("transport messages %q = %d, want 1", subject, n)
	}
}

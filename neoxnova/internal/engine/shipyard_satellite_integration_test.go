package engine

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"neoxnova/internal/auth"
	"neoxnova/internal/game"
	"neoxnova/internal/store"
)

// TestResolveShipyardSatelliteRecomputesEnergy proves a completed solar-satellite
// batch rewrites the celestial's cached energy output. Before this fix energy
// only refreshed when some later building completed, so the resource bar showed
// stale solar-satellite output. Skipped unless DATABASE_URL is set.
func TestResolveShipyardSatelliteRecomputesEnergy(t *testing.T) {
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

	var codeName string
	if err := db.QueryRowContext(ctx, `SELECT code_name FROM universes ORDER BY code_name LIMIT 1`).Scan(&codeName); err != nil {
		t.Skipf("no universe seeded: %v", err)
	}
	as := store.NewAuthStore(db)
	uname := fmt.Sprintf("engsat_%d", time.Now().UnixNano())
	hash, err := auth.HashPassword("s3cret-password!")
	if err != nil {
		t.Fatal(err)
	}
	userID, planetID, err := as.CreateUser(ctx, codeName, uname, uname+"@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM shipyard_queues WHERE celestial_id = $1`, planetID)
		db.ExecContext(ctx, `DELETE FROM planet_ships WHERE celestial_id = $1`, planetID)
		db.ExecContext(ctx, `DELETE FROM planet_structures WHERE celestial_id = $1`, planetID)
		db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID)
		db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_structures (celestial_id, structure_code, level)
		VALUES ($1, 'shipyard', 1)
		ON CONFLICT (celestial_id, structure_code) DO UPDATE SET level = 1
	`, planetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE celestial_objects SET metal=1e12, crystal=1e12, deuterium=1e12 WHERE id=$1`, planetID); err != nil {
		t.Fatal(err)
	}

	var tempMax, before int
	if err := db.QueryRowContext(ctx, `SELECT temp_max, energy_max FROM celestial_objects WHERE id=$1`, planetID).Scan(&tempMax, &before); err != nil {
		t.Fatal(err)
	}

	bs := store.NewBuildStore(db)
	res, err := bs.EnqueueShipyard(ctx, planetID, "212", 1)
	if err != nil {
		t.Fatalf("enqueue satellite: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		UPDATE shipyard_queues
		SET start_time = NOW() - interval '5 s', end_time = NOW() - interval '1 s'
		WHERE id = $1
	`, res.QueueID); err != nil {
		t.Fatal(err)
	}

	eng := NewEventEngine(nil, db)
	eng.resolveShipyard(ctx, res.QueueID)

	var count int64
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(quantity,0) FROM planet_ships WHERE celestial_id=$1 AND ship_code='212'`, planetID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("docked satellites = %d, want 1", count)
	}

	var after int
	if err := db.QueryRowContext(ctx, `SELECT energy_max FROM celestial_objects WHERE id=$1`, planetID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	want := before + game.SatelliteEnergy(tempMax)
	if after != want {
		t.Fatalf("energy_max = %d, want %d (baseline %d + satellite %d)", after, want, before, game.SatelliteEnergy(tempMax))
	}
}

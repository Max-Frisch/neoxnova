package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"neoxnova/internal/auth"
)

// TestEnqueueShipyardLargeFastBatch guards the regression where a big, very fast
// batch (500 solar satellites) floored its total duration to 1s, so
// build_time_per_unit rounded to 0 and the old `> 0` check rejected the insert.
func TestEnqueueShipyardLargeFastBatch(t *testing.T) {
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
	var codeName string
	if err := db.QueryRowContext(ctx, `SELECT code_name FROM universes LIMIT 1`).Scan(&codeName); err != nil {
		t.Skipf("no universe seeded: %v", err)
	}

	as := NewAuthStore(db)
	uname := fmt.Sprintf("shiptst_%d", time.Now().UnixNano())
	hash, err := auth.HashPassword("s3cret-password!")
	if err != nil {
		t.Fatal(err)
	}
	userID, planetID, err := as.CreateUser(ctx, codeName, uname, uname+"@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM shipyard_queues WHERE celestial_id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM planet_structures WHERE celestial_id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)

	// High shipyard + nanite so 500 units still fit inside the 1s duration floor.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_structures (celestial_id, structure_code, level)
		VALUES ($1,'shipyard',25), ($1,'nanite_factory',10)
	`, planetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE celestial_objects SET metal=1e12, crystal=1e12, deuterium=1e12 WHERE id=$1`, planetID); err != nil {
		t.Fatal(err)
	}

	bs := NewBuildStore(db)
	if _, err := bs.EnqueueShipyard(ctx, planetID, "212", 500); err != nil {
		t.Fatalf("queueing 500 solar satellites: %v", err)
	}
}

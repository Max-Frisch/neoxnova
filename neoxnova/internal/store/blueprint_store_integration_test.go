package store

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// TestListOwnedPlanets verifies the apply-to-all/status planet listing against a
// live Postgres. Skipped unless DATABASE_URL is set.
func TestListOwnedPlanets(t *testing.T) {
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

	var owner int64
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(user_id, 0) FROM celestial_objects WHERE id = 1
	`).Scan(&owner); err != nil || owner == 0 {
		t.Skipf("seed celestial 1 missing/unowned: %v", err)
	}

	bs := NewBlueprintStore(db)
	planets, err := bs.ListOwnedPlanets(ctx, owner)
	if err != nil {
		t.Fatalf("ListOwnedPlanets: %v", err)
	}
	if len(planets) == 0 {
		t.Fatal("owner has no planets")
	}
	for _, p := range planets {
		if p.CelestialID == 0 || p.Name == "" {
			t.Fatalf("empty planet row: %+v", p)
		}
		if p.Galaxy < 1 || p.Galaxy > 9 || p.System < 1 || p.System > 499 || p.Position < 1 || p.Position > 21 {
			t.Fatalf("planet out of coordinate domain: %+v", p)
		}
	}
	// The seed homeworld (1:1:1) must be present.
	var found bool
	for _, p := range planets {
		if p.Galaxy == 1 && p.System == 1 && p.Position == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("seed homeworld 1:1:1 not returned; got %+v", planets)
	}
}

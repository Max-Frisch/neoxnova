package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	"neoxnova/internal/blueprint"

	_ "github.com/lib/pq"
)

// TestAdvanceBlueprintEnqueues requires DATABASE_URL like the other engine
// integration tests. It inserts a planet blueprint that targets one level above
// the current metal mine and asserts the planner enqueues it, then does not
// double-enqueue while the queue is busy.
func TestAdvanceBlueprintEnqueues(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping DB integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Register the close first so it runs last (t.Cleanup is LIFO); the data
	// cleanup registered below then still has an open pool.
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("db not reachable: %v", err)
	}

	var universeID string
	var userID int64
	if err := db.QueryRowContext(ctx, `
		SELECT universe_id::text, COALESCE(user_id, 0) FROM celestial_objects WHERE id = 1
	`).Scan(&universeID, &userID); err != nil {
		t.Skipf("seed missing celestial 1: %v", err)
	}
	if userID == 0 {
		t.Skip("celestial 1 has no owner")
	}

	var current int
	if err := db.QueryRowContext(ctx, `
		SELECT COALESCE(level, 0) FROM planet_structures WHERE celestial_id = 1 AND structure_code = 'metal_mine'
	`).Scan(&current); err != nil && err != sql.ErrNoRows {
		t.Fatalf("read metal_mine: %v", err)
	}

	// Clear any residue from a prior interrupted run (one enabled blueprint per
	// celestial is enforced by a partial unique index).
	db.ExecContext(ctx, `DELETE FROM build_blueprints WHERE celestial_id = 1`)
	db.ExecContext(ctx, `DELETE FROM construction_queues WHERE celestial_id = 1 AND structure_code = 'metal_mine' AND status = 'IN_PROGRESS'`)

	spec, _ := json.Marshal(blueprint.Spec{Buildings: map[string]int{"metal_mine": current + 1}})
	if _, err := db.ExecContext(ctx, `
		INSERT INTO build_blueprints (universe_id, user_id, celestial_id, name, scope, spec, enabled)
		VALUES ($1, $2, 1, 'test', 'planet', $3, true)
	`, universeID, userID, spec); err != nil {
		t.Fatalf("insert blueprint: %v", err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM build_blueprints WHERE celestial_id = 1`)
		db.ExecContext(ctx, `DELETE FROM construction_queues WHERE celestial_id = 1 AND structure_code = 'metal_mine' AND status = 'IN_PROGRESS'`)
	})

	eng := NewEventEngine(nil, db)
	eng.AdvanceBlueprint(ctx, 1)

	var queued int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM construction_queues
		WHERE celestial_id = 1 AND structure_code = 'metal_mine' AND status = 'IN_PROGRESS'
	`).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("construction queue rows = %d, want 1", queued)
	}

	// A second advance while the queue is busy must not add another row.
	eng.AdvanceBlueprint(ctx, 1)
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM construction_queues
		WHERE celestial_id = 1 AND structure_code = 'metal_mine' AND status = 'IN_PROGRESS'
	`).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 1 {
		t.Fatalf("construction queue rows after second advance = %d, want 1 (busy)", queued)
	}
}

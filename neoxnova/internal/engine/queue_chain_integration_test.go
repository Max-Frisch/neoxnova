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
	"neoxnova/internal/store"
)

// TestResolveConstructionChain proves the scheduler resolves a chained
// multi-order build back-to-back, in order: a 3-level metal_mine enqueue ends
// with metal_mine at level 3 and every order COMPLETED. Skipped unless
// DATABASE_URL is set.
func TestResolveConstructionChain(t *testing.T) {
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
	uname := fmt.Sprintf("engchain_%d", time.Now().UnixNano())
	hash, err := auth.HashPassword("s3cret-password!")
	if err != nil {
		t.Fatal(err)
	}
	userID, planetID, err := as.CreateUser(ctx, codeName, uname, uname+"@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM construction_queues WHERE celestial_id = $1`, planetID)
		db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID)
		db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_structures (celestial_id, structure_code, level)
		VALUES ($1, 'metal_mine', 0)
		ON CONFLICT (celestial_id, structure_code) DO UPDATE SET level = 0
	`, planetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE celestial_objects SET metal=1e12, crystal=1e12, deuterium=1e12 WHERE id=$1`, planetID); err != nil {
		t.Fatal(err)
	}

	bs := store.NewBuildStore(db)
	if _, err := bs.EnqueueStructure(ctx, planetID, "metal_mine", 3); err != nil {
		t.Fatalf("enqueue chain: %v", err)
	}

	// Make every chained order due at once (keeping start < end valid).
	if _, err := db.ExecContext(ctx, `
		UPDATE construction_queues
		SET start_time = NOW() - interval '5 s', end_time = NOW() - interval '1 s'
		WHERE celestial_id = $1 AND status = 'IN_PROGRESS'
	`, planetID); err != nil {
		t.Fatal(err)
	}

	ids := make([]int64, 0, 3)
	rows, err := db.QueryContext(ctx, `
		SELECT id FROM construction_queues WHERE celestial_id = $1 ORDER BY target_level
	`, planetID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 3 {
		t.Fatalf("queued rows = %d, want 3", len(ids))
	}

	eng := NewEventEngine(nil, db)
	for _, id := range ids {
		eng.resolveConstruction(ctx, id)
	}

	var level int
	if err := db.QueryRowContext(ctx, `SELECT level FROM planet_structures WHERE celestial_id=$1 AND structure_code='metal_mine'`, planetID).Scan(&level); err != nil {
		t.Fatal(err)
	}
	if level != 3 {
		t.Fatalf("metal_mine level = %d, want 3", level)
	}
	var completed int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM construction_queues WHERE celestial_id=$1 AND status='COMPLETED'`, planetID).Scan(&completed); err != nil {
		t.Fatal(err)
	}
	if completed != 3 {
		t.Fatalf("completed rows = %d, want 3", completed)
	}
}

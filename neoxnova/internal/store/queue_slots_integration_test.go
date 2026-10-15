package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"neoxnova/internal/auth"
)

// Q1 of docs/PLAYABILITY_FIXES_PLAN.md: the queue schema moves from one active
// row per scope to multi-order chains. These tests prove the 0020 migration is
// idempotent (columns added, single-active indexes dropped) and that multiple
// IN_PROGRESS rows now coexist per scope. Skipped unless DATABASE_URL is set.

func openQueueTestDB(t *testing.T) *sql.DB {
	t.Helper()
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
	return db
}

// TestQueueSlotsMigrationIdempotent re-runs 0020_queue_slots.sql (twice) and
// asserts the resulting schema: two slot columns with the locked defaults and
// none of the retired single-active unique indexes.
func TestQueueSlotsMigrationIdempotent(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()

	sqlBytes, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0020_queue_slots.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := db.ExecContext(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply migration (run %d): %v", i+1, err)
		}
	}

	defaults := map[string]string{"build_queue_slots": "10", "research_queue_slots": "4"}
	for col, want := range defaults {
		var dataType, def string
		err := db.QueryRowContext(ctx, `
			SELECT data_type, COALESCE(column_default, '')
			FROM information_schema.columns
			WHERE table_name = 'universes' AND column_name = $1
		`, col).Scan(&dataType, &def)
		if err != nil {
			t.Fatalf("column %s missing: %v", col, err)
		}
		if dataType != "smallint" {
			t.Errorf("%s type = %q, want smallint", col, dataType)
		}
		if def != want {
			t.Errorf("%s default = %q, want %q", col, def, want)
		}
	}

	for _, idx := range []string{"uq_active_construction", "uq_active_research", "uq_active_shipyard"} {
		var n int
		if err := db.QueryRowContext(ctx, `SELECT count(*) FROM pg_indexes WHERE indexname = $1`, idx).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("index %s still present; multi-order queues need it dropped", idx)
		}
	}
}

// TestQueueAllowsMultipleActive proves all three scopes accept several
// IN_PROGRESS rows at once (the old partial unique indexes rejected this).
func TestQueueAllowsMultipleActive(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()

	var codeName string
	if err := db.QueryRowContext(ctx, `SELECT code_name FROM universes LIMIT 1`).Scan(&codeName); err != nil {
		t.Skipf("no universe seeded: %v", err)
	}

	as := NewAuthStore(db)
	uname := fmt.Sprintf("queueslots_%d", time.Now().UnixNano())
	hash, err := auth.HashPassword("s3cret-password!")
	if err != nil {
		t.Fatal(err)
	}
	userID, planetID, err := as.CreateUser(ctx, codeName, uname, uname+"@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM construction_queues WHERE celestial_id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM shipyard_queues WHERE celestial_id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM research_queues WHERE user_id = $1`, userID)
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)

	end := time.Now().Add(time.Hour)
	for i, code := range []string{"metal_mine", "crystal_mine", "deuterium_synthesizer"} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO construction_queues (celestial_id, structure_code, target_level, start_time, end_time, status)
			VALUES ($1, $2, $3, NOW(), $4, 'IN_PROGRESS')
		`, planetID, code, i+1, end); err != nil {
			t.Fatalf("insert construction row %d: %v", i, err)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO shipyard_queues (celestial_id, unit_code, quantity_total, quantity_completed, build_time_per_unit, start_time, end_time, status)
			VALUES ($1, '202', 5, 0, 1.0, NOW(), $2, 'IN_PROGRESS')
		`, planetID, end); err != nil {
			t.Fatalf("insert shipyard row %d: %v", i, err)
		}
	}
	for i, code := range []string{"energy_tech", "combustion_drive", "espionage_tech"} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO research_queues (user_id, celestial_id, tech_code, target_level, start_time, end_time, status)
			VALUES ($1, $2, $3, $4, NOW(), $5, 'IN_PROGRESS')
		`, userID, planetID, code, i+1, end); err != nil {
			t.Fatalf("insert research row %d: %v", i, err)
		}
	}

	for _, probe := range []struct {
		name  string
		query string
		want  int
	}{
		{"construction", `SELECT count(*) FROM construction_queues WHERE celestial_id = $1 AND status = 'IN_PROGRESS'`, 3},
		{"shipyard", `SELECT count(*) FROM shipyard_queues WHERE celestial_id = $1 AND status = 'IN_PROGRESS'`, 2},
		{"research", `SELECT count(*) FROM research_queues WHERE user_id = $1 AND status = 'IN_PROGRESS'`, 3},
	} {
		arg := planetID
		if probe.name == "research" {
			arg = userID
		}
		var got int
		if err := db.QueryRowContext(ctx, probe.query, arg).Scan(&got); err != nil {
			t.Fatalf("%s count: %v", probe.name, err)
		}
		if got != probe.want {
			t.Errorf("%s active rows = %d, want %d", probe.name, got, probe.want)
		}
	}
}

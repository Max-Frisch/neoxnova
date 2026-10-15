package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"neoxnova/internal/auth"
)

// TestCancelQueueRefundsConstruction verifies the reskin-I2 cancel path: the
// active construction is marked CANCELLED, its full cost is refunded to the
// celestial, and a second cancel is a no-op (ErrNotFound). Skips without
// DATABASE_URL like the other DB integration tests.
func TestCancelQueueRefundsConstruction(t *testing.T) {
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
	uname := fmt.Sprintf("cancelbuild_%d", time.Now().UnixNano())
	hash, err := auth.HashPassword("s3cret-password!")
	if err != nil {
		t.Fatal(err)
	}
	userID, planetID, err := as.CreateUser(ctx, codeName, uname, uname+"@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM construction_queues WHERE celestial_id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM planet_structures WHERE celestial_id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)

	if _, err := db.ExecContext(ctx, `UPDATE celestial_objects SET metal=1e9, crystal=1e9, deuterium=1e9 WHERE id=$1`, planetID); err != nil {
		t.Fatal(err)
	}

	bs := NewBuildStore(db)
	if _, err := bs.EnqueueStructure(ctx, planetID, "metal_mine", 0); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	var afterEnqueue float64
	if err := db.QueryRowContext(ctx, `SELECT metal FROM celestial_objects WHERE id=$1`, planetID).Scan(&afterEnqueue); err != nil {
		t.Fatal(err)
	}

	cost, err := bs.CancelQueue(ctx, CancelConstruction, planetID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cost.Metal <= 0 {
		t.Fatalf("refund metal = %d, want > 0", cost.Metal)
	}

	var afterCancel float64
	if err := db.QueryRowContext(ctx, `SELECT metal FROM celestial_objects WHERE id=$1`, planetID).Scan(&afterCancel); err != nil {
		t.Fatal(err)
	}
	if afterCancel <= afterEnqueue {
		t.Fatalf("metal after cancel %.0f not greater than after enqueue %.0f", afterCancel, afterEnqueue)
	}

	var status string
	if err := db.QueryRowContext(ctx, `
		SELECT status FROM construction_queues WHERE celestial_id=$1 ORDER BY id DESC LIMIT 1
	`, planetID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "CANCELLED" {
		t.Fatalf("queue status = %q, want CANCELLED", status)
	}

	if _, err := bs.CancelQueue(ctx, CancelConstruction, planetID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second cancel = %v, want ErrNotFound", err)
	}
}

// TestCancelQueueRefundsShipyard verifies the reskin-I3 ship/defense cancel: the
// active shipyard build is marked CANCELLED and its full cost is refunded to the
// celestial. Skips without DATABASE_URL.
func TestCancelQueueRefundsShipyard(t *testing.T) {
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
	uname := fmt.Sprintf("cancelship_%d", time.Now().UnixNano())
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

	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_structures (celestial_id, structure_code, level) VALUES ($1,'shipyard',1)
		ON CONFLICT (celestial_id, structure_code) DO UPDATE SET level = EXCLUDED.level
	`, planetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE celestial_objects SET metal=1e9, crystal=1e9, deuterium=1e9 WHERE id=$1`, planetID); err != nil {
		t.Fatal(err)
	}

	bs := NewBuildStore(db)
	// Missile Launcher (401) only needs a shipyard, keeping the fixture small.
	if _, err := bs.EnqueueShipyard(ctx, planetID, "401", 3); err != nil {
		t.Fatalf("enqueue shipyard: %v", err)
	}
	cost, err := bs.CancelQueue(ctx, CancelShipyard, planetID)
	if err != nil {
		t.Fatalf("cancel shipyard: %v", err)
	}
	if cost.Metal <= 0 {
		t.Fatalf("refund metal = %d, want > 0", cost.Metal)
	}

	var status string
	if err := db.QueryRowContext(ctx, `
		SELECT status FROM shipyard_queues WHERE celestial_id=$1 ORDER BY id DESC LIMIT 1
	`, planetID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "CANCELLED" {
		t.Fatalf("shipyard status = %q, want CANCELLED", status)
	}

	if _, err := bs.CancelQueue(ctx, CancelShipyard, planetID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second cancel = %v, want ErrNotFound", err)
	}
}

// TestCancelQueueRefundsResearch covers the empire-wide research scope: the
// refund lands on the celestial the research was started from and the entry is
// marked CANCELLED.
func TestCancelQueueRefundsResearch(t *testing.T) {
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
	uname := fmt.Sprintf("cancelres_%d", time.Now().UnixNano())
	hash, err := auth.HashPassword("s3cret-password!")
	if err != nil {
		t.Fatal(err)
	}
	userID, planetID, err := as.CreateUser(ctx, codeName, uname, uname+"@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM research_queues WHERE user_id = $1`, userID)
	defer db.ExecContext(ctx, `DELETE FROM planet_structures WHERE celestial_id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)

	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_structures (celestial_id, structure_code, level) VALUES ($1,'research_lab',1)
		ON CONFLICT (celestial_id, structure_code) DO UPDATE SET level = EXCLUDED.level
	`, planetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE celestial_objects SET metal=1e9, crystal=1e9, deuterium=1e9 WHERE id=$1`, planetID); err != nil {
		t.Fatal(err)
	}

	bs := NewBuildStore(db)
	if _, err := bs.EnqueueResearch(ctx, planetID, "energy_tech", 0); err != nil {
		t.Fatalf("enqueue research: %v", err)
	}
	cost, err := bs.CancelQueue(ctx, CancelResearch, userID)
	if err != nil {
		t.Fatalf("cancel research: %v", err)
	}
	if cost.Crystal <= 0 {
		t.Fatalf("refund crystal = %d, want > 0", cost.Crystal)
	}

	var status string
	if err := db.QueryRowContext(ctx, `
		SELECT status FROM research_queues WHERE user_id=$1 ORDER BY id DESC LIMIT 1
	`, userID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "CANCELLED" {
		t.Fatalf("research status = %q, want CANCELLED", status)
	}
}

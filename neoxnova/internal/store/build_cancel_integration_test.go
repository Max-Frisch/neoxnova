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
	"neoxnova/internal/game"
)

// cancelTestPlanet creates a throwaway account whose homeworld has production
// disabled (so refunds are exact) and resources/capacity well above any build
// cost used by these tests. The caller must defer the returned cleanup.
func cancelTestPlanet(t *testing.T, ctx context.Context, db *sql.DB, prefix string) (userID, planetID int64) {
	t.Helper()
	var codeName string
	if err := db.QueryRowContext(ctx, `SELECT code_name FROM universes LIMIT 1`).Scan(&codeName); err != nil {
		t.Skipf("no universe seeded: %v", err)
	}
	as := NewAuthStore(db)
	uname := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	hash, err := auth.HashPassword("s3cret-password!")
	if err != nil {
		t.Fatal(err)
	}
	if userID, planetID, err = as.CreateUser(ctx, codeName, uname, uname+"@example.com", hash); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return userID, planetID
}

func openCancelTestDB(t *testing.T, ctx context.Context) *sql.DB {
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
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	return db
}

func seedTestResources(t *testing.T, ctx context.Context, db *sql.DB, planetID int64) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
		UPDATE celestial_objects
		SET metal=1e9, crystal=1e9, deuterium=1e9,
		    metal_capacity=1e15, crystal_capacity=1e15, deuterium_capacity=1e15,
		    metal_prod_hourly=0, crystal_prod_hourly=0, deuterium_prod_hourly=0
		WHERE id=$1
	`, planetID); err != nil {
		t.Fatal(err)
	}
}

func readRes(t *testing.T, ctx context.Context, db *sql.DB, planetID int64) (m, c, d float64) {
	t.Helper()
	if err := db.QueryRowContext(ctx,
		`SELECT metal, crystal, deuterium FROM celestial_objects WHERE id=$1`, planetID,
	).Scan(&m, &c, &d); err != nil {
		t.Fatal(err)
	}
	return
}

// TestCancelQueueRefundsConstructionByID cancels a *middle* order by its queue
// id: the row is marked CANCELLED, its frozen cost is refunded, and the tail is
// renumbered down one level (refunding the level-cost difference) and pulled
// forward. Skips without DATABASE_URL like the other DB integration tests.
func TestCancelQueueRefundsConstructionByID(t *testing.T) {
	ctx := context.Background()
	db := openCancelTestDB(t, ctx)
	userID, planetID := cancelTestPlanet(t, ctx, db, "cancelbuild")
	defer db.ExecContext(ctx, `DELETE FROM construction_queues WHERE celestial_id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM planet_structures WHERE celestial_id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)

	seedTestResources(t, ctx, db, planetID)

	bs := NewBuildStore(db)
	if _, err := bs.EnqueueStructure(ctx, planetID, "metal_mine", 3); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	type qrow struct {
		id     int64
		target int
		end    time.Time
	}
	var orders []qrow
	rows, err := db.QueryContext(ctx, `
		SELECT id, target_level, end_time FROM construction_queues
		WHERE celestial_id = $1 AND status = 'IN_PROGRESS' ORDER BY id
	`, planetID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var o qrow
		if err := rows.Scan(&o.id, &o.target, &o.end); err != nil {
			t.Fatal(err)
		}
		orders = append(orders, o)
	}
	rows.Close()
	if len(orders) != 3 {
		t.Fatalf("queued %d orders, want 3", len(orders))
	}

	beforeM, beforeC, _ := readRes(t, ctx, db, planetID)

	mid := orders[1]
	cost, err := bs.CancelQueue(ctx, CancelConstruction, mid.id, planetID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	wantMid, _ := game.StructureCost("metal_mine", 2)
	if cost != wantMid {
		t.Fatalf("refunded %+v, want %+v (level 2)", cost, wantMid)
	}

	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM construction_queues WHERE id=$1`, mid.id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "CANCELLED" {
		t.Fatalf("queue status = %q, want CANCELLED", status)
	}

	// The tail order (was level 3) shifts down to level 2 and is pulled forward.
	var tailTarget int
	var tailEnd time.Time
	if err := db.QueryRowContext(ctx,
		`SELECT target_level, end_time FROM construction_queues WHERE id=$1`, orders[2].id,
	).Scan(&tailTarget, &tailEnd); err != nil {
		t.Fatal(err)
	}
	if tailTarget != 2 {
		t.Fatalf("tail target_level = %d, want 2", tailTarget)
	}
	if !tailEnd.Before(orders[2].end) {
		t.Fatalf("tail end_time %v not pulled forward from %v", tailEnd, orders[2].end)
	}
	// The head order (level 1) is untouched.
	var headTarget int
	if err := db.QueryRowContext(ctx,
		`SELECT target_level FROM construction_queues WHERE id=$1`, orders[0].id,
	).Scan(&headTarget); err != nil {
		t.Fatal(err)
	}
	if headTarget != 1 {
		t.Fatalf("head target_level = %d, want 1", headTarget)
	}

	// Net refund equals the highest level that fell off the chain (cost of 3).
	wantTotal, _ := game.StructureCost("metal_mine", 3)
	afterM, afterC, _ := readRes(t, ctx, db, planetID)
	if int64(afterM-beforeM) != wantTotal.Metal || int64(afterC-beforeC) != wantTotal.Crystal {
		t.Fatalf("refund (+%d metal, +%d crystal), want (+%d, +%d)",
			int64(afterM-beforeM), int64(afterC-beforeC), wantTotal.Metal, wantTotal.Crystal)
	}

	if _, err := bs.CancelQueue(ctx, CancelConstruction, mid.id, planetID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second cancel = %v, want ErrNotFound", err)
	}
}

// TestCancelQueueRefundsShipyardByID cancels one ship/defense batch by id: the
// batch is marked CANCELLED, its full cost is refunded, and the later batch of
// the same kind is pulled forward. Skips without DATABASE_URL.
func TestCancelQueueRefundsShipyardByID(t *testing.T) {
	ctx := context.Background()
	db := openCancelTestDB(t, ctx)
	userID, planetID := cancelTestPlanet(t, ctx, db, "cancelship")
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
	seedTestResources(t, ctx, db, planetID)

	bs := NewBuildStore(db)
	// Missile Launcher (401) only needs a shipyard.
	first, err := bs.EnqueueShipyard(ctx, planetID, "401", 3)
	if err != nil {
		t.Fatalf("enqueue first: %v", err)
	}
	if _, err := bs.EnqueueShipyard(ctx, planetID, "401", 5); err != nil {
		t.Fatalf("enqueue second: %v", err)
	}

	beforeM, _, _ := readRes(t, ctx, db, planetID)
	var secondStart time.Time
	if err := db.QueryRowContext(ctx, `
		SELECT start_time FROM shipyard_queues
		WHERE celestial_id = $1 AND status = 'IN_PROGRESS' ORDER BY id DESC LIMIT 1
	`, planetID).Scan(&secondStart); err != nil {
		t.Fatal(err)
	}

	cost, err := bs.CancelQueue(ctx, CancelShipyard, first.QueueID, planetID)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	want, _ := game.UnitCost("401", 3)
	if cost != want {
		t.Fatalf("refunded %+v, want %+v", cost, want)
	}

	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM shipyard_queues WHERE id=$1`, first.QueueID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "CANCELLED" {
		t.Fatalf("shipyard status = %q, want CANCELLED", status)
	}

	afterM, _, _ := readRes(t, ctx, db, planetID)
	if int64(afterM-beforeM) != want.Metal {
		t.Fatalf("refund +%d metal, want +%d", int64(afterM-beforeM), want.Metal)
	}

	var newStart time.Time
	if err := db.QueryRowContext(ctx, `
		SELECT start_time FROM shipyard_queues
		WHERE celestial_id = $1 AND status = 'IN_PROGRESS' ORDER BY id DESC LIMIT 1
	`, planetID).Scan(&newStart); err != nil {
		t.Fatal(err)
	}
	if !newStart.Before(secondStart) {
		t.Fatalf("tail start %v not pulled forward from %v", newStart, secondStart)
	}

	if _, err := bs.CancelQueue(ctx, CancelShipyard, first.QueueID, planetID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second cancel = %v, want ErrNotFound", err)
	}
}

// TestCancelQueueRefundsResearchByID covers the empire-wide research scope: the
// refund lands on the celestial the research was started from, a middle order's
// cancellation renumbers the tail down a level, and the entry is marked
// CANCELLED. Cancelling with the wrong owner is rejected.
func TestCancelQueueRefundsResearchByID(t *testing.T) {
	ctx := context.Background()
	db := openCancelTestDB(t, ctx)
	userID, planetID := cancelTestPlanet(t, ctx, db, "cancelres")
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
	seedTestResources(t, ctx, db, planetID)

	bs := NewBuildStore(db)
	if _, err := bs.EnqueueResearch(ctx, planetID, "energy_tech", 3); err != nil {
		t.Fatalf("enqueue research: %v", err)
	}

	var ids []int64
	rows, err := db.QueryContext(ctx, `
		SELECT id FROM research_queues WHERE user_id = $1 AND status = 'IN_PROGRESS' ORDER BY id
	`, userID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 3 {
		t.Fatalf("queued %d research orders, want 3", len(ids))
	}

	beforeM, beforeC, beforeD := readRes(t, ctx, db, planetID)
	cost, err := bs.CancelQueue(ctx, CancelResearch, ids[1], userID)
	if err != nil {
		t.Fatalf("cancel research: %v", err)
	}
	wantMid, _ := game.TechCost("energy_tech", 2)
	if cost != wantMid {
		t.Fatalf("refunded %+v, want %+v (level 2)", cost, wantMid)
	}

	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM research_queues WHERE id=$1`, ids[1]).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "CANCELLED" {
		t.Fatalf("research status = %q, want CANCELLED", status)
	}

	var tailTarget int
	if err := db.QueryRowContext(ctx, `
		SELECT target_level FROM research_queues WHERE id=$1
	`, ids[2]).Scan(&tailTarget); err != nil {
		t.Fatal(err)
	}
	if tailTarget != 2 {
		t.Fatalf("tail target_level = %d, want 2", tailTarget)
	}

	wantTotal, _ := game.TechCost("energy_tech", 3)
	afterM, afterC, afterD := readRes(t, ctx, db, planetID)
	if int64(afterC-beforeC) != wantTotal.Crystal || int64(afterD-beforeD) != wantTotal.Deuterium {
		t.Fatalf("refund (+%d crystal, +%d deut), want (+%d, +%d)",
			int64(afterC-beforeC), int64(afterD-beforeD), wantTotal.Crystal, wantTotal.Deuterium)
	}
	if int64(afterM-beforeM) != 0 {
		t.Fatalf("metal changed by %d, want 0", int64(afterM-beforeM))
	}

	// A different user cannot cancel another account's research.
	if _, err := bs.CancelQueue(ctx, CancelResearch, ids[2], userID+1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign cancel = %v, want ErrNotFound", err)
	}
}

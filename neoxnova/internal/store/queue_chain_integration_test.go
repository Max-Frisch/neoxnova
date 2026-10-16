package store

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"neoxnova/internal/auth"
	"neoxnova/internal/game"
)

// Q2 of docs/PLAYABILITY_FIXES_PLAN.md: buildings and research become chained
// multi-order queues (target level, slot caps, partial-on-poverty); ships and
// defenses chain per kind with no cap. Skipped unless DATABASE_URL is set.

type chainPlanet struct {
	userID   int64
	planetID int64
	universe string
}

func newChainPlanet(t *testing.T, db *sql.DB, ctx context.Context) chainPlanet {
	t.Helper()
	var codeName string
	if err := db.QueryRowContext(ctx, `SELECT code_name FROM universes ORDER BY code_name LIMIT 1`).Scan(&codeName); err != nil {
		t.Skipf("no universe seeded: %v", err)
	}
	as := NewAuthStore(db)
	uname := fmt.Sprintf("chain_%d", time.Now().UnixNano())
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
		db.ExecContext(ctx, `DELETE FROM shipyard_queues WHERE celestial_id = $1`, planetID)
		db.ExecContext(ctx, `DELETE FROM research_queues WHERE user_id = $1`, userID)
		db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID)
		db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})
	return chainPlanet{userID: userID, planetID: planetID, universe: codeName}
}

func setStructure(ctx context.Context, t *testing.T, db *sql.DB, planetID int64, code string, level int) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_structures (celestial_id, structure_code, level)
		VALUES ($1, $2, $3)
		ON CONFLICT (celestial_id, structure_code) DO UPDATE SET level = EXCLUDED.level
	`, planetID, code, level); err != nil {
		t.Fatalf("set %s=%d: %v", code, level, err)
	}
}

// setResources pins a celestial's stock to exact values with production off and
// huge capacity, so tests observe exactly what they fund (update_celestial_resources
// otherwise accrues and clamps to capacity).
func setResources(ctx context.Context, t *testing.T, db *sql.DB, planetID int64, metal, crystal, deuterium float64) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
		UPDATE celestial_objects SET
			metal=$2, crystal=$3, deuterium=$4,
			metal_capacity=1e15, crystal_capacity=1e15, deuterium_capacity=1e15,
			metal_prod_hourly=0, crystal_prod_hourly=0, deuterium_prod_hourly=0
		WHERE id=$1
	`, planetID, metal, crystal, deuterium); err != nil {
		t.Fatal(err)
	}
}

func giveResources(ctx context.Context, t *testing.T, db *sql.DB, planetID int64, amount float64) {
	t.Helper()
	setResources(ctx, t, db, planetID, amount, amount, amount)
}

// TestEnqueueStructureChain audits a multi-level building enqueue: the chain
// targets 1..3 in order, end times climb, and the total cost is the sum of the
// per-level costs.
func TestEnqueueStructureChain(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()
	p := newChainPlanet(t, db, ctx)
	setStructure(ctx, t, db, p.planetID, "metal_mine", 0)
	giveResources(ctx, t, db, p.planetID, 1e12)

	var metalBefore float64
	db.QueryRowContext(ctx, `SELECT metal FROM celestial_objects WHERE id=$1`, p.planetID).Scan(&metalBefore)

	bs := NewBuildStore(db)
	res, err := bs.EnqueueStructure(ctx, p.planetID, "metal_mine", 3)
	if err != nil {
		t.Fatalf("enqueue chain: %v", err)
	}
	if res.Queued != 3 || res.TargetLevel != 3 {
		t.Fatalf("queued=%d target=%d, want 3/3", res.Queued, res.TargetLevel)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT target_level, start_time, end_time FROM construction_queues
		WHERE celestial_id=$1 AND status='IN_PROGRESS' ORDER BY target_level
	`, p.planetID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var wantCost game.Cost
	var prevEnd time.Time
	var got int
	for rows.Next() {
		var lvl int
		var start, end time.Time
		if err := rows.Scan(&lvl, &start, &end); err != nil {
			t.Fatal(err)
		}
		if lvl != got+1 {
			t.Fatalf("target levels out of order: got %d after %d", lvl, got)
		}
		if got > 0 && start.Before(prevEnd) {
			t.Fatalf("level %d starts %v before previous end %v (not chained)", lvl, start, prevEnd)
		}
		prevEnd = end
		got++
		c, _ := game.StructureCost("metal_mine", lvl)
		wantCost = wantCost.Add(c)
	}
	if got != 3 {
		t.Fatalf("rows = %d, want 3", got)
	}
	if res.Cost != wantCost {
		t.Fatalf("cost = %+v, want %+v", res.Cost, wantCost)
	}

	var metalAfter float64
	db.QueryRowContext(ctx, `SELECT metal FROM celestial_objects WHERE id=$1`, p.planetID).Scan(&metalAfter)
	if int64(metalBefore-metalAfter) != wantCost.Metal {
		t.Fatalf("metal deducted = %d, want %d", int64(metalBefore-metalAfter), wantCost.Metal)
	}
}

// TestEnqueueStructureSlotCap proves the chain stops at the universe's
// build_queue_slots cap (a partial enqueue that still succeeds).
func TestEnqueueStructureSlotCap(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()
	p := newChainPlanet(t, db, ctx)

	var orig int
	if err := db.QueryRowContext(ctx, `SELECT build_queue_slots FROM universes WHERE code_name=$1`, p.universe).Scan(&orig); err != nil {
		t.Fatal(err)
	}
	db.ExecContext(ctx, `UPDATE universes SET build_queue_slots=2 WHERE code_name=$1`, p.universe)
	t.Cleanup(func() {
		db.ExecContext(ctx, `UPDATE universes SET build_queue_slots=$2 WHERE code_name=$1`, p.universe, orig)
	})

	setStructure(ctx, t, db, p.planetID, "metal_mine", 0)
	giveResources(ctx, t, db, p.planetID, 1e12)

	bs := NewBuildStore(db)
	res, err := bs.EnqueueStructure(ctx, p.planetID, "metal_mine", 5)
	if err != nil {
		t.Fatalf("enqueue under cap: %v", err)
	}
	if res.Queued != 2 {
		t.Fatalf("queued=%d, want 2 (slot cap)", res.Queued)
	}
	var n int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM construction_queues WHERE celestial_id=$1 AND status='IN_PROGRESS'`, p.planetID).Scan(&n)
	if n != 2 {
		t.Fatalf("rows=%d, want 2", n)
	}
}

// TestEnqueueStructurePartialOnPoverty proves an unaffordable later level stops
// the chain but still returns the affordable prefix.
func TestEnqueueStructurePartialOnPoverty(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()
	p := newChainPlanet(t, db, ctx)
	setStructure(ctx, t, db, p.planetID, "metal_mine", 0)

	// Fund level 1 plus a partial (1 unit short) of level 2, so only level 1 is
	// affordable.
	l1, _ := game.StructureCost("metal_mine", 1)
	l2, _ := game.StructureCost("metal_mine", 2)
	setResources(ctx, t, db, p.planetID, float64(l1.Metal+l2.Metal-1), float64(l1.Crystal), 0)

	bs := NewBuildStore(db)
	res, err := bs.EnqueueStructure(ctx, p.planetID, "metal_mine", 5)
	if err != nil {
		t.Fatalf("partial enqueue should succeed: %v", err)
	}
	if res.Queued != 1 || res.TargetLevel != 1 {
		t.Fatalf("queued=%d target=%d, want 1/1 (partial)", res.Queued, res.TargetLevel)
	}
}

// TestEnqueueResearchChain proves research also chains multiple levels.
func TestEnqueueResearchChain(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()
	p := newChainPlanet(t, db, ctx)
	setStructure(ctx, t, db, p.planetID, "research_lab", 3)
	giveResources(ctx, t, db, p.planetID, 1e12)

	bs := NewBuildStore(db)
	res, err := bs.EnqueueResearch(ctx, p.planetID, "energy_tech", 3)
	if err != nil {
		t.Fatalf("enqueue research chain: %v", err)
	}
	if res.Queued != 3 || res.TargetLevel != 3 {
		t.Fatalf("queued=%d target=%d, want 3/3", res.Queued, res.TargetLevel)
	}
	var n int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM research_queues WHERE user_id=$1 AND status='IN_PROGRESS'`, p.userID).Scan(&n)
	if n != 3 {
		t.Fatalf("research rows=%d, want 3", n)
	}
}

// TestEnqueueShipyardChainsByKind proves ship batches chain behind the previous
// ship batch, while a defense batch starts its own (independent) line.
func TestEnqueueShipyardChainsByKind(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()
	p := newChainPlanet(t, db, ctx)
	setStructure(ctx, t, db, p.planetID, "shipyard", 10)
	setStructure(ctx, t, db, p.planetID, "nanite_factory", 1)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO user_technologies (user_id, tech_code, level)
		VALUES ($1,'combustion_drive',10) ON CONFLICT (user_id, tech_code) DO UPDATE SET level=EXCLUDED.level
	`, p.userID); err != nil {
		t.Fatal(err)
	}
	giveResources(ctx, t, db, p.planetID, 1e12)

	bs := NewBuildStore(db)
	if _, err := bs.EnqueueShipyard(ctx, p.planetID, "202", 5); err != nil {
		t.Fatalf("first ship batch: %v", err)
	}
	if _, err := bs.EnqueueShipyard(ctx, p.planetID, "202", 5); err != nil {
		t.Fatalf("second ship batch: %v", err)
	}
	if _, err := bs.EnqueueShipyard(ctx, p.planetID, "401", 5); err != nil {
		t.Fatalf("defense batch: %v", err)
	}

	type row struct {
		code       string
		start, end time.Time
	}
	rows, err := db.QueryContext(ctx, `
		SELECT unit_code, start_time, end_time FROM shipyard_queues
		WHERE celestial_id=$1 AND status='IN_PROGRESS' ORDER BY id
	`, p.planetID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ships, defs []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.code, &r.start, &r.end); err != nil {
			t.Fatal(err)
		}
		if r.code == "202" {
			ships = append(ships, r)
		} else {
			defs = append(defs, r)
		}
	}
	if len(ships) != 2 || len(defs) != 1 {
		t.Fatalf("ships=%d defs=%d, want 2/1", len(ships), len(defs))
	}
	if ships[1].start.Before(ships[0].end) {
		t.Fatalf("second ship batch starts %v before first ends %v (not chained)", ships[1].start, ships[0].end)
	}
	if !defs[0].start.Before(ships[0].end) {
		t.Fatalf("defense batch start %v chained behind the ship line (first ship ends %v)", defs[0].start, ships[0].end)
	}
}

// TestEnqueueShipyardPartialWhenUnaffordable proves an oversized batch is capped
// to what the planet can pay for instead of erroring.
func TestEnqueueShipyardPartialWhenUnaffordable(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()
	p := newChainPlanet(t, db, ctx)
	// Light Cargo (202) is gated on Shipyard 2 + Combustion Drive 2 by the
	// tech-tree back-fill; it costs 2000 metal / 2000 crystal per unit.
	setStructure(ctx, t, db, p.planetID, "shipyard", 2)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO user_technologies (user_id, tech_code, level)
		VALUES ($1, 'combustion_drive', 2)
		ON CONFLICT (user_id, tech_code) DO UPDATE SET level = EXCLUDED.level
	`, p.userID); err != nil {
		t.Fatal(err)
	}
	// This funds exactly two Light Cargo.
	setResources(ctx, t, db, p.planetID, 5000, 5000, 0)

	got, err := NewBuildStore(db).EnqueueShipyard(ctx, p.planetID, "202", 5)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if got.Quantity != 2 {
		t.Fatalf("Quantity = %d, want 2 (partial)", got.Quantity)
	}
	if got.Cost.Metal != 4000 || got.Cost.Crystal != 4000 {
		t.Fatalf("Cost = %+v, want 4000/4000", got.Cost)
	}
	var total int64
	if err := db.QueryRowContext(ctx,
		`SELECT quantity_total FROM shipyard_queues WHERE id=$1`, got.QueueID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("queued quantity_total = %d, want 2", total)
	}
}

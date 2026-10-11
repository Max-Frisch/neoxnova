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

// TestEnqueueResearchNotCappedByLab verifies the intended niburu rule: the
// research lab only *unlocks* technologies (via the tech tree prerequisites),
// it does not cap the target level. A fresh account with lab 3 and a tech at 3
// must be able to queue level 4.
func TestEnqueueResearchNotCappedByLab(t *testing.T) {
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
	uname := fmt.Sprintf("rescaptest_%d", time.Now().UnixNano())
	hash, err := auth.HashPassword("s3cret-password!")
	if err != nil {
		t.Fatal(err)
	}
	userID, planetID, err := as.CreateUser(ctx, codeName, uname, uname+"@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM research_queues WHERE user_id = $1`, userID)
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)

	// research_lab 3 unlocks energy_tech (requires lab 1); set the tech to 3 so
	// the next target (4) is above the lab. Give plenty of resources.
	if _, err := db.ExecContext(ctx, `INSERT INTO planet_structures (celestial_id, structure_code, level) VALUES ($1,'research_lab',3)`, planetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO user_technologies (user_id, tech_code, level) VALUES ($1,'energy_tech',3), ($1,'espionage_tech',3), ($1,'impulse_drive',3), ($1,'combustion_drive',3)`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE celestial_objects SET metal=1e9, crystal=1e9, deuterium=1e9 WHERE id=$1`, planetID); err != nil {
		t.Fatal(err)
	}

	bs := NewBuildStore(db)
	if _, err := bs.EnqueueResearch(ctx, planetID, "energy_tech"); err != nil {
		t.Fatalf("research above lab level should be allowed, got %v", err)
	}

	// The tech-tree prerequisites must still gate unlocks: astrophysics needs
	// espionage_tech/impulse_drive 3 (met) but a lab is required too — drop it.
	if _, err := db.ExecContext(ctx, `UPDATE planet_structures SET level=0 WHERE celestial_id=$1 AND structure_code='research_lab'`, planetID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM research_queues WHERE user_id=$1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := bs.EnqueueResearch(ctx, planetID, "astrophysics"); !errors.Is(err, ErrPrerequisites) {
		t.Fatalf("astrophysics with no lab should be ErrPrerequisites, got %v", err)
	}
}

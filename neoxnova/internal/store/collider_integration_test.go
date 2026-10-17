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

// TestColliderIntegration exercises the Moon Collider store paths against a live
// Postgres: moon-only enforcement, the Antimatter spend, the level/output
// cache and the account-wide Dark Matter accrual. Skipped unless DATABASE_URL
// is set.
func TestColliderIntegration(t *testing.T) {
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
	var codeName, universeID string
	if err := db.QueryRowContext(ctx, `SELECT code_name, id::text FROM universes LIMIT 1`).Scan(&codeName, &universeID); err != nil {
		t.Skipf("no universe seeded: %v", err)
	}

	as := NewAuthStore(db)
	uname := fmt.Sprintf("collidertest_%d", time.Now().UnixNano())
	hash, err := auth.HashPassword("s3cret-password!")
	if err != nil {
		t.Fatal(err)
	}
	userID, planetID, err := as.CreateUser(ctx, codeName, uname, uname+"@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)

	moons := NewMoonStore(db)
	planets := NewPlanetStore(db)

	var moonID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO celestial_objects
			(universe_id, user_id, name, object_type, galaxy, system, position,
			 diameter_km, fields_used, fields_max, base_fields_max, temp_min, temp_max)
		VALUES ($1, $2, 'Collider Moon', 'MOON', 2, 50, 5, 8426, 0, 0, 0, -20, 40)
		RETURNING id
	`, universeID, userID).Scan(&moonID); err != nil {
		t.Fatalf("insert moon: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, moonID)

	moonIDstr := fmt.Sprintf("%d", moonID)

	// Moon-only enforcement: a planet and a foreign/missing moon are rejected.
	if _, err := moons.ColliderStatusFor(ctx, userID, planetID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("collider on planet = %v, want ErrNotFound", err)
	}
	if _, err := moons.ColliderStatusFor(ctx, userID+99999, moonID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("collider on foreign moon = %v, want ErrNotFound", err)
	}

	// Level 0: no output, next level is L1 at 4752 DM/h for 850 ATM.
	st, err := moons.ColliderStatusFor(ctx, userID, moonID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Level != 0 || st.ProductionPerHour != 0 || st.NextLevel != 1 ||
		st.NextProduction != 4752 || st.NextCostAtm != 850 || st.Antimatter != 0 {
		t.Fatalf("initial status = %+v", st)
	}

	// Not enough Antimatter.
	if _, err := moons.UpgradeCollider(ctx, userID, moonID); !errors.Is(err, ErrInsufficientAntimatter) {
		t.Fatalf("upgrade with 0 ATM = %v, want ErrInsufficientAntimatter", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET antimatter = 850 WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	st, err = moons.UpgradeCollider(ctx, userID, moonID)
	if err != nil {
		t.Fatalf("first upgrade: %v", err)
	}
	if st.Level != 1 || st.ProductionPerHour != 4752 || st.Antimatter != 0 {
		t.Fatalf("after L1 = %+v", st)
	}
	// The celestial + account caches agree with the level.
	var celestialProd, accountProd float64
	var level int
	db.QueryRowContext(ctx, `SELECT collider_level, collider_dm_prod_hourly FROM celestial_objects WHERE id=$1`, moonID).Scan(&level, &celestialProd)
	db.QueryRowContext(ctx, `SELECT dark_matter_prod_hourly FROM users WHERE id=$1`, userID).Scan(&accountProd)
	if level != 1 || celestialProd != 4752 || accountProd != 4752 {
		t.Fatalf("caches after L1: level=%d celestial=%v account=%v", level, celestialProd, accountProd)
	}

	// Second level costs 1148.
	if _, err := db.ExecContext(ctx, `UPDATE users SET antimatter = 1147 WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := moons.UpgradeCollider(ctx, userID, moonID); !errors.Is(err, ErrInsufficientAntimatter) {
		t.Fatalf("upgrade with 1147 ATM = %v, want ErrInsufficientAntimatter", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET antimatter = 1148 WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	st, err = moons.UpgradeCollider(ctx, userID, moonID)
	if err != nil {
		t.Fatalf("second upgrade: %v", err)
	}
	if st.Level != 2 || st.ProductionPerHour != 10454 || st.NextCostAtm != 1549 {
		t.Fatalf("after L2 = %+v", st)
	}

	// Dark Matter accrues account-wide on read at the cached rate.
	if _, err := db.ExecContext(ctx, `
		UPDATE users SET dark_matter = 0, dark_matter_acc = 0,
		       dark_matter_prod_hourly = 10454,
		       dark_matter_calc_at = NOW() - interval '1 hour'
		WHERE id = $1
	`, userID); err != nil {
		t.Fatal(err)
	}
	rs, err := planets.GetResourceState(ctx, moonIDstr)
	if err != nil {
		t.Fatalf("GetResourceState: %v", err)
	}
	if rs.DarkMatter < 10454 {
		t.Fatalf("accrued DM = %d, want >= 10454", rs.DarkMatter)
	}
	if rs.DarkMatterHourlyRate != 10454 {
		t.Fatalf("DM rate = %v, want 10454", rs.DarkMatterHourlyRate)
	}

	// The overview carries the collider level + this moon's DM/h.
	ov, err := planets.GetOverview(ctx, moonIDstr)
	if err != nil {
		t.Fatalf("GetOverview: %v", err)
	}
	if ov.ColliderLevel != 2 || ov.ColliderDMPerHour != 10454 {
		t.Fatalf("overview collider = level %d dm/h %v, want 2/10454", ov.ColliderLevel, ov.ColliderDMPerHour)
	}

	// The reference game catalog agrees on the captured curve.
	if game.ColliderCost(1) != 850 || game.ColliderProduction(2) != 10454 {
		t.Fatal("game collider catalog drifted from the captured values")
	}
}

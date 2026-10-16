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

// TestPeacefulStoreLifecycle covers peaceful time accrual, the derived level and
// the Online Bonus button cooldown.
func TestPeacefulStoreLifecycle(t *testing.T) {
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
	uname := fmt.Sprintf("peacetst_%d", time.Now().UnixNano())
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

	// Seed a high peaceful total, the button ready and a 10-minute-old seen time.
	if _, err := db.ExecContext(ctx, `
		UPDATE users SET peaceful_seconds = 1e9, peaceful_seen_at = NOW() - interval '10 minutes',
		    bonus_button_ready_at = NOW(), bonus_last_click_at = NULL
		WHERE id = $1
	`, userID); err != nil {
		t.Fatal(err)
	}

	ps := NewPeacefulStore(db)
	st, err := ps.Status(ctx, userID)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Level < 1 {
		t.Fatalf("level = %d, want > 0 for 1e9 seconds", st.Level)
	}
	if !st.BonusReady {
		t.Fatalf("button should be ready")
	}

	// Accrual credits at most the per-gap cap.
	if err := ps.AccrueOnlineTime(ctx, userID); err != nil {
		t.Fatalf("accrue: %v", err)
	}
	after, err := ps.Status(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if gained := after.Seconds - st.Seconds; gained <= 0 || gained > peacefulAccrualCap {
		t.Fatalf("accrued %d seconds, want 1..%d", gained, peacefulAccrualCap)
	}

	// First claim pays out; the immediate second is refused.
	reward, err := ps.ClaimBonusButton(ctx, userID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if reward.DarkMatter <= 0 {
		t.Fatalf("reward DM = %d, want > 0", reward.DarkMatter)
	}
	if _, err := ps.ClaimBonusButton(ctx, userID); !errors.Is(err, ErrBonusNotReady) {
		t.Fatalf("second claim = %v, want ErrBonusNotReady", err)
	}

	var dm int64
	if err := db.QueryRowContext(ctx, `SELECT dark_matter FROM users WHERE id = $1`, userID).Scan(&dm); err != nil {
		t.Fatal(err)
	}
	if dm != reward.DarkMatter {
		t.Fatalf("dark_matter = %d, want %d", dm, reward.DarkMatter)
	}
	// The resbar reads its balances from the planet's resource state, so the
	// rewarded DM must surface there too (regression: it used to always be 0).
	rs, err := NewPlanetStore(db).GetResourceState(ctx, fmt.Sprintf("%d", planetID))
	if err != nil {
		t.Fatalf("resource state: %v", err)
	}
	if rs.DarkMatter != reward.DarkMatter {
		t.Fatalf("resource state DM = %d, want %d", rs.DarkMatter, reward.DarkMatter)
	}
}

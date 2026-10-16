package store

import (
	"context"
	"errors"
	"testing"

	"neoxnova/internal/game"
)

// TestGovernatorLifecycle exercises the two-phase governator model against a
// real database: improve (Achievement Points), hire (Dark Matter per day) and
// expiry. Skipped unless DATABASE_URL is set.
func TestGovernatorLifecycle(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()
	p := newChainPlanet(t, db, ctx)
	ss := NewSenateStore(db)

	if _, err := db.ExecContext(ctx, `UPDATE users SET achievement_points = 2000 WHERE id = $1`, p.userID); err != nil {
		t.Fatal(err)
	}

	// Improve Resource Upgrade 0 -> 10 costs 1,045 AP and locks the level in.
	cost, err := ss.ImproveGovernator(ctx, p.userID, game.GovCodeResource, 10)
	if err != nil {
		t.Fatalf("improve: %v", err)
	}
	if cost != 1045 {
		t.Fatalf("improve cost = %d, want 1045", cost)
	}
	if ap, _ := ss.AchievementPoints(ctx, p.userID); ap != 955 {
		t.Fatalf("achievement points = %d, want 955", ap)
	}
	states, _ := ss.GovernatorStates(ctx, p.userID)
	if got := states[game.GovCodeResource].Level; got != 10 {
		t.Fatalf("level = %d, want 10", got)
	}
	// Improving to an equal-or-lower level is rejected.
	if _, err := ss.ImproveGovernator(ctx, p.userID, game.GovCodeResource, 10); !errors.Is(err, ErrGovernatorLevel) {
		t.Fatalf("re-improve = %v, want ErrGovernatorLevel", err)
	}

	// Insufficient Achievement Points.
	if _, err := db.ExecContext(ctx, `UPDATE users SET achievement_points = 10 WHERE id = $1`, p.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := ss.ImproveGovernator(ctx, p.userID, game.GovCodeEnergy, 5); !errors.Is(err, ErrInsufficientAchievement) {
		t.Fatalf("broke improve = %v, want ErrInsufficientAchievement", err)
	}

	// Hire Resource Upgrade for 1 day at level 10 -> 37,293 Dark Matter.
	if _, err := db.ExecContext(ctx, `UPDATE users SET dark_matter = 100000 WHERE id = $1`, p.userID); err != nil {
		t.Fatal(err)
	}
	total, err := ss.HireGovernator(ctx, p.userID, game.GovCodeResource, 1)
	if err != nil {
		t.Fatalf("hire: %v", err)
	}
	if total != 37293 {
		t.Fatalf("hire total = %d, want 37293", total)
	}
	var dm int64
	if err := db.QueryRowContext(ctx, `SELECT dark_matter FROM users WHERE id=$1`, p.userID).Scan(&dm); err != nil {
		t.Fatal(err)
	}
	if dm != 100000-37293 {
		t.Fatalf("dark matter = %d, want %d", dm, 100000-37293)
	}
	states, _ = ss.GovernatorStates(ctx, p.userID)
	if !states[game.GovCodeResource].ExpiresAt.Valid {
		t.Fatal("resource governator should be active after hire")
	}

	// It is listed as active.
	active, _ := ss.ActiveGovernators(ctx, p.userID)
	if active[game.GovCodeResource] != 10 {
		t.Fatalf("active[resource] = %d, want 10", active[game.GovCodeResource])
	}

	// Expiry keeps the bought level but clears the activation.
	if _, err := db.ExecContext(ctx, `
		UPDATE user_governators SET expires_at = NOW() - interval '1 minute'
		WHERE user_id = $1 AND governator_code = $2`, p.userID, game.GovCodeResource); err != nil {
		t.Fatal(err)
	}
	if err := ss.ExpireGovernators(ctx); err != nil {
		t.Fatalf("expire: %v", err)
	}
	states, _ = ss.GovernatorStates(ctx, p.userID)
	if states[game.GovCodeResource].ExpiresAt.Valid {
		t.Fatal("expiry should clear the activation")
	}
	if states[game.GovCodeResource].Level != 10 {
		t.Fatalf("expiry changed the level to %d, want 10", states[game.GovCodeResource].Level)
	}
}

// TestGovernatorProductionBonusApplied proves an active Resource governator
// raises the celestial's cached hourly production and reverts on expiry.
func TestGovernatorProductionBonusApplied(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()
	p := newChainPlanet(t, db, ctx)
	setStructure(ctx, t, db, p.planetID, "metal_mine", 10)
	setResources(ctx, t, db, p.planetID, 1e6, 1e6, 1e6)

	read := func() float64 {
		t.Helper()
		var v float64
		if err := db.QueryRowContext(ctx, `SELECT metal_prod_hourly FROM celestial_objects WHERE id=$1`, p.planetID).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	bs := NewBuildStore(db)
	if err := bs.RecomputeAccountProduction(ctx, p.userID); err != nil {
		t.Fatalf("baseline recompute: %v", err)
	}
	base := read()

	ss := NewSenateStore(db)
	if _, err := db.ExecContext(ctx, `UPDATE users SET achievement_points = 5000, dark_matter = 1e9 WHERE id=$1`, p.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := ss.ImproveGovernator(ctx, p.userID, game.GovCodeResource, 10); err != nil {
		t.Fatalf("improve: %v", err)
	}
	if _, err := ss.HireGovernator(ctx, p.userID, game.GovCodeResource, 1); err != nil {
		t.Fatalf("hire: %v", err)
	}
	boosted := read()
	if boosted <= base {
		t.Fatalf("active resource governator did not raise production: base=%v boosted=%v", base, boosted)
	}

	// Expire it: production reverts.
	if _, err := db.ExecContext(ctx, `
		UPDATE user_governators SET expires_at = NOW() - interval '1 minute'
		WHERE user_id = $1 AND governator_code = $2`, p.userID, game.GovCodeResource); err != nil {
		t.Fatal(err)
	}
	if err := ss.ExpireGovernators(ctx); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if got := read(); !almostEqual(got, base) {
		t.Fatalf("production after expiry = %v, want %v", got, base)
	}
}

func almostEqual(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.01
}

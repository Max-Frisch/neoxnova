package store

import (
	"context"
	"errors"
	"testing"

	"neoxnova/internal/game"
)

// TestAcademyLifecycle exercises the Academy skill tree against a real database:
// purchasing levels with Academy points, prerequisite gating, partial purchases
// and branch resets. Skipped unless DATABASE_URL is set.
func TestAcademyLifecycle(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()
	p := newChainPlanet(t, db, ctx)
	as := NewAcademyStore(db)

	if _, err := db.ExecContext(ctx, `UPDATE users SET academy_points = 5000 WHERE id = $1`, p.userID); err != nil {
		t.Fatal(err)
	}

	// Weaponry (root) 0 -> 5: buy 5 levels.
	weaponry, _ := game.AcademySkillByCode(game.SkillWeaponry)
	wantWeaponryCost := game.AcademyCostRange(weaponry, 0, 5)
	up, err := as.Upgrade(ctx, p.userID, game.SkillWeaponry, 5)
	if err != nil {
		t.Fatalf("upgrade Weaponry: %v", err)
	}
	if up.Level != 5 || up.Partial || up.Spent != wantWeaponryCost {
		t.Fatalf("Weaponry upgrade = %+v, want level 5 / spent %d / full", up, wantWeaponryCost)
	}
	if pts, _ := as.Points(ctx, p.userID); pts != 5000-wantWeaponryCost {
		t.Fatalf("points = %d, want %d", pts, 5000-wantWeaponryCost)
	}

	// Weaponry is now a prerequisite for Weapons Class A.
	up, err = as.Upgrade(ctx, p.userID, game.SkillWeaponsClassA, 1)
	if err != nil {
		t.Fatalf("upgrade Weapons Class A: %v", err)
	}
	if up.Level != 1 {
		t.Fatalf("Weapons Class A level = %d, want 1", up.Level)
	}

	// Prerequisite gating: Double attack needs Weapons Class A 7.
	if _, err := as.Upgrade(ctx, p.userID, game.SkillDoubleAttack, 1); !errors.Is(err, ErrAcademyPrereq) {
		t.Fatalf("gated upgrade = %v, want ErrAcademyPrereq", err)
	}

	// Partial purchase: only enough points for a couple of levels.
	if _, err := db.ExecContext(ctx, `UPDATE users SET academy_points = 4 WHERE id = $1`, p.userID); err != nil {
		t.Fatal(err)
	}
	partial, err := as.Upgrade(ctx, p.userID, game.SkillWeaponry, 50)
	if err != nil {
		t.Fatalf("partial upgrade: %v", err)
	}
	if !partial.Partial || partial.Level <= 5 {
		t.Fatalf("partial = %+v, want partial above level 5", partial)
	}

	// Fully broke: cannot afford even one level.
	if _, err := db.ExecContext(ctx, `UPDATE users SET academy_points = 0 WHERE id = $1`, p.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := as.Upgrade(ctx, p.userID, game.SkillWeaponry, 60); !errors.Is(err, ErrInsufficientAcademyPoints) {
		t.Fatalf("broke upgrade = %v, want ErrInsufficientAcademyPoints", err)
	}

	// Economy branch: buy Production speed, then reset the branch for free (50%).
	levels, _ := as.Skills(ctx, p.userID)
	_ = levels
	if _, err := db.ExecContext(ctx, `UPDATE users SET academy_points = 500 WHERE id = $1`, p.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := as.Upgrade(ctx, p.userID, game.SkillProductionSpeed, 3); err != nil {
		t.Fatalf("upgrade Production speed: %v", err)
	}
	spentEconomy := game.AcademyCostRange(mustSkill(t, game.SkillProductionSpeed), 0, 3)
	res, err := as.ResetBranch(ctx, p.userID, game.AcademyEconomy, AcademyResetFreePct)
	if err != nil {
		t.Fatalf("reset economy: %v", err)
	}
	if res.Refunded != spentEconomy/2 || res.Cost != 0 {
		t.Fatalf("free reset = %+v, want refund %d / cost 0", res, spentEconomy/2)
	}
	after, _ := as.Skills(ctx, p.userID)
	if after[game.SkillProductionSpeed] != 0 {
		t.Fatalf("production speed after reset = %d, want 0", after[game.SkillProductionSpeed])
	}
	// Resetting an empty branch is rejected.
	if _, err := as.ResetBranch(ctx, p.userID, game.AcademyEconomy, AcademyResetFreePct); !errors.Is(err, ErrNothingToReset) {
		t.Fatalf("empty reset = %v, want ErrNothingToReset", err)
	}

	// Full reset needs Antimatter.
	if _, err := db.ExecContext(ctx, `UPDATE users SET academy_points = 500, antimatter = 0 WHERE id = $1`, p.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := as.Upgrade(ctx, p.userID, game.SkillDefensiveStrategy, 3); err != nil {
		t.Fatalf("upgrade Defensive strategy: %v", err)
	}
	if _, err := as.ResetBranch(ctx, p.userID, game.AcademyDefence, AcademyResetFullPct); !errors.Is(err, ErrInsufficientAntimatter) {
		t.Fatalf("antimatter reset = %v, want ErrInsufficientAntimatter", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET antimatter = 20000 WHERE id = $1`, p.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := as.ResetBranch(ctx, p.userID, game.AcademyDefence, AcademyResetFullPct); err != nil {
		t.Fatalf("full reset: %v", err)
	}
}

func mustSkill(t *testing.T, code string) game.AcademySkill {
	t.Helper()
	s, ok := game.AcademySkillByCode(code)
	if !ok {
		t.Fatalf("unknown skill %s", code)
	}
	return s
}

// TestAcademyShipCostDiscount proves Standardisation (Fleet) reduces the
// resources deducted when building ships in the shipyard.
func TestAcademyShipCostDiscount(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()
	p := newChainPlanet(t, db, ctx)
	bs := NewBuildStore(db)

	setStructure(ctx, t, db, p.planetID, "shipyard", 5)
	setStructure(ctx, t, db, p.planetID, "nanite_factory", 5)
	setResources(ctx, t, db, p.planetID, 1e12, 1e12, 1e12)

	// 20% fleet-cost reduction.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO user_academy_skills (user_id, skill_code, level) VALUES ($1, '1104', 20)
		ON CONFLICT (user_id, skill_code) DO UPDATE SET level = 20
	`, p.userID); err != nil {
		t.Fatal(err)
	}

	base, _ := game.UnitCost("204", 1)
	want := game.DiscountCost(base, 20)

	var before float64
	if err := db.QueryRowContext(ctx, `SELECT metal FROM celestial_objects WHERE id = $1`, p.planetID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := bs.EnqueueShipyard(ctx, p.planetID, "204", 1); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	var after float64
	if err := db.QueryRowContext(ctx, `SELECT metal FROM celestial_objects WHERE id = $1`, p.planetID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if int64(before-after) != want.Metal {
		t.Fatalf("metal deducted = %d, want %d (discounted from %d)", int64(before-after), want.Metal, base.Metal)
	}
}

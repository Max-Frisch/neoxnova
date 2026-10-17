package game

import "testing"

// TestColliderCost locks the captured Antimatter cost series
// (850 / 1148 / 1549) and the ×1.35 growth.
func TestColliderCost(t *testing.T) {
	cases := map[int]int64{0: 850, 1: 850, 2: 1148, 3: 1549}
	for level, want := range cases {
		if got := ColliderCost(level); got != want {
			t.Errorf("ColliderCost(%d) = %d, want %d", level, got, want)
		}
	}
	// Growth is a clean ×1.35.
	if prev := ColliderCost(1); ColliderCost(2) != int64(1.35*float64(prev)+0.5) {
		t.Errorf("ColliderCost(2) = %d, not ×1.35 of %d", ColliderCost(2), prev)
	}
}

// TestColliderProductionTable locks the captured DM/h values and the level-0
// zero, and checks the provisional extrapolation stays monotonic.
func TestColliderProductionTable(t *testing.T) {
	cases := map[int]int64{0: 0, 1: 4752, 2: 10454, 3: 17250}
	for level, want := range cases {
		if got := ColliderProduction(level); got != want {
			t.Errorf("ColliderProduction(%d) = %d, want %d", level, got, want)
		}
	}
	prev := ColliderProduction(3)
	for level := 4; level <= 10; level++ {
		got := ColliderProduction(level)
		if got <= prev {
			t.Fatalf("ColliderProduction(%d) = %d not increasing past %d", level, got, prev)
		}
		prev = got
	}
}

// TestColliderMultipliers verifies the combat-level and premium hooks scale the
// base production and that zero multipliers are a no-op.
func TestColliderMultipliers(t *testing.T) {
	base := ColliderProduction(2)
	if got := ColliderProductionFor(2, ColliderMultipliers{}); got != base {
		t.Errorf("ColliderProductionFor no-op = %d, want %d", got, base)
	}
	// +3% per combat level: level 2 with combat level 10 → ×1.30.
	if got := ColliderProductionFor(2, ColliderMultipliers{CombatLevel: 10}); got != int64(float64(base)*1.3+0.5) {
		t.Errorf("ColliderProductionFor combat = %d, want ~%d", got, int64(float64(base)*1.3))
	}
	// Premium is additive on its own factor.
	if got := ColliderProductionFor(2, ColliderMultipliers{PremiumPct: 100}); got != base*2 {
		t.Errorf("ColliderProductionFor premium = %d, want %d", got, base*2)
	}
	if got := ColliderProductionFor(0, ColliderMultipliers{CombatLevel: 5}); got != 0 {
		t.Errorf("ColliderProductionFor(0) = %d, want 0", got)
	}
}

// TestColliderMoonOnly ensures the collider is restricted to moons.
func TestColliderMoonOnly(t *testing.T) {
	if !ColliderMoonOnly("MOON") {
		t.Error("ColliderMoonOnly(MOON) = false, want true")
	}
	if ColliderMoonOnly("PLANET") {
		t.Error("ColliderMoonOnly(PLANET) = true, want false")
	}
	if ColliderDef.ID != ColliderID || ColliderDef.Code != ColliderCode {
		t.Errorf("ColliderDef = %+v, want id %d code %q", ColliderDef, ColliderID, ColliderCode)
	}
}

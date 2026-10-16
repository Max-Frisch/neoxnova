package game

import "testing"

// TestGovernatorBonus is the captured live formula: floor((0.1 + 0.01·level)·100)
// = 10 + level.
func TestGovernatorBonus(t *testing.T) {
	def, ok := GovernatorByCode(GovCodeWeapons)
	if !ok {
		t.Fatal("weapons governator missing")
	}
	for _, tc := range []struct{ level, want int }{{0, 10}, {4, 14}, {16, 26}, {26, 36}, {65, 75}} {
		if got := def.BonusPct(tc.level); got != tc.want {
			t.Errorf("weapons bonus at level %d = %d, want %d", tc.level, got, tc.want)
		}
	}
}

// TestGovernatorAPCost replays the captured Resource Upgrade improve 0→10, which
// cost exactly 1,045 Achievement Points (AllRep 3,849 → 2,804).
func TestGovernatorAPCost(t *testing.T) {
	def, _ := GovernatorByCode(GovCodeResource)
	if got := def.APCost(0, 10); got != 1045 {
		t.Errorf("resource APCost(0,10) = %d, want 1045", got)
	}
	// Reaching a target sums the levels strictly above the current level.
	if got := def.APCost(4, 4); got != 0 {
		t.Errorf("APCost(4,4) = %d, want 0", got)
	}
	// The four-level preview captured for Weapons (0→4) summed to 222.
	w, _ := GovernatorByCode(GovCodeWeapons)
	if got := w.APCost(0, 4); got != 222 {
		t.Errorf("weapons APCost(0,4) = %d, want 222", got)
	}
}

// TestGovernatorDMPrice replays the captured per-day prices (level 10 resource =
// 37,294; the four-level previews).
func TestGovernatorDMPrice(t *testing.T) {
	r, _ := GovernatorByCode(GovCodeResource)
	// The live page rendered 37,293 at level 10 (the actual debit differed by 1
	// Dark Matter, a display-rounding artefact); we match the rendered price.
	if got := r.DMPricePerDay(10); got != 37293 {
		t.Errorf("resource DM/day at level 10 = %d, want 37293", got)
	}
	if got := r.DMPricePerDay(0); got != 30000 {
		t.Errorf("resource DM/day at level 0 = %d, want 30000", got)
	}
	w, _ := GovernatorByCode(GovCodeWeapons)
	if got := w.DMPricePerDay(4); got != 58564 {
		t.Errorf("weapons DM/day at level 4 = %d, want 58564", got)
	}
	f, _ := GovernatorByCode(GovCodeFleet)
	if got := f.DMPricePerDay(4); got != 87450 {
		t.Errorf("fleet DM/day at level 4 = %d, want 87450", got)
	}
}

func TestGovernatorProductionBonus(t *testing.T) {
	b := GovernatorProductionBonus(map[int]int{GovCodeResource: 10, GovCodeEnergy: 5})
	if b.Metal != 20 || b.Crystal != 20 || b.Deuterium != 20 {
		t.Fatalf("resource governator bonus = %+v, want all 20", b)
	}
	if b.Energy != 15 {
		t.Fatalf("energy governator bonus = %v, want 15", b.Energy)
	}
	if empty := GovernatorProductionBonus(nil); empty != (ProductionBonus{}) {
		t.Fatalf("empty active map = %+v, want zero", empty)
	}
}

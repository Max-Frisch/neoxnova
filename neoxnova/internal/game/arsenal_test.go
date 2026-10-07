package game

import (
	"reflect"
	"testing"
)

func TestArsenalCatalog(t *testing.T) {
	if len(Upgrades) != 19 {
		t.Fatalf("catalog has %d upgrades, want 19", len(Upgrades))
	}
	jet, ok := UpgradeByCode(11)
	if !ok || jet.Name != "Jet engine" || jet.PerLevel != 0.6 {
		t.Fatalf("upgrade 11 = %+v ok=%v", jet, ok)
	}
	if got, ok := UpgradeByKey("combustion"); !ok || got.Code != 11 {
		t.Fatalf("combustion key = %+v ok=%v, want code 11", got, ok)
	}
	if _, ok := UpgradeByCode(20); ok {
		t.Fatal("code 20 should not resolve")
	}
	for code, def := range Upgrades {
		if def.Code != code || def.Name == "" || def.Group == "" || def.PerLevel <= 0 {
			t.Fatalf("malformed upgrade %d: %+v", code, def)
		}
	}
}

func TestArsenalTier(t *testing.T) {
	cases := []struct {
		points int64
		want   int
	}{
		{0, 0}, {4999, 0},
		{5000, 1}, {49999, 1},
		{50000, 2}, {249999, 2},
		{250000, 3}, {1_000_000, 3},
	}
	for _, c := range cases {
		if got := ArsenalTier(c.points); got != c.want {
			t.Fatalf("ArsenalTier(%d) = %d, want %d", c.points, got, c.want)
		}
	}
	if ArsenalTierMinPoints(1) != 5000 || ArsenalTierMinPoints(2) != 50000 || ArsenalTierMinPoints(3) != 250000 {
		t.Fatal("tier lower bounds drifted")
	}
	if DropPool(0) != nil {
		t.Fatal("tier 0 must have no drop pool")
	}
	has := func(pool []int, code int) bool {
		for _, c := range pool {
			if c == code {
				return true
			}
		}
		return false
	}
	t1, t2, t3 := DropPool(1), DropPool(2), DropPool(3)
	if !has(t1, 11) || has(t1, 12) {
		t.Fatalf("tier 1 pool wrong: %v", t1)
	}
	if !has(t2, 12) || has(t2, 13) {
		t.Fatalf("tier 2 pool wrong: %v", t2)
	}
	if !has(t3, 13) || !has(t3, 4) {
		t.Fatalf("tier 3 pool wrong: %v", t3)
	}
}

// TestDropPoolsPartitionEveryUpgrade locks the owner-confirmed categorization:
// each of the 19 upgrades is in exactly one light/medium/heavy pool, and the
// tiers are cumulative (the tier gates the pool, not the flat drop chance).
func TestDropPoolsPartitionEveryUpgrade(t *testing.T) {
	seen := map[int]int{}
	for _, pool := range [][]int{lightPool, mediumPool, heavyPool} {
		for _, code := range pool {
			if _, ok := UpgradeByCode(code); !ok {
				t.Fatalf("pool references unknown upgrade %d", code)
			}
			seen[code]++
		}
	}
	if len(seen) != 19 {
		t.Fatalf("pools cover %d upgrades, want 19", len(seen))
	}
	for code, n := range seen {
		if n != 1 {
			t.Fatalf("upgrade %d appears in %d pools, want 1", code, n)
		}
	}

	light := []int{1, 5, 8, 11, 14, 17, 18, 19}
	medium := append(append([]int{}, light...), 2, 3, 6, 9, 12, 15)
	heavy := append(append([]int{}, medium...), 4, 7, 10, 13, 16)
	for _, tc := range []struct {
		tier int
		want []int
	}{{1, light}, {2, medium}, {3, heavy}} {
		if got := DropPool(tc.tier); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("DropPool(%d) = %v, want %v", tc.tier, got, tc.want)
		}
	}
}

func TestRollDrop(t *testing.T) {
	// A chance roll at/above the drop chance never yields an item.
	if code, ok := RollDrop(1, ArsenalDropChance, 0); ok || code != 0 {
		t.Fatalf("miss roll dropped %d ok=%v, want none", code, ok)
	}
	// Deterministic pick: pickRoll 0 -> first pool entry, ~1 -> last.
	if code, ok := RollDrop(1, 0, 0); !ok || code != 1 {
		t.Fatalf("tier 1 first pick = %d ok=%v, want 1", code, ok)
	}
	if code, ok := RollDrop(3, 0, 0.999999); !ok || code != 16 {
		t.Fatalf("tier 3 last pick = %d ok=%v, want 16", code, ok)
	}
	// Tier 0 has no pool, so it can never drop.
	if code, ok := RollDrop(0, 0, 0); ok || code != 0 {
		t.Fatalf("tier 0 dropped %d ok=%v, want none", code, ok)
	}
}

func TestActivationChance(t *testing.T) {
	cases := []struct {
		level int
		want  float64
	}{
		{0, 1.0}, {9, 1.0},
		{10, 0.98}, {11, 0.96}, {21, 0.76}, {22, 0.75}, {50, 0.75},
	}
	for _, c := range cases {
		if got := ActivationChance(c.level); got != c.want {
			t.Fatalf("ActivationChance(%d) = %v, want %v", c.level, got, c.want)
		}
	}
}

func TestActivate(t *testing.T) {
	def, _ := UpgradeByCode(1) // Laser weapons, +0.75/level

	// Guaranteed success: level 0 -> 1, value gains PerLevel.
	lvl, val, ok, chance := Activate(def, 0, 0, 0.999)
	if !ok || lvl != 1 || chance != 1.0 {
		t.Fatalf("guaranteed activation: lvl=%d ok=%v chance=%v", lvl, ok, chance)
	}
	approx(t, "value", val, 0.75)

	// Failure above level 10: level stays, value loses 10% of PerLevel.
	lvl, val, ok, chance = Activate(def, 10, 7.5, 0.999)
	if ok || lvl != 10 {
		t.Fatalf("failed activation: lvl=%d ok=%v", lvl, ok)
	}
	approx(t, "chance", chance, 0.98)
	approx(t, "value", val, 7.425)

	// Value never goes negative.
	_, val, _, _ = Activate(def, 10, 0.0, 0.999)
	approx(t, "value floor", val, 0.0)
}

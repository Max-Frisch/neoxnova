package game

import "testing"

// TestRelocationCostCaptured locks the cost curve to the Planetarium samples
// captured on 2026-10-06 (planet 3:125:12, docs/screenshots_niburu/).
func TestRelocationCostCaptured(t *testing.T) {
	const fg, fs, fp = 3, 125, 12
	cases := []struct {
		tg, ts, tp int
		want       int64
	}{
		{3, 125, 12, 0},     // same coordinates
		{3, 125, 13, 2500},  // +1 position
		{3, 125, 14, 5000},  // +2 positions
		{3, 125, 15, 7500},  // +3 positions
		{3, 126, 12, 1000},  // +1 system
		{3, 126, 13, 3500},  // +1 system +1 position
		{3, 126, 14, 6000},  // +1 system +2 positions
		{3, 127, 12, 2000},  // +2 systems
		{3, 127, 13, 4500},  // +2 systems +1 position
		{3, 127, 14, 7000},  // +2 systems +2 positions
		{2, 125, 12, 15000}, // +1 galaxy
		{2, 126, 13, 18500}, // +1 galaxy +1 system +1 position
		{2, 127, 14, 22000}, // +1 galaxy +2 systems +2 positions
	}
	for _, c := range cases {
		if got := RelocationCost(fg, fs, fp, c.tg, c.ts, c.tp); got != c.want {
			t.Errorf("RelocationCost(3:125:12 -> %d:%d:%d) = %d, want %d", c.tg, c.ts, c.tp, got, c.want)
		}
	}
}

func TestRelocationCostLinear(t *testing.T) {
	// Each axis is independent and monotonic: moving further always costs more.
	if RelocationCost(1, 1, 1, 1, 1, 2) != 2500 {
		t.Fatal("position step should be 2500")
	}
	if RelocationCost(1, 1, 1, 1, 2, 1) != 1000 {
		t.Fatal("system step should be 1000")
	}
	if RelocationCost(1, 1, 1, 2, 1, 1) != 15000 {
		t.Fatal("galaxy step should be 15000")
	}
	// Symmetry.
	if RelocationCost(2, 30, 5, 4, 10, 9) != RelocationCost(4, 10, 9, 2, 30, 5) {
		t.Fatal("relocation cost should be symmetric")
	}
}

func TestRelocationLeavesSystem(t *testing.T) {
	cases := []struct {
		fg, fs, tg, ts int
		want           bool
	}{
		{3, 125, 3, 125, false}, // same system
		{3, 125, 3, 126, true},
		{3, 125, 2, 125, true},
	}
	for _, c := range cases {
		if got := RelocationLeavesSystem(c.fg, c.fs, c.tg, c.ts); got != c.want {
			t.Errorf("RelocationLeavesSystem(%d,%d,%d,%d) = %v, want %v", c.fg, c.fs, c.tg, c.ts, got, c.want)
		}
	}
}

func TestValidRelocationTarget(t *testing.T) {
	const perSystem = 20
	cases := []struct {
		g, s, p int
		want    bool
	}{
		{1, 1, 1, true},
		{9, 499, 20, true},
		{0, 1, 1, false},
		{10, 1, 1, false},
		{1, 0, 1, false},
		{1, 500, 1, false},
		{1, 1, 0, false},
		{1, 1, 21, false}, // expedition slot is never a planet
	}
	for _, c := range cases {
		if got := ValidRelocationTarget(c.g, c.s, c.p, perSystem); got != c.want {
			t.Errorf("ValidRelocationTarget(%d,%d,%d) = %v, want %v", c.g, c.s, c.p, got, c.want)
		}
	}
}

package game

import "testing"

func TestMoonDiameterKm(t *testing.T) {
	cases := []struct {
		pct, roll, want int
	}{
		{20, 11, 8426}, // the live acc1 moon
		{20, 10, 8366}, // minimum at the 20% cap
		{20, 20, 8944}, // maximum
		{1, 10, 3605},  // minimum overall
		// Out-of-range rolls clamp into [10,20].
		{20, 0, 8366},
		{20, 99, 8944},
		// A chance below 1% is treated as 1%.
		{0, 10, 3605},
	}
	for _, c := range cases {
		if got := MoonDiameterKm(c.pct, c.roll); got != c.want {
			t.Errorf("MoonDiameterKm(%d,%d) = %d, want %d", c.pct, c.roll, got, c.want)
		}
	}
}

func TestMoonFields(t *testing.T) {
	if got := MoonFields(8426); got != 70 { // (8.426)^2 = 70.99 -> 70
		t.Fatalf("MoonFields(8426) = %d, want 70", got)
	}
	if got := MoonFields(0); got != 0 {
		t.Fatalf("MoonFields(0) = %d, want 0", got)
	}
}

func TestMoonDestruction(t *testing.T) {
	// 8,426 km, 1 Battle Fortress: (100-sqrt(8426))*1 ~= 8.21%, rip loss ~= 45.9%.
	destroy, loss := MoonDestruction(8426, 1)
	if destroy < 8.1 || destroy > 8.3 {
		t.Fatalf("destroy(8426,1) = %.3f, want ~8.21", destroy)
	}
	if loss < 45.8 || loss > 46.0 {
		t.Fatalf("ripLoss(8426,1) = %.3f, want ~45.9", loss)
	}
	// Enough Battle Fortresses to guarantee the kill.
	if d, _ := MoonDestruction(8426, 150); d != 100 {
		t.Fatalf("destroy(8426,150) = %.2f, want capped 100", d)
	}
	// No ships / no moon -> no chance.
	if d, l := MoonDestruction(8426, 0); d != 0 || l != 0 {
		t.Fatalf("destroy(8426,0) = %v,%v want 0,0", d, l)
	}
	// >= 10,000 km is indestructible.
	if d, l := MoonDestruction(10000, 500); d != 0 || l != 0 {
		t.Fatalf("destroy(10000,500) = %v,%v want 0,0", d, l)
	}
}

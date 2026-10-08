package game

import (
	"testing"
	"time"
)

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

func TestMoonFieldsMax(t *testing.T) {
	// Fresh moon: 1 base field. The account's +2 premium ("+2 fields on the moon")
	// makes the observed 0-used / 3-max on the page; Moon base 20 = 61 +2 = 63.
	if got := MoonFieldsMax(0); got != 1 {
		t.Fatalf("MoonFieldsMax(0) = %d, want 1 (fresh moon base)", got)
	}
	if got := MoonFieldsMax(20); got != 61 {
		t.Fatalf("MoonFieldsMax(20) = %d, want 61", got)
	}
}

func TestMoonCreation(t *testing.T) {
	// Zero chance never creates.
	if created, dia := MoonCreation(0, 12345); created || dia != 0 {
		t.Fatalf("chance 0 created=%v dia=%d, want false/0", created, dia)
	}
	// 100% always creates, with a diameter in the 20% band (8,366..8,944 km).
	for seed := int64(0); seed < 50; seed++ {
		created, dia := MoonCreation(100, seed)
		if !created {
			t.Fatalf("chance 100 seed %d did not create", seed)
		}
		if dia < 8366 || dia > 8944 {
			t.Fatalf("chance 100 seed %d dia=%d out of [8366,8944]", seed, dia)
		}
	}
	// Deterministic for a given seed.
	c1, d1 := MoonCreation(20, 999)
	c2, d2 := MoonCreation(20, 999)
	if c1 != c2 || d1 != d2 {
		t.Fatalf("MoonCreation not deterministic: (%v,%d) vs (%v,%d)", c1, d1, c2, d2)
	}
	// A low chance yields a valid diameter whenever it does create.
	for seed := int64(0); seed < 200; seed++ {
		if created, dia := MoonCreation(5, seed); created {
			if dia < MoonDiameterKm(5, 10) || dia > MoonDiameterKm(5, 20) {
				t.Fatalf("chance 5 seed %d dia=%d out of range", seed, dia)
			}
		}
	}
}

func TestPhalanxRange(t *testing.T) {
	cases := map[int]int{0: 0, 1: 0, 2: 3, 5: 24, 10: 99}
	for level, want := range cases {
		if got := PhalanxRange(level); got != want {
			t.Fatalf("PhalanxRange(%d) = %d, want %d", level, got, want)
		}
	}
}

func TestJumpgateCooldown(t *testing.T) {
	cases := map[int]time.Duration{0: time.Hour, 1: time.Hour, 2: 30 * time.Minute, 3: 15 * time.Minute}
	for level, want := range cases {
		if got := JumpgateCooldown(level); got != want {
			t.Fatalf("JumpgateCooldown(%d) = %v, want %v", level, got, want)
		}
	}
}

// TestPhalanxInRange locks the measured live behaviour: same-galaxy only, reach
// level^2-1 systems (sensor 2:188, level 2 => systems 185..191).
func TestPhalanxInRange(t *testing.T) {
	cases := []struct {
		tg, ts int
		want   bool
	}{
		{2, 185, true},  // distance 3 = 2^2-1
		{2, 191, true},  // distance 3
		{2, 188, true},  // own system
		{2, 184, false}, // distance 4
		{3, 125, false}, // other galaxy, even at distance 0
	}
	for _, c := range cases {
		if got := PhalanxInRange(2, 188, c.tg, c.ts, 2); got != c.want {
			t.Fatalf("PhalanxInRange(2,188 -> %d:%d, L2) = %v, want %v", c.tg, c.ts, got, c.want)
		}
	}
	if PhalanxInRange(2, 188, 2, 188, 0) {
		t.Fatal("level 0 sensor must not scan")
	}
}

func TestMoonBaseDestructionReduction(t *testing.T) {
	cases := map[int]float64{0: 0, 1: 0, 2: 0.03, 3: 0.03, 20: 0.30}
	for level, want := range cases {
		if got := MoonBaseDestructionReduction(level); got != want {
			t.Fatalf("MoonBaseDestructionReduction(%d) = %v, want %v", level, got, want)
		}
	}
}

func TestMoonStructures(t *testing.T) {
	mb, ok := MoonStructure("moon_base")
	if !ok || mb.ID != 41 || mb.BaseCost != (Cost{Metal: 20000, Crystal: 40000, Deuterium: 20000}) {
		t.Fatalf("moon_base = %+v ok=%v", mb, ok)
	}
	px, ok := MoonStructure("phalanx_sensor")
	if !ok || px.ID != 42 || px.Requires["moon_base"] != 1 {
		t.Fatalf("phalanx_sensor = %+v ok=%v", px, ok)
	}
	jg, ok := MoonStructure("jumpgate")
	if !ok || jg.ID != 43 || jg.BaseCost.Metal != 2000000 || jg.Requires["shipyard"] != 1 {
		t.Fatalf("jumpgate = %+v ok=%v", jg, ok)
	}
	// A moon-legal planet building reuses its planet definition.
	rf, ok := MoonStructure("robotics_factory")
	if !ok || rf.ID != 14 {
		t.Fatalf("robotics_factory on moon = %+v ok=%v", rf, ok)
	}
	// Mines / power / silo / unknown are not moon-legal.
	for _, code := range []string{"metal_mine", "solar_plant", "missile_silo", "nope"} {
		if _, ok := MoonStructure(code); ok {
			t.Fatalf("%q should not be moon-legal", code)
		}
	}
	if d, ok := MoonOnlyStructureByID(43); !ok || d.Code != "jumpgate" {
		t.Fatalf("MoonOnlyStructureByID(43) = %+v ok=%v", d, ok)
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

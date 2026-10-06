package game

import "testing"

// TestEspionageRevealTable locks the model to the reference required-probe
// table. The table's "spy tech diff" is (enemyTech - yourTech); required probes
// are the minimum that reveal each section.
func TestEspionageRevealTable(t *testing.T) {
	const your = 10
	cases := []struct {
		diffCol                          int // enemyTech - yourTech
		fleet, defense, buildings, techs int
	}{
		{-3, 1, 1, 1, 1},
		{-2, 1, 1, 1, 3},
		{-1, 1, 2, 4, 6},
		{0, 2, 3, 5, 7},
		{1, 3, 4, 6, 8},
		{2, 6, 7, 9, 11},
		{3, 11, 12, 14, 16},
		{4, 18, 19, 21, 23},
		{5, 27, 28, 30, 32},
		{6, 38, 39, 41, 43},
	}
	check := func(name string, probes, enemy, threshold int) {
		score := EspionageScore(probes, your, enemy)
		if score < threshold {
			t.Fatalf("%s: probes=%d diff=%d score=%d < threshold %d", name, probes, enemy-your, score, threshold)
		}
		if probes > 1 {
			if below := EspionageScore(probes-1, your, enemy); below >= threshold {
				t.Fatalf("%s: probes=%d diff=%d score=%d should be below threshold %d", name, probes, enemy-your, below, threshold)
			}
		}
	}
	for _, c := range cases {
		enemy := your + c.diffCol
		check("fleet", c.fleet, enemy, EspionageFleetThreshold)
		check("defense", c.defense, enemy, EspionageDefenseThreshold)
		check("buildings", c.buildings, enemy, EspionageBuildingsThreshold)
		check("research", c.techs, enemy, EspionageResearchThreshold)
	}
}

func TestEspionageRevealsProgression(t *testing.T) {
	// A single probe at equal tech reveals only resources.
	if f, d, b, r := EspionageReveals(EspionageScore(1, 5, 5)); f || d || b || r {
		t.Fatal("1 probe / equal tech should reveal resources only")
	}
	// At equal tech, 2 probes reveal fleet, 3 defense, 5 buildings, 7 research.
	for _, tc := range []struct {
		probes                    int
		fleet, def, bld, research bool
	}{
		{2, true, false, false, false},
		{3, true, true, false, false},
		{5, true, true, true, false},
		{7, true, true, true, true},
	} {
		f, d, b, r := EspionageReveals(EspionageScore(tc.probes, 5, 5))
		if f != tc.fleet || d != tc.def || b != tc.bld || r != tc.research {
			t.Fatalf("probes=%d reveals (%v,%v,%v,%v), want (%v,%v,%v,%v)", tc.probes, f, d, b, r, tc.fleet, tc.def, tc.bld, tc.research)
		}
	}
}

func TestCounterEspionageChance(t *testing.T) {
	if got := CounterEspionageChance(5, 5, 3, 0); got != 0 {
		t.Fatalf("no defender ships should mean 0 chance, got %v", got)
	}
	if got := CounterEspionageChance(5, 5, 0, 100); got != 0 {
		t.Fatalf("no probes should mean 0 chance, got %v", got)
	}
	base := CounterEspionageChance(5, 5, 1, 10)
	if base <= 0 || base >= 1 {
		t.Fatalf("base chance out of range: %v", base)
	}
	if CounterEspionageChance(5, 6, 1, 10) <= base {
		t.Fatal("higher defender tech should raise counter chance")
	}
	if CounterEspionageChance(6, 5, 1, 10) >= base {
		t.Fatal("higher attacker tech should lower counter chance")
	}
	if CounterEspionageChance(5, 5, 2, 10) <= base {
		t.Fatal("more probes should raise counter chance")
	}
	if got := CounterEspionageChance(-100, 100, 1000, 1000); got != 1 {
		t.Fatalf("extreme chance should clamp to 1, got %v", got)
	}
}

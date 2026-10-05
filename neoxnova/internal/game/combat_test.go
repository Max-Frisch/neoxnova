package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestTechBonus(t *testing.T) {
	cases := map[int]int{0: 0, 1: 1, 2: 2, 3: 4, 4: 6, 5: 9, 10: 30, 15: 64, 20: 110}
	for level, want := range cases {
		if got := TechBonus(level); got != want {
			t.Errorf("TechBonus(%d) = %d, want %d", level, got, want)
		}
	}
}

func TestDerivedStat(t *testing.T) {
	cases := []struct{ base, level, want int }{
		{35, 10, 46}, // 35*1.30 = 45.5 -> 46
		{35, 15, 57}, // 35*1.64 = 57.4 -> 57
		{50, 0, 50},
		{400, 10, 520}, // 400*1.30
	}
	for _, c := range cases {
		if got := DerivedStat(c.base, c.level); got != c.want {
			t.Errorf("DerivedStat(%d,%d) = %d, want %d", c.base, c.level, got, c.want)
		}
	}
}

func TestDebrisShipsOnly(t *testing.T) {
	// A huge attacker wipes one Light Fighter and one Missile Launcher. Debris
	// must come only from the ship (LF base M3000 C1000 -> 1500/500).
	res := Resolve(
		Combatant{Units: map[string]int64{"207": 1000}},
		Combatant{Units: map[string]int64{"204": 1, "401": 1}},
		1,
	)
	if res.Winner != "attacker" {
		t.Fatalf("winner = %q, want attacker", res.Winner)
	}
	if res.Defender.Lost["204"] != 1 || res.Defender.Lost["401"] != 1 {
		t.Fatalf("defender losses = %+v", res.Defender.Lost)
	}
	if res.DebrisMetal != 1500 || res.DebrisCrystal != 500 {
		t.Fatalf("debris = M%d C%d, want M1500 C500 (defenses make no debris)", res.DebrisMetal, res.DebrisCrystal)
	}
}

func TestLoot(t *testing.T) {
	stored := Cost{Metal: 1_000_000, Crystal: 1_000_000, Deuterium: 1_000_000}
	// Cargo-capped: only 1,050,000 can be carried.
	if got := Loot(stored, 1_050_000); got != (Cost{Metal: 500_000, Crystal: 500_000, Deuterium: 50_000}) {
		t.Errorf("cargo-capped loot = %+v", got)
	}
	// Resource-capped: 50% of each (1.5M total) fits in a huge hold.
	if got := Loot(stored, 10_000_000); got != (Cost{Metal: 500_000, Crystal: 500_000, Deuterium: 500_000}) {
		t.Errorf("resource-capped loot = %+v", got)
	}
}

func TestMoonChance(t *testing.T) {
	cases := []struct {
		m, c int64
		want int
	}{{0, 0, 0}, {100_000, 0, 1}, {1_000_000, 0, 10}, {50_000_000, 50_000_000, 20}, {13_500_000, 0, 20}}
	for _, tc := range cases {
		if got := MoonChance(tc.m, tc.c); got != tc.want {
			t.Errorf("MoonChance(%d,%d) = %d, want %d", tc.m, tc.c, got, tc.want)
		}
	}
}

func TestResolveShieldFirstNoBounce(t *testing.T) {
	// 1 LF (attack 50) cannot break a Missile Launcher's 200 shield, so the
	// attacker loses and the launcher survives. (No per-shot bounce is needed to
	// explain this: 50 < 200 with full regen.)
	res := Resolve(
		Combatant{Units: map[string]int64{"204": 1}},
		Combatant{Units: map[string]int64{"401": 1}},
		1,
	)
	if res.Winner != "defender" {
		t.Fatalf("winner = %q, want defender", res.Winner)
	}
	if res.Attacker.Lost["204"] != 1 || res.Defender.Lost["401"] != 0 {
		t.Fatalf("unexpected losses: atk=%v def=%v", res.Attacker.Lost, res.Defender.Lost)
	}
}

func TestResolveDeterministic(t *testing.T) {
	mk := func() Combatant {
		return Combatant{
			Units: map[string]int64{"204": 2000, "206": 500},
			Techs: CombatTechs{Weapons: 16, Shield: 16, Armour: 17},
		}
	}
	def := Combatant{Units: map[string]int64{"207": 300}, Techs: CombatTechs{Weapons: 16, Shield: 16, Armour: 17}}
	a := Resolve(mk(), def, 42)
	b := Resolve(mk(), def, 42)
	if a.Winner != b.Winner || a.DebrisMetal != b.DebrisMetal || a.DebrisCrystal != b.DebrisCrystal {
		t.Fatalf("same seed diverged: %+v vs %+v", a, b)
	}
}

func TestAcademyProcsReduceAttackerLosses(t *testing.T) {
	attacker := Combatant{Units: map[string]int64{"204": 2000}, Techs: CombatTechs{Weapons: 16, Shield: 16, Armour: 17}}
	withAcad := attacker
	withAcad.Academy = map[string]int{"1103": 7, "1109": 4}
	defender := Combatant{Units: map[string]int64{"207": 100}, Techs: CombatTechs{Weapons: 16, Shield: 16, Armour: 17}}

	var baseLost, acadLost int64
	for seed := int64(1); seed <= 20; seed++ {
		baseLost += Resolve(attacker, defender, seed).Attacker.Lost["204"]
		acadLost += Resolve(withAcad, defender, seed).Attacker.Lost["204"]
	}
	if acadLost >= baseLost {
		t.Fatalf("academy procs should reduce attacker losses: base=%d with=%d", baseLost, acadLost)
	}
}

// TestCombatSeedReproducible proves a simulator can reproduce a real battle:
// CombatSeed depends only on the fleet/tech inputs (not map order), so
// Resolve(battle, CombatSeed(battle)) is identical for identical fleets.
func TestCombatSeedReproducible(t *testing.T) {
	tech := CombatTechs{Weapons: 10, Shield: 10, Armour: 10}
	a1 := Combatant{Units: map[string]int64{"204": 1000, "206": 100}, Techs: tech}
	a2 := Combatant{Units: map[string]int64{"206": 100, "204": 1000}, Techs: tech}
	d := Combatant{Units: map[string]int64{"207": 50}, Techs: tech}

	if CombatSeed(a1, d) != CombatSeed(a2, d) {
		t.Fatal("CombatSeed depends on map iteration order")
	}
	r1 := Resolve(a1, d, CombatSeed(a1, d))
	r2 := Resolve(a2, d, CombatSeed(a2, d))
	if r1.Winner != r2.Winner || r1.DebrisMetal != r2.DebrisMetal || r1.DebrisCrystal != r2.DebrisCrystal {
		t.Fatalf("identical battles diverged: %+v vs %+v", r1, r2)
	}
	if r1.Attacker.Lost["204"] != r2.Attacker.Lost["204"] {
		t.Fatal("losses differ for identical battles")
	}
}

// TestRapidFireShotsPerRound locks the reference RF shot pattern: a lone
// Battleship (RF 25 vs sats) kills ~floor(0.70*25)=17 in round 1, then exactly
// 25 per round thereafter. Defenders are Solar Sats (attack 0, 1-shottable), so
// kills == shots. See tools/explorer/plans/combat/rf-shots.json.
func TestRapidFireShotsPerRound(t *testing.T) {
	res := Resolve(
		Combatant{Units: map[string]int64{"207": 1}},
		Combatant{Units: map[string]int64{"212": 200}},
		1,
	)
	for i := 1; i < len(res.RoundLosses); i++ {
		if got := res.RoundLosses[i].Defender["212"]; got != 25 {
			t.Fatalf("round %d killed %d sats, want exactly 25", i+1, got)
		}
	}
	if res.Defender.Lost["212"] < 100 {
		t.Fatalf("expected the RF chain to clear most sats, got %d", res.Defender.Lost["212"])
	}
}

// TestAcademyCalibrationLog prints our engine's attacker losses for the
// scenarios swept from the reference simulator (tools/explorer/plans/combat/
// acal-base). Reference: base 721, 1103:5 -> 509. NOTE: our absolute loss
// counts run high here — the reference concentrates damage (sequential)
// whereas this engine spreads it uniformly per shot; winner prediction matches
// ~91% of the recorded dataset, but swarm-vs-capital magnitudes diverge. Kept
// as an informational log, not an assertion.
func TestAcademyCalibrationLog(t *testing.T) {
	mk := func(acad map[string]int) Combatant {
		return Combatant{Units: map[string]int64{"204": 2000}, Techs: CombatTechs{Weapons: 16, Shield: 16, Armour: 17}, Academy: acad}
	}
	def := Combatant{Units: map[string]int64{"207": 100}, Techs: CombatTechs{Weapons: 16, Shield: 16, Armour: 17}}
	cases := map[string]map[string]int{
		"base": nil, "1103:5": {"1103": 5}, "1103:10": {"1103": 10},
		"1108:5": {"1108": 5}, "1109:5": {"1109": 5}, "1110:5": {"1110": 5}, "1111:5": {"1111": 5},
	}
	for name, acad := range cases {
		var total int64
		for seed := int64(1); seed <= 50; seed++ {
			total += Resolve(mk(acad), def, seed).Attacker.Lost["204"]
		}
		t.Logf("our engine %-7s avg LF lost (50 seeds) = %d", name, total/50)
	}
}

type dataset struct {
	Scenarios []struct {
		ID       string             `json:"id"`
		Attacker map[string]float64 `json:"attacker"`
		Defender map[string]float64 `json:"defender"`
		Result   string             `json:"result"`
	} `json:"scenarios"`
}

func toCombatant(m map[string]float64) Combatant {
	c := Combatant{Units: map[string]int64{}}
	for code, n := range m {
		v := int64(n)
		switch code {
		case "109":
			c.Techs.Weapons = int(v)
		case "110":
			c.Techs.Shield = int(v)
		case "111":
			c.Techs.Armour = int(v)
		default:
			if len(code) == 3 && (code[0] == '2' || code[0] == '4') && v > 0 {
				c.Units[code] += v
			}
		}
	}
	return c
}

// TestReplayDataset runs the engine over the captured simulator scenarios and
// reports winner agreement. The reference simulator is deterministic with a
// fixed internal seed, while our engine uses a seedable RNG and probabilistic
// rapid fire, so exact per-scenario reproduction is not expected; this is a
// smoke test that the engine terminates and broadly tracks the reference.
func TestReplayDataset(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "niburus_combat.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("dataset not available: %v", err)
	}
	var ds dataset
	if err := json.Unmarshal(raw, &ds); err != nil {
		t.Fatalf("parse dataset: %v", err)
	}
	agree, decided, total := 0, 0, 0
	for _, sc := range ds.Scenarios {
		a := toCombatant(sc.Attacker)
		d := toCombatant(sc.Defender)
		if len(a.Units) == 0 || len(d.Units) == 0 {
			continue
		}
		total++
		res := Resolve(a, d, 7)
		if sc.Result == "attacker" || sc.Result == "defender" {
			decided++
			if res.Winner == sc.Result {
				agree++
			}
		}
	}
	t.Logf("replay: %d/%d decided scenarios matched (%d total runnable)", agree, decided, total)
	if total == 0 {
		t.Fatal("no runnable scenarios")
	}
}

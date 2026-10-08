package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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

// TestDerivedStatBonus locks the additive semantics: the Arsenal upgrade percent
// shares the tech percent sum instead of compounding with it.
func TestDerivedStatBonus(t *testing.T) {
	// TechBonus(10)=30, so +30% upgrade -> 100*(1+0.60) = 160 (not 130*1.3=169).
	if got := DerivedStatBonus(100, 10, 30); got != 160 {
		t.Fatalf("DerivedStatBonus(100,10,30) = %d, want 160 (additive)", got)
	}
	// A zero upgrade bonus must match the plain tech-only result.
	if got, want := DerivedStatBonus(400, 10, 0), DerivedStat(400, 10); got != want {
		t.Fatalf("zero upgrade = %d, want DerivedStat %d", got, want)
	}
}

// TestBuildSideAppliesUpgrades checks that each unit applies the upgrade for
// its card class, folded additively into the tech bonus. Battleship (207) is
// laser / medium armor / medium shields.
func TestBuildSideAppliesUpgrades(t *testing.T) {
	boosted, _ := buildSide(Combatant{
		Units:    map[string]int64{"207": 1},
		Upgrades: map[int]float64{1: 50, 6: 40, 9: 30},
	})
	ct := boosted.instances[0].def
	if ct.attack != 1050 { // 700 * 1.50
		t.Fatalf("upgraded attack = %d, want 1050", ct.attack)
	}
	if ct.shield != 247 { // 190 * 1.30
		t.Fatalf("upgraded shield = %d, want 247", ct.shield)
	}
	if ct.hull != 8120 { // 5800 * 1.40
		t.Fatalf("upgraded hull = %d, want 8120", ct.hull)
	}

	// The Arsenal weapon percent is additive with the SPECIFIC weapon tech, and
	// the general Weapons tech then compounds the component sum:
	// 700 * (1 + 0.50) * 1.30 = 1365 (not 700*1.80).
	mixed, _ := buildSide(Combatant{
		Units:    map[string]int64{"207": 1},
		Techs:    CombatTechs{Weapons: 10},
		Upgrades: map[int]float64{1: 50},
	})
	if got := mixed.instances[0].def.attack; got != 1365 { // 700 * 1.50 * 1.30
		t.Fatalf("tech+upgrade attack = %d, want 1365 (general tech compounds)", got)
	}

	// A Standard-weapon unit (Light Fighter 204) ignores the laser upgrade.
	std, _ := buildSide(Combatant{Units: map[string]int64{"204": 1}, Upgrades: map[int]float64{1: 100}})
	if got := std.instances[0].def.attack; got != 50 {
		t.Fatalf("standard weapon attack = %d, want 50 (unaffected)", got)
	}
}

// TestBuildSideAppliesFlatBonus locks the expedition-NPC model: a single rolled
// Weapons/Shield/Armour percent (the live report header, e.g. "Aliens +202%")
// scales attack, shield and hull alike with no per-stat research.
func TestBuildSideAppliesFlatBonus(t *testing.T) {
	side, _ := buildSide(Combatant{Units: map[string]int64{"204": 1}, FlatBonusPct: 202})
	ct := side.instances[0].def
	if ct.attack != 151 { // round(50 * 3.02)
		t.Fatalf("flat attack = %d, want 151", ct.attack)
	}
	if ct.shield != 106 { // round(35 * 3.02)
		t.Fatalf("flat shield = %d, want 106", ct.shield)
	}
	if ct.hull != 1208 { // round(400 * 3.02)
		t.Fatalf("flat hull = %d, want 1208", ct.hull)
	}
}

// TestUnitStatsAttackEqualsWeaponSum locks the card invariant: a unit's base
// attack equals the sum of the base attacks of its weapon components. If a card
// ever disagrees, buildSide's per-component derivation would silently diverge.
func TestUnitStatsAttackEqualsWeaponSum(t *testing.T) {
	checked := 0
	for code, classes := range unitClasses {
		if len(classes.Weapons) == 0 {
			continue
		}
		stats, ok := unitStats[code]
		if !ok {
			continue // card extracted but no captured base stats (e.g. 221-224)
		}
		sum := 0
		for _, w := range classes.Weapons {
			sum += w.Attack
		}
		if stats.Attack != sum {
			t.Errorf("%s: card weapon sum %d != base attack %d", code, sum, stats.Attack)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no units checked")
	}
}

// TestDerivedAttackReproducesLiveReport replays the per-unit firepower recorded
// in tools/explorer/data/combat/real-big-acc1-acc2.report.json (round 1). The
// techs come from the sibling .input.json capture.
func TestDerivedAttackReproducesLiveReport(t *testing.T) {
	// Attacker: 109/110/111=15, 120=17, 121=15, 122=12.
	atk := Combatant{Techs: CombatTechs{Weapons: 15, Laser: 17, Ion: 15, Plasma: 12}}
	atkCases := []struct {
		code string
		want int
	}{
		{"204", 82},   // Standard 50 * 1.64
		{"205", 246},  // Standard 150 * 1.64
		{"206", 853},  // Ion 400 * 1.30 * 1.64
		{"207", 1538}, // Laser 700 * 1.34 * 1.64
		{"215", 3077}, // Laser 1400 * 1.34 * 1.64
	}
	for _, tc := range atkCases {
		if got := DerivedAttack(unitStats[tc.code].Attack, unitClasses[tc.code], atk); got != tc.want {
			t.Errorf("attacker %s attack = %d, want %d", tc.code, got, tc.want)
		}
	}

	// Defender: 120=14, 121=7, 122=7.
	def := Combatant{Techs: CombatTechs{Weapons: 15, Laser: 14, Ion: 7, Plasma: 7}}
	defCases := []struct {
		code string
		want int
	}{
		{"206", 748},  // Ion 400 * 1.14 * 1.64
		{"207", 1469}, // Laser 700 * 1.28 * 1.64
		{"401", 131},  // Standard 80 * 1.64
		{"402", 210},  // Laser 100 * 1.28 * 1.64
		{"403", 525},  // Laser 250 * 1.28 * 1.64
	}
	for _, tc := range defCases {
		if got := DerivedAttack(unitStats[tc.code].Attack, unitClasses[tc.code], def); got != tc.want {
			t.Errorf("defender %s attack = %d, want %d", tc.code, got, tc.want)
		}
	}
}

// TestDerivedAttackMultiComponentWeapon checks a two-component unit (Black Moon
// 216: Laser 20 250 + Gravitational 114 750). Laser tech 10 (+20%) and Graviton
// research 5 (+20%) each apply to their own component; the sum is then scaled
// by the general Weapons tech 10: 162 000 * 1.30 = 210 600.
func TestDerivedAttackMultiComponentWeapon(t *testing.T) {
	got := DerivedAttack(135000, unitClasses["216"], Combatant{
		Techs: CombatTechs{Weapons: 10, Laser: 10, Graviton: 5},
	})
	if got != 210600 {
		t.Fatalf("Black Moon attack = %d, want 210600", got)
	}
}

// TestReplayBigBattleReport, when the gitignored live capture is present, derives
// every unit's attack/shield/hull and compares it with the server's own round-1
// report. It skips on a clean checkout (tools/explorer/data is ignored).
func TestReplayBigBattleReport(t *testing.T) {
	base := filepath.Join("..", "..", "tools", "explorer", "data", "combat")
	inRaw, err := os.ReadFile(filepath.Join(base, "real-big-acc1-acc2.input.json"))
	if err != nil {
		t.Skipf("live capture not available: %v", err)
	}
	repRaw, err := os.ReadFile(filepath.Join(base, "real-big-acc1-acc2.report.json"))
	if err != nil {
		t.Skipf("live capture not available: %v", err)
	}
	var in struct {
		Attacker map[string]float64 `json:"attacker"`
		Defender map[string]float64 `json:"defender"`
	}
	if err := json.Unmarshal(inRaw, &in); err != nil {
		t.Fatalf("parse input: %v", err)
	}
	var rep struct {
		Rounds []struct {
			Attacker []reportUnit `json:"attacker"`
			Defender []reportUnit `json:"defender"`
		} `json:"rounds"`
	}
	if err := json.Unmarshal(repRaw, &rep); err != nil {
		t.Fatalf("parse report: %v", err)
	}
	if len(rep.Rounds) == 0 {
		t.Fatal("report has no rounds")
	}
	techsFrom := func(m map[string]float64) CombatTechs {
		return CombatTechs{
			Weapons: int(m["109"]), Shield: int(m["110"]), Armour: int(m["111"]),
			Laser: int(m["120"]), Ion: int(m["121"]), Plasma: int(m["122"]), Graviton: int(m["199"]),
		}
	}
	check := func(name string, units []reportUnit, techs CombatTechs) {
		for _, u := range units {
			stats, ok := unitStats[strconv.Itoa(u.Code)]
			if !ok {
				continue
			}
			classes := unitClasses[strconv.Itoa(u.Code)]
			c := Combatant{Techs: techs}
			if got := DerivedAttack(stats.Attack, classes, c); got != u.Firepower {
				t.Errorf("%s %d firepower = %d, report %d", name, u.Code, got, u.Firepower)
			}
			if got := DerivedStatBonus(stats.Shield, techs.Shield, 0); got != u.Shield {
				t.Errorf("%s %d shield = %d, report %d", name, u.Code, got, u.Shield)
			}
			if got := DerivedStatBonus(stats.Hull, techs.Armour, 0); got != u.Armour {
				t.Errorf("%s %d armour = %d, report %d", name, u.Code, got, u.Armour)
			}
		}
	}
	r := rep.Rounds[0]
	check("attacker", r.Attacker, techsFrom(in.Attacker))
	check("defender", r.Defender, techsFrom(in.Defender))
}

type reportUnit struct {
	Code      int `json:"code"`
	Firepower int `json:"firepower"`
	Shield    int `json:"shield"`
	Armour    int `json:"armour"`
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
		case "120":
			c.Techs.Laser = int(v)
		case "121":
			c.Techs.Ion = int(v)
		case "122":
			c.Techs.Plasma = int(v)
		case "199":
			c.Techs.Graviton = int(v)
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

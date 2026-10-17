package game

import "testing"

func TestAcademyCatalogIntegrity(t *testing.T) {
	roots := map[AcademyBranch]bool{}
	byCode := map[string]AcademySkill{}
	for _, s := range AcademySkills {
		if s.Code == "" || s.Name == "" || s.EffectText == "" {
			t.Fatalf("incomplete skill entry: %+v", s)
		}
		if s.Tier < 1 {
			t.Fatalf("%s: tier %d", s.Code, s.Tier)
		}
		if s.MaxLevel < 1 || s.CostBase <= 0 || s.CostFactor <= 1 {
			t.Fatalf("%s: bad cost/max: base=%.3f factor=%.3f max=%d", s.Code, s.CostBase, s.CostFactor, s.MaxLevel)
		}
		byCode[s.Code] = s
		if s.Prereq.Code == "" {
			roots[s.Branch] = true
		}
	}
	for _, s := range AcademySkills {
		if s.Prereq.Code != "" {
			if _, ok := byCode[s.Prereq.Code]; !ok {
				t.Fatalf("%s preq points at unknown skill %s", s.Code, s.Prereq.Code)
			}
			if byCode[s.Prereq.Code].Branch != s.Branch {
				t.Fatalf("%s preq crosses branches (%s)", s.Code, s.Prereq.Code)
			}
		}
	}
	for _, b := range []AcademyBranch{AcademyAttack, AcademyEconomy, AcademyDefence} {
		if !roots[b] {
			t.Fatalf("branch %s has no root", b.Label())
		}
	}
}

// TestAcademyCostAnchors checks that round(CostBase*CostFactor^(L-1)) reproduces
// the live point cost captured from the academy tooltips.
func TestAcademyCostAnchors(t *testing.T) {
	cases := []struct {
		code  string
		level int
		want  int64
	}{
		{SkillWeaponry, 6, 2},
		{SkillWeaponry, 19, 26},
		{SkillWeaponsClassA, 8, 9},
		{SkillDoubleAttack, 9, 32},
		{SkillStandardFleet, 17, 435},
		{SkillEngineLimitation, 10, 22},
		{SkillEngineLimitation, 16, 85},
		{SkillEngineEconomy, 6, 11},
		{SkillAccurateShots, 7, 97},
		{SkillChainReaction, 10, 95},
		{SkillStrengthenExplode, 8, 119},
		{SkillFocusing, 12, 86},
		{SkillProductionSpeed, 1, 1},
		{SkillProductionSpeed, 19, 55},
		{SkillMagistracy, 7, 19},
		{SkillCosmoDepot, 6, 9},
		{SkillPackingOfCargo, 10, 14},
		{SkillDefensiveStrategy, 18, 22},
		{SkillDefenceClassA, 5, 4},
		{SkillDoubleShields, 10, 42},
		{SkillHeavyArmour, 8, 214},
		{SkillSealingShields, 7, 11},
		{SkillMechanics, 1, 2},
		{SkillComputerization, 1, 3},
	}
	for _, c := range cases {
		def, ok := AcademySkillByCode(c.code)
		if !ok {
			t.Fatalf("unknown skill %s", c.code)
		}
		if got := AcademyCost(def, c.level); got != c.want {
			t.Errorf("AcademyCost(%s, %d) = %d, want %d", c.code, c.level, got, c.want)
		}
	}
}

// TestAcademyAttackAnchor reproduces the captured attacker stack bonus: Weaponry
// 19 (+19), Weapons Class A 8 (+16), Empire 1 (+1), Defence class A 5 (-10) =
// +26% attack.
func TestAcademyAttackAnchor(t *testing.T) {
	levels := map[string]int{
		SkillWeaponry:      19,
		SkillWeaponsClassA: 8,
		SkillEmpire:        1,
		SkillDefenceClassA: 5,
	}
	if got := AcademyAttackPct(levels); got != 26 {
		t.Fatalf("attack pct = %v, want 26", got)
	}
	if got := AcademyRFReductionPct(map[string]int{SkillHeavyArmour: 8}); got != 16 {
		t.Fatalf("RF reduction = %v, want 16", got)
	}
}

func TestAcademyProductionBonus(t *testing.T) {
	b := AcademyProductionBonus(map[string]int{SkillProductionSpeed: 19, SkillEnergetics: 8})
	if b.Metal != 95 || b.Crystal != 95 || b.Deuterium != 95 || b.Energy != 16 {
		t.Fatalf("academy production bonus = %+v", b)
	}
}

func TestAcademyStorageAndMisc(t *testing.T) {
	if got := AcademyStoragePct(map[string]int{SkillCosmoDepot: 6}); got != 30 {
		t.Fatalf("storage pct = %v, want 30", got)
	}
	if got := AcademyResearchPct(map[string]int{SkillMagistracy: 7}); got != 14 {
		t.Fatalf("research pct = %v, want 14", got)
	}
	if got := AcademyFleetSlots(map[string]int{SkillComputerization: 3}); got != 3 {
		t.Fatalf("fleet slots = %d, want 3", got)
	}
	if got := AcademyIRN(map[string]int{SkillAcademyScience: 1}); got != 1 {
		t.Fatalf("IRN = %d, want 1", got)
	}
	if got := AcademyPlanets(map[string]int{SkillExpandingEmpire: 2}); got != 2 {
		t.Fatalf("planets = %d, want 2", got)
	}
}

func TestAcademyPrereqAndAffordable(t *testing.T) {
	weaponsA, _ := AcademySkillByCode(SkillWeaponsClassA) // needs Weaponry 3
	if AcademyPrereqMet(weaponsA, map[string]int{SkillWeaponry: 2}) {
		t.Fatal("prereq should not be met at Weaponry 2")
	}
	if !AcademyPrereqMet(weaponsA, map[string]int{SkillWeaponry: 3}) {
		t.Fatal("prereq should be met at Weaponry 3")
	}
	weaponry, _ := AcademySkillByCode(SkillWeaponry)
	// Cost of levels 1..N is small; 0 points buys nothing.
	if got := AcademyAffordableLevel(weaponry, 0, 5, 0); got != 0 {
		t.Fatalf("affordable with 0 points = %d, want 0", got)
	}
	var points int64
	for lvl := 1; lvl <= 3; lvl++ {
		points += AcademyCost(weaponry, lvl)
	}
	if got := AcademyAffordableLevel(weaponry, 0, 5, points); got != 3 {
		t.Fatalf("affordable = %d, want 3", got)
	}
}

// TestBonusButtonAcademyPoints verifies the owner-verified payout: 34 Academy
// points at peaceful level 77 with the premium "Button Bonus ×4", i.e. an 8.5
// base.
func TestBonusButtonAcademyPoints(t *testing.T) {
	if got := BonusButtonReward(77).AcademyPoints; got != 9 {
		t.Fatalf("base academy points at L77 = %d, want 9 (round of 8.5)", got)
	}
	if got := BonusButtonRewardPremium(77, LivePeacefulPremium()).AcademyPoints; got != 34 {
		t.Fatalf("premium academy points at L77 = %d, want 34", got)
	}
	if got := BonusButtonReward(3).AcademyPoints; got != 1 {
		t.Fatalf("academy points at L3 = %d, want 1", got)
	}
	if got := BonusButtonReward(72).AcademyPoints; got != 8 {
		t.Fatalf("academy points at L72 = %d, want 8", got)
	}
}

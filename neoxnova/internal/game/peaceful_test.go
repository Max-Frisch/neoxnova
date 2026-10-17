package game

import (
	"math"
	"testing"
)

func TestPeacefulBonuses(t *testing.T) {
	l3 := PeacefulBonuses(3)
	if l3.MinesPct != 3 || l3.StudiesPct != 3 || l3.EnergyPct != 3 {
		t.Fatalf("L3 percent bonuses = %+v, want 3/3/3", l3)
	}
	if l3.FleetSlots != 0 || l3.FieldsPlanet != 0 || l3.FieldsMoon != 0 ||
		l3.SmallFactory != 0 || l3.MediumFactory != 0 || l3.LargeFactory != 0 {
		t.Fatalf("L3 step bonuses = %+v, want all zero", l3)
	}

	l76 := PeacefulBonuses(76)
	if l76.MinesPct != 76 || l76.StudiesPct != 76 || l76.EnergyPct != 76 {
		t.Errorf("L76 percent = %+v, want 76", l76)
	}
	if l76.FleetSlots != 7 || l76.FieldsPlanet != 15 || l76.FieldsMoon != 6 ||
		l76.SmallFactory != 7 || l76.MediumFactory != 3 || l76.LargeFactory != 1 {
		t.Errorf("L76 step = %+v, want 7/15/6/7/3/1", l76)
	}

	if got := PeacefulBonuses(-5); got != (PeacefulBonus{}) {
		t.Errorf("negative level = %+v, want zero", got)
	}
}

func TestPeacefulProductionBonus(t *testing.T) {
	b := PeacefulProductionBonus(76)
	if b.Metal != 76 || b.Crystal != 76 || b.Deuterium != 76 || b.Energy != 76 {
		t.Fatalf("production bonus = %+v, want all 76", b)
	}
	if got := PeacefulProductionBonus(0); got != (ProductionBonus{}) {
		t.Errorf("level 0 = %+v, want zero", got)
	}
}

func TestPeacefulLevelState(t *testing.T) {
	if lvl, pct, next := PeacefulLevelState(0); lvl != 0 || pct != 0 || next != int64(peacefulLevelSeconds(1)) {
		t.Fatalf("zero seconds = %d/%v/%d", lvl, pct, next)
	}

	// Exactly at the cumulative time for level 5 → level 5, 0% progress.
	total := cumulativePeacefulSeconds(5)
	lvl, pct, next := PeacefulLevelState(total)
	if lvl != 5 || pct != 0 || next != int64(peacefulLevelSeconds(6)) {
		t.Fatalf("at L5 boundary = %d/%v/%d", lvl, pct, next)
	}

	// Halfway through the 5→6 level.
	total += peacefulLevelSeconds(6) / 2
	lvl, pct, _ = PeacefulLevelState(total)
	if lvl != 5 {
		t.Fatalf("halfway level = %d, want 5", lvl)
	}
	if pct < 49.9 || pct > 50.1 {
		t.Fatalf("halfway progress = %v, want ~50", pct)
	}
}

func TestBonusButtonReward(t *testing.T) {
	r := BonusButtonReward(3)
	if r.DarkMatter != bonusButtonDarkMatterBase+3*bonusButtonDarkMatterSlope {
		t.Errorf("L3 DM = %d", r.DarkMatter)
	}
	if r.PeacefulSeconds <= 0 {
		t.Errorf("L3 peaceful seconds = %d, want > 0", r.PeacefulSeconds)
	}
	if BonusButtonReward(76).DarkMatter <= BonusButtonReward(3).DarkMatter {
		t.Errorf("DM should grow with level")
	}
}

// withinPct fails when got is off want by more than pct percent.
func withinPct(t *testing.T, name string, got, want, pct float64) {
	t.Helper()
	if math.Abs(got-want) > math.Abs(want)*pct/100 {
		t.Fatalf("%s = %v, want %v (±%v%%)", name, got, want, pct)
	}
}

// TestPeacefulCurveAnchors checks the refit online-time curve against the
// captured per-level durations (all no-premium).
func TestPeacefulCurveAnchors(t *testing.T) {
	for _, c := range []struct {
		level int
		want  float64
	}{{3, 4227}, {4, 4460}, {5, 4705}, {76, 210624}, {77, 222217}} {
		withinPct(t, "levelSeconds", peacefulLevelSeconds(c.level), c.want, 2)
	}
}

// TestPeacefulPremiumHalvesTime models the exactly-2× premium time reduction
// seen in the captures (the "+100% experience" multiplier).
func TestPeacefulPremiumHalvesTime(t *testing.T) {
	p := LivePeacefulPremium()
	if got, want := peacefulLevelSecondsPremium(76, p), peacefulLevelSeconds(76)/2; math.Abs(got-want) > 1e-6 {
		t.Fatalf("premium L76 seconds = %v, want %v", got, want)
	}
	// At the same wall-clock time, premium buys a strictly higher level.
	total := cumulativePeacefulSeconds(5)
	base, _, _ := PeacefulLevelStatePremium(total, PeacefulPremium{})
	prem, _, _ := PeacefulLevelStatePremium(total, p)
	if prem <= base {
		t.Fatalf("premium level = %d, want > base level %d", prem, base)
	}
	// Premium state never yields a lower level.
	if prem != 8 {
		t.Fatalf("premium level at base-L5 time = %d, want 8", prem)
	}
}

// TestBonusButtonRewardPremium locks the premium multipliers: Button Bonus ×4
// scales the Dark Matter and "+100% experience" doubles the XP.
func TestBonusButtonRewardPremium(t *testing.T) {
	base := BonusButtonReward(10)
	prem := BonusButtonRewardPremium(10, LivePeacefulPremium())
	if prem.DarkMatter != base.DarkMatter*4 {
		t.Fatalf("premium DM = %d, want 4×%d", prem.DarkMatter, base.DarkMatter)
	}
	if prem.ExperiencePercent != base.ExperiencePercent*2 {
		t.Fatalf("premium XP%% = %v, want 2×%v", prem.ExperiencePercent, base.ExperiencePercent)
	}
	if prem.CombatXP != base.CombatXP*2 {
		t.Fatalf("premium combat XP = %d, want 2×%d", prem.CombatXP, base.CombatXP)
	}
	// The granted progress is a fraction of the (halved) premium requirement.
	withinPct(t, "premium progress", float64(prem.PeacefulSeconds), float64(base.PeacefulSeconds)/2, 1)
}

func TestBonusButtonInterval(t *testing.T) {
	if got, want := BonusButtonInterval(PeacefulPremium{}), 20*60*1e9; float64(got) != want {
		t.Fatalf("base interval = %v, want %v", got, want)
	}
	if got := BonusButtonInterval(LivePeacefulPremium()); float64(got) != 10*60*1e9 {
		t.Fatalf("premium interval = %v, want 10m", got)
	}
}

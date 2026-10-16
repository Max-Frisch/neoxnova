package game

import "testing"

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

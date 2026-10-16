package game

import "math"

// Peaceful level ("peaceful level" / online level) is the time-based progression
// captured from the live reference server (universe5/niburuspace, Oct 2026). It
// accrues from time spent online and grants account-wide passive bonuses.
//
// IMPORTANT: the captures cover only two peaceful levels (3 and 76), so the
// percentage bonuses are exact (= level) while the step bonuses and the
// online-time curve are **calibrated estimates** — see docs/BACKLOG.md. Only the
// constants below need changing once more levels are captured; every formula
// reads from them.

// Step-bonus divisors: each step bonus is floor(level / divisor). Captured at
// level 76 → fleet slots 7, planet fields 15, moon fields 6, small/medium/large
// factory 7/3/1 (all zero at level 3). Single nonzero data point, so these are
// best-fit estimates.
const (
	peacefulStepFleetSlots    = 10
	peacefulStepFieldsPlanet  = 5
	peacefulStepFieldsMoon    = 12
	peacefulStepSmallFactory  = 10
	peacefulStepMediumFactory = 25
	peacefulStepLargeFactory  = 50
)

// Online-time curve. The per-level duration grows linearly with the level:
// levelSeconds(n) = peacefulLevelBaseSeconds*n. Calibrated from a level-76
// capture with the "+100% Amount of earn experience" premium active: the display
// implied ≈29.25 h for the 76→77 level, i.e. ≈58.5 h base / 76 ⇒ ≈2770 s per
// level unit. Not enough data to fit a curve shape — linear is the tunable
// default.
const peacefulLevelBaseSeconds = 2770.0

// Online Bonus button: base interval between a click and the button becoming
// available again. The live capture showed the premium "Update of Bonus button"
// (+100% appearance) active; the base interval is an estimate.
const BonusButtonIntervalSeconds = 20 * 60

// Bonus rewards are calibrated from the captured clicks:
//
//	peaceful 3 → 4666 DM / 9683 DM · 3-4% · 1 exp
//	peaceful 5 → 8838 DM · 11% · 4 exp
//	peaceful 76 → 20318 / 66284 DM · 16% · 17/34 exp
//
// The DM amount varies at the same level (premium "Button Bonus" ×4 and other
// factors are not modelled yet), so a single base+slope fit is used.
const (
	bonusButtonDarkMatterBase  = 3000
	bonusButtonDarkMatterSlope = 650
	bonusButtonProgressPct     = 10.0
)

// PeacefulBonus is the passive account bonus for one peaceful level.
type PeacefulBonus struct {
	MinesPct      float64 // mine extraction, percent (= level)
	StudiesPct    float64 // research speed, percent (= level)
	EnergyPct     float64 // energy production, percent (= level)
	FleetSlots    int     // extra fleet slots
	FieldsPlanet  int
	FieldsMoon    int
	SmallFactory  int // units per second
	MediumFactory int
	LargeFactory  int
}

// PeacefulBonuses returns the bonus for a peaceful level. Negative levels clamp
// to zero.
func PeacefulBonuses(level int) PeacefulBonus {
	if level < 0 {
		level = 0
	}
	return PeacefulBonus{
		MinesPct:      float64(level),
		StudiesPct:    float64(level),
		EnergyPct:     float64(level),
		FleetSlots:    level / peacefulStepFleetSlots,
		FieldsPlanet:  level / peacefulStepFieldsPlanet,
		FieldsMoon:    level / peacefulStepFieldsMoon,
		SmallFactory:  level / peacefulStepSmallFactory,
		MediumFactory: level / peacefulStepMediumFactory,
		LargeFactory:  level / peacefulStepLargeFactory,
	}
}

// PeacefulProductionBonus folds the peaceful mines/energy percentage into an
// account-wide ProductionBonus (additive with the other sources).
func PeacefulProductionBonus(level int) ProductionBonus {
	if level <= 0 {
		return ProductionBonus{}
	}
	pct := float64(level)
	return ProductionBonus{Metal: pct, Crystal: pct, Deuterium: pct, Energy: pct}
}

// peacefulLevelSeconds is the online time to complete one level at the given
// level (level n → n+1).
func peacefulLevelSeconds(level int) float64 {
	if level < 1 {
		level = 1
	}
	return peacefulLevelBaseSeconds * float64(level)
}

// PeacefulLevelState derives the peaceful level, progress within that level
// (percent, 0-100) and seconds remaining to the next level from the account's
// accumulated online seconds. The cumulative time to reach level L is
// levelBase·L(L+1)/2.
func PeacefulLevelState(totalSeconds float64) (level int, progressPct float64, nextSeconds int64) {
	if totalSeconds <= 0 {
		return 0, 0, int64(peacefulLevelSeconds(1))
	}
	// Largest L with cumulative(L) <= totalSeconds, by solving the quadratic
	// cumulative(L)=base·L(L+1)/2 <= s, then correcting for rounding.
	l := int((math.Sqrt(1+8*totalSeconds/peacefulLevelBaseSeconds) - 1) / 2)
	if l < 0 {
		l = 0
	}
	// cumulative(l) may overshoot/undershoot by one near boundaries.
	for l > 0 && cumulativePeacefulSeconds(l) > totalSeconds {
		l--
	}
	for cumulativePeacefulSeconds(l+1) <= totalSeconds {
		l++
	}
	done := totalSeconds - cumulativePeacefulSeconds(l)
	dur := peacefulLevelSeconds(l + 1)
	if dur <= 0 {
		return l, 0, 0
	}
	pct := done / dur * 100.0
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return l, pct, int64(math.Ceil(dur - done))
}

// cumulativePeacefulSeconds is the online time to reach level L from zero.
func cumulativePeacefulSeconds(level int) float64 {
	if level < 0 {
		level = 0
	}
	return peacefulLevelBaseSeconds * float64(level) * float64(level+1) / 2.0
}

// BonusReward is the payout of one Online Bonus button click.
type BonusReward struct {
	DarkMatter        int64
	PeacefulSeconds   int64
	ExperiencePercent float64
	CombatXP          int64
}

// BonusButtonReward computes the calibrated payout of the Online Bonus button
// for an account at the given peaceful level. The premium "Button Bonus" ×N and
// "Update of Bonus button" multipliers are not modelled yet.
func BonusButtonReward(peacefulLevel int) BonusReward {
	if peacefulLevel < 0 {
		peacefulLevel = 0
	}
	progressSeconds := int64(math.Round(peacefulLevelSeconds(peacefulLevel+1) * bonusButtonProgressPct / 100.0))
	return BonusReward{
		DarkMatter:        int64(bonusButtonDarkMatterBase + bonusButtonDarkMatterSlope*peacefulLevel),
		PeacefulSeconds:   progressSeconds,
		ExperiencePercent: bonusButtonProgressPct,
		CombatXP:          int64(peacefulLevel/2 + 1),
	}
}

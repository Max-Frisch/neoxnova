package game

import (
	"math"
	"time"
)

// Peaceful level ("peaceful level" / online level) is the time-based progression
// captured from the live reference server (universe5/niburuspace, Oct 2026). It
// accrues from time spent online and grants account-wide passive bonuses.
//
// The captures (`info/Bonus Button Peaceful`) cover peaceful levels 3, 4, 72, 76
// and 77, but **mix premium states**: the "Amount of earn experience +100%"
// premium doubles the effective XP, which shows up as an exactly-2× shorter
// "Online Time" requirement (the level-76 capture is half the level-77 one). The
// non-premium captures are used below; the premium is modelled explicitly by
// PeacefulPremium so a future, deliberately premium-tagged run can be fitted
// without touching the formulas.

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

// Online-time curve. The per-level duration is *not* linear from origin — the
// low-level captures are near-flat (L3→4 ≈ 4227 s, L4→5 ≈ 4460 s, L5→6 ≈ 4705 s)
// while the high levels grow super-linearly (L76→77 ≈ 210 624 s, L77→78 ≈
// 222 217 s). A quadratic least-squares fit weighted toward the (more reliable)
// low-level points,
//
//	levelSeconds(n) = 37·n² − 57.3·n + 4076
//
// reproduces every anchor within ~1.5% (A/B/C are the tunable constants; the
// whole curve lives here). Capture noise is ≥10% at the low end, so treat this
// as calibrated but tunable until a premium-tagged run lands.
const (
	peacefulLevelQuadA = 37.0
	peacefulLevelQuadB = -57.3
	peacefulLevelQuadC = 4076.0
)

// Online Bonus button: base interval between a click and the button becoming
// available again. Premium "Update of Bonus button +100%" halves it.
const BonusButtonIntervalSeconds = 20 * 60

// Bonus rewards are calibrated from the captured clicks. The live captures mix
// premium states, so these are central estimates of the *un-premium* payout:
//
//	L3  → 4666 / 9066 / 9683 DM · 3-8% · 1 exp
//	L4  → 7990 / 9267 DM · 6-10% · 1 exp
//	L72 → 50249 DM · 3% · 8 exp
//	L76 → 20318 / 66284 DM · 16% · 17/34 exp
//	L77 → 58779 / 69905 DM · 20% · 34 exp
//
// The DM amount varies at the same level (random roll × the premium "Button
// Bonus ×4"), so a single base+slope central fit is used. ProgressPct is the
// mean observed experience gain per click.
const (
	bonusButtonDarkMatterBase  = 7000
	bonusButtonDarkMatterSlope = 630
	bonusButtonProgressPct     = 10.0
)

// PeacefulPremium models the live premium ("Premium Account") effects that
// touch the peaceful system. Its zero value is "no premium", which is what the
// server stores today (there is no premium flag yet); the percentages are the
// live multipliers lifted from the captured premium panel.
type PeacefulPremium struct {
	ButtonBonusPct     float64 // `Button "Bonus" ×4` → +300 (×4 payout)
	BonusAppearancePct float64 // `Update of "Bonus" button +100%` → +100 (halved cooldown)
	ExperiencePct      float64 // `Amount of earn experience +100%` → +100 (×2 XP, half the online time)
}

// LivePeacefulPremium is the full premium set captured in Oct 2026. Kept as a
// named constructor so tests and a future premium flag share one definition.
func LivePeacefulPremium() PeacefulPremium {
	return PeacefulPremium{ButtonBonusPct: 300, BonusAppearancePct: 100, ExperiencePct: 100}
}

func (p PeacefulPremium) darkMatterMultiplier() float64 { return 1 + p.ButtonBonusPct/100 }
func (p PeacefulPremium) experienceMultiplier() float64 { return 1 + p.ExperiencePct/100 }

// onlineTimeFactor scales the online-time requirement: doubling earned XP halves
// the wall-clock time to the next level.
func (p PeacefulPremium) onlineTimeFactor() float64 { return 1 / (1 + p.ExperiencePct/100) }

// BonusButtonInterval is the cooldown after a Bonus-button click for the given
// premium state. Without premium it is BonusButtonIntervalSeconds.
func BonusButtonInterval(p PeacefulPremium) time.Duration {
	secs := float64(BonusButtonIntervalSeconds) / (1 + p.BonusAppearancePct/100)
	if secs < 1 {
		secs = 1
	}
	return time.Duration(math.Round(secs)) * time.Second
}

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

// peacefulLevelSecondsRaw is the base (no-premium) online time to complete one
// level (level n → n+1), from the quadratic curve.
func peacefulLevelSecondsRaw(level int) float64 {
	if level < 1 {
		level = 1
	}
	n := float64(level)
	return peacefulLevelQuadA*n*n + peacefulLevelQuadB*n + peacefulLevelQuadC
}

// peacefulLevelSeconds is the base (no-premium) online time to complete one
// level (level n → n+1).
func peacefulLevelSeconds(level int) float64 { return peacefulLevelSecondsRaw(level) }

// peacefulLevelSecondsPremium applies the premium experience multiplier.
func peacefulLevelSecondsPremium(level int, p PeacefulPremium) float64 {
	return peacefulLevelSecondsRaw(level) * p.onlineTimeFactor()
}

// cumulativePeacefulSecondsRaw is the base (no-premium) online time to reach
// level L from zero: the closed-form sum of the per-level quadratic.
func cumulativePeacefulSecondsRaw(level int) float64 {
	if level < 0 {
		level = 0
	}
	n := float64(level)
	s1 := n * (n + 1) / 2
	s2 := n * (n + 1) * (2*n + 1) / 6
	return peacefulLevelQuadA*s2 + peacefulLevelQuadB*s1 + peacefulLevelQuadC*n
}

// cumulativePeacefulSeconds is the base (no-premium) online time to reach level
// L from zero.
func cumulativePeacefulSeconds(level int) float64 { return cumulativePeacefulSecondsRaw(level) }

// PeacefulLevelState derives the peaceful level, progress within that level
// (percent, 0-100) and seconds remaining to the next level from the account's
// accumulated online seconds, without premium.
func PeacefulLevelState(totalSeconds float64) (level int, progressPct float64, nextSeconds int64) {
	return PeacefulLevelStatePremium(totalSeconds, PeacefulPremium{})
}

// PeacefulLevelStatePremium is PeacefulLevelState with an explicit premium
// state: the same wall-clock time buys more progress when the experience
// multiplier is active.
func PeacefulLevelStatePremium(totalSeconds float64, p PeacefulPremium) (level int, progressPct float64, nextSeconds int64) {
	factor := p.onlineTimeFactor()
	if totalSeconds <= 0 {
		return 0, 0, int64(peacefulLevelSecondsPremium(1, p))
	}
	// Work in un-premium ("base") seconds so the level lookup uses one curve.
	base := totalSeconds / factor
	// Largest L with cumulative(L) <= base: bracket then binary search (the
	// curve is monotonic, so this inverts any per-level shape).
	lo, hi := 0, 1
	for cumulativePeacefulSecondsRaw(hi) <= base {
		hi *= 2
	}
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if cumulativePeacefulSecondsRaw(mid) <= base {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	l := lo
	done := base - cumulativePeacefulSecondsRaw(l)
	dur := peacefulLevelSecondsRaw(l + 1)
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
	return l, pct, int64((dur - done) * factor)
}

// Academy points ("Experience points" on the live Info dialog) are the currency
// of the Academy skill tree. They are granted by the Online Bonus button and
// scale linearly with the peaceful level. Owner-verified 2026-10-17: at peaceful
// level 77 the button granted 34 points *including* the premium "Button Bonus
// ×4" multiplier, so the un-premium base is 8.5. The low captures (L3/L4 = 1,
// L72 = 8) fit the same line through (3, 1) and (77, 8.5).
const (
	academyPointsBaseAtL3  = 1.0 // captured: L3/L4 grant 1 point
	academyPointsBaseAtL77 = 8.5 // captured: 34 ÷ premium ×4
)

// academyPointsBase is the un-premium Academy-point base for a peaceful level:
// the straight line through the captured (3, 1) and (77, 8.5) anchors.
func academyPointsBase(peacefulLevel int) float64 {
	if peacefulLevel < 0 {
		peacefulLevel = 0
	}
	return academyPointsBaseAtL3 +
		(academyPointsBaseAtL77-academyPointsBaseAtL3)*float64(peacefulLevel-3)/74.0
}

// BonusReward is the payout of one Online Bonus button click.
type BonusReward struct {
	DarkMatter        int64
	PeacefulSeconds   int64
	ExperiencePercent float64
	CombatXP          int64
	AcademyPoints     int64
}

// BonusButtonReward computes the calibrated payout of the Online Bonus button
// for an account at the given peaceful level, without premium.
func BonusButtonReward(peacefulLevel int) BonusReward {
	return BonusButtonRewardPremium(peacefulLevel, PeacefulPremium{})
}

// BonusButtonRewardPremium is BonusButtonReward with an explicit premium state:
// the "Button Bonus ×4" multiplies the Dark Matter, and the "+100% experience"
// multiplier doubles the granted experience (peaceful level and combat XP).
func BonusButtonRewardPremium(peacefulLevel int, p PeacefulPremium) BonusReward {
	if peacefulLevel < 0 {
		peacefulLevel = 0
	}
	progressSeconds := int64(math.Round(peacefulLevelSecondsPremium(peacefulLevel+1, p) * bonusButtonProgressPct / 100.0))
	dm := float64(bonusButtonDarkMatterBase+bonusButtonDarkMatterSlope*peacefulLevel) * p.darkMatterMultiplier()
	// The "Button Bonus ×4" premium multiplies the Academy points too (verified:
	// 34 = 8.5 × 4 at level 77), so apply the same multiplier before rounding.
	academyPoints := academyPointsBase(peacefulLevel) * p.darkMatterMultiplier()
	return BonusReward{
		DarkMatter:        int64(math.Round(dm)),
		PeacefulSeconds:   progressSeconds,
		ExperiencePercent: bonusButtonProgressPct * p.experienceMultiplier(),
		CombatXP:          int64(math.Round(float64(peacefulLevel/2+1) * p.experienceMultiplier())),
		AcademyPoints:     int64(math.Round(academyPoints)),
	}
}

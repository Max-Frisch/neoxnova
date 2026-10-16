package game

import "math"

// Senate — Governators.
//
// Captured from the live client (info/Senate/governators.har, page=gubernators).
// Each governator is a two-phase, account-wide bonus:
//
//   - Level: bought permanently and irreversibly with Achievement Points. The
//     cost of level i is round(APPrice[0] * APPrice[1]^(i-1)); reaching a target
//     level sums the levels in between.
//   - Activation ("hire"): makes the accumulated bonus live for up to
//     MaxGovernatorDays days, priced in Dark Matter per day as
//     round(DMPrice[0] * DMPrice[1]^level).
//
// The bonus percent at a level is floor((BonusBase + BonusPerLevel*level) * 100)
// — with the live constants (0.1 / 0.01) that is a flat base +10% plus +1% per
// level (e.g. level 10 => +20%).
type Governator struct {
	Code          int
	Key           string
	Name          string
	MaxLevel      int
	APPrice       [2]float64 // [base, factor] for the per-level AP cost
	DMPrice       [2]float64 // [base, factor] for the per-day DM price
	BonusBase     float64    // effect fraction at level 0 (0.1 = +10%)
	BonusPerLevel float64    // effect fraction added per level (0.01 = +1%)
	Effects       []GovernatorEffect
}

// GovernatorEffect is one stat a governator modifies. Decrease marks effects
// that reduce their target (construction/research time).
type GovernatorEffect struct {
	Key      string
	Label    string
	Decrease bool
}

// Governator codes (live ids 701..707).
const (
	GovCodeWeapons  = 701
	GovCodeShield   = 702
	GovCodeBuilding = 703
	GovCodeResource = 704
	GovCodeEnergy   = 705
	GovCodeResearch = 706
	GovCodeFleet    = 707
)

// Effect target keys.
const (
	GovEffectAttack       = "attack"
	GovEffectArmor        = "armor"
	GovEffectShield       = "shield"
	GovEffectBuildTime    = "buildtime"
	GovEffectResource     = "resource"
	GovEffectEnergy       = "energy"
	GovEffectResearchTime = "researchtime"
	GovEffectFlyTime      = "fleettime"
)

// MaxGovernatorDays caps a single hire (the live "Days" input max).
const MaxGovernatorDays = 7

// Governators is the catalog in live display order. Prices are the exact live
// priceAP/priceDM bases and factors (GOVERNORS in page=gubernators).
var Governators = []Governator{
	{Code: GovCodeWeapons, Key: "dm_attack", Name: "Weapons Upgrade", MaxLevel: 65,
		APPrice: [2]float64{50, 1.07}, DMPrice: [2]float64{40000, 1.1}, BonusBase: 0.1, BonusPerLevel: 0.01,
		Effects: []GovernatorEffect{{Key: GovEffectAttack, Label: "Attack"}, {Key: GovEffectArmor, Label: "Armor"}}},
	{Code: GovCodeShield, Key: "dm_defensive", Name: "Shield Upgrade", MaxLevel: 65,
		APPrice: [2]float64{50, 1.075}, DMPrice: [2]float64{40000, 1.1}, BonusBase: 0.1, BonusPerLevel: 0.01,
		Effects: []GovernatorEffect{{Key: GovEffectShield, Label: "for shields"}}},
	{Code: GovCodeBuilding, Key: "dm_buildtime", Name: "Building Upgrade", MaxLevel: 50,
		APPrice: [2]float64{20, 1.1}, DMPrice: [2]float64{7500, 1.08}, BonusBase: 0.1, BonusPerLevel: 0.01,
		Effects: []GovernatorEffect{{Key: GovEffectBuildTime, Label: "construction time", Decrease: true}}},
	{Code: GovCodeResource, Key: "dm_resource", Name: "Ressource Upgrade", MaxLevel: 250,
		APPrice: [2]float64{100, 1.01}, DMPrice: [2]float64{30000, 1.022}, BonusBase: 0.1, BonusPerLevel: 0.01,
		Effects: []GovernatorEffect{{Key: GovEffectResource, Label: "the extraction of materials"}}},
	{Code: GovCodeEnergy, Key: "dm_energie", Name: "Energy Upgrade", MaxLevel: 100,
		APPrice: [2]float64{50, 1.022}, DMPrice: [2]float64{10000, 1.055}, BonusBase: 0.1, BonusPerLevel: 0.01,
		Effects: []GovernatorEffect{{Key: GovEffectEnergy, Label: "to energy production"}}},
	{Code: GovCodeResearch, Key: "dm_researchtime", Name: "Research Upgrade", MaxLevel: 40,
		APPrice: [2]float64{100, 1.12}, DMPrice: [2]float64{25000, 1.14}, BonusBase: 0.1, BonusPerLevel: 0.01,
		Effects: []GovernatorEffect{{Key: GovEffectResearchTime, Label: "time research", Decrease: true}}},
	{Code: GovCodeFleet, Key: "dm_fleettime", Name: "Fleet Upgrade", MaxLevel: 20,
		APPrice: [2]float64{100, 1.13}, DMPrice: [2]float64{50000, 1.15}, BonusBase: 0.1, BonusPerLevel: 0.01,
		Effects: []GovernatorEffect{{Key: GovEffectFlyTime, Label: "ship flying time"}}},
}

// GovernatorByCode finds a governator by its live id.
func GovernatorByCode(code int) (Governator, bool) {
	for _, g := range Governators {
		if g.Code == code {
			return g, true
		}
	}
	return Governator{}, false
}

// BonusPct is the effect percentage a governator grants at the given level:
// floor((BonusBase + BonusPerLevel*level) * 100).
func (g Governator) BonusPct(level int) int {
	if level < 0 {
		level = 0
	}
	return int(math.Floor((g.BonusBase + g.BonusPerLevel*float64(level)) * 100))
}

// APCost is the Achievement-Point cost of raising the level from `from` to `to`
// (exclusive start): the sum of round(APPrice[0]*APPrice[1]^(i-1)) for every
// level i in (from, to]. It returns 0 when to <= from.
func (g Governator) APCost(from, to int) int64 {
	if g.MaxLevel > 0 && to > g.MaxLevel {
		to = g.MaxLevel
	}
	if to <= from || from < 0 {
		return 0
	}
	var total int64
	for i := from + 1; i <= to; i++ {
		total += int64(math.Round(g.APPrice[0] * math.Pow(g.APPrice[1], float64(i-1))))
	}
	return total
}

// DMPricePerDay is the Dark Matter price to activate one day at the given level:
// round(DMPrice[0] * DMPrice[1]^level).
func (g Governator) DMPricePerDay(level int) int64 {
	if level < 0 {
		level = 0
	}
	return int64(math.Round(g.DMPrice[0] * math.Pow(g.DMPrice[1], float64(level))))
}

// GovernatorProductionBonus folds the active Resource (all three resources) and
// Energy governators into an account-wide ProductionBonus. Inactive governators
// are absent from active.
func GovernatorProductionBonus(active map[int]int) ProductionBonus {
	var b ProductionBonus
	if lvl, on := active[GovCodeResource]; on {
		if def, ok := GovernatorByCode(GovCodeResource); ok {
			pct := float64(def.BonusPct(lvl))
			b.Metal, b.Crystal, b.Deuterium = pct, pct, pct
		}
	}
	if lvl, on := active[GovCodeEnergy]; on {
		if def, ok := GovernatorByCode(GovCodeEnergy); ok {
			b.Energy = float64(def.BonusPct(lvl))
		}
	}
	return b
}

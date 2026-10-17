package game

import "math"

// Academy — the account-wide skill tree.
//
// Captured from the live reference server (private ops repo:
// `tools/explorer/data/academy-map.json`, the full level-0 tree in
// `_academy.html`/`_academy2.html`, and the acc1/acc2 levelled tooltips). The
// tree has three branches:
//
//	11xx Attack      fleet damage, procs, speed and logistics
//	12xx Economy     extraction, energy, research, storage, cargo, empire size
//	13xx Defence     shield/armour, rapid-fire resistance, defence recovery
//
// Skills are bought with **Academy points** (the "Experience points" granted by
// the Online Bonus button — see peaceful.go). Each skill has a prerequisite
// (a (code, level) gate; roots have none) and a per-level Academy-point cost.
//
// Only the skills whose effect is confirmed by the combat captures are folded
// into the engine (attack/shield/hull, rapid-fire resistance, the economy
// production/research/storage bonuses); the rest are carried as effect text and
// accounted for in the UI so the tree matches the live client. Anything still
// unmodelled is marked in the comments.

// AcademyBranch is one of the three tree branches.
type AcademyBranch int

const (
	AcademyAttack AcademyBranch = iota + 1
	AcademyEconomy
	AcademyDefence
)

func (b AcademyBranch) Label() string {
	switch b {
	case AcademyAttack:
		return "Attack"
	case AcademyEconomy:
		return "Economy"
	case AcademyDefence:
		return "Defence"
	default:
		return ""
	}
}

// AcademyPrereq is a gate on another skill. The zero value (Code == "") is a
// branch root.
type AcademyPrereq struct {
	Code  string
	Level int
}

// AcademyEffect is a skill's per-level effect, in percent (or count for the
// integer fields). A skill only populates the components it modifies.
type AcademyEffect struct {
	AttackPct       float64 // + attack
	ShieldPct       float64 // + shields
	HullPct         float64 // + armour/hull
	RFReductPct     float64 // - rapid fire suffered ("Heavy Armour")
	DoubleShieldPct float64 // + double-shield probability
	MinePct         float64 // + mine extraction
	EnergyPct       float64 // + energy production
	ResearchPct     float64 // - research ("studying") time
	StoragePct      float64 // + storage capacity
	CargoPct        float64 // + fleet cargo space
	FleetSpeedPct   float64 // + fleet speed
	FleetCostPct    float64 // - fleet build cost
	FleetSlot       int     // + fleet slots
	IRN             int     // + Intergalactic Research Network
	Planets         int     // + max planets
}

// AcademySkill describes one skill in the tree.
type AcademySkill struct {
	Code       string
	Name       string
	Branch     AcademyBranch
	Tier       int // 1 = branch root, then graph depth
	MaxLevel   int
	EffectText string // the live "+X%/level ..." line
	Prereq     AcademyPrereq
	PerLevel   AcademyEffect
	// Academy-point cost of the next level: cost(L) = round(CostBase*CostFactor^(L-1)).
	// CostBase/Factor are fitted to the captured (level, cost) anchors (see the
	// comments in AcademySkills); unanchored skills use the defaults below.
	CostBase   float64
	CostFactor float64
}

// Defaults for skills with no captured cost anchor (live costs are read from the
// tooltip's "Required: N Academy points" line; the anchors are documented per
// skill).
const (
	academyDefaultCostBase   = 1.0
	academyDefaultCostFactor = 1.25
	academyDefaultMaxLevel   = 50
)

// AcademySkills is the full catalog. Costs are fitted so
// round(CostBase*CostFactor^(L-1)) reproduces the captured point cost at each
// skill's captured level(s) (acc1 in `academy-map.json`; acc2 in `_academy2.html`
// for Weaponry/Engine limitation). Roots cost 1 point for level 1.
var AcademySkills = buildAcademySkills()

// Skill codes, for the effect aggregation and the persistence layer.
const (
	SkillWeaponry          = "1101"
	SkillWeaponsClassA     = "1102"
	SkillDoubleAttack      = "1103"
	SkillStandardFleet     = "1104"
	SkillEngineLimitation  = "1105"
	SkillComputerization   = "1106"
	SkillEngineEconomy     = "1107"
	SkillAccurateShots     = "1108"
	SkillChainReaction     = "1109"
	SkillStrengthenExplode = "1110"
	SkillFocusing          = "1111"
	SkillReliefShips       = "1112"

	SkillProductionSpeed = "1201"
	SkillEnergetics      = "1202"
	SkillMagistracy      = "1203"
	SkillCosmoDepot      = "1204"
	SkillAcademyScience  = "1205"
	SkillExpandingEmpire = "1206"
	SkillPackingOfCargo  = "1207"
	SkillGlobalDefense   = "1208"
	SkillThermonuclear   = "1209"
	SkillSolarPanels     = "1210"
	SkillEmpire          = "1211"

	SkillDefensiveStrategy = "1301"
	SkillDefenceClassA     = "1302"
	SkillDoubleShields     = "1303"
	SkillMechanics         = "1304"
	SkillShieldComplex     = "1305"
	SkillDefenseComplex    = "1306"
	SkillSealShieldDome    = "1307"
	SkillHeavyArmour       = "1308"
	SkillMaximumDefense    = "1309"
	SkillStandardDefence   = "1310"
	SkillSealingShields    = "1311"
)

func buildAcademySkills() []AcademySkill {
	defs := []AcademySkill{
		// ---- 11xx Attack -------------------------------------------------
		{Code: SkillWeaponry, Name: "Weaponry", Branch: AcademyAttack, MaxLevel: academyDefaultMaxLevel,
			EffectText: "+1% Attack", Prereq: AcademyPrereq{},
			PerLevel:   AcademyEffect{AttackPct: 1},
			CostFactor: 1.1985}, // (L6=2, L19=26)
		{Code: SkillWeaponsClassA, Name: "Weapons Class A", Branch: AcademyAttack,
			EffectText: "+2% Attack, -1% Shields and armor", Prereq: AcademyPrereq{SkillWeaponry, 3},
			PerLevel:   AcademyEffect{AttackPct: 2, ShieldPct: -1, HullPct: -1},
			CostFactor: 1.369}, // L8=9
		{Code: SkillStandardFleet, Name: "Standardisation (Fleet)", Branch: AcademyAttack,
			EffectText: "-1% fleet cost", Prereq: AcademyPrereq{SkillWeaponry, 5},
			PerLevel:   AcademyEffect{FleetCostPct: 1},
			CostFactor: 1.46195}, // L17=435
		{Code: SkillEngineLimitation, Name: "Engine limitation", Branch: AcademyAttack,
			EffectText: "+3% Fleet speed", Prereq: AcademyPrereq{SkillWeaponry, 5},
			PerLevel: AcademyEffect{FleetSpeedPct: 3},
			CostBase: 2.897, CostFactor: 1.2527}, // (L10=22, L16=85)
		{Code: SkillComputerization, Name: "Computerization", Branch: AcademyAttack,
			EffectText: "+1% slots to the fleet", Prereq: AcademyPrereq{SkillStandardFleet, 3},
			PerLevel: AcademyEffect{FleetSlot: 1},
			CostBase: 3}, // L1=3
		{Code: SkillDoubleAttack, Name: "Double attack", Branch: AcademyAttack,
			EffectText: "+2% cumulative probability of shot shell", Prereq: AcademyPrereq{SkillWeaponsClassA, 7},
			CostFactor: 1.542}, // L9=32
		{Code: SkillEngineEconomy, Name: "Engine economication", Branch: AcademyAttack,
			EffectText: "-2% deuterium consumption", Prereq: AcademyPrereq{SkillEngineLimitation, 5},
			CostFactor: 1.615}, // L6=11
		{Code: SkillAccurateShots, Name: "Accurate shots", Branch: AcademyAttack,
			EffectText: "+2% to the maximum number of destroys per round", Prereq: AcademyPrereq{SkillDoubleAttack, 5},
			CostFactor: 2.143}, // L7=97
		{Code: SkillChainReaction, Name: "Chain reaction", Branch: AcademyAttack,
			EffectText: "+1% chance of a chain reaction", Prereq: AcademyPrereq{SkillDoubleAttack, 7},
			CostFactor: 1.659}, // L10=95
		{Code: SkillStrengthenExplode, Name: "Strengthening explosion", Branch: AcademyAttack,
			EffectText: "+8% destruction of additional units", Prereq: AcademyPrereq{SkillDoubleAttack, 7},
			CostFactor: 1.979}, // L8=119
		{Code: SkillReliefShips, Name: "Relief ships", Branch: AcademyAttack,
			EffectText: "-2% Fuel consumption, -2% Armour", Prereq: AcademyPrereq{SkillEngineEconomy, 10}},
		{Code: SkillFocusing, Name: "Focusing", Branch: AcademyAttack,
			EffectText: "+1% ship focus", Prereq: AcademyPrereq{SkillAccurateShots, 5},
			CostFactor: 1.499}, // L12=86

		// ---- 12xx Economy ------------------------------------------------
		{Code: SkillProductionSpeed, Name: "Production speed", Branch: AcademyEconomy, MaxLevel: academyDefaultMaxLevel,
			EffectText: "+5% extraction of mines", Prereq: AcademyPrereq{},
			PerLevel:   AcademyEffect{MinePct: 5},
			CostFactor: 1.249}, // L1=1, L19=55
		{Code: SkillEnergetics, Name: "Energetics", Branch: AcademyEconomy,
			EffectText: "+2% to energy production", Prereq: AcademyPrereq{SkillProductionSpeed, 3},
			PerLevel:   AcademyEffect{EnergyPct: 2},
			CostFactor: 1.369}, // L8=9
		{Code: SkillMagistracy, Name: "Magistracy", Branch: AcademyEconomy,
			EffectText: "-2% studying time", Prereq: AcademyPrereq{SkillEnergetics, 7},
			PerLevel:   AcademyEffect{ResearchPct: 2},
			CostFactor: 1.634}, // L7=19
		{Code: SkillCosmoDepot, Name: "Cosmo depot", Branch: AcademyEconomy,
			EffectText: "+5% more space in ress storage", Prereq: AcademyPrereq{SkillEnergetics, 7},
			PerLevel:   AcademyEffect{StoragePct: 5},
			CostFactor: 1.552}, // L6=9
		{Code: SkillAcademyScience, Name: "Academy of Sciences", Branch: AcademyEconomy,
			EffectText: "+1 to Intergalactic Research Network", Prereq: AcademyPrereq{SkillMagistracy, 12},
			PerLevel: AcademyEffect{IRN: 1}},
		{Code: SkillExpandingEmpire, Name: "Expanding Empire", Branch: AcademyEconomy,
			EffectText: "+1 the maximum number of planets in the Empire", Prereq: AcademyPrereq{SkillAcademyScience, 3},
			PerLevel: AcademyEffect{Planets: 1}},
		{Code: SkillPackingOfCargo, Name: "Packing of cargo", Branch: AcademyEconomy,
			EffectText: "+5% more fleet space", Prereq: AcademyPrereq{SkillCosmoDepot, 5},
			PerLevel:   AcademyEffect{CargoPct: 5},
			CostFactor: 1.341}, // L10=14
		{Code: SkillGlobalDefense, Name: "Global defense", Branch: AcademyEconomy,
			EffectText: "+1 maximum number of shield domes", Prereq: AcademyPrereq{SkillEnergetics, 15}},
		{Code: SkillThermonuclear, Name: "Thermonuclear reaction", Branch: AcademyEconomy,
			EffectText: "+3% to power generation fusion reactor", Prereq: AcademyPrereq{SkillEnergetics, 10}},
		{Code: SkillSolarPanels, Name: "Solar panels", Branch: AcademyEconomy,
			EffectText: "+3% to develop solar power satellites", Prereq: AcademyPrereq{SkillEnergetics, 10}},
		{Code: SkillEmpire, Name: "Empire", Branch: AcademyEconomy,
			EffectText: "+1% attack, armor, shields and ship speed", Prereq: AcademyPrereq{SkillExpandingEmpire, 5},
			PerLevel: AcademyEffect{AttackPct: 1, ShieldPct: 1, HullPct: 1, FleetSpeedPct: 1}},

		// ---- 13xx Defence ------------------------------------------------
		{Code: SkillDefensiveStrategy, Name: "Defensive strategy", Branch: AcademyDefence, MaxLevel: academyDefaultMaxLevel,
			EffectText: "+1% to shields and armor", Prereq: AcademyPrereq{},
			PerLevel:   AcademyEffect{ShieldPct: 1, HullPct: 1},
			CostFactor: 1.199}, // L18=22
		{Code: SkillMechanics, Name: "Mechanics", Branch: AcademyDefence,
			EffectText: "+1% Defense recover", Prereq: AcademyPrereq{SkillDefensiveStrategy, 8},
			CostBase: 2}, // L1=2
		{Code: SkillDefenceClassA, Name: "Defence class A", Branch: AcademyDefence,
			EffectText: "+1% Shields and Armors, -2% Attack", Prereq: AcademyPrereq{SkillDefensiveStrategy, 10},
			PerLevel:   AcademyEffect{ShieldPct: 1, HullPct: 1, AttackPct: -2},
			CostFactor: 1.414}, // L5=4
		{Code: SkillShieldComplex, Name: "Shield complex", Branch: AcademyDefence,
			EffectText: "+1% shields the defense", Prereq: AcademyPrereq{SkillMechanics, 8}},
		{Code: SkillDefenseComplex, Name: "Defense complex", Branch: AcademyDefence,
			EffectText: "+1% armor in defense", Prereq: AcademyPrereq{SkillMechanics, 8}},
		{Code: SkillDoubleShields, Name: "Double shields", Branch: AcademyDefence,
			EffectText: "+2% the probability of double shields", Prereq: AcademyPrereq{SkillDefenceClassA, 4},
			PerLevel:   AcademyEffect{DoubleShieldPct: 2},
			CostFactor: 1.515}, // L10=42
		{Code: SkillSealShieldDome, Name: "Seal Shield Dome", Branch: AcademyDefence,
			EffectText: "+2% the probability of the missile destruction", Prereq: AcademyPrereq{SkillShieldComplex, 3}},
		{Code: SkillStandardDefence, Name: "Standardisation (defense)", Branch: AcademyDefence,
			EffectText: "-1% to the defense value", Prereq: AcademyPrereq{SkillMechanics, 10}},
		{Code: SkillHeavyArmour, Name: "Heavy Armour", Branch: AcademyDefence,
			EffectText: "-2% destruction of rapid fire", Prereq: AcademyPrereq{SkillDoubleShields, 7},
			PerLevel:   AcademyEffect{RFReductPct: 2},
			CostFactor: 2.152}, // L8=214
		{Code: SkillMaximumDefense, Name: "Maximum defense", Branch: AcademyDefence,
			EffectText: "+1 the maximum number of orbital bases", Prereq: AcademyPrereq{SkillDefenseComplex, 20}},
		{Code: SkillSealingShields, Name: "Sealing double shields", Branch: AcademyDefence,
			EffectText: "+3% to density double shield", Prereq: AcademyPrereq{SkillDoubleShields, 5},
			CostFactor: 1.491}, // L7=11
	}
	// Fill defaults and derive the display tier from the prerequisite depth.
	for i := range defs {
		if defs[i].MaxLevel == 0 {
			defs[i].MaxLevel = academyDefaultMaxLevel
		}
		if defs[i].CostBase == 0 {
			defs[i].CostBase = academyDefaultCostBase
		}
		if defs[i].CostFactor == 0 {
			defs[i].CostFactor = academyDefaultCostFactor
		}
	}
	for i := range defs {
		defs[i].Tier = academyTier(defs, defs[i].Code, 0)
	}
	return defs
}

func academyTier(defs []AcademySkill, code string, depth int) int {
	if depth > 8 {
		return depth // cycle guard; the captured graph is a DAG
	}
	for _, d := range defs {
		if d.Code == code {
			if d.Prereq.Code == "" {
				return 1
			}
			return academyTier(defs, d.Prereq.Code, depth+1) + 1
		}
	}
	return depth
}

// AcademySkillByCode resolves a skill by its code.
func AcademySkillByCode(code string) (AcademySkill, bool) {
	for _, d := range AcademySkills {
		if d.Code == code {
			return d, true
		}
	}
	return AcademySkill{}, false
}

// AcademyCost is the Academy-point cost of raising a skill from level-1 to
// level (i.e. the price of that single level).
func AcademyCost(s AcademySkill, level int) int64 {
	if level < 1 {
		return 0
	}
	return int64(math.Round(s.CostBase * math.Pow(s.CostFactor, float64(level-1))))
}

// AcademyCostRange is the total Academy-point cost of raising a skill from
// level `from` up to `to` (exclusive of `from`, inclusive of `to`).
func AcademyCostRange(s AcademySkill, from, to int) int64 {
	if to > s.MaxLevel {
		to = s.MaxLevel
	}
	if to <= from || from < 0 {
		return 0
	}
	var total int64
	for lvl := from + 1; lvl <= to; lvl++ {
		total += AcademyCost(s, lvl)
	}
	return total
}

// AcademyAffordableLevel returns the highest level in (from, to] whose
// cumulative cost does not exceed points; `from` when even the next level is
// unaffordable. `to` is clamped to MaxLevel.
func AcademyAffordableLevel(s AcademySkill, from, to int, points int64) int {
	if to > s.MaxLevel {
		to = s.MaxLevel
	}
	best := from
	var cost int64
	for lvl := from + 1; lvl <= to; lvl++ {
		cost += AcademyCost(s, lvl)
		if cost > points {
			break
		}
		best = lvl
	}
	return best
}

// academySum sums level*coefficient over the owned skills for one effect
// component.
func academySum(levels map[string]int, pick func(AcademyEffect) float64) float64 {
	var total float64
	for _, s := range AcademySkills {
		if lvl := levels[s.Code]; lvl > 0 {
			total += float64(lvl) * pick(s.PerLevel)
		}
	}
	return total
}

// AcademyAttackPct is the flat attack bonus (percent) from attack- and
// defence-branch skills (Weaponry, Weapons Class A, Empire, Defence class A's
// penalty).
func AcademyAttackPct(levels map[string]int) float64 {
	return academySum(levels, func(e AcademyEffect) float64 { return e.AttackPct })
}

// AcademyShieldPct / AcademyHullPct are the flat shield/armour bonuses (percent).
func AcademyShieldPct(levels map[string]int) float64 {
	return academySum(levels, func(e AcademyEffect) float64 { return e.ShieldPct })
}

func AcademyHullPct(levels map[string]int) float64 {
	return academySum(levels, func(e AcademyEffect) float64 { return e.HullPct })
}

// AcademyRFReductionPct is the rapid-fire reduction (percent) this account's
// units inflict (Heavy Armour).
func AcademyRFReductionPct(levels map[string]int) float64 {
	return academySum(levels, func(e AcademyEffect) float64 { return e.RFReductPct })
}

// AcademyDoubleShieldPct is the double-shield proc probability (percent).
func AcademyDoubleShieldPct(levels map[string]int) float64 {
	return academySum(levels, func(e AcademyEffect) float64 { return e.DoubleShieldPct })
}

// AcademyResearchPct is the research-time reduction (percent) from Magistracy.
func AcademyResearchPct(levels map[string]int) float64 {
	return academySum(levels, func(e AcademyEffect) float64 { return e.ResearchPct })
}

// AcademyStoragePct is the storage-capacity bonus (percent) from Cosmo depot.
func AcademyStoragePct(levels map[string]int) float64 {
	return academySum(levels, func(e AcademyEffect) float64 { return e.StoragePct })
}

// AcademyCargoPct is the fleet cargo-space bonus (percent) from Packing of cargo.
func AcademyCargoPct(levels map[string]int) float64 {
	return academySum(levels, func(e AcademyEffect) float64 { return e.CargoPct })
}

// AcademyFleetSpeedPct is the fleet-speed bonus (percent).
func AcademyFleetSpeedPct(levels map[string]int) float64 {
	return academySum(levels, func(e AcademyEffect) float64 { return e.FleetSpeedPct })
}

// AcademyFleetSlots is the extra fleet-slot count (Computerization).
func AcademyFleetSlots(levels map[string]int) int {
	total := 0
	for _, s := range AcademySkills {
		if lvl := levels[s.Code]; lvl > 0 {
			total += lvl * s.PerLevel.FleetSlot
		}
	}
	return total
}

// AcademyIRN is the extra Intergalactic Research Network levels (Academy of
// Sciences).
func AcademyIRN(levels map[string]int) int {
	total := 0
	for _, s := range AcademySkills {
		if lvl := levels[s.Code]; lvl > 0 {
			total += lvl * s.PerLevel.IRN
		}
	}
	return total
}

// AcademyPlanets is the extra maximum-planet count (Expanding Empire).
func AcademyPlanets(levels map[string]int) int {
	total := 0
	for _, s := range AcademySkills {
		if lvl := levels[s.Code]; lvl > 0 {
			total += lvl * s.PerLevel.Planets
		}
	}
	return total
}

// AcademyProductionBonus folds the Production-speed (mine extraction, all three
// resources) and Energetics (energy) skills into an account-wide ProductionBonus.
func AcademyProductionBonus(levels map[string]int) ProductionBonus {
	mines := academySum(levels, func(e AcademyEffect) float64 { return e.MinePct })
	energy := academySum(levels, func(e AcademyEffect) float64 { return e.EnergyPct })
	return ProductionBonus{Metal: mines, Crystal: mines, Deuterium: mines, Energy: energy}
}

// AcademyPrereqMet reports whether the prerequisite for a skill is satisfied by
// the current levels.
func AcademyPrereqMet(s AcademySkill, levels map[string]int) bool {
	if s.Prereq.Code == "" {
		return true
	}
	return levels[s.Prereq.Code] >= s.Prereq.Level
}

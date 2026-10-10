package game

import (
	"math"
	"sort"
	"time"
)

// Cost is a resource price. All amounts are whole units.
type Cost struct {
	Metal     int64
	Crystal   int64
	Deuterium int64
}

// Add returns c + o.
func (c Cost) Add(o Cost) Cost {
	return Cost{Metal: c.Metal + o.Metal, Crystal: c.Crystal + o.Crystal, Deuterium: c.Deuterium + o.Deuterium}
}

// Mul scales every component by n.
func (c Cost) Mul(n int64) Cost {
	return Cost{Metal: c.Metal * n, Crystal: c.Crystal * n, Deuterium: c.Deuterium * n}
}

// StructureDef describes a planet building. Code is the stable string key used
// in the database; ID is the server's numeric building id (captured from HAR).
type StructureDef struct {
	Code        string
	ID          int
	Name        string
	BaseCost    Cost
	CostFactor  float64
	BaseTimeSec float64
	Requires    map[string]int
}

// TechDef describes an empire-wide research technology.
type TechDef struct {
	Code        string
	ID          int
	Name        string
	BaseCost    Cost
	CostFactor  float64
	BaseTimeSec float64
	Requires    map[string]int
}

// UnitDef describes a shipyard-produced ship or a defensive unit. Costs are per
// single unit. Code is the server's numeric unit id.
type UnitDef struct {
	Code        string
	Name        string
	BaseCost    Cost
	BaseTimeSec float64
	Requires    map[string]int
}

// time-calibration constants fitted from the niburuspace.com HAR capture.
// See docs/BALANCE_DATA_NEEDED.md — these are empirical and should be refined
// with captures at varying Robotics/Nanite/University levels.
// buildingTimeCalibration fitted from the fresh Bratwurst account (Robotics 15,
// Nanite 0) across 6 samples: T = (M+C)/(2500*(1+robotics)) * 0.5^nanite
// * 3600/game_speed * calibration. The buffed Fogigy capture runs ~1.88x faster
// (officer / peaceful-level bonuses), which is not modelled here.
const (
	buildingTimeCalibration = 1.20
	unitTimeCalibration     = 1.0

	// researchTimeCalibration is fitted against the captured account's research
	// page (Research Lab 24, University 6, game_speed 4000): every captured tech
	// followed time_seconds ≈ (metal+crystal) · 7.466e-7, i.e. speed
	// 1+0.10·24+0.16·6 = 4.36. See docs/BALANCE_DATA_NEEDED.md.
	researchTimeCalibration = 0.003617

	// Research-speed bonuses, both **local to the planet where the research is
	// started**: each research-lab level adds 10% and each University level adds
	// 16%. The University is NOT shared account-wide via IRN — IRN only links
	// additional research labs (see EffectiveResearchLabLevel).
	researchLabSpeedPerLevel = 0.10
	universitySpeedPerLevel  = 0.16
)

func buildStructures(defs []StructureDef) map[string]StructureDef {
	m := make(map[string]StructureDef, len(defs))
	for _, d := range defs {
		m[d.Code] = d
	}
	return m
}

func buildTechs(defs []TechDef) map[string]TechDef {
	m := make(map[string]TechDef, len(defs))
	for _, d := range defs {
		m[d.Code] = d
	}
	return m
}

func buildUnits(defs []UnitDef) map[string]UnitDef {
	m := make(map[string]UnitDef, len(defs))
	for _, d := range defs {
		m[d.Code] = d
	}
	return m
}

// Structures is the static catalog of planet buildings, calibrated from the
// niburuspace.com capture (base cost = observed cost / factor^level).
var Structures = buildStructures([]StructureDef{
	{Code: "metal_mine", ID: 1, Name: "Metal Mine", BaseCost: Cost{Metal: 60, Crystal: 15}, CostFactor: 1.5},
	{Code: "crystal_mine", ID: 2, Name: "Crystal Mine", BaseCost: Cost{Metal: 48, Crystal: 24}, CostFactor: 1.5},
	{Code: "deuterium_synthesizer", ID: 3, Name: "Deuterium Refinery", BaseCost: Cost{Metal: 225, Crystal: 75}, CostFactor: 1.5},
	{Code: "solar_plant", ID: 4, Name: "Solar Power Plant", BaseCost: Cost{Metal: 75, Crystal: 30}, CostFactor: 1.5},
	{Code: "university", ID: 6, Name: "University", BaseCost: Cost{Metal: 100000000, Crystal: 50000000, Deuterium: 25000000}, CostFactor: 2.0},
	{Code: "deuterium_power_plant", ID: 12, Name: "Deuterium Power Plant", BaseCost: Cost{Metal: 900, Crystal: 360, Deuterium: 180}, CostFactor: 2.0},
	{Code: "robotics_factory", ID: 14, Name: "Robot Factory", BaseCost: Cost{Metal: 400, Crystal: 120, Deuterium: 200}, CostFactor: 2.0},
	{Code: "nanite_factory", ID: 15, Name: "Nanite Factory", BaseCost: Cost{Metal: 1000000, Crystal: 500000, Deuterium: 100000}, CostFactor: 2.0, Requires: map[string]int{"robotics_factory": 10, "shipyard": 8}},
	{Code: "shipyard", ID: 21, Name: "Shipyard", BaseCost: Cost{Metal: 400, Crystal: 200, Deuterium: 100}, CostFactor: 2.0, Requires: map[string]int{"robotics_factory": 2}},
	{Code: "metal_storage", ID: 22, Name: "Metal Storage", BaseCost: Cost{Metal: 2000}, CostFactor: 2.0},
	{Code: "crystal_storage", ID: 23, Name: "Crystal Storage", BaseCost: Cost{Metal: 2000, Crystal: 1000}, CostFactor: 2.0},
	{Code: "deuterium_storage", ID: 24, Name: "Deuterium Storage", BaseCost: Cost{Metal: 2000, Crystal: 2000}, CostFactor: 2.0},
	{Code: "research_lab", ID: 31, Name: "Research Lab", BaseCost: Cost{Metal: 200, Crystal: 400, Deuterium: 200}, CostFactor: 2.0},
	{Code: "terraformer", ID: 33, Name: "Terraformer", BaseCost: Cost{Crystal: 50000, Deuterium: 100000}, CostFactor: 2.0},
	{Code: "alliance_depot", ID: 34, Name: "Alliance Depot", BaseCost: Cost{Metal: 20000, Crystal: 40000}, CostFactor: 2.0},
	{Code: "missile_silo", ID: 44, Name: "Missile Silo", BaseCost: Cost{Metal: 20000, Crystal: 20000, Deuterium: 1000}, CostFactor: 2.0},
	{Code: "light_conveyor", ID: 71, Name: "Light Conveyor", BaseCost: Cost{Metal: 1500000, Crystal: 500000, Deuterium: 200000}, CostFactor: 2.0},
	{Code: "average_conveyor", ID: 72, Name: "Average Conveyor", BaseCost: Cost{Metal: 3500000, Crystal: 1500000, Deuterium: 600000}, CostFactor: 2.0},
	{Code: "heavy_conveyor", ID: 73, Name: "Heavy Conveyor", BaseCost: Cost{Metal: 7500000, Crystal: 3500000, Deuterium: 1500000}, CostFactor: 2.0},
})

// Techs is the static catalog of research technologies, calibrated from HAR.
var Techs = buildTechs([]TechDef{
	{Code: "espionage_tech", ID: 106, Name: "Spy Technology", BaseCost: Cost{Metal: 200, Crystal: 1000, Deuterium: 200}, CostFactor: 2.0},
	{Code: "computer_tech", ID: 108, Name: "Computer Technology", BaseCost: Cost{Crystal: 400, Deuterium: 600}, CostFactor: 2.0},
	{Code: "weapons_tech", ID: 109, Name: "Weapons Technology", BaseCost: Cost{Metal: 800, Crystal: 200}, CostFactor: 2.0},
	{Code: "shielding_tech", ID: 110, Name: "Shield Technology", BaseCost: Cost{Metal: 200, Crystal: 600}, CostFactor: 2.0},
	{Code: "armour_tech", ID: 111, Name: "Armour Technology", BaseCost: Cost{Metal: 1000}, CostFactor: 2.0},
	{Code: "energy_tech", ID: 113, Name: "Energy Technology", BaseCost: Cost{Crystal: 800, Deuterium: 400}, CostFactor: 2.0},
	{Code: "hyperspace_tech", ID: 114, Name: "Hyperspace Technology", BaseCost: Cost{Crystal: 4000, Deuterium: 2000}, CostFactor: 2.0},
	{Code: "combustion_drive", ID: 115, Name: "Combustion Engine", BaseCost: Cost{Metal: 400, Deuterium: 600}, CostFactor: 2.0},
	{Code: "impulse_drive", ID: 117, Name: "Impulse Engine", BaseCost: Cost{Metal: 2000, Crystal: 4000, Deuterium: 600}, CostFactor: 2.0},
	{Code: "hyperspace_drive", ID: 118, Name: "Hyperspace Engine", BaseCost: Cost{Metal: 10000, Crystal: 20000, Deuterium: 6000}, CostFactor: 2.0},
	{Code: "laser_tech", ID: 120, Name: "Laser Technology", BaseCost: Cost{Metal: 200, Crystal: 100}, CostFactor: 2.0},
	{Code: "ion_tech", ID: 121, Name: "Ion Technology", BaseCost: Cost{Metal: 1000, Crystal: 300, Deuterium: 100}, CostFactor: 2.0},
	{Code: "plasma_tech", ID: 122, Name: "Plasma Technology", BaseCost: Cost{Metal: 2000, Crystal: 4000, Deuterium: 1000}, CostFactor: 2.0},
	{Code: "intergalactic_research_network", ID: 123, Name: "Intergalactic Research Network", BaseCost: Cost{Metal: 240000, Crystal: 400000, Deuterium: 160000}, CostFactor: 2.0},
	{Code: "astrophysics", ID: 124, Name: "Astrophysics", BaseCost: Cost{Metal: 4000, Crystal: 8000, Deuterium: 4000}, CostFactor: 4.8961, Requires: map[string]int{"espionage_tech": 4, "research_lab": 3}},
	{Code: "brother_hood", ID: 125, Name: "Brother Hood", BaseCost: Cost{Metal: 237, Crystal: 415}, CostFactor: 2.0},
	{Code: "mineral_research", ID: 131, Name: "Mineral Research", BaseCost: Cost{Metal: 750, Crystal: 500, Deuterium: 250}, CostFactor: 2.0},
	{Code: "semi_crystals_research", ID: 132, Name: "Semi-Crystals Research", BaseCost: Cost{Metal: 1000, Crystal: 750, Deuterium: 500}, CostFactor: 2.0},
	{Code: "fuel_research", ID: 133, Name: "Fuel Research", BaseCost: Cost{Metal: 1250, Crystal: 1000, Deuterium: 750}, CostFactor: 2.0},
	{Code: "graviton_research", ID: 199, Name: "Graviton Research", BaseCost: Cost{}, CostFactor: 2.0},
})

// Ships is the static catalog of ships, keyed by the server's numeric code.
var Ships = buildUnits([]UnitDef{
	{Code: "202", Name: "Light Cargo", BaseCost: Cost{Metal: 2000, Crystal: 2000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "203", Name: "Heavy Cargo", BaseCost: Cost{Metal: 6000, Crystal: 6000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "204", Name: "Light Fighter", BaseCost: Cost{Metal: 3000, Crystal: 1000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "205", Name: "Heavy Fighter", BaseCost: Cost{Metal: 8000, Crystal: 3000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "206", Name: "Cruiser", BaseCost: Cost{Metal: 15000, Crystal: 9500, Deuterium: 2000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "207", Name: "Battleship", BaseCost: Cost{Metal: 41000, Crystal: 17000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "208", Name: "Colony Ship", BaseCost: Cost{Metal: 250000, Crystal: 200000, Deuterium: 100000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "209", Name: "Recycler", BaseCost: Cost{Metal: 10000, Crystal: 6000, Deuterium: 2000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "210", Name: "Spy Probe", BaseCost: Cost{Crystal: 7500}, Requires: map[string]int{"shipyard": 1}},
	{Code: "211", Name: "Planet Bomber", BaseCost: Cost{Metal: 70000, Crystal: 45000, Deuterium: 5000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "212", Name: "Solar Satellite", BaseCost: Cost{Crystal: 4000, Deuterium: 2000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "213", Name: "Star Fighter", BaseCost: Cost{Metal: 60000, Crystal: 50000, Deuterium: 15000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "214", Name: "Battle Fortress", BaseCost: Cost{Metal: 6000000, Crystal: 3500000, Deuterium: 1000000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "215", Name: "Battle Cruiser", BaseCost: Cost{Metal: 50000, Crystal: 40000, Deuterium: 10000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "216", Name: "Black Moon", BaseCost: Cost{Metal: 8000000, Crystal: 4000000, Deuterium: 500000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "217", Name: "Battle Transporter", BaseCost: Cost{Metal: 35000, Crystal: 20000, Deuterium: 1500}, Requires: map[string]int{"shipyard": 1}},
	{Code: "218", Name: "Avatar", BaseCost: Cost{}, Requires: map[string]int{"shipyard": 1}},
	{Code: "219", Name: "Battle Recycler", BaseCost: Cost{Metal: 1000000, Crystal: 600000, Deuterium: 200000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "220", Name: "Dark Matter Collector", BaseCost: Cost{Metal: 60000000, Crystal: 70000000, Deuterium: 30000000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "221", Name: "Battleship class ONill", BaseCost: Cost{}, Requires: map[string]int{"shipyard": 1}},
	{Code: "222", Name: "Flying Death", BaseCost: Cost{}, Requires: map[string]int{"shipyard": 1}},
	{Code: "223", Name: "Scrappy", BaseCost: Cost{Metal: 50000, Crystal: 2500000, Deuterium: 10000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "225", Name: "Galleon", BaseCost: Cost{Metal: 900000, Crystal: 700000, Deuterium: 200000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "226", Name: "Destroyer", BaseCost: Cost{Metal: 3000000, Crystal: 2000000, Deuterium: 200000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "227", Name: "Frigate", BaseCost: Cost{Metal: 30000000, Crystal: 10000000, Deuterium: 2000000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "228", Name: "Black Wanderer", BaseCost: Cost{Metal: 80000000, Crystal: 40000000, Deuterium: 7000000}, Requires: map[string]int{"shipyard": 1}},
})

// Defenses is the static catalog of defensive units, keyed by server numeric code.
var Defenses = buildUnits([]UnitDef{
	{Code: "401", Name: "Missile Launcher", BaseCost: Cost{Metal: 2000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "402", Name: "Light Laser Turret", BaseCost: Cost{Metal: 1500, Crystal: 500}, Requires: map[string]int{"shipyard": 1}},
	{Code: "403", Name: "Heavy Laser Turret", BaseCost: Cost{Metal: 6000, Crystal: 2000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "404", Name: "Gauss Cannon", BaseCost: Cost{Metal: 20000, Crystal: 15000, Deuterium: 2000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "405", Name: "Ion Cannon", BaseCost: Cost{Metal: 2000, Crystal: 6000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "406", Name: "Plasma Cannon", BaseCost: Cost{Metal: 50000, Crystal: 50000, Deuterium: 30000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "407", Name: "Small Shield Dome", BaseCost: Cost{Metal: 1000000, Crystal: 1000000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "408", Name: "Large Shield Dome", BaseCost: Cost{Metal: 5000000, Crystal: 5000000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "409", Name: "Atmospheric Shield", BaseCost: Cost{}, Requires: map[string]int{"shipyard": 1}},
	{Code: "410", Name: "Gravitons Cannon", BaseCost: Cost{}, Requires: map[string]int{"shipyard": 1}},
	{Code: "411", Name: "Orbital Defence Platform", BaseCost: Cost{}, Requires: map[string]int{"shipyard": 1}},
	{Code: "412", Name: "Lepton Gun", BaseCost: Cost{}, Requires: map[string]int{"shipyard": 1}},
	{Code: "413", Name: "Proton Gun", BaseCost: Cost{}, Requires: map[string]int{"shipyard": 1}},
	{Code: "414", Name: "Canyon", BaseCost: Cost{}, Requires: map[string]int{"shipyard": 1}},
	{Code: "415", Name: "Quantum Gun", BaseCost: Cost{}, Requires: map[string]int{"shipyard": 1}},
	{Code: "416", Name: "Hydrogen Gun", BaseCost: Cost{Metal: 200000, Crystal: 150000, Deuterium: 50000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "417", Name: "Dora Gun", BaseCost: Cost{Metal: 350000, Crystal: 150000, Deuterium: 75000}, Requires: map[string]int{"shipyard": 1}},
	{Code: "418", Name: "Photon Cannon", BaseCost: Cost{}, Requires: map[string]int{"shipyard": 1}},
	{Code: "419", Name: "Particle Emitter", BaseCost: Cost{}, Requires: map[string]int{"shipyard": 1}},
	{Code: "502", Name: "Interceptor", BaseCost: Cost{}, Requires: map[string]int{"shipyard": 1}},
	{Code: "503", Name: "Interplanetary Missiles", BaseCost: Cost{}, Requires: map[string]int{"shipyard": 1}},
})

// scaleCost grows a base price by factor^(targetLevel-1), rounding up.
func scaleCost(base Cost, factor float64, targetLevel int) Cost {
	if targetLevel < 1 {
		targetLevel = 1
	}
	mult := pow(factor, float64(targetLevel-1))
	return Cost{
		Metal:     int64(math.Round(float64(base.Metal) * mult)),
		Crystal:   int64(math.Round(float64(base.Crystal) * mult)),
		Deuterium: int64(math.Round(float64(base.Deuterium) * mult)),
	}
}

// StructureCost returns the cost to reach targetLevel of a structure.
func StructureCost(code string, targetLevel int) (Cost, bool) {
	def, ok := Structures[code]
	if !ok {
		return Cost{}, false
	}
	return scaleCost(def.BaseCost, def.CostFactor, targetLevel), true
}

// TechCost returns the cost to reach targetLevel of a technology.
func TechCost(code string, targetLevel int) (Cost, bool) {
	def, ok := Techs[code]
	if !ok {
		return Cost{}, false
	}
	return scaleCost(def.BaseCost, def.CostFactor, targetLevel), true
}

// LookupUnit finds a ship or defense by numeric code.
func LookupUnit(code string) (UnitDef, bool) {
	if d, ok := Ships[code]; ok {
		return d, true
	}
	if d, ok := Defenses[code]; ok {
		return d, true
	}
	return UnitDef{}, false
}

// UnitCost returns the cost for quantity units of a ship or defense.
func UnitCost(code string, quantity int64) (Cost, bool) {
	def, ok := LookupUnit(code)
	if !ok || quantity < 1 {
		return Cost{}, false
	}
	return def.BaseCost.Mul(quantity), true
}

// ShipCost is retained for the ship-only call sites/tests.
func ShipCost(code string, quantity int64) (Cost, bool) {
	return UnitCost(code, quantity)
}

// RequiresMet reports whether every requirement in requires is satisfied by
// the supplied levels map. Missing entries count as level 0.
func RequiresMet(requires map[string]int, levels map[string]int) bool {
	for code, needed := range requires {
		if levels[code] < needed {
			return false
		}
	}
	return true
}

// StructureDuration is the build time to reach targetLevel, shortened by the
// planet's robotics factory and halved per nanite factory level.
func StructureDuration(code string, targetLevel, roboticsFactory, naniteFactory int, gameSpeed float64) time.Duration {
	if _, ok := Structures[code]; !ok {
		return time.Second
	}
	cost, _ := StructureCost(code, targetLevel)
	return buildingDuration(cost.Metal+cost.Crystal, roboticsFactory, naniteFactory, gameSpeed, buildingTimeCalibration)
}

// EffectiveResearchLabLevel is the research-lab level used for the research-time
// speed bonus on the planet where research is started. That planet's own lab
// always counts; the Intergalactic Research Network then connects up to irnLevel
// further colonies, highest lab level first. A colony without a research lab
// (level 0) contributes nothing, so IRN levels beyond the number of lab-bearing
// colonies are wasted. colonyLabs are the lab levels of the *other* celestials.
func EffectiveResearchLabLevel(localLab, irnLevel int, colonyLabs []int) int {
	labs := make([]int, 0, len(colonyLabs))
	for _, l := range colonyLabs {
		if l > 0 {
			labs = append(labs, l)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(labs)))
	total := localLab
	for i := 0; i < irnLevel && i < len(labs); i++ {
		total += labs[i]
	}
	return total
}

// TechDuration is the research time to reach targetLevel. The starting planet's
// effective research-lab level (its own lab plus IRN-connected colony labs) and
// its University shorten the time: each research-lab level adds 10% research
// speed and each University level adds 16%, both local to that planet.
func TechDuration(code string, targetLevel, labLevel, universityLevel int, gameSpeed float64) time.Duration {
	cost, ok := TechCost(code, targetLevel)
	if !ok {
		return time.Second
	}
	base := float64(cost.Metal + cost.Crystal)
	speed := 1.0 + researchLabSpeedPerLevel*float64(labLevel) + universitySpeedPerLevel*float64(universityLevel)
	hours := base / (1000.0 * speed)
	if gameSpeed > 0 {
		hours /= gameSpeed
	}
	return secondsToDuration(hours * 3600.0 * researchTimeCalibration)
}

// UnitDuration is the total time to build quantity ships/defenses, shortened by
// the shipyard and halved per nanite factory level.
func UnitDuration(code string, quantity int64, shipyard, naniteFactory int, gameSpeed float64) time.Duration {
	def, ok := LookupUnit(code)
	if !ok || quantity < 1 {
		return time.Second
	}
	base := float64(def.BaseCost.Metal+def.BaseCost.Crystal) * float64(quantity)
	return buildingDuration(int64(base), shipyard, naniteFactory, gameSpeed, unitTimeCalibration)
}

// ShipDuration is retained for ship-only call sites/tests.
func ShipDuration(code string, quantity int64, shipyard, naniteFactory int, gameSpeed float64) time.Duration {
	return UnitDuration(code, quantity, shipyard, naniteFactory, gameSpeed)
}

func buildingDuration(resourceSum int64, roboticsFactory, naniteFactory int, gameSpeed, calibration float64) time.Duration {
	hours := float64(resourceSum) / (2500.0 * (1.0 + float64(roboticsFactory)))
	hours *= pow(0.5, float64(naniteFactory))
	if gameSpeed > 0 {
		hours /= gameSpeed
	}
	return secondsToDuration(hours * 3600.0 * calibration)
}

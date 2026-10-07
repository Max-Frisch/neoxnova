package game

import (
	"math"
	"math/rand"
	"time"
)

// Moon formulas + catalog, captured from the live acc1 moon (cp=1725, 3:125:12,
// diameter 8,426 km). See docs/MOONS.md for the raw captures.
//
// The creation chance itself lives in combat.go: MoonChance(metal, crystal) =
// min(floor((metal+crystal)/100000), 20) percent.

// MoonIndestructibleKm is the server rule: a moon at or above this diameter
// cannot be destroyed. Standard moons cap out at 8,944 km, so this is a
// server-specific safeguard.
const MoonIndestructibleKm = 10000

// MoonDiameterKm is the standard OGame moon diameter:
//
//	floor( sqrt(x + 3*p) * 1000 ) km
//
// where p is the creation chance (percent) and x is a uniform integer in
// [10,20]. x=10..20 yields 3,605 km (p=1) up to 8,944 km (p=20); at the 20 %
// cap a moon is always >= 8,366 km. roll is clamped into [10,20].
func MoonDiameterKm(chancePct, roll int) int {
	if chancePct < 1 {
		chancePct = 1
	}
	if chancePct > 20 {
		chancePct = 20 // the creation chance is capped at 20%
	}
	if roll < 10 {
		roll = 10
	}
	if roll > 20 {
		roll = 20
	}
	return int(math.Floor(math.Sqrt(float64(roll+3*chancePct)) * 1000.0))
}

// MoonCreation rolls whether an attack at a planet spawns a moon, given the
// creation chance (percent) and a deterministic seed. It returns the creation
// flag and the rolled diameter (0 when none). The chance roll and the diameter
// x-roll are drawn in a fixed order, so a retried event resolves identically.
func MoonCreation(chancePct int, seed int64) (bool, int) {
	if chancePct <= 0 {
		return false, 0
	}
	rng := rand.New(rand.NewSource(seed))
	if rng.Intn(100) >= chancePct {
		return false, 0
	}
	x := 10 + rng.Intn(11) // 10..20
	return true, MoonDiameterKm(chancePct, x)
}

// MoonFieldsMax is a moon's field capacity granted by the Moon base: the live
// info card says "each level increases the free fields on the moon by 3" and
// "one field occupies itself Moon Base". A fresh moon starts at 0 (but can still
// build its first Moon base). Other field sources — the premium/cashshop
// "+N fields on the moon" bonus and Planetarium Dark-Matter buys — are added by
// the caller. Each building level occupies one field.
func MoonFieldsMax(moonBaseLevel int) int {
	if moonBaseLevel < 0 {
		moonBaseLevel = 0
	}
	return 3 * moonBaseLevel
}

// MoonFields is the classic diameter-derived field count, floor((d/1000)^2).
// Not used by this server (see MoonFieldsMax); kept for reference.
func MoonFields(diameterKm int) int {
	if diameterKm <= 0 {
		return 0
	}
	f := float64(diameterKm) / 1000.0
	return int(f * f)
}

// PhalanxRange is the number of systems a Phalanx Sensor scans, per its live
// info card: level^2 - 1. Level 0 => 0.
func PhalanxRange(level int) int {
	if level <= 0 {
		return 0
	}
	return level*level - 1
}

// JumpgateCooldown is the recharge between jumps. The live info card says the
// base is at least one hour and "with each level, cooldown [is] reduced by 2
// times". PROVISIONAL (needs a second moon to measure).
func JumpgateCooldown(level int) time.Duration {
	if level < 0 {
		level = 0
	}
	if level > 12 {
		level = 12
	}
	return time.Duration(3600>>uint(level)) * time.Second
}

// MoonDestruction returns the percent chance to destroy a moon of diameterKm
// using `deathstars` Battle Fortresses, and the percent chance the attacking
// Battle Fortresses are destroyed (a single roll for the whole fleet):
//
//	destroy  = (100 - sqrt(S)) * sqrt(D)
//	rip loss = sqrt(S) / 2
//
// A moon at/above MoonIndestructibleKm can never be destroyed. PROVISIONAL:
// standard OGame, not yet measured on this server. Apply the Moon base reduction
// separately with MoonBaseDestructionReduction.
func MoonDestruction(diameterKm, deathstars int) (destroyPct, deathstarLossPct float64) {
	if diameterKm <= 0 || deathstars <= 0 || diameterKm >= MoonIndestructibleKm {
		return 0, 0
	}
	root := math.Sqrt(float64(diameterKm))
	destroyPct = (100.0 - root) * math.Sqrt(float64(deathstars))
	if destroyPct < 0 {
		destroyPct = 0
	}
	if destroyPct > 100 {
		destroyPct = 100
	}
	deathstarLossPct = root / 2.0
	return destroyPct, deathstarLossPct
}

// MoonBaseDestructionReduction is the fraction (0..1) by which the Moon base
// lowers the moon-destruction chance: the live card says "each 2 [levels] reduce
// [it] by 3 %". PROVISIONAL form (multiplicative).
func MoonBaseDestructionReduction(moonBaseLevel int) float64 {
	if moonBaseLevel < 2 {
		return 0
	}
	r := 0.03 * float64(moonBaseLevel/2)
	if r > 1 {
		r = 1
	}
	return r
}

// moonOnlyStructures are buildings that exist only on a moon (live `page=buildings`
// capture). Costs are base values; the level-N cost is base * 2^(N-1).
var moonOnlyStructures = map[string]StructureDef{
	"moon_base":      {Code: "moon_base", ID: 41, Name: "Moon base", BaseCost: Cost{Metal: 20000, Crystal: 40000, Deuterium: 20000}, CostFactor: 2.0},
	"phalanx_sensor": {Code: "phalanx_sensor", ID: 42, Name: "Phalax Sensor", BaseCost: Cost{Metal: 20000, Crystal: 40000, Deuterium: 20000}, CostFactor: 2.0, Requires: map[string]int{"moon_base": 1}},
	"jumpgate":       {Code: "jumpgate", ID: 43, Name: "Jumpgate", BaseCost: Cost{Metal: 2000000, Crystal: 4000000, Deuterium: 2000000}, CostFactor: 2.0, Requires: map[string]int{"moon_base": 1, "shipyard": 1}},
}

// moonLegalPlanetStructures are the planet buildings that are also buildable on
// a moon (live capture). Mines, power plants, Terraformer and the Missile Silo
// are NOT moon-legal.
var moonLegalPlanetStructures = map[string]bool{
	"robotics_factory": true,
	"nanite_factory":   true,
	"shipyard":         true,
	"alliance_depot":   true,
	"light_conveyor":   true,
	"average_conveyor": true,
	"heavy_conveyor":   true,
}

// MoonStructure resolves a building code buildable on a moon to its definition.
// Moon-only buildings resolve from the moon catalog; moon-legal planet buildings
// reuse their planet definition. Research is account-wide (not a moon building).
func MoonStructure(code string) (StructureDef, bool) {
	if d, ok := moonOnlyStructures[code]; ok {
		return d, true
	}
	if moonLegalPlanetStructures[code] {
		d, ok := Structures[code]
		return d, ok
	}
	return StructureDef{}, false
}

// MoonOnlyStructureByID resolves a live moon-building numeric id (41/42/43).
func MoonOnlyStructureByID(id int) (StructureDef, bool) {
	for _, d := range moonOnlyStructures {
		if d.ID == id {
			return d, true
		}
	}
	return StructureDef{}, false
}

package game

// Arsenal (upgrades) — pure catalog and rules.
//
// Source of truth: docs/ARSENAL_UPGRADES_IMPLEMENTATION.md and the live capture
// docs/ARSENAL_LIVE_2026-10-06.md. The per-activation values below are the live
// `page=arsenal` `(+Y)` brackets; they supersede the older Russian-manual
// percentages. Anything still marked provisional is called out in comments.
//
// An upgrade is a fleet-wide bonus that drops as an un-activated "drawing"
// (upgrade item). Activating one item raises its level/value. Only the
// persistence half lives in internal/store; every rule here is pure.

// UpgradeDef describes one of the 19 Arsenal upgrades.
//
// Code is the market/sell-form `type` id (1..19). Key is the live activate-form
// `greid` internal name where known (only "combustion" has been observed, so the
// rest are left empty rather than guessed). PerLevel is the value added by one
// successful activation, in the same unit the Arsenal page displays (`+Y`).
type UpgradeDef struct {
	Code     int
	Key      string
	Name     string
	Group    string // weapon | armor | shield | engine | conveyor | production
	Class    string // catalog class key; see unit_classes.go for the card mapping
	PerLevel float64
}

var (
	upgradeByCode = buildUpgrades([]UpgradeDef{
		{Code: 1, Name: "Laser weapons", Group: "weapon", Class: "laser", PerLevel: 0.75},
		{Code: 2, Name: "Ion cannon", Group: "weapon", Class: "ion", PerLevel: 0.75},
		{Code: 3, Name: "Plasma gun", Group: "weapon", Class: "plasma", PerLevel: 0.75},
		{Code: 4, Name: "Gravitational gun", Group: "weapon", Class: "gravitational", PerLevel: 0.75},
		{Code: 5, Name: "Light armor", Group: "armor", Class: "light", PerLevel: 0.6},
		{Code: 6, Name: "Medium armor", Group: "armor", Class: "medium", PerLevel: 0.5},
		{Code: 7, Name: "Heavy armor", Group: "armor", Class: "heavy", PerLevel: 0.4},
		{Code: 8, Name: "Light shields", Group: "shield", Class: "light", PerLevel: 0.6},
		{Code: 9, Name: "Medium shields", Group: "shield", Class: "medium", PerLevel: 0.5},
		{Code: 10, Name: "Heavy shields", Group: "shield", Class: "heavy", PerLevel: 0.4},
		{Code: 11, Key: "combustion", Name: "Jet engine", Group: "engine", Class: "combustion", PerLevel: 0.6},
		{Code: 12, Name: "Impulse engine", Group: "engine", Class: "impulse", PerLevel: 0.5},
		{Code: 13, Name: "Hyperspace engine", Group: "engine", Class: "hyperspace", PerLevel: 0.4},
		{Code: 14, Name: "Light conveyor", Group: "conveyor", Class: "light", PerLevel: 0.6},
		{Code: 15, Name: "Average conveyor", Group: "conveyor", Class: "medium", PerLevel: 0.5},
		{Code: 16, Name: "Heavy conveyor", Group: "conveyor", Class: "heavy", PerLevel: 0.4},
		{Code: 17, Name: "Metal production", Group: "production", PerLevel: 0.5},
		{Code: 18, Name: "Crystal production", Group: "production", PerLevel: 0.45},
		{Code: 19, Name: "Deuterium production", Group: "production", PerLevel: 0.4},
	})
)

func buildUpgrades(defs []UpgradeDef) map[int]UpgradeDef {
	m := make(map[int]UpgradeDef, len(defs))
	for _, d := range defs {
		m[d.Code] = d
	}
	return m
}

// Upgrades is the static upgrade catalog keyed by the 1..19 `type` code.
var Upgrades = upgradeByCode

// UpgradeByKey resolves a live `greid` activate-form key to its definition.
func UpgradeByKey(key string) (UpgradeDef, bool) {
	for _, d := range Upgrades {
		if d.Key != "" && d.Key == key {
			return d, true
		}
	}
	return UpgradeDef{}, false
}

// UpgradeByCode resolves the market `type` id to its definition.
func UpgradeByCode(code int) (UpgradeDef, bool) {
	d, ok := Upgrades[code]
	return d, ok
}

// upgradeCodeByClass indexes the catalog by "group/class" so a unit's declared
// card class (see unit_classes.go) resolves to the upgrade that applies to it.
var upgradeCodeByClass = func() map[string]int {
	m := make(map[string]int, len(Upgrades))
	for _, d := range Upgrades {
		if d.Class != "" {
			m[d.Group+"/"+d.Class] = d.Code
		}
	}
	return m
}()

// Activation chances (manual "Важно").
const (
	arsenalGuaranteedLevel  = 10   // first 10 activations always succeed
	arsenalChanceStep       = 0.02 // -2% per success above level 10
	arsenalChanceFloor      = 0.75 // never below 75%
	arsenalFailureValueDrop = 0.10 // a failed activation loses 10% of the last gain
)

// ArsenalTier returns 0 (<5k), 1 (>=5k), 2 (>=50k) or 3 (>=250k) from a fleet's
// points. The user-confirmed tiers; the manual's 75,000-pt figure is ignored.
func ArsenalTier(points int64) int {
	switch {
	case points >= 250_000:
		return 3
	case points >= 50_000:
		return 2
	case points >= 5_000:
		return 1
	default:
		return 0
	}
}

// ArsenalTierMinPoints returns the lower bound (inclusive) of a tier, or 0.
func ArsenalTierMinPoints(tier int) int64 {
	switch tier {
	case 1:
		return 5_000
	case 2:
		return 50_000
	case 3:
		return 250_000
	default:
		return 0
	}
}

// Drop chance constants (live: ~10% of expedition combat wins; all four live
// finds were entry tier at ~7.7k–10.9k points).
const ArsenalDropChance = 0.10

// lightPool/mediumPool/heavyPool mirror the Hostail Barbarian/Pirate/Alien race
// drops. Generic conveyors/production upgrades are available from tier 1 on.
//
// PROVISIONAL: whether a tier gates the *type pool* or the *drop chance* (or
// both) is unconfirmed — see docs/ARSENAL_UPGRADES_IMPLEMENTATION.md §2.2/§5.
var (
	lightPool  = []int{1, 2, 5, 8, 11}
	mediumPool = []int{3, 6, 9, 12}
	heavyPool  = []int{4, 7, 10, 13}
	genericDef = []int{14, 15, 16, 17, 18, 19}
)

// DropPool returns the upgrade codes that can drop at a tier. Tier 0 yields nil.
func DropPool(tier int) []int {
	if tier <= 0 {
		return nil
	}
	pool := append([]int{}, genericDef...)
	pool = append(pool, lightPool...)
	if tier >= 2 {
		pool = append(pool, mediumPool...)
	}
	if tier >= 3 {
		pool = append(pool, heavyPool...)
	}
	return pool
}

// ActivationChance is the success probability of the activation that would take
// an upgrade from level to level+1. The first 10 levels are guaranteed; each
// success above level 10 subtracts 2%, floored at 75%.
func ActivationChance(level int) float64 {
	if level < arsenalGuaranteedLevel {
		return 1.0
	}
	chance := 1.0 - arsenalChanceStep*float64(level-arsenalGuaranteedLevel+1)
	if chance < arsenalChanceFloor {
		return arsenalChanceFloor
	}
	return chance
}

// Activate resolves one activation. roll must be in [0,1). It returns the new
// level and value, whether the activation succeeded, and the chance it faced.
//
// A success raises level by one and adds PerLevel to the value. A failure leaves
// the level unchanged and shaves 10% of PerLevel off the value (never below 0).
// The item is consumed by the caller either way (open question in the brief;
// matches the manual's "value drops" wording).
func Activate(def UpgradeDef, level int, value, roll float64) (newLevel int, newValue float64, success bool, chance float64) {
	chance = ActivationChance(level)
	if roll < chance {
		return level + 1, value + def.PerLevel, true, chance
	}
	newValue = value - arsenalFailureValueDrop*def.PerLevel
	if newValue < 0 {
		newValue = 0
	}
	return level, newValue, false, chance
}

// ShippedBonus is the accumulated value of an owned upgrade (sum of successful
// activations, minus failure penalties). Kept as a named helper so the API and
// tests agree on the meaning of the stored value column.
func ShippedBonus(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}

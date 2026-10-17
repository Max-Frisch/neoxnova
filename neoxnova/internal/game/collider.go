package game

import "math"

// Moon Collider (live `page=kollaider`, numeric building id 69). It is a
// moon-only passive structure bought with Antimatter that produces Dark Matter
// per hour, independent of the moon's size or fields. Because it runs on
// antimatter and cannot be built from resources, it is deliberately **not** part
// of the resource-cost structure catalog (Structures/MoonOnlyStructures): it is
// levelled directly on its own page (see /collider/{moon}) and stored in
// celestial_objects.collider_level.
//
// Capture (2026-10-17, page at level L shows level L+1):
//
//	level 0 → 4 752 DM/h, 850 ATM
//	level 1 → 10 454 DM/h, 1 148 ATM
//	level 2 → 17 250 DM/h, 1 549 ATM
//
// The cost curve is clean (×1.35 per level). The production curve is
// **under-determined** by three points: the table below is authoritative for
// the captured levels and a quadratic is used only to extrapolate beyond them.
// Capture levels 3–6 before locking a formula (see
// docs/COLLIDER_EMPIRE_PLAYBOOK.md §1).

// ColliderID is the reference server's numeric building id for the Collider.
const ColliderID = 69

// ColliderCode is the stable catalog key.
const ColliderCode = "collider"

// ColliderName is the displayed name.
const ColliderName = "Collider"

// ColliderDef describes the Collider for id/sprite/name resolution. It carries
// no resource cost: the level price is Antimatter (ColliderCost).
var ColliderDef = StructureDef{
	Code: ColliderCode, ID: ColliderID, Name: ColliderName, CostFactor: 1.35,
}

const (
	colliderBaseAtmCost = 850.0
	colliderCostFactor  = 1.35
)

// ColliderCost returns the Antimatter price of reaching targetLevel from
// targetLevel-1: round(850 · 1.35^(level-1)). Levels below 1 clamp to 1.
func ColliderCost(targetLevel int) int64 {
	if targetLevel < 1 {
		targetLevel = 1
	}
	return int64(math.Round(colliderBaseAtmCost * pow(colliderCostFactor, float64(targetLevel-1))))
}

// colliderProductionTable holds the captured Dark-Matter-per-hour output by
// level (index 0 = level 1). Extend it as higher levels are captured; the
// quadratic below only fills the gap beyond the table.
var colliderProductionTable = []int64{4752, 10454, 17250}

// colliderProductionQuadratic is the 3-parameter fit through the captured
// points (547·L² + 4061·L + 144), used only for levels past the lookup table.
// PROVISIONAL — replace with captured values as they arrive.
const (
	colliderProdQuadA = 547.0
	colliderProdQuadB = 4061.0
	colliderProdQuadC = 144.0
)

// ColliderProduction returns the base Dark Matter per hour for a collider at
// the given level (0 at level 0). Captured levels come from the lookup table;
// higher levels fall back to the provisional quadratic.
func ColliderProduction(level int) int64 {
	if level <= 0 {
		return 0
	}
	if level <= len(colliderProductionTable) {
		return colliderProductionTable[level-1]
	}
	l := float64(level)
	return int64(math.Round(colliderProdQuadA*l*l + colliderProdQuadB*l + colliderProdQuadC))
}

// ColliderMultipliers are the optional production modifiers recalled from the
// live client. Both are hooks: the exact values are not captured yet.
type ColliderMultipliers struct {
	// CombatLevel applies the combat-level tooltip bonus (+3% per level,
	// colliderCombatPctPerLevel below). PROVISIONAL.
	CombatLevel int
	// PremiumPct is the premium "Generation collider" production bonus in
	// percent. Defaults to zero until the premium value is captured.
	PremiumPct float64
}

// colliderCombatPctPerLevel is the advertised "+3% to the development of the
// collider" from the combat-level tooltip. PROVISIONAL.
const colliderCombatPctPerLevel = 3.0

// Factor is the combined production multiplier (1.0 = no bonus).
func (m ColliderMultipliers) Factor() float64 {
	combat := 1.0 + colliderCombatPctPerLevel*float64(m.CombatLevel)/100.0
	premium := 1.0 + m.PremiumPct/100.0
	if combat < 0 {
		combat = 0
	}
	if premium < 0 {
		premium = 0
	}
	return combat * premium
}

// ColliderProductionFor returns the modified Dark-Matter-per-hour output for a
// collider at the given level under the supplied multipliers.
func ColliderProductionFor(level int, m ColliderMultipliers) int64 {
	if level <= 0 {
		return 0
	}
	return int64(math.Round(float64(ColliderProduction(level)) * m.Factor()))
}

// ColliderMoonOnly reports whether the collider may be levelled on a celestial
// of the given object type. It exists only on moons (page=kollaider).
func ColliderMoonOnly(objectType string) bool {
	return objectType == "MOON"
}

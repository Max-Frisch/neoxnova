package game

import "math"

// Dark-Matter field expansion — the Planetarium's "Increase the amount of
// fields on the current planet" option (Black market -> Planetarium). Captured
// 2026-10-06 (the private ops repo/): the cumulative price for +1..+6 fields
// is 200 / 420 / 662 / 928 / 1221 / 1543 DM. That is geometric with ratio 1.1
// starting from 200 DM, and (observed live) independent of the planet's base
// field count. Each further field costs 1.1x the previous:
//
//	cost(purchased, additional) = 2000 * 1.1^purchased * (1.1^additional - 1)
const (
	// FieldExpansionBase is the price of the first extra field.
	FieldExpansionBase = 200.0
	// FieldExpansionFactor is the per-field price multiplier.
	FieldExpansionFactor = 1.1
)

// FieldExpansionCost returns the Dark Matter cost to add `additional` fields to
// a planet that has already bought `purchased` fields with Dark Matter. A
// non-positive request costs nothing.
func FieldExpansionCost(purchased, additional int) int64 {
	if additional <= 0 {
		return 0
	}
	if purchased < 0 {
		purchased = 0
	}
	// base/(factor-1) == 2000, the closed-form scale of the geometric series.
	scale := FieldExpansionBase / (FieldExpansionFactor - 1.0)
	cost := scale *
		math.Pow(FieldExpansionFactor, float64(purchased)) *
		(math.Pow(FieldExpansionFactor, float64(additional)) - 1.0)
	return int64(math.Round(cost))
}

// Stardust diameter expansion — the Planetarium / Black-market "The increase in
// diameter" option (captured 2026-10-15, screenshot of planet 3:125:12). It
// consumes the metal + crystal debris field at the celestial's own coordinates
// plus one unit of Stardust to raise the celestial's diameter and field capacity
// by a rolled amount. Planets and moons (which share their planet's
// coordinates) use the same cost and ranges; the field gain is separate from
// the Dark-Matter field purchase above.
const (
	// DiameterExpansionMetalDebris is the metal debris spent per expansion.
	DiameterExpansionMetalDebris int64 = 100_000_000_000
	// DiameterExpansionCrystalDebris is the crystal debris spent per expansion.
	DiameterExpansionCrystalDebris int64 = 50_000_000_000
	// DiameterExpansionStardust is the Stardust spent per expansion.
	DiameterExpansionStardust int64 = 1
	// DiameterExpansionMinKm/MaxKm bound the diameter gain (inclusive).
	DiameterExpansionMinKm = 276
	DiameterExpansionMaxKm = 414
	// DiameterExpansionMinFields/MaxFields bound the field gain (inclusive).
	DiameterExpansionMinFields = 12
	DiameterExpansionMaxFields = 18
)

// DiameterExpansionGains maps two uniform [0,1) rolls onto the captured diameter
// (km) and field-count gains (each inclusive of both bounds). Rolls outside
// [0,1) are clamped so the result always stays within the captured ranges.
func DiameterExpansionGains(diameterRoll, fieldsRoll float64) (diameterKm, fields int) {
	return expansionRoll(DiameterExpansionMinKm, DiameterExpansionMaxKm, diameterRoll),
		expansionRoll(DiameterExpansionMinFields, DiameterExpansionMaxFields, fieldsRoll)
}

// expansionRoll maps a [0,1) roll onto lo..hi inclusive.
func expansionRoll(lo, hi int, roll float64) int {
	if roll < 0 {
		roll = 0
	}
	if roll >= 1 {
		roll = 0.999999999
	}
	return lo + int(roll*float64(hi-lo+1))
}

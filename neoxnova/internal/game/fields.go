package game

import "math"

// Dark-Matter field expansion — the Planetarium's "Increase the amount of
// fields on the current planet" option (Black market -> Planetarium). Captured
// 2026-10-06 (docs/screenshots_niburu/): the cumulative price for +1..+6 fields
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

package game

import "fmt"

// Simulator bounds. The unit cap keeps a hand-typed (or API) battle from
// feeding the round engine an unbounded workload while still covering every
// realistic fleet.
const (
	// SimMaxUnitsPerSide caps the total unit count on one side.
	SimMaxUnitsPerSide = 5_000_000
)

// NewCombatant validates one simulator side and returns the Combatant the pure
// engine consumes. Unknown unit codes, empty fleets and out-of-range sizes are
// rejected with a descriptive error so callers can surface a 400 rather than
// silently dropping input. Zero/negative counts are ignored (treated as absent).
func NewCombatant(units map[string]int64, techs CombatTechs, academy map[string]int, upgrades map[int]float64) (Combatant, error) {
	clean := make(map[string]int64, len(units))
	var total int64
	for code, n := range units {
		if n <= 0 {
			continue
		}
		if _, ok := LookupUnit(code); !ok {
			return Combatant{}, fmt.Errorf("unknown unit %q", code)
		}
		clean[code] = n
		total += n
		if total > SimMaxUnitsPerSide {
			return Combatant{}, fmt.Errorf("fleet too large (max %d units)", SimMaxUnitsPerSide)
		}
	}
	if len(clean) == 0 {
		return Combatant{}, fmt.Errorf("fleet is empty")
	}
	return Combatant{Units: clean, Techs: techs, Academy: academy, Upgrades: upgrades}, nil
}

// SimulateBattle validates both sides and resolves a deterministic battle. When
// seed is nil the seed is derived from the fleets (game.CombatSeed) so identical
// inputs always reproduce the reference resolution; the effective seed is
// returned alongside the result.
func SimulateBattle(attacker, defender Combatant, seed *int64) (CombatResult, int64, error) {
	if len(attacker.Units) == 0 || len(defender.Units) == 0 {
		return CombatResult{}, 0, fmt.Errorf("both fleets need at least one unit")
	}
	effective := CombatSeed(attacker, defender)
	if seed != nil {
		effective = *seed
	}
	return Resolve(attacker, defender, effective), effective, nil
}

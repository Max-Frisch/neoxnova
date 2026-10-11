package game

import "testing"

func TestNewCombatantValidation(t *testing.T) {
	t.Run("known units pass", func(t *testing.T) {
		c, err := NewCombatant(map[string]int64{"204": 5, "202": 2}, CombatTechs{}, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.Units["204"] != 5 || c.Units["202"] != 2 {
			t.Fatalf("units not carried through: %+v", c.Units)
		}
	})

	t.Run("zero and negative counts are dropped", func(t *testing.T) {
		if _, err := NewCombatant(map[string]int64{"204": 0, "202": -3}, CombatTechs{}, nil, nil); err == nil {
			t.Fatal("expected empty-fleet error when all counts are <= 0")
		}
		c, err := NewCombatant(map[string]int64{"204": 4, "202": 0}, CombatTechs{}, nil, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := c.Units["202"]; ok {
			t.Fatal("zero-count unit should not be present")
		}
	})

	t.Run("empty fleet rejected", func(t *testing.T) {
		if _, err := NewCombatant(nil, CombatTechs{}, nil, nil); err == nil {
			t.Fatal("expected error for nil units")
		}
		if _, err := NewCombatant(map[string]int64{}, CombatTechs{}, nil, nil); err == nil {
			t.Fatal("expected error for empty units")
		}
	})

	t.Run("unknown unit rejected", func(t *testing.T) {
		if _, err := NewCombatant(map[string]int64{"999": 1}, CombatTechs{}, nil, nil); err == nil {
			t.Fatal("expected error for unknown unit code")
		}
	})

	t.Run("oversized fleet rejected", func(t *testing.T) {
		if _, err := NewCombatant(map[string]int64{"202": SimMaxUnitsPerSide + 1}, CombatTechs{}, nil, nil); err == nil {
			t.Fatal("expected error for oversized fleet")
		}
		// Exactly at the cap is fine.
		if _, err := NewCombatant(map[string]int64{"202": SimMaxUnitsPerSide}, CombatTechs{}, nil, nil); err != nil {
			t.Fatalf("fleet at the cap should pass: %v", err)
		}
	})
}

func TestSimulateBattle(t *testing.T) {
	atk := Combatant{Units: map[string]int64{"207": 100}}
	def := Combatant{Units: map[string]int64{"204": 50}}

	r1, seed1, err := SimulateBattle(atk, def, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r2, seed2, err := SimulateBattle(atk, def, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seed1 != seed2 {
		t.Fatalf("derived seed not deterministic: %d vs %d", seed1, seed2)
	}
	if r1.Winner != r2.Winner || r1.Rounds != r2.Rounds {
		t.Fatalf("derived-seed resolution not reproducible: %+v vs %+v", r1, r2)
	}

	// An explicit seed is honoured verbatim and overrides the derived one.
	explicit := int64(42)
	_, seed, err := SimulateBattle(atk, def, &explicit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seed != explicit {
		t.Fatalf("explicit seed not used: got %d want %d", seed, explicit)
	}

	// Empty sides are rejected before the engine runs.
	if _, _, err := SimulateBattle(Combatant{Units: nil}, def, nil); err == nil {
		t.Fatal("expected error for empty attacker")
	}
	if _, _, err := SimulateBattle(atk, Combatant{Units: map[string]int64{}}, nil); err == nil {
		t.Fatal("expected error for empty defender")
	}
}

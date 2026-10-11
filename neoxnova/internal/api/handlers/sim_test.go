package handlers

import (
	"testing"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
)

func TestSimCombatantConversion(t *testing.T) {
	t.Run("maps techs and units", func(t *testing.T) {
		c, err := simCombatant(models.SimSide{
			Ships:   map[string]int64{"204": 5},
			Techs:   models.SimTechs{Weapons: 3, Shield: 2, Armour: 1, Laser: 4, Ion: 5, Plasma: 6, Graviton: 7},
			Academy: map[string]int{"1103": 1},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.Units["204"] != 5 {
			t.Fatalf("units lost: %+v", c.Units)
		}
		want := game.CombatTechs{Weapons: 3, Shield: 2, Armour: 1, Laser: 4, Ion: 5, Plasma: 6, Graviton: 7}
		if c.Techs != want {
			t.Fatalf("techs = %+v, want %+v", c.Techs, want)
		}
		if c.Academy["1103"] != 1 {
			t.Fatalf("academy lost: %+v", c.Academy)
		}
	})

	t.Run("unknown unit rejected", func(t *testing.T) {
		if _, err := simCombatant(models.SimSide{Ships: map[string]int64{"999": 1}}); err == nil {
			t.Fatal("expected error for unknown unit")
		}
	})

	t.Run("empty fleet rejected", func(t *testing.T) {
		if _, err := simCombatant(models.SimSide{}); err == nil {
			t.Fatal("expected error for empty fleet")
		}
	})
}

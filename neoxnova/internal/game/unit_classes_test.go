package game

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// unitClassFixture mirrors testdata/niburus_unit_classes.json (the committed
// source of truth for unit_classes.go).
type unitClassFixture struct {
	Units map[string]struct {
		Name    string `json:"name"`
		Weapon  string `json:"weapon"`
		Weapons []struct {
			Class  string `json:"class"`
			Attack int    `json:"attack"`
		} `json:"weapons"`
		Armor  string `json:"armor"`
		Shield string `json:"shield"`
		Engine string `json:"engine"`
	} `json:"units"`
}

func TestUnitClassesMatchFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "niburus_unit_classes.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fx unitClassFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(fx.Units) == 0 {
		t.Fatal("fixture has no units")
	}
	if len(fx.Units) != len(unitClasses) {
		t.Fatalf("unitClasses has %d units, fixture has %d", len(unitClasses), len(fx.Units))
	}
	for code, want := range fx.Units {
		got, ok := UnitClassOf(code)
		if !ok {
			t.Errorf("unit %s missing from unitClasses", code)
			continue
		}
		exp := UnitClass{Weapon: want.Weapon, Armor: want.Armor, Shield: want.Shield, Engine: want.Engine}
		for _, w := range want.Weapons {
			exp.Weapons = append(exp.Weapons, WeaponClass{Class: w.Class, Attack: w.Attack})
		}
		if !reflect.DeepEqual(got, exp) {
			t.Errorf("unit %s (%s) = %+v, want %+v", code, want.Name, got, exp)
		}
	}
}

func TestUpgradeCodeForClass(t *testing.T) {
	cases := []struct {
		group, class string
		want         int
	}{
		{"weapon", "laser", 1},
		{"weapon", "ion", 2},
		{"weapon", "plasma", 3},
		{"weapon", "gravitational", 4},
		{"weapon", "standard", 0},
		{"weapon", "", 0},
		{"armor", "light", 5},
		{"armor", "medium", 6},
		{"armor", "heavy", 7},
		{"shield", "light", 8},
		{"shield", "medium", 9},
		{"shield", "heavy", 10},
		{"engine", "combustion", 11},
		{"engine", "impulse", 12},
		{"engine", "hyperspace", 13},
	}
	for _, c := range cases {
		if got := UpgradeCodeForClass(c.group, c.class); got != c.want {
			t.Errorf("UpgradeCodeForClass(%q,%q) = %d, want %d", c.group, c.class, got, c.want)
		}
	}
}

// Every class a unit declares (except "standard" weapons) must resolve to a
// catalog upgrade, so a card class can never silently go unmodelled.
func TestUnitClassesResolve(t *testing.T) {
	for code, c := range unitClasses {
		probes := []struct{ group, class string }{{"armor", c.Armor}, {"shield", c.Shield}, {"engine", c.Engine}}
		for _, w := range c.Weapons {
			probes = append(probes, struct{ group, class string }{"weapon", w.Class})
		}
		if len(c.Weapons) == 0 && c.Weapon != "" {
			probes = append(probes, struct{ group, class string }{"weapon", c.Weapon})
		}
		for _, probe := range probes {
			if probe.class == "" || probe.class == "standard" {
				continue
			}
			if UpgradeCodeForClass(probe.group, probe.class) == 0 {
				t.Errorf("unit %s class %s/%s has no upgrade", code, probe.group, probe.class)
			}
		}
	}
}

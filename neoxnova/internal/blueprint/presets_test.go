package blueprint

import "testing"

// TestPresetsValid locks every built-in preset to real catalog codes and a
// non-empty target set, so a typo can never ship as a broken template.
func TestPresetsValid(t *testing.T) {
	presets := Presets()
	if len(presets) == 0 {
		t.Fatal("Presets() returned none")
	}
	seen := map[string]bool{}
	for _, p := range presets {
		if p.Name == "" {
			t.Errorf("preset with empty name: %+v", p)
		}
		if seen[p.Name] {
			t.Errorf("duplicate preset name %q", p.Name)
		}
		seen[p.Name] = true
		if !ValidateSpec(p.Spec) {
			t.Errorf("preset %q references unknown catalog codes", p.Name)
		}
		if !p.Spec.Enabled() {
			t.Errorf("preset %q targets nothing", p.Name)
		}
	}
}

// TestValidateSpec makes sure unknown structures/techs/units/caps are rejected.
func TestValidateSpec(t *testing.T) {
	cases := []struct {
		name string
		spec Spec
		ok   bool
	}{
		{"empty", Spec{}, true},
		{"known", Spec{Buildings: map[string]int{"metal_mine": 1}}, true},
		{"unknown building", Spec{Buildings: map[string]int{"nope": 1}}, false},
		{"unknown research", Spec{Research: map[string]int{"nope": 1}}, false},
		{"unknown ship", Spec{Ships: map[string]int{"999": 1}}, false},
		{"unknown gradual", Spec{Gradual: []string{"nope"}}, false},
		{"unknown cap code", Spec{Caps: map[string]int{"nope": 1}}, false},
		{"zero cap", Spec{Caps: map[string]int{"metal_mine": 0}}, false},
		{"known gradual", Spec{Gradual: []string{"metal_mine"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidateSpec(tc.spec); got != tc.ok {
				t.Fatalf("ValidateSpec = %v, want %v", got, tc.ok)
			}
		})
	}
}

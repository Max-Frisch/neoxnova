package blueprint

import "neoxnova/internal/game"

// Preset is a named, ready-to-apply blueprint spec surfaced by the templates
// endpoint. Presets are authored with Go catalog codes (not the explorer's
// numeric ids) so the server can validate them directly.
type Preset struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Spec        Spec   `json:"spec"`
}

// Presets returns the built-in blueprint templates. The "gradual economy" set
// is the promoted explorer `plans/colo-grow.json` (AUTO_BUILD_DESIGN Appendix B,
// numeric ids mapped to catalog codes).
func Presets() []Preset {
	return []Preset{
		{
			Name:        "gradual-economy",
			Description: "Bootstrap mines/builders, then grow economy +1 per cycle up to caps.",
			Spec: Spec{
				Mode: ModeGradual,
				Buildings: map[string]int{
					"robotics_factory":      10,
					"metal_storage":         7,
					"crystal_storage":       7,
					"deuterium_storage":     7,
					"shipyard":              10,
					"research_lab":          10,
					"metal_mine":            22,
					"crystal_mine":          20,
					"deuterium_synthesizer": 17,
					"solar_plant":           21,
				},
				Ships: map[string]int{"212": 200},
				Gradual: []string{
					"metal_mine", "crystal_mine", "deuterium_synthesizer",
					"shipyard", "metal_storage", "crystal_storage", "deuterium_storage",
				},
				BumpBuilders: []string{"robotics_factory", "nanite_factory"},
				EnergySats:   200,
				Caps: map[string]int{
					"metal_mine":            40,
					"crystal_mine":          38,
					"deuterium_synthesizer": 35,
					"shipyard":              14,
					"metal_storage":         12,
					"crystal_storage":       12,
					"deuterium_storage":     12,
					"robotics_factory":      18,
					"nanite_factory":        7,
				},
				BuilderBumpSec: 360,
				Order: []string{
					"robotics_factory", "metal_storage", "crystal_storage",
					"deuterium_storage", "shipyard", "research_lab",
					"metal_mine", "crystal_mine", "deuterium_synthesizer", "solar_plant",
				},
			},
		},
		{
			Name:        "simple-bootstrap",
			Description: "Minimum viable colony: builders, economy and a first shipyard, then stop.",
			Spec: Spec{
				Mode: ModeSimple,
				Buildings: map[string]int{
					"robotics_factory":      4,
					"shipyard":              4,
					"research_lab":          3,
					"metal_mine":            12,
					"crystal_mine":          10,
					"deuterium_synthesizer": 8,
					"solar_plant":           12,
				},
				Ships:      map[string]int{"212": 50},
				EnergySats: 50,
				Order: []string{
					"robotics_factory", "shipyard", "research_lab",
					"metal_mine", "crystal_mine", "deuterium_synthesizer", "solar_plant",
				},
			},
		},
	}
}

// ValidateSpec reports whether every code referenced by the spec exists in the
// catalog. An empty spec is structurally valid (it simply targets nothing).
func ValidateSpec(s Spec) bool {
	okStructure := func(c string) bool { _, ok := game.Structures[c]; return ok }
	okTech := func(c string) bool { _, ok := game.Techs[c]; return ok }
	okUnit := func(c string) bool { _, ok := game.LookupUnit(c); return ok }

	for c := range s.Buildings {
		if !okStructure(c) {
			return false
		}
	}
	for c := range s.Research {
		if !okTech(c) {
			return false
		}
	}
	for c := range s.Ships {
		if !okUnit(c) {
			return false
		}
	}
	for c := range s.Defenses {
		if !okUnit(c) {
			return false
		}
	}
	for _, c := range s.Gradual {
		if !okStructure(c) {
			return false
		}
	}
	for _, c := range s.BumpBuilders {
		if !okStructure(c) {
			return false
		}
	}
	for _, c := range s.Order {
		if !okStructure(c) && !okTech(c) && !okUnit(c) {
			return false
		}
	}
	for c, lvl := range s.Caps {
		if !okStructure(c) || lvl <= 0 {
			return false
		}
	}
	return true
}

package game

// TechTree is the complete prerequisite graph for buildings, technologies,
// ships and defenses, ported from the live niburuspace.com capture
// (tools/explorer/data/techtree-graph.json, numeric ids mapped to catalog
// codes). The per-definition Requires maps in catalog.go are incomplete; init()
// fills them from this graph so the build store and the auto-build planner agree
// on prerequisites.
//
// Codes that exist only in the graph (premium/custom units) are still listed;
// they simply have no catalog definition and are never buildable.
var TechTree = map[string]map[string]int{
	"university":                     {"robotics_factory": 20, "research_lab": 22, "nanite_factory": 4, "computer_tech": 12, "intergalactic_research_network": 3},
	"deuterium_power_plant":          {"deuterium_synthesizer": 5, "energy_tech": 3},
	"nanite_factory":                 {"robotics_factory": 10, "computer_tech": 10},
	"shipyard":                       {"robotics_factory": 2},
	"terraformer":                    {"nanite_factory": 1, "energy_tech": 12},
	"phalanx_sensor":                 {"moon_base": 1},
	"jumpgate":                       {"moon_base": 1, "hyperspace_tech": 7},
	"missile_silo":                   {"shipyard": 1},
	"light_conveyor":                 {"nanite_factory": 5, "shipyard": 10},
	"average_conveyor":               {"nanite_factory": 8, "shipyard": 14},
	"heavy_conveyor":                 {"nanite_factory": 10, "shipyard": 18},
	"espionage_tech":                 {"research_lab": 3},
	"computer_tech":                  {"research_lab": 1},
	"weapons_tech":                   {"research_lab": 4},
	"shielding_tech":                 {"energy_tech": 3, "research_lab": 6},
	"armour_tech":                    {"research_lab": 2},
	"energy_tech":                    {"research_lab": 1},
	"hyperspace_tech":                {"energy_tech": 5, "shielding_tech": 5, "research_lab": 7},
	"combustion_drive":               {"energy_tech": 1, "research_lab": 1},
	"impulse_drive":                  {"energy_tech": 1, "research_lab": 2},
	"hyperspace_drive":               {"hyperspace_tech": 3, "research_lab": 7},
	"laser_tech":                     {"research_lab": 1, "energy_tech": 2},
	"ion_tech":                       {"research_lab": 4, "laser_tech": 5, "energy_tech": 4},
	"plasma_tech":                    {"research_lab": 5, "energy_tech": 8, "laser_tech": 10, "ion_tech": 5},
	"intergalactic_research_network": {"research_lab": 10, "computer_tech": 8, "hyperspace_tech": 8},
	"astrophysics":                   {"espionage_tech": 3, "impulse_drive": 3, "research_lab": 3},
	"brother_hood":                   {"alliance_depot": 1, "research_lab": 3},
	"mineral_research":               {"research_lab": 8, "energy_tech": 5},
	"semi_crystals_research":         {"research_lab": 8, "energy_tech": 5},
	"fuel_research":                  {"research_lab": 8, "energy_tech": 5},
	"graviton_research":              {"research_lab": 12},
	"202":                            {"shipyard": 2, "combustion_drive": 2},
	"203":                            {"shipyard": 4, "combustion_drive": 6},
	"204":                            {"shipyard": 1, "combustion_drive": 1},
	"205":                            {"shipyard": 3, "armour_tech": 2, "impulse_drive": 2},
	"206":                            {"shipyard": 5, "impulse_drive": 4, "ion_tech": 2},
	"207":                            {"shipyard": 7, "hyperspace_drive": 4},
	"208":                            {"shipyard": 4, "impulse_drive": 3},
	"209":                            {"shipyard": 4, "combustion_drive": 6, "shielding_tech": 2},
	"210":                            {"shipyard": 3, "combustion_drive": 3, "espionage_tech": 2},
	"211":                            {"impulse_drive": 6, "shipyard": 8, "plasma_tech": 5},
	"212":                            {"shipyard": 1},
	"213":                            {"shipyard": 9, "hyperspace_drive": 6, "hyperspace_tech": 5},
	"214":                            {"shipyard": 12, "hyperspace_drive": 7, "hyperspace_tech": 6, "graviton_research": 1},
	"215":                            {"hyperspace_tech": 5, "laser_tech": 12, "hyperspace_drive": 5, "shipyard": 8},
	"216":                            {"shipyard": 15, "weapons_tech": 14, "shielding_tech": 14, "armour_tech": 15, "hyperspace_tech": 10, "laser_tech": 20, "graviton_research": 3},
	"217":                            {"armour_tech": 10, "shipyard": 14, "hyperspace_tech": 10, "shielding_tech": 14, "impulse_drive": 15},
	"218":                            {"shipyard": 18, "weapons_tech": 20, "shielding_tech": 20, "armour_tech": 20, "hyperspace_tech": 15, "hyperspace_drive": 20, "laser_tech": 25, "graviton_research": 8},
	"219":                            {"shipyard": 15, "weapons_tech": 15, "shielding_tech": 15, "armour_tech": 15, "hyperspace_drive": 8},
	"220":                            {"shipyard": 9, "hyperspace_tech": 5, "hyperspace_drive": 6},
	"221":                            {"shipyard": 20, "weapons_tech": 22, "shielding_tech": 22, "armour_tech": 22, "hyperspace_tech": 17, "hyperspace_drive": 22, "graviton_research": 9, "plasma_tech": 20},
	"222":                            {"shipyard": 17, "weapons_tech": 18, "shielding_tech": 18, "armour_tech": 18, "hyperspace_drive": 20, "laser_tech": 23, "graviton_research": 8, "plasma_tech": 18},
	"223":                            {"shipyard": 1},
	"224":                            {"shipyard": 12, "hyperspace_drive": 7, "hyperspace_tech": 6, "laser_tech": 12, "ion_tech": 8},
	"225":                            {"laser_tech": 14, "shipyard": 11, "ion_tech": 14, "weapons_tech": 12, "shielding_tech": 12, "armour_tech": 12, "hyperspace_drive": 6},
	"226":                            {"shipyard": 14, "laser_tech": 12, "ion_tech": 17, "weapons_tech": 14, "shielding_tech": 13, "armour_tech": 13, "hyperspace_drive": 10},
	"227":                            {"shipyard": 16, "laser_tech": 17, "ion_tech": 19, "graviton_research": 5, "weapons_tech": 16, "shielding_tech": 16, "armour_tech": 17, "hyperspace_drive": 14},
	"228":                            {"shipyard": 18, "ion_tech": 20, "plasma_tech": 20, "graviton_research": 7, "weapons_tech": 17, "shielding_tech": 18, "armour_tech": 18, "hyperspace_drive": 16},
	"229":                            {"armour_tech": 6, "weapons_tech": 7, "shipyard": 7, "plasma_tech": 7, "ion_tech": 7, "laser_tech": 6},
	"230":                            {"ion_tech": 22, "shipyard": 18, "shielding_tech": 20, "weapons_tech": 21, "plasma_tech": 19, "laser_tech": 19, "armour_tech": 20},
	"401":                            {"shipyard": 1},
	"402":                            {"energy_tech": 1, "shipyard": 2, "laser_tech": 3},
	"403":                            {"energy_tech": 3, "shipyard": 4, "laser_tech": 6},
	"404":                            {"shipyard": 6, "energy_tech": 6, "weapons_tech": 3, "shielding_tech": 1},
	"405":                            {"shipyard": 4, "ion_tech": 4},
	"406":                            {"shipyard": 8, "plasma_tech": 7},
	"407":                            {"shielding_tech": 2, "shipyard": 1},
	"408":                            {"shielding_tech": 6, "shipyard": 6},
	"409":                            {"shipyard": 10},
	"410":                            {"graviton_research": 7, "shipyard": 18, "weapons_tech": 20},
	"411":                            {"graviton_research": 10, "shielding_tech": 22, "plasma_tech": 20, "computer_tech": 15, "armour_tech": 25, "energy_tech": 20, "shipyard": 20},
	"412":                            {"plasma_tech": 17, "laser_tech": 24, "energy_tech": 20, "weapons_tech": 20, "shipyard": 18},
	"413":                            {"graviton_research": 8, "shielding_tech": 21, "plasma_tech": 20, "armour_tech": 22, "energy_tech": 18, "shipyard": 18},
	"414":                            {"shipyard": 20, "weapons_tech": 23, "shielding_tech": 21, "ion_tech": 24, "graviton_research": 6},
	"415":                            {"shipyard": 22, "nanite_factory": 17, "weapons_tech": 23, "shielding_tech": 23, "armour_tech": 23, "ion_tech": 18, "plasma_tech": 23, "graviton_research": 9},
	"416":                            {"shipyard": 7, "laser_tech": 5, "ion_tech": 7},
	"417":                            {"shipyard": 9, "ion_tech": 8, "plasma_tech": 11},
	"418":                            {"shipyard": 17, "weapons_tech": 18, "shielding_tech": 17, "armour_tech": 18, "ion_tech": 14, "plasma_tech": 14, "graviton_research": 4},
	"419":                            {"shipyard": 20, "weapons_tech": 20, "shielding_tech": 19, "armour_tech": 19, "ion_tech": 17, "plasma_tech": 19, "graviton_research": 7},
	"420":                            {"shipyard": 20, "shielding_tech": 14, "laser_tech": 17, "ion_tech": 17, "plasma_tech": 15},
	"421":                            {"armour_tech": 2, "weapons_tech": 2, "shipyard": 3, "ion_tech": 2, "laser_tech": 2, "plasma_tech": 2},
	"422":                            {"shipyard": 9, "weapons_tech": 7, "shielding_tech": 6, "armour_tech": 6, "ion_tech": 6, "laser_tech": 6, "plasma_tech": 5},
	"502":                            {"missile_silo": 2, "shipyard": 1},
	"503":                            {"missile_silo": 4, "shipyard": 1, "impulse_drive": 1},
}

// Requirements returns the prerequisite levels for a catalog code, preferring
// the complete live graph and falling back to the definition's own Requires.
func Requirements(code string) map[string]int {
	if reqs, ok := TechTree[code]; ok {
		return reqs
	}
	if def, ok := Structures[code]; ok {
		return def.Requires
	}
	if def, ok := Techs[code]; ok {
		return def.Requires
	}
	if def, ok := LookupUnit(code); ok {
		return def.Requires
	}
	return nil
}

// init back-fills the per-definition Requires maps from TechTree so the build
// store and planner share one source of truth. The moon-only catalog keeps its
// own hand-verified requirements.
func init() {
	fill := func(m map[string]StructureDef) {
		for code, def := range m {
			if reqs, ok := TechTree[code]; ok {
				def.Requires = reqs
				m[code] = def
			}
		}
	}
	fill(Structures)
	for code, def := range Techs {
		if reqs, ok := TechTree[code]; ok {
			def.Requires = reqs
			Techs[code] = def
		}
	}
	for code, def := range Ships {
		if reqs, ok := TechTree[code]; ok {
			def.Requires = reqs
			Ships[code] = def
		}
	}
	for code, def := range Defenses {
		if reqs, ok := TechTree[code]; ok {
			def.Requires = reqs
			Defenses[code] = def
		}
	}
}

package blueprint

import (
	"testing"

	"neoxnova/internal/game"
)

func richState() State {
	return State{
		Structures: map[string]int{},
		Techs:      map[string]int{},
		Ships:      map[string]int64{},
		Defenses:   map[string]int64{},
		Resources:  game.Cost{Metal: 1e15, Crystal: 1e15, Deuterium: 1e15},
		FieldsUsed: 0,
		FieldsMax:  1000,
		GameSpeed:  1,
	}
}

// TestAdvanceBuildsPrerequisites: an unmet research goal must first raise the
// Research Lab (a structural prerequisite) before the tech can be researched.
func TestAdvanceBuildsPrerequisites(t *testing.T) {
	spec := Spec{Research: map[string]int{"astrophysics": 1}}
	actions := Advance(spec, richState())
	if len(actions) == 0 {
		t.Fatal("expected a construction action")
	}
	if actions[0].Kind != ActionStructure || actions[0].Code != "research_lab" {
		t.Fatalf("first action = %+v, want research_lab", actions[0])
	}
}

// TestAdvanceRespectsOrder: the explicit order decides among equally-ready codes.
func TestAdvanceRespectsOrder(t *testing.T) {
	spec := Spec{
		Buildings: map[string]int{"metal_mine": 1, "crystal_mine": 1},
		Order:     []string{"crystal_mine", "metal_mine"},
	}
	a, ok := nextStructure(spec, richState())
	if !ok || a.Code != "crystal_mine" {
		t.Fatalf("nextStructure = %+v ok=%v, want crystal_mine", a, ok)
	}
}

// TestGradualGrowth: gradual mode raises a code by one level per cycle up to
// its cap, then stops.
func TestGradualGrowth(t *testing.T) {
	spec := Spec{
		Mode:    ModeGradual,
		Gradual: []string{"metal_mine"},
		Caps:    map[string]int{"metal_mine": 3},
	}
	st := richState()
	st.Structures["metal_mine"] = 1
	a, ok := nextStructure(spec, st)
	if !ok || a.Code != "metal_mine" || a.TargetLevel != 2 {
		t.Fatalf("gradual action = %+v ok=%v, want metal_mine -> 2", a, ok)
	}
	st.Structures["metal_mine"] = 3 // at cap
	if _, ok := nextStructure(spec, st); ok {
		t.Fatal("gradual should not build past the cap")
	}
}

// TestBumpBuilders: when a gradual mine's next build exceeds the threshold,
// the configured Robot/Nanite factory is raised instead.
func TestBumpBuilders(t *testing.T) {
	spec := Spec{
		Mode:           ModeGradual,
		Gradual:        []string{"metal_mine"},
		Caps:           map[string]int{"metal_mine": 30, "robotics_factory": 5},
		BumpBuilders:   []string{"robotics_factory"},
		BuilderBumpSec: 1,
	}
	st := richState()
	st.Structures["metal_mine"] = 30 // at cap, so only the bump remains
	a, ok := nextStructure(spec, st)
	if !ok || a.Code != "robotics_factory" {
		t.Fatalf("bump action = %+v ok=%v, want robotics_factory", a, ok)
	}
}

// TestEnergyDeficitBuildsSolarPlant: a power deficit adds a Solar Plant goal.
func TestEnergyDeficitBuildsSolarPlant(t *testing.T) {
	spec := Spec{}
	st := richState()
	st.EnergyUsed = 100
	st.EnergyMax = 10
	a, ok := nextStructure(spec, st)
	if !ok || a.Code != "solar_plant" {
		t.Fatalf("energy action = %+v ok=%v, want solar_plant", a, ok)
	}
}

// TestEnergyDeficitSolarSatellites: with a shipyard and EnergySats configured,
// satellites are used to close the deficit.
func TestEnergyDeficitSolarSatellites(t *testing.T) {
	spec := Spec{EnergySats: 50}
	st := richState()
	st.Structures["shipyard"] = 1
	st.EnergyUsed = 100
	st.EnergyMax = 10
	a, ok := nextUnit(spec, st)
	if !ok || a.Code != "212" || a.Quantity != 50 {
		t.Fatalf("satellite action = %+v ok=%v, want 212 x50", a, ok)
	}
}

// TestUnitBuild: a fixed ship target is built as the deficit.
func TestUnitBuild(t *testing.T) {
	spec := Spec{Ships: map[string]int{"212": 100}}
	st := richState()
	st.Structures["shipyard"] = 1
	st.Ships["212"] = 30
	a, ok := nextUnit(spec, st)
	if !ok || a.Code != "212" || a.Quantity != 70 {
		t.Fatalf("unit action = %+v ok=%v, want 212 x70", a, ok)
	}
}

// TestDoneAndIdle: done once targets are met, idle while resources are short.
func TestDoneAndIdle(t *testing.T) {
	spec := Spec{Buildings: map[string]int{"metal_mine": 5}}
	st := richState()
	st.Structures["metal_mine"] = 5
	if a := NextAction(spec, st); a.Kind != ActionDone {
		t.Fatalf("NextAction = %+v, want done", a)
	}
	st.Structures["metal_mine"] = 0
	st.Resources = game.Cost{}
	if a := NextAction(spec, st); a.Kind != ActionIdle {
		t.Fatalf("NextAction = %+v, want idle", a)
	}
}

// TestPrereqClosureSkipsSatisfiedGoals guards the classic bug: an already-met
// goal must not force its prerequisites.
func TestPrereqClosureSkipsSatisfiedGoals(t *testing.T) {
	spec := Spec{Buildings: map[string]int{"shipyard": 1}}
	st := richState()
	st.Structures["shipyard"] = 1
	st.Structures["robotics_factory"] = 2
	if a := NextAction(spec, st); a.Kind != ActionDone {
		t.Fatalf("NextAction = %+v, want done (shipyard already built)", a)
	}
}

// TestTechTreeComplete locks the live prerequisite graph against regressions
// (the nanite factory needs Computer Tech, not the old Shipyard entry).
func TestTechTreeComplete(t *testing.T) {
	if got := game.Requirements("nanite_factory")["computer_tech"]; got != 10 {
		t.Fatalf("nanite computer_tech prereq = %d, want 10", got)
	}
	if game.Requirements("metal_mine") != nil {
		t.Fatal("metal_mine should have no prerequisites")
	}
}

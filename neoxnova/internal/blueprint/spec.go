// Package blueprint implements the pure auto-build planner (backlog item 3,
// docs/AUTO_BUILD_DESIGN.md). It has no database or engine dependencies: given a
// parsed blueprint spec and a state snapshot it returns the next build actions,
// one per independent queue scope (construction / research / shipyard).
package blueprint

import "neoxnova/internal/game"

// Mode selects how a blueprint behaves once its fixed targets are met.
type Mode string

const (
	// ModeSimple builds the fixed targets, then stops.
	ModeSimple Mode = "simple"
	// ModeGradual bootstraps the fixed targets, then raises every code in
	// Gradual by one level per cycle up to its Cap.
	ModeGradual Mode = "gradual"
	// ModeHybrid is simple first, then gradual.
	ModeHybrid Mode = "hybrid"
)

// Spec is the persisted blueprint language (AUTO_BUILD_DESIGN §5). All fields
// are optional; string catalog codes are used throughout.
type Spec struct {
	Mode           Mode           `json:"mode,omitempty"`
	Buildings      map[string]int `json:"buildings,omitempty"`
	Research       map[string]int `json:"research,omitempty"`
	Ships          map[string]int `json:"ships,omitempty"`
	Defenses       map[string]int `json:"defenses,omitempty"`
	Order          []string       `json:"order,omitempty"`
	Gradual        []string       `json:"gradual,omitempty"`
	Caps           map[string]int `json:"caps,omitempty"`
	BumpBuilders   []string       `json:"bumpBuilders,omitempty"`
	AllowSlow      []string       `json:"allowSlow,omitempty"`
	EnergySats     int            `json:"energySats,omitempty"`
	BuilderBumpSec int            `json:"builderBumpSec,omitempty"`
	MinReserve     *game.Cost     `json:"minReserve,omitempty"`
}

// State is the snapshot the planner reasons over. Levels/counts default to 0.
type State struct {
	Structures map[string]int
	Techs      map[string]int
	Ships      map[string]int64
	Defenses   map[string]int64

	Resources game.Cost

	EnergyMax  float64
	EnergyUsed float64

	FieldsUsed int
	FieldsMax  int

	// QueueBusy flags: at most one item per scope may be in flight.
	BuildBusy    bool
	ShipBusy     bool
	ResearchBusy bool

	GameSpeed float64
}

// ActionKind is the scope of a recommended action.
type ActionKind string

const (
	ActionStructure ActionKind = "structure"
	ActionResearch  ActionKind = "research"
	ActionShip      ActionKind = "ship"
	ActionDefense   ActionKind = "defense"
	ActionIdle      ActionKind = "idle"
	ActionDone      ActionKind = "done"
)

// Action is one recommended build. TargetLevel applies to structures/research;
// Quantity applies to ships/defenses.
type Action struct {
	Kind        ActionKind `json:"kind"`
	Code        string     `json:"code"`
	TargetLevel int        `json:"target_level,omitempty"`
	Quantity    int64      `json:"quantity,omitempty"`
	Reason      string     `json:"reason,omitempty"`
}

// Enabled reports whether the spec has any target at all.
func (s Spec) Enabled() bool {
	return len(s.Buildings) > 0 || len(s.Research) > 0 || len(s.Ships) > 0 ||
		len(s.Defenses) > 0 || len(s.Gradual) > 0
}

func (s Spec) gradualMode() bool {
	return s.Mode == ModeGradual || s.Mode == ModeHybrid
}

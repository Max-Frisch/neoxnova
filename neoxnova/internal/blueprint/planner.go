package blueprint

import (
	"sort"

	"neoxnova/internal/game"
)

// Advance returns the next recommended action for each independent queue scope
// that is currently free (at most one construction, one research and one
// shipyard action). An empty slice means nothing can be built right now.
func Advance(spec Spec, st State) []Action {
	var out []Action
	if !st.BuildBusy {
		if a, ok := nextStructure(spec, st); ok {
			out = append(out, a)
		}
	}
	if !st.ResearchBusy {
		if a, ok := nextResearch(spec, st); ok {
			out = append(out, a)
		}
	}
	if !st.ShipBusy {
		if a, ok := nextUnit(spec, st); ok {
			out = append(out, a)
		}
	}
	return out
}

// NextAction collapses Advance to a single action (or idle/done) for a UI
// "next action" badge. It reports done only when no goal is left outstanding.
func NextAction(spec Spec, st State) Action {
	if actions := Advance(spec, st); len(actions) > 0 {
		return actions[0]
	}
	if PendingGoals(spec, st) {
		return Action{Kind: ActionIdle, Reason: "waiting for resources, fields or a free queue"}
	}
	return Action{Kind: ActionDone, Reason: "all targets reached"}
}

// PendingGoals reports whether any target (or its prerequisite closure) is
// still below the wanted level, ignoring affordability and queue state.
func PendingGoals(spec Spec, st State) bool {
	all := goalWants(spec, st)
	for code, lvl := range all {
		if _, ok := game.Structures[code]; ok && st.Structures[code] < lvl {
			return true
		}
		if _, ok := game.Techs[code]; ok && st.Techs[code] < lvl {
			return true
		}
	}
	if st.EnergyUsed > st.EnergyMax {
		return true
	}
	return false
}

// nextStructure picks the highest-priority unmet, buildable structure.
func nextStructure(spec Spec, st State) (Action, bool) {
	want := structureWants(spec, st)
	ready := make([]string, 0, len(want))
	for code, lvl := range want {
		if st.Structures[code] >= lvl {
			continue
		}
		if !game.RequiresMet(game.Requirements(code), combinedLevels(st)) {
			continue
		}
		if st.FieldsUsed+1 > st.FieldsMax {
			continue
		}
		if !canAfford(spec, st, structureCost(code, st.Structures[code]+1)) {
			continue
		}
		ready = append(ready, code)
	}
	if len(ready) == 0 {
		return Action{}, false
	}
	sort.Slice(ready, func(i, j int) bool { return actionLess(spec, ready[i], ready[j]) })
	code := ready[0]
	return Action{
		Kind:        ActionStructure,
		Code:        code,
		TargetLevel: st.Structures[code] + 1,
		Reason:      "raise " + code,
	}, true
}

// nextResearch picks the highest-priority unmet, buildable technology.
func nextResearch(spec Spec, st State) (Action, bool) {
	want := map[string]int{}
	for code, lvl := range spec.Research {
		closure(want, code, lvl, 0)
		// Researching a tech to level L needs a Research Lab of at least L.
		closure(want, "research_lab", lvl, 0)
	}
	ready := make([]string, 0, len(want))
	for code, lvl := range want {
		if _, ok := game.Techs[code]; !ok {
			continue
		}
		if st.Techs[code] >= lvl {
			continue
		}
		if !game.RequiresMet(game.Requirements(code), combinedLevels(st)) {
			continue
		}
		target := st.Techs[code] + 1
		if st.Structures["research_lab"] < target {
			continue
		}
		if !canAfford(spec, st, techCost(code, target)) {
			continue
		}
		ready = append(ready, code)
	}
	if len(ready) == 0 {
		return Action{}, false
	}
	sort.Slice(ready, func(i, j int) bool { return actionLess(spec, ready[i], ready[j]) })
	code := ready[0]
	return Action{
		Kind:        ActionResearch,
		Code:        code,
		TargetLevel: st.Techs[code] + 1,
		Reason:      "research " + code,
	}, true
}

// nextUnit picks the highest-priority unmet, buildable ship or defense.
func nextUnit(spec Spec, st State) (Action, bool) {
	targets := map[string]int64{}
	for code, n := range spec.Ships {
		targets[code] += int64(n)
	}
	for code, n := range spec.Defenses {
		targets[code] += int64(n)
	}
	// Energy deficit is covered by Solar Satellites (ship 212) when configured.
	if st.EnergyUsed > st.EnergyMax && spec.EnergySats > 0 {
		if targets["212"] < int64(spec.EnergySats) {
			targets["212"] = int64(spec.EnergySats)
		}
	}
	codes := make([]string, 0, len(targets))
	for code := range targets {
		codes = append(codes, code)
	}
	sort.Slice(codes, func(i, j int) bool { return actionLess(spec, codes[i], codes[j]) })

	for _, code := range codes {
		def, ok := game.LookupUnit(code)
		if !ok {
			continue
		}
		_, isShip := game.Ships[code]
		have := st.Defenses[code]
		kind := ActionDefense
		if isShip {
			have = st.Ships[code]
			kind = ActionShip
		}
		deficit := targets[code] - have
		if deficit <= 0 {
			continue
		}
		if !game.RequiresMet(def.Requires, combinedLevels(st)) {
			continue
		}
		if !canAfford(spec, st, unitCost(code, deficit)) {
			continue
		}
		return Action{Kind: kind, Code: code, Quantity: deficit, Reason: "build " + code}, true
	}
	return Action{}, false
}

// structureWants is the wanted structure level map: fixed targets, their
// prerequisite closure, and — once the fixed targets are met — the gradual,
// builder-bump and energy goals.
func structureWants(spec Spec, st State) map[string]int {
	all := goalWants(spec, st)
	want := map[string]int{}
	for code, lvl := range all {
		if _, ok := game.Structures[code]; ok {
			want[code] = lvl
		}
	}
	if !fixedStructuresMet(want, st) {
		return want
	}
	if spec.gradualMode() {
		for _, code := range spec.Gradual {
			if _, ok := game.Structures[code]; !ok {
				continue
			}
			next := st.Structures[code] + 1
			if cap := spec.Caps[code]; cap > 0 && next > cap {
				continue
			}
			if want[code] < next {
				want[code] = next
			}
		}
		if bumpNeeded(spec, st) {
			for _, code := range spec.BumpBuilders {
				if _, ok := game.Structures[code]; !ok {
					continue
				}
				next := st.Structures[code] + 1
				if cap := spec.Caps[code]; cap > 0 && next > cap {
					continue
				}
				if want[code] < next {
					want[code] = next
				}
			}
		}
	}
	if st.EnergyUsed > st.EnergyMax {
		if _, ok := game.Structures["solar_plant"]; ok {
			if want["solar_plant"] < st.Structures["solar_plant"]+1 {
				want["solar_plant"] = st.Structures["solar_plant"] + 1
			}
		}
	}
	return want
}

// goalWants computes the fixed goals (buildings, research, ships, defenses) and
// their full prerequisite closure, keyed by catalog code.
func goalWants(spec Spec, st State) map[string]int {
	all := map[string]int{}
	for code, lvl := range spec.Buildings {
		closure(all, code, lvl, 0)
	}
	for code, lvl := range spec.Research {
		closure(all, code, lvl, 0)
		closure(all, "research_lab", lvl, 0)
	}
	for code, qty := range spec.Ships {
		closure(all, code, int(qty), 0)
	}
	for code, qty := range spec.Defenses {
		closure(all, code, int(qty), 0)
	}
	return all
}

// closure raises want[code] to level and recursively adds prerequisites.
func closure(want map[string]int, code string, level int, depth int) {
	if depth > 32 || level <= 0 {
		return
	}
	if want[code] >= level {
		return
	}
	want[code] = level
	for req, lvl := range game.Requirements(code) {
		closure(want, req, lvl, depth+1)
	}
}

// fixedStructuresMet reports whether every wanted structure level is reached.
func fixedStructuresMet(want map[string]int, st State) bool {
	for code, lvl := range want {
		if st.Structures[code] < lvl {
			return false
		}
	}
	return true
}

// bumpNeeded reports whether a gradual mine's next build would exceed the
// configured builder-bump threshold, so Robot/Nanite should be raised.
func bumpNeeded(spec Spec, st State) bool {
	if spec.BuilderBumpSec <= 0 || len(spec.BumpBuilders) == 0 {
		return false
	}
	for _, code := range spec.Gradual {
		def, ok := game.Structures[code]
		if !ok {
			continue
		}
		if def.ID != 1 && def.ID != 2 && def.ID != 3 {
			continue // only mines trigger builder bumps
		}
		dur := game.StructureDuration(code, st.Structures[code]+1,
			st.Structures["robotics_factory"], st.Structures["nanite_factory"], st.GameSpeed)
		if dur.Seconds() > float64(spec.BuilderBumpSec) {
			return true
		}
	}
	return false
}

// priority orders candidate codes: explicit order first, then gradual, then
// builder bumps, then alphabetical.
func priority(spec Spec, code string) int {
	for i, c := range spec.Order {
		if c == code {
			return i
		}
	}
	for i, c := range spec.Gradual {
		if c == code {
			return 1000 + i
		}
	}
	for i, c := range spec.BumpBuilders {
		if c == code {
			return 2000 + i
		}
	}
	return 3000
}

// actionLess orders candidates by priority, breaking ties by code so the
// result is deterministic regardless of map iteration order.
func actionLess(spec Spec, a, b string) bool {
	pa, pb := priority(spec, a), priority(spec, b)
	if pa != pb {
		return pa < pb
	}
	return a < b
}

// combinedLevels merges structure and tech levels for prerequisite checks
// (unit and research prerequisites may span both scopes).
func combinedLevels(st State) map[string]int {
	m := make(map[string]int, len(st.Structures)+len(st.Techs))
	for k, v := range st.Structures {
		m[k] = v
	}
	for k, v := range st.Techs {
		if v > m[k] {
			m[k] = v
		}
	}
	return m
}

func canAfford(spec Spec, st State, cost game.Cost) bool {
	reserve := game.Cost{}
	if spec.MinReserve != nil {
		reserve = *spec.MinReserve
	}
	return st.Resources.Metal >= cost.Metal+reserve.Metal &&
		st.Resources.Crystal >= cost.Crystal+reserve.Crystal &&
		st.Resources.Deuterium >= cost.Deuterium+reserve.Deuterium
}

func structureCost(code string, level int) game.Cost {
	c, _ := game.StructureCost(code, level)
	return c
}

func techCost(code string, level int) game.Cost {
	c, _ := game.TechCost(code, level)
	return c
}

func unitCost(code string, qty int64) game.Cost {
	c, _ := game.UnitCost(code, qty)
	return c
}

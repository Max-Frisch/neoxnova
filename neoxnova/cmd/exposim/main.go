// Command exposim sweeps candidate expedition fleet compositions against the
// observed live enemy model (enemy = fleet x roll + small template) using the
// same pure combat engine as the scheduler. Run:
//
//	go run ./cmd/exposim
//
// It reports, per composition, the rebuild time (from the live shipyard
// "N per second" tiers), the fleet's build throughput (points/s), own losses as
// a percent of fleet points, win rate and cargo capacity. Use it to pick the
// expedition fleet before changing the farm plan.
package main

import (
	"fmt"
	"math"
	"sort"

	"neoxnova/internal/game"
)

// Live shipyard "Building: N per second" tiers (data/_sy.html, 2026-10-08).
var buildRatePerSec = map[string]float64{
	"202": 288, "203": 288, "204": 288, "205": 288, "212": 288,
	"206": 132, "207": 132, "215": 132, "217": 132, "219": 132,
	"214": 20, "216": 20, "225": 20, "226": 20, "227": 20,
	"228": 76, // Black Wanderer (data/_sy_live.html, 2026-10-10)
}

// Server-scaled cargo holds (data/unit-info.json + combat.cargoCapacity).
var cargoHold = map[string]float64{
	"202": 25000, "203": 50000, "209": 20000,
	"217": 400000000, "219": 200000000,
	"216": 15000000, "225": 500000, "226": 1000000,
	"228": 2000000, // Black Wanderer (data/unit-info.json)
}

// Real acc1 combat techs. Only the general Weapons/Shield/Armour research
// (109/110/111) is mirrored onto the pirates/aliens; the specific weapon techs
// (120/121/122/199), arsenal upgrades, academy and governors are attacker-only.
var (
	atkTechs = game.CombatTechs{Weapons: 18, Shield: 17, Armour: 18, Laser: 27, Ion: 25, Plasma: 22, Graviton: 5}
	atkAcad  = map[string]int{"1103": 1} // Double attack L1
	// Full live player stack (2026-10-10 BW analysis): flat attacker bonuses the
	// combat report does not mirror onto the NPC.
	atkDmg   = 26.0 // Weaponry19 + Weapons Class A16 + Empire1 − DefClassA10
	atkShld  = 16.0 // defence-branch academy shields
	atkHull  = 14.0 // defence-branch academy hull
	atkRFRed = 16.0 // Heavy Armour: −16% rapid fire suffered
	// Arsenal upgrades owned by acc1, as accumulated percent per upgrade code.
	atkUpg = map[int]float64{1: 1.5, 2: 3, 3: 5.25, 4: 2.25, 7: 0.8, 10: 0.4}
)

// General (109/110/111) research the NPC mirrors. The enemy's combat report
// shows ONE rolled Weapons/Shield/Armour value on all three stats; it scales
// with the strongest of the general techs. Observed: Pirates ~+10–139%, Aliens
// up to +202% (the wiping fights) — see docs/EXPEDITIONS_LIVE_2026-10-06.md §8.
// Modelled as a flat bonus on the mirrored fleet with no specific weapon techs
// (those are attacker-only).
var genBonus = game.MirroredResearchBonus(atkTechs)

const (
	budget = 2.0e9
	// Black Wanderer sweep: one live per-fleet slice (60,500 BW / 9 slots ≈
	// 6,722 BW × 120M pts) so the fixed enemy template (~165 ships) is noise and
	// the fleet mirror dominates, as it does live.
	bwBudget = 8.067e11
	seeds    = 200
)

var template = map[string]int64{"203": 81, "204": 21, "206": 40, "207": 9, "213": 14}

func pointsOf(code string) int64 {
	d, ok := game.LookupUnit(code)
	if !ok {
		return 0
	}
	return d.BaseCost.Metal + d.BaseCost.Crystal
}

func scaleComp(weights map[string]float64, b float64) map[string]int64 {
	var total float64
	for code, w := range weights {
		total += w * float64(pointsOf(code))
	}
	s := b / total
	out := map[string]int64{}
	for code, w := range weights {
		if n := int64(math.Round(w * s)); n > 0 {
			out[code] = n
		}
	}
	return out
}

func mirrorOf(units map[string]int64, factor float64, tmpl bool) map[string]int64 {
	out := map[string]int64{}
	for code, n := range units {
		if m := int64(math.Round(factor * float64(n))); m > 0 {
			out[code] = m
		}
	}
	if tmpl {
		for code, n := range template {
			out[code] += n
		}
	}
	return out
}

func fleetPoints(u map[string]int64) float64 {
	var p float64
	for c, n := range u {
		p += float64(n) * float64(pointsOf(c))
	}
	return p
}

func buildSeconds(u map[string]int64) float64 {
	var s float64
	for c, n := range u {
		r := buildRatePerSec[c]
		if r == 0 {
			r = 132
		}
		s += float64(n) / r
	}
	return s
}

func fleetCargo(u map[string]int64) float64 {
	var c float64
	for code, n := range u {
		c += float64(n) * cargoHold[code]
	}
	return c
}

func lossPct(atk map[string]int64, factor, flat float64, tmpl bool) (loss, win, rounds float64) {
	a := game.Combatant{
		Units: atk, Techs: atkTechs, Academy: atkAcad, Upgrades: atkUpg,
		AcademyDamagePct: atkDmg, AcademyShieldPct: atkShld, AcademyHullPct: atkHull,
		AcademyRFReductionPct: atkRFRed,
	}
	d := game.Combatant{Units: mirrorOf(atk, factor, tmpl), FlatBonusPct: flat}
	fp := fleetPoints(atk)
	for s := int64(1); s <= seeds; s++ {
		res := game.Resolve(a, d, s)
		if res.Winner == "attacker" {
			win++
		}
		rounds += float64(res.Rounds)
		for code, n := range res.Attacker.Lost {
			loss += float64(n) * float64(pointsOf(code))
		}
	}
	return loss / seeds / fp * 100, win / seeds * 100, rounds / seeds
}

type row struct {
	name     string
	points   float64
	buildSec float64
	ptsPerS  float64
	lossPir  float64
	lossAln  float64
	lossWipe float64
	win      float64
	winWipe  float64
	rounds   float64
	cargo    float64
}

func main() {
	comps := []struct {
		name    string
		weights map[string]float64
		budget  float64 // 0 => default budget
	}{
		{"current BB+5HC+BR", map[string]float64{"207": 1, "203": 5, "219": 0.004}, 0},
		{"BB only", map[string]float64{"207": 1}, 0},
		{"BB+20x217", map[string]float64{"207": 1, "217": 0.0006}, 0},
		{"Galleon only", map[string]float64{"225": 1}, 0},
		{"Galleon+5x217", map[string]float64{"225": 1, "217": 0.0035}, 0},
		{"BlackMoon only", map[string]float64{"216": 1}, 0},
		{"BlackMoon+5x217+2x219", map[string]float64{"216": 1, "217": 0.032, "219": 0.013}, 0},
		{"Frigate only", map[string]float64{"227": 1}, 0},
		{"Live Frigate+BR 1:1", map[string]float64{"227": 1, "219": 1}, 0},
		{"Frigate+BR 1:25", map[string]float64{"227": 1, "219": 0.04}, 0},
		{"Frigate+20x217+5x219", map[string]float64{"227": 1, "217": 0.4, "219": 0.1}, 0},
		{"Frigate+BM", map[string]float64{"227": 1, "216": 2}, 0},
		{"BM+4HF", map[string]float64{"216": 1, "205": 4}, 0},
		{"BB+2HC", map[string]float64{"207": 1, "203": 2}, 0},
		// Black Wanderer comps at the live per-fleet size (~6722 BW/fleet),
		// so the fixed enemy template is negligible and the mirror dominates.
		{"BW only", map[string]float64{"228": 1}, bwBudget},
		{"BW + BR 0.2/BW", map[string]float64{"228": 1, "219": 0.2}, bwBudget},
		{"BW + BR 0.5/BW", map[string]float64{"228": 1, "219": 0.5}, bwBudget},
		{"BW + BR 1.0/BW", map[string]float64{"228": 1, "219": 1.0}, bwBudget},
		{"BW + BT 0.1/BW", map[string]float64{"228": 1, "217": 0.1}, bwBudget},
		{"BW + BT 1/BW", map[string]float64{"228": 1, "217": 1.0}, bwBudget},
		{"BW + BT 8/BW (whale)", map[string]float64{"228": 1, "217": 8.0}, bwBudget},
		{"BW + BR0.5 + BT1", map[string]float64{"228": 1, "219": 0.5, "217": 1.0}, bwBudget},
		{"BW whale 8BT+0.5BR", map[string]float64{"228": 1, "217": 8.0, "219": 0.5}, bwBudget},
	}

	var rows []row
	for _, c := range comps {
		b := c.budget
		if b == 0 {
			b = budget
		}
		atk := scaleComp(c.weights, b)
		// Typical pirate fight: mirror 0.60–0.69 (med 0.64), soft strength roll
		// (med ~0.70x the general 109 bonus) per docs §11.
		lPir, _, _ := lossPct(atk, 0.64, 0.70*genBonus, true)
		// Hard alien fight: mirror 0.85–0.94 (med 0.90), ~2.2x the general tech (the wipes).
		lAln, win, rnd := lossPct(atk, 0.90, 2.2*genBonus, false)
		// Wipe tail: the observed ~2.2x–3.0x alien W/S/A roll that annihilates fleets.
		lWipe, wWipe, _ := lossPct(atk, 0.94, 3.0*genBonus, false)
		bs := buildSeconds(atk)
		rows = append(rows, row{
			name: c.name, points: b, buildSec: bs, ptsPerS: b / bs,
			lossPir: lPir, lossAln: lAln, lossWipe: lWipe, win: win, winWipe: wWipe, rounds: rnd, cargo: fleetCargo(atk),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].lossPir < rows[j].lossPir })

	fmt.Printf("\n%d seeds; legacy comps %.2g pts, BW comps %.2g pts (~%d BW/fleet)\n",
		seeds, budget, bwBudget, int(bwBudget/float64(pointsOf("228"))))
	fmt.Printf("attacker: acc1 techs (109/110/111 + laser/ion/plasma/graviton) + academy\n")
	fmt.Printf("defender: fleet mirror (pirate 0.60-0.69, alien 0.85-0.94) + single rolled W/S/A\n")
	fmt.Printf("          pirate strength ~0.70x / hard alien ~2.2x our 109 bonus = %.0f%%\n", 2.2*genBonus)
	fmt.Printf("%-22s %9s %8s %10s %8s %8s %8s %6s %8s %7s %10s\n",
		"comp", "fleetPts", "rebuild", "pts/s", "lossPir", "lossAln", "lossWipe", "win%", "wipeWin", "rounds", "cargo")
	for _, r := range rows {
		fmt.Printf("%-22s %9.3g %7.0fs %10.3g %7.1f%% %7.1f%% %8.1f%% %5.0f%% %7.0f%% %7.1f %10.3g\n",
			r.name, r.points, r.buildSec, r.ptsPerS, r.lossPir, r.lossAln, r.lossWipe, r.win, r.winWipe, r.rounds, r.cargo)
	}
	fmt.Println()
}

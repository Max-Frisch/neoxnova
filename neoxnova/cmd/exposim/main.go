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
}

// Server-scaled cargo holds (data/unit-info.json + combat.cargoCapacity).
var cargoHold = map[string]float64{
	"202": 25000, "203": 50000, "209": 20000,
	"217": 400000000, "219": 200000000,
	"216": 15000000, "225": 500000, "226": 1000000,
}

// Real acc1 combat techs. Only the general Weapons/Shield/Armour research
// (109/110/111) is mirrored onto the pirates/aliens; the specific weapon techs
// (120/121/122/199), arsenal upgrades, academy and governors are attacker-only.
var (
	atkTechs = game.CombatTechs{Weapons: 18, Shield: 17, Armour: 18, Laser: 27, Ion: 25, Plasma: 22, Graviton: 5}
	atkAcad  = map[string]int{"1103": 1} // Double attack L1
	atkDmg   = 8.0                       // Weaponry L6 + Weapons Class A L1
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
	seeds  = 40
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
	a := game.Combatant{Units: atk, Techs: atkTechs, Academy: atkAcad, AcademyDamagePct: atkDmg}
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
	buildSec float64
	ptsPerS  float64
	lossPir  float64
	lossAln  float64
	win      float64
	rounds   float64
	cargo    float64
}

func main() {
	comps := []struct {
		name    string
		weights map[string]float64
	}{
		{"current BB+5HC+BR", map[string]float64{"207": 1, "203": 5, "219": 0.004}},
		{"BB only", map[string]float64{"207": 1}},
		{"BB+20x217", map[string]float64{"207": 1, "217": 0.0006}},
		{"Galleon only", map[string]float64{"225": 1}},
		{"Galleon+5x217", map[string]float64{"225": 1, "217": 0.0035}},
		{"BlackMoon only", map[string]float64{"216": 1}},
		{"BlackMoon+5x217+2x219", map[string]float64{"216": 1, "217": 0.032, "219": 0.013}},
		{"Frigate only", map[string]float64{"227": 1}},
		{"Live Frigate+BR 1:1", map[string]float64{"227": 1, "219": 1}},
		{"Frigate+BR 1:25", map[string]float64{"227": 1, "219": 0.04}},
		{"Frigate+20x217+5x219", map[string]float64{"227": 1, "217": 0.4, "219": 0.1}},
		{"Frigate+BM", map[string]float64{"227": 1, "216": 2}},
		{"BM+4HF", map[string]float64{"216": 1, "205": 4}},
		{"BB+2HC", map[string]float64{"207": 1, "203": 2}},
	}

	var rows []row
	for _, c := range comps {
		atk := scaleComp(c.weights, budget)
		// Typical pirate fight: fleet mirror 0.66, soft research roll.
		lPir, _, _ := lossPct(atk, 0.66, 0.6*genBonus, true)
		// Hard alien fight: high fleet roll, ~2.2x the general tech (the wipes).
		lAln, win, rnd := lossPct(atk, 0.90, 2.2*genBonus, false)
		bs := buildSeconds(atk)
		rows = append(rows, row{
			name: c.name, buildSec: bs, ptsPerS: budget / bs,
			lossPir: lPir, lossAln: lAln, win: win, rounds: rnd, cargo: fleetCargo(atk),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].lossPir < rows[j].lossPir })

	fmt.Printf("\nbudget %.2g pts/fleet, %d seeds\n", budget, seeds)
	fmt.Printf("attacker: acc1 techs (109/110/111 + laser/ion/plasma/graviton) + academy\n")
	fmt.Printf("defender: fleet mirror + single rolled W/S/A (pirate ~0.6x, hard alien ~2.2x our 109 bonus = %.0f%%)\n", 2.2*genBonus)
	fmt.Printf("%-20s %8s %10s %8s %8s %6s %7s %10s\n",
		"comp", "rebuild", "pts/s", "lossPir", "lossAln", "win%", "rounds", "cargo")
	for _, r := range rows {
		fmt.Printf("%-20s %7.0fs %10.3g %7.1f%% %7.1f%% %5.0f%% %7.1f %10.3g\n",
			r.name, r.buildSec, r.ptsPerS, r.lossPir, r.lossAln, r.win, r.rounds, r.cargo)
	}
	fmt.Println()
}

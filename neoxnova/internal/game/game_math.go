package game

import (
	"math"
	"time"
)

// CalculateCoordinateDistance returns the fleet travel distance between two
// coordinate vectors, in the reference server's units. Piecewise rules fitted to
// live reference-server samples (see the private ops repo §6):
//
//	same system:           1000 + 5*|dp|           (2:188:9 -> 2:188:16 = 1035)
//	same galaxy, diff sys: 2700 + 95*|ds|          (2:188:9 -> 2:186:9  = 2890)
//	                        (+ 1000 + 5*|dp| when the positions also differ)
//	cross galaxy:          28000 + 100*|dg|        (standard; one sample only)
func CalculateCoordinateDistance(g1, s1, p1, g2, s2, p2 int) float64 {
	dg := math.Abs(float64(g1 - g2))
	ds := math.Abs(float64(s1 - s2))
	dp := math.Abs(float64(p1 - p2))

	if dg != 0 {
		return 28000.0 + 100.0*dg
	}
	if ds == 0 {
		return 1000.0 + 5.0*dp
	}
	d := 2700.0 + 95.0*ds
	if dp > 0 {
		d += 1000.0 + 5.0*dp
	}
	return d
}

// unitSpeed is the base travel speed of each unit (defaults; refine with
// captures). Engine techs multiply the relevant drive's units by +10%/level.
var unitSpeed = map[string]int{
	"202": 7500, "203": 15000, "204": 12500, "205": 10000, "206": 15000,
	"207": 10000, "208": 2500, "209": 2000, "210": 100000000, "211": 4000,
	"212": 0, "213": 10000, "214": 100, "215": 10000, "216": 10000,
	"217": 10000, "219": 2000, "220": 0, "222": 1000, "225": 15000, "226": 5000,
	"227": 15000, "228": 10000,
}

// stationaryUnits are units that never leave the ground: the Solar Satellite
// feeds the planet's energy grid, so it can be built but never dispatched.
var stationaryUnits = map[string]bool{"212": true}

// Movable reports whether a unit can be launched in a fleet. Stationary units
// (Solar Satellites) are excluded from fleet dispatch.
func Movable(code string) bool { return !stationaryUnits[code] }

// unitDrive maps a unit to its engine tech code: 115 combustion, 117 impulse,
// 118 hyperspace.
var unitDrive = map[string]string{
	"202": "115", "203": "115", "204": "115", "209": "115", "210": "115", "219": "115",
	"205": "117", "206": "117", "208": "117", "211": "117", "213": "117", "225": "117",
	"207": "118", "214": "118", "215": "118", "216": "118", "217": "118", "222": "118",
	"226": "118", "227": "118", "228": "118",
}

// engineUpgradeByDrive maps a drive tech code to its Arsenal engine upgrade
// code (11 combustion, 12 impulse, 13 hyperspace).
var engineUpgradeByDrive = map[string]int{"115": 11, "117": 12, "118": 13}

// FleetMaxSpeed returns the fleet's limiting speed: the slowest ship's base
// speed boosted by its drive technology (+10% per level) plus the matching
// Arsenal engine upgrade (additive percent). academyEnginePct is the flat
// percent from Academy "Engine limitation" (+3%/level) / "Empire", applied to
// every drive. upgrades may be nil. Defaults to 10000 when no known ship is
// present.
func FleetMaxSpeed(units map[string]int64, combustion, impulse, hyperspace int, upgrades map[int]float64, academyEnginePct float64) int {
	levels := map[string]int{"115": combustion, "117": impulse, "118": hyperspace}
	slowest := 0
	for code, n := range units {
		if n <= 0 {
			continue
		}
		base := unitSpeed[code]
		if base <= 0 {
			continue
		}
		drive := unitDrive[code]
		pct := 0.1*float64(levels[drive]) + upgrades[engineUpgradeByDrive[drive]]/100.0 + academyEnginePct/100.0
		speed := int(math.Round(float64(base) * (1.0 + pct)))
		if slowest == 0 || speed < slowest {
			slowest = speed
		}
	}
	if slowest == 0 {
		slowest = 10000
	}
	return slowest
}

// flightTimeCoefficient is fitted so that the reference sample
// 2:188:9 -> 2:188:16 (distance 1035, fleet speed 15x override) resolves to
// roughly 300 s one-way, as observed live. It needs more samples to pin down.
const flightTimeCoefficient = 4275.0

// CalculateFlightDuration returns the one-way trip time in seconds.
// fleetSpeed is universes.fleet_speed (15 on the reference server with the player
// override); speedPercent is the 10..100 velocity toggle.
func CalculateFlightDuration(distance float64, baseMaxSpeed int, speedPercent int, fleetSpeed float64) int64 {
	if fleetSpeed <= 0 {
		fleetSpeed = 1.0
	}
	if baseMaxSpeed <= 0 {
		baseMaxSpeed = 10000
	}
	velocityModifier := float64(speedPercent) / 100.0
	if velocityModifier <= 0 {
		velocityModifier = 1.0
	}
	raw := flightTimeCoefficient / velocityModifier * math.Sqrt(distance*10.0/float64(baseMaxSpeed))
	return int64(math.Max(1, math.Round(raw/fleetSpeed)))
}

// CalculateHoldDuration compresses a player-selected hold window (the expedition
// "exp_time" / generic "Hold (h)") by the universe fleet-speed multiplier, the
// same factor CalculateFlightDuration applies to the flight legs. That way the
// whole mission scales as 1/fleet_speed: an expedition at 15x totals ~1/3 of the
// same expedition at 5x. A non-zero hold never collapses below one second.
func CalculateHoldDuration(hold time.Duration, fleetSpeed float64) time.Duration {
	if hold <= 0 {
		return 0
	}
	if fleetSpeed <= 0 {
		fleetSpeed = 1.0
	}
	scaled := time.Duration(math.Round(float64(hold) / fleetSpeed))
	if scaled < time.Second {
		scaled = time.Second
	}
	return scaled
}

// fuelDivisor converts (base fuel x distance x speed) into deuterium, fitted to
// the live samples (e.g. 1 Light Cargo, distance 2985, 100% -> 7 deuterium).
const fuelDivisor = 8750.0

// unitFuel is the engine's per-unit base fuel consumption. Entries marked
// (derived) were solved from live samples; the rest are sane defaults.
var unitFuel = map[string]int{
	"202": 20,  // Light Cargo (derived)
	"203": 50,  // Heavy Cargo
	"204": 20,  // Light Fighter
	"205": 100, // Heavy Fighter
	"206": 300, // Cruiser
	"207": 250, // Battleship (derived)
	"208": 1000,
	"209": 300, // Recycler
	"210": 1,   // Spy Probe
	"211": 500, // Planet Bomber
	"212": 0,   // Solar Satellite (stationary)
	"213": 500, // Star Fighter
	"214": 1000,
	"215": 250, // Battle Cruiser
	"216": 1000,
	"217": 100, // Battle Transporter
	"219": 300, // Battle Recycler
	"220": 0,
	"222": 950, // Flying Death (live info card, 2026-10-12)
	"225": 320, // Galleon (derived)
	"226": 500, // Destroyer
	"227": 500, // Frigate
	"228": 1000,
}

// UnitFuel returns the base fuel consumption of a unit (0 if unknown).
func UnitFuel(code string) int { return unitFuel[code] }

// FleetFuelBase sums base fuel x count over a fleet.
func FleetFuelBase(units map[string]int64) int64 {
	var total int64
	for code, n := range units {
		if n > 0 {
			total += int64(unitFuel[code]) * n
		}
	}
	return total
}

// CalculateDeuteriumConsumption returns the fuel burned for a trip:
//
//	fuel = max(1, round(baseFuelTotal * distance / fuelDivisor * speedMod * (1-academyFuel/100)))
//
// where speedMod is the velocity percentage (0..1) and academyFuelReductionPct is
// the Academy "Engine economication"/"Relief ships" global reduction. Note the
// fitted reference behaviour: lower speed burns *less* fuel, so fuel scales with
// speedPercent.
func CalculateDeuteriumConsumption(baseFuelTotal int64, distance float64, speedPercent int, academyFuelReductionPct float64) int64 {
	if baseFuelTotal <= 0 {
		return 0
	}
	speedMod := float64(speedPercent) / 100.0
	if speedMod <= 0 {
		speedMod = 1.0
	}
	fuel := float64(baseFuelTotal) * distance / fuelDivisor * speedMod
	if academyFuelReductionPct > 0 {
		fuel *= 1.0 - academyFuelReductionPct/100.0
	}
	return int64(math.Max(1, math.Round(fuel)))
}

package game

import (
	"math"
	"time"
)

// Economy is the derived hourly output of a celestial object.
type Economy struct {
	MetalPerHour   float64
	CrystalPerHour float64
	DeutPerHour    float64
	EnergyUsed     float64
	EnergyMax      float64
}

// incomeForLevel returns the classic OGame-style geometric income curve.
func incomeForLevel(base, level float64) float64 {
	return base * level * math.Pow(1.1, level)
}

// RecomputeProduction derives hourly production, energy draw and energy output
// from a celestial's structure levels. resourceSpeed is the universe's resource
// production multiplier (e.g. 10000 on niburuspace.com). When energy demand
// exceeds supply, all production is scaled down proportionally.
func RecomputeProduction(levels map[string]int, tempMax int, resourceSpeed float64) Economy {
	lvl := func(code string) float64 { return float64(levels[code]) }

	if resourceSpeed <= 0 {
		resourceSpeed = 1.0
	}

	metal := incomeForLevel(30, lvl("metal_mine")) * resourceSpeed
	crystal := incomeForLevel(20, lvl("crystal_mine")) * resourceSpeed
	deutBase := incomeForLevel(10, lvl("deuterium_synthesizer"))
	tempFactor := 1.36 - 0.002*float64(tempMax)
	if tempFactor < 0 {
		tempFactor = 0
	}
	deut := deutBase * tempFactor * resourceSpeed

	energyUsed := incomeForLevel(10, lvl("metal_mine")) +
		incomeForLevel(10, lvl("crystal_mine")) +
		incomeForLevel(20, lvl("deuterium_synthesizer"))
	energyMax := incomeForLevel(20, lvl("solar_plant"))

	if energyUsed > energyMax && energyUsed > 0 {
		efficiency := energyMax / energyUsed
		metal *= efficiency
		crystal *= efficiency
		deut *= efficiency
	}

	return Economy{
		MetalPerHour:   metal,
		CrystalPerHour: crystal,
		DeutPerHour:    deut,
		EnergyUsed:     energyUsed,
		EnergyMax:      energyMax,
	}
}

// pow is a tiny wrapper so catalog formulas read cleanly.
func pow(base, exp float64) float64 { return math.Pow(base, exp) }

// secondsToDuration converts seconds to a Duration with a 1s floor.
func secondsToDuration(secs float64) time.Duration {
	if secs < 1 {
		return time.Second
	}
	return time.Duration(math.Round(secs)) * time.Second
}

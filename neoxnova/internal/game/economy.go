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

// Production upgrade codes (Arsenal types 17/18/19).
const (
	upgradeMetalProduction     = 17
	upgradeCrystalProduction   = 18
	upgradeDeuteriumProduction = 19
)

// Base (structure-free) production freebie, expressed per unit of the universe's
// resource_speed so it scales with the server rate. On niburu (resource_speed
// 10000) this is 30M metal / 20M crystal / 10M deuterium per hour. It is a flat
// floor: it needs no energy and is not scaled by an energy deficit, so a
// freshly-founded colony with zero structures still accrues resources.
const (
	BaseMetalPerHour   = 3000.0
	BaseCrystalPerHour = 2000.0
	BaseDeutPerHour    = 1000.0
)

// ProductionBonus is an account's Arsenal production bonus in percent for each
// resource. The zero value adds nothing.
type ProductionBonus struct {
	Metal     float64
	Crystal   float64
	Deuterium float64
}

// ProductionBonusFor extracts the production upgrade percentages (17/18/19)
// from an account's Arsenal upgrade map (code -> accumulated percent).
func ProductionBonusFor(upgrades map[int]float64) ProductionBonus {
	b := ProductionBonus{
		Metal:     upgrades[upgradeMetalProduction],
		Crystal:   upgrades[upgradeCrystalProduction],
		Deuterium: upgrades[upgradeDeuteriumProduction],
	}
	if b.Metal < 0 {
		b.Metal = 0
	}
	if b.Crystal < 0 {
		b.Crystal = 0
	}
	if b.Deuterium < 0 {
		b.Deuterium = 0
	}
	return b
}

// IsProductionUpgrade reports whether an Arsenal code affects resource
// production and therefore requires the cached production columns to be
// recomputed when it changes.
func IsProductionUpgrade(code int) bool {
	switch code {
	case upgradeMetalProduction, upgradeCrystalProduction, upgradeDeuteriumProduction:
		return true
	default:
		return false
	}
}

// RecomputeProduction derives hourly production, energy draw and energy output
// from a celestial's structure levels. resourceSpeed is the universe's resource
// production multiplier (e.g. 10000 on niburuspace.com). bonus is the owner's
// account-wide Arsenal production bonus. When energy demand exceeds supply, all
// mine production is scaled down proportionally. When baseOn is true a flat,
// energy-free baseline (see Base*PerHour) is added to every resource, so a
// planet with zero structures still produces.
func RecomputeProduction(levels map[string]int, tempMax int, resourceSpeed float64, satCount int, bonus ProductionBonus, baseOn bool) Economy {
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
	// Solar satellites produce energy scaled by the planet's temperature.
	if satCount > 0 {
		energyMax += float64(satCount) * float64(SatelliteEnergy(tempMax))
	}

	if energyUsed > energyMax && energyUsed > 0 {
		efficiency := energyMax / energyUsed
		metal *= efficiency
		crystal *= efficiency
		deut *= efficiency
	}

	// The base freebie sits on top of the (deficit-scaled) mine output.
	if baseOn {
		metal += BaseMetalPerHour * resourceSpeed
		crystal += BaseCrystalPerHour * resourceSpeed
		deut += BaseDeutPerHour * resourceSpeed
	}

	// Arsenal production upgrades: additive percentage on the matching resource.
	metal *= 1.0 + bonus.Metal/100.0
	crystal *= 1.0 + bonus.Crystal/100.0
	deut *= 1.0 + bonus.Deuterium/100.0

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

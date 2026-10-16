package game

import "math"

// ProductionLine is one row of the resources-page production breakdown. Amounts
// are the actual (energy-deficit-scaled) hourly figures for that source; Energy
// is signed (negative = consumption, positive = generation). Percent is the
// energy efficiency applied to that line (100 when the planet is not short on
// energy). A moon/empty Code marks a computed row rather than a catalog item.
type ProductionLine struct {
	Code      string
	Name      string
	Level     int
	Quantity  int64
	Metal     float64
	Crystal   float64
	Deuterium float64
	Energy    float64
	Percent   int
}

// ProductionTotals is a metal / crystal / deuterium / energy tuple. Energy is
// the net (supply minus demand) in the total row, and the storage capacity in
// the storage row.
type ProductionTotals struct {
	Metal     float64
	Crystal   float64
	Deuterium float64
	Energy    float64
}

// ProductionReport is the per-source table rendered by GET /resources/{id}.
// Lines holds the flat basic freebie (when baseOn) followed by every mine,
// plant, and the solar satellite; Bonus holds the account-wide extraction
// research + Arsenal production bonus amounts; Storage holds the per-resource
// capacities; Total reconciles to RecomputeProduction, which owns the cached
// hourly figures shown everywhere else.
type ProductionReport struct {
	Lines   []ProductionLine
	Bonus   ProductionLine
	Storage ProductionTotals
	Total   ProductionTotals
}

// ProductionBreakdown derives the resources-page table from a celestial's
// structure levels, temperature, universe resource speed, solar-satellite
// count and account-wide production bonus. Its Total matches
// RecomputeProduction exactly (the same curves and ordering), and every line
// plus Bonus sums to that total for the three resources.
func ProductionBreakdown(levels map[string]int, tempMax int, resourceSpeed float64, satCount int, bonus ProductionBonus, baseOn bool) ProductionReport {
	if resourceSpeed <= 0 {
		resourceSpeed = 1.0
	}
	lvl := func(code string) float64 { return float64(levels[code]) }

	metalMine := incomeForLevel(30, lvl("metal_mine")) * resourceSpeed
	crystalMine := incomeForLevel(20, lvl("crystal_mine")) * resourceSpeed
	deutBase := incomeForLevel(10, lvl("deuterium_synthesizer"))
	tempFactor := 1.36 - 0.002*float64(tempMax)
	if tempFactor < 0 {
		tempFactor = 0
	}
	deutSynth := deutBase * tempFactor * resourceSpeed

	energyUsed := incomeForLevel(10, lvl("metal_mine")) +
		incomeForLevel(10, lvl("crystal_mine")) +
		incomeForLevel(20, lvl("deuterium_synthesizer"))
	energyMax := incomeForLevel(20, lvl("solar_plant"))
	if satCount > 0 {
		energyMax += float64(satCount) * float64(SatelliteEnergy(tempMax))
	}
	// Match RecomputeProduction: the Energy governator scales energy output.
	energyMax *= 1.0 + bonus.Energy/100.0

	// Energy deficit scales the three mines proportionally, exactly as
	// RecomputeProduction does.
	eff := 1.0
	if energyUsed > energyMax && energyUsed > 0 {
		eff = energyMax / energyUsed
	}
	percent := int(math.Round(eff * 100))

	baseMetal, baseCrystal, baseDeut := 0.0, 0.0, 0.0
	var lines []ProductionLine
	if baseOn {
		baseMetal = BaseMetalPerHour * resourceSpeed
		baseCrystal = BaseCrystalPerHour * resourceSpeed
		baseDeut = BaseDeutPerHour * resourceSpeed
		lines = append(lines, ProductionLine{
			Name: "Basic Production", Metal: baseMetal, Crystal: baseCrystal, Deuterium: baseDeut,
		})
	}

	lines = append(lines,
		ProductionLine{Code: "metal_mine", Name: "Metal Mine", Level: levels["metal_mine"],
			Metal: metalMine * eff, Energy: -incomeForLevel(10, lvl("metal_mine")), Percent: percent},
		ProductionLine{Code: "crystal_mine", Name: "Crystal Mine", Level: levels["crystal_mine"],
			Crystal: crystalMine * eff, Energy: -incomeForLevel(10, lvl("crystal_mine")), Percent: percent},
		ProductionLine{Code: "deuterium_synthesizer", Name: "Deuterium Refinery", Level: levels["deuterium_synthesizer"],
			Deuterium: deutSynth * eff, Energy: -incomeForLevel(20, lvl("deuterium_synthesizer")), Percent: percent},
		ProductionLine{Code: "solar_plant", Name: "Solar Power plant", Level: levels["solar_plant"],
			Energy: incomeForLevel(20, lvl("solar_plant")), Percent: 100},
	)
	// The deuterium power plant is catalogued but its energy is not modelled by
	// the engine, so it only shows when the player has one.
	if levels["deuterium_power_plant"] > 0 {
		lines = append(lines, ProductionLine{Code: "deuterium_power_plant", Name: "Deuterium Power Plant",
			Level: levels["deuterium_power_plant"], Percent: 100})
	}
	if satCount > 0 {
		lines = append(lines, ProductionLine{Code: "212", Name: "Solar Satellite", Quantity: int64(satCount),
			Energy: float64(satCount) * float64(SatelliteEnergy(tempMax)), Percent: 100})
	}

	preMetal := metalMine*eff + baseMetal
	preCrystal := crystalMine*eff + baseCrystal
	preDeut := deutSynth*eff + baseDeut

	return ProductionReport{
		Lines: lines,
		Bonus: ProductionLine{
			Code:      "bonus",
			Name:      "Bonus (Officers/Bonus)",
			Metal:     preMetal * bonus.Metal / 100.0,
			Crystal:   preCrystal * bonus.Crystal / 100.0,
			Deuterium: preDeut * bonus.Deuterium / 100.0,
		},
		Storage: ProductionTotals{
			Metal:     float64(StorageCapacity(levels["metal_storage"], resourceSpeed)),
			Crystal:   float64(StorageCapacity(levels["crystal_storage"], resourceSpeed)),
			Deuterium: float64(StorageCapacity(levels["deuterium_storage"], resourceSpeed)),
		},
		Total: ProductionTotals{
			Metal:     preMetal * (1.0 + bonus.Metal/100.0),
			Crystal:   preCrystal * (1.0 + bonus.Crystal/100.0),
			Deuterium: preDeut * (1.0 + bonus.Deuterium/100.0),
			Energy:    energyMax - energyUsed,
		},
	}
}

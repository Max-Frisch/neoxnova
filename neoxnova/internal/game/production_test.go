package game

import "testing"

func lineByCode(bd ProductionReport, code string) (ProductionLine, bool) {
	for _, l := range bd.Lines {
		if l.Code == code {
			return l, true
		}
	}
	return ProductionLine{}, false
}

func TestProductionBreakdownReconcilesToRecompute(t *testing.T) {
	levels := map[string]int{
		"metal_mine": 10, "crystal_mine": 8,
		"deuterium_synthesizer": 6, "solar_plant": 12,
		"metal_storage": 4, "crystal_storage": 3, "deuterium_storage": 2,
	}
	bonus := ProductionBonus{Metal: 12.5, Crystal: 3, Deuterium: 0.4}
	bd := ProductionBreakdown(levels, 40, 1.0, 0, bonus, true)
	eco := RecomputeProduction(levels, 40, 1.0, 0, bonus, true)

	// The total row must mirror the authoritative cached production figures.
	approx(t, "total metal", bd.Total.Metal, eco.MetalPerHour)
	approx(t, "total crystal", bd.Total.Crystal, eco.CrystalPerHour)
	approx(t, "total deuterium", bd.Total.Deuterium, eco.DeutPerHour)
	approx(t, "total energy", bd.Total.Energy, eco.EnergyMax-eco.EnergyUsed)

	// Every line plus the bonus row sums to the total.
	sumM, sumC, sumD := bd.Bonus.Metal, bd.Bonus.Crystal, bd.Bonus.Deuterium
	for _, l := range bd.Lines {
		sumM += l.Metal
		sumC += l.Crystal
		sumD += l.Deuterium
	}
	approx(t, "lines+bonus metal", sumM, bd.Total.Metal)
	approx(t, "lines+bonus crystal", sumC, bd.Total.Crystal)
	approx(t, "lines+bonus deuterium", sumD, bd.Total.Deuterium)

	// Storage capacities read the storage-building levels.
	if got, want := int64(bd.Storage.Metal), StorageCapacity(levels["metal_storage"], 1.0); got != want {
		t.Fatalf("storage metal = %d, want %d", got, want)
	}
}

func TestProductionBreakdownBaseFreebie(t *testing.T) {
	bd := ProductionBreakdown(nil, 40, 2.0, 0, ProductionBonus{}, true)
	if len(bd.Lines) == 0 || bd.Lines[0].Name != "Basic Production" {
		t.Fatalf("first line = %+v, want Basic Production", bd.Lines)
	}
	approx(t, "base metal", bd.Lines[0].Metal, BaseMetalPerHour*2)
	approx(t, "base crystal", bd.Lines[0].Crystal, BaseCrystalPerHour*2)
	approx(t, "base deuterium", bd.Lines[0].Deuterium, BaseDeutPerHour*2)
	if bd.Lines[0].Code != "" {
		t.Fatal("basic line must have an empty code (no percent column)")
	}

	off := ProductionBreakdown(nil, 40, 2.0, 0, ProductionBonus{}, false)
	if off.Lines[0].Name == "Basic Production" {
		t.Fatal("basic line must be omitted when baseOn is false")
	}
	approx(t, "no base metal", off.Total.Metal, 0)
}

func TestProductionBreakdownEnergyDeficit(t *testing.T) {
	// A mine with no energy plant: every mined line is scaled to zero and the
	// efficiency percent reflects the shortfall.
	bd := ProductionBreakdown(map[string]int{"metal_mine": 10}, 40, 1.0, 0, ProductionBonus{}, true)
	if _, ok := lineByCode(bd, "solar_plant"); !ok {
		t.Fatal("solar plant line should still render")
	}
	mine, _ := lineByCode(bd, "metal_mine")
	approx(t, "deficit mine metal", mine.Metal, 0)
	if mine.Percent != 0 {
		t.Fatalf("deficit percent = %d, want 0", mine.Percent)
	}
	// Base freebie survives the deficit.
	approx(t, "deficit total metal", bd.Total.Metal, BaseMetalPerHour)
}

func TestProductionBreakdownSatelliteAndBonus(t *testing.T) {
	levels := map[string]int{"solar_plant": 1}
	bd := ProductionBreakdown(levels, 40, 1.0, 3, ProductionBonus{Metal: 100}, true)
	sat, ok := lineByCode(bd, "212")
	if !ok {
		t.Fatal("solar satellite line missing")
	}
	if sat.Quantity != 3 {
		t.Fatalf("satellite quantity = %d, want 3", sat.Quantity)
	}
	approx(t, "satellite energy", sat.Energy, 3*float64(SatelliteEnergy(40)))
	// +100% bonus on the flat base doubles the metal total.
	approx(t, "bonus metal", bd.Bonus.Metal, BaseMetalPerHour)
	approx(t, "bonus total metal", bd.Total.Metal, BaseMetalPerHour*2)
}

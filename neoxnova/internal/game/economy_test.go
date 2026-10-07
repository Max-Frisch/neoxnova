package game

import (
	"math"
	"testing"
)

func approx(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestStructureCost(t *testing.T) {
	c1, ok := StructureCost("metal_mine", 1)
	if !ok || c1 != (Cost{Metal: 60, Crystal: 15}) {
		t.Fatalf("metal_mine lvl1 = %+v ok=%v, want {60 15 0}", c1, ok)
	}
	c2, _ := StructureCost("metal_mine", 2)
	if c2 != (Cost{Metal: 90, Crystal: 23}) {
		t.Fatalf("metal_mine lvl2 = %+v, want {90 23 0}", c2)
	}
	if _, ok := StructureCost("nope", 1); ok {
		t.Fatal("unknown structure should not resolve")
	}
}

func TestShipAndTechCost(t *testing.T) {
	c, ok := ShipCost("202", 10)
	if !ok || c != (Cost{Metal: 20000, Crystal: 20000}) {
		t.Fatalf("202 x10 = %+v ok=%v", c, ok)
	}
	if _, ok := ShipCost("202", 0); ok {
		t.Fatal("quantity < 1 should be rejected")
	}
	tc, _ := TechCost("energy_tech", 1)
	if tc != (Cost{Crystal: 800, Deuterium: 400}) {
		t.Fatalf("energy_tech lvl1 = %+v", tc)
	}
}

func TestRequiresMet(t *testing.T) {
	req := map[string]int{"robotics_factory": 2}
	if RequiresMet(req, map[string]int{"robotics_factory": 1}) {
		t.Fatal("should fail at level 1")
	}
	if !RequiresMet(req, map[string]int{"robotics_factory": 2}) {
		t.Fatal("should pass at level 2")
	}
}

func TestRecomputeProduction(t *testing.T) {
	levels := map[string]int{
		"metal_mine": 10, "crystal_mine": 8,
		"deuterium_synthesizer": 6, "solar_plant": 12,
	}
	eco := RecomputeProduction(levels, 40, 1.0, 0, ProductionBonus{})
	approx(t, "metal", eco.MetalPerHour, 778.122738030)
	approx(t, "crystal", eco.CrystalPerHour, 342.9742096)
	approx(t, "deut", eco.DeutPerHour, 136.0558848)
	approx(t, "energyUsed", eco.EnergyUsed, 643.44867081)
	approx(t, "energyMax", eco.EnergyMax, 753.2228104)
}

func TestRecomputeProductionEnergyDeficit(t *testing.T) {
	eco := RecomputeProduction(map[string]int{"metal_mine": 30}, 40, 1.0, 0, ProductionBonus{})
	if eco.EnergyMax != 0 {
		t.Fatalf("energyMax = %v, want 0", eco.EnergyMax)
	}
	if eco.MetalPerHour != 0 {
		t.Fatalf("metal with zero energy = %v, want 0 (deficit penalty)", eco.MetalPerHour)
	}
}

func TestRecomputeProductionResourceSpeed(t *testing.T) {
	levels := map[string]int{
		"metal_mine": 10, "crystal_mine": 8,
		"deuterium_synthesizer": 6, "solar_plant": 12,
	}
	base := RecomputeProduction(levels, 40, 1.0, 0, ProductionBonus{})
	scaled := RecomputeProduction(levels, 40, 10000.0, 0, ProductionBonus{})
	approx(t, "metal x10000", scaled.MetalPerHour, base.MetalPerHour*10000)
	approx(t, "crystal x10000", scaled.CrystalPerHour, base.CrystalPerHour*10000)
	approx(t, "deut x10000", scaled.DeutPerHour, base.DeutPerHour*10000)
	// Energy is not scaled by the resource multiplier.
	approx(t, "energyUsed unchanged", scaled.EnergyUsed, base.EnergyUsed)
}

func TestRecomputeProductionUpgradeBonus(t *testing.T) {
	levels := map[string]int{
		"metal_mine": 10, "crystal_mine": 8,
		"deuterium_synthesizer": 6, "solar_plant": 12,
	}
	base := RecomputeProduction(levels, 40, 1.0, 0, ProductionBonus{})
	boosted := RecomputeProduction(levels, 40, 1.0, 0, ProductionBonus{Metal: 100, Crystal: 50, Deuterium: 25})
	approx(t, "metal +100%", boosted.MetalPerHour, base.MetalPerHour*2)
	approx(t, "crystal +50%", boosted.CrystalPerHour, base.CrystalPerHour*1.5)
	approx(t, "deut +25%", boosted.DeutPerHour, base.DeutPerHour*1.25)
}

func TestProductionBonusFor(t *testing.T) {
	ups := map[int]float64{17: 12.5, 18: 3, 19: 0.4, 1: 99}
	b := ProductionBonusFor(ups)
	if b.Metal != 12.5 || b.Crystal != 3 || b.Deuterium != 0.4 {
		t.Fatalf("ProductionBonusFor = %+v", b)
	}
}

func TestDurationsPositive(t *testing.T) {
	if d := StructureDuration("metal_mine", 1, 0, 0, 1.0); d < 0 {
		t.Fatalf("structure duration = %v", d)
	}
	if d := TechDuration("energy_tech", 1, 0, 1.0); d < 0 {
		t.Fatalf("tech duration = %v", d)
	}
	if d := ShipDuration("202", 1, 0, 0, 1.0); d < 0 {
		t.Fatalf("ship duration = %v", d)
	}
}

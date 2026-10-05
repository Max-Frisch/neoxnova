package game

import "testing"

func TestRollPlanetRanges(t *testing.T) {
	for slot := 1; slot <= MaxPlanetSlots; slot++ {
		lo, hi, ok := PlanetFieldsRange(slot)
		if !ok {
			t.Fatalf("slot %d missing range", slot)
		}
		for seed := int64(0); seed < 100; seed++ {
			sp, ok := RollPlanet(seed, slot)
			if !ok {
				t.Fatalf("RollPlanet(slot %d) failed", slot)
			}
			if sp.FieldsMax < lo || sp.FieldsMax > hi {
				t.Fatalf("slot %d fields %d out of range [%d,%d]", slot, sp.FieldsMax, lo, hi)
			}
			if sp.TempMax < sp.TempMin {
				t.Fatalf("slot %d tempMin %d > tempMax %d", slot, sp.TempMin, sp.TempMax)
			}
			if sp.Diameter <= 0 {
				t.Fatalf("slot %d diameter %d", slot, sp.Diameter)
			}
		}
	}
	if _, ok := RollPlanet(1, MaxPlanetSlots+1); ok {
		t.Fatal("slot beyond max should fail")
	}
}

func TestRollPlanetDeterministic(t *testing.T) {
	a, _ := RollPlanet(FieldSlotSeed(2, 186, 9), 9)
	b, _ := RollPlanet(FieldSlotSeed(2, 186, 9), 9)
	if a != b {
		t.Fatalf("same seed rolled differently: %+v vs %+v", a, b)
	}
}

func TestSatelliteEnergySamples(t *testing.T) {
	// tempMax -> expected satellite energy, taken from the captured tooltips.
	cases := []struct{ temp, want int }{
		{305, 78}, {260, 70}, {80, 40}, {40, 33}, {-20, 23}, {-60, 17},
	}
	for _, c := range cases {
		if got := SatelliteEnergy(c.temp); got != c.want {
			t.Errorf("SatelliteEnergy(%d) = %d, want %d", c.temp, got, c.want)
		}
	}
}

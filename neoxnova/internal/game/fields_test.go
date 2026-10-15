package game

import "testing"

// TestFieldExpansionCostCaptured locks the price curve to the Planetarium
// capture (the private ops repo/): +1..+6 = 200/420/662/928/1221/1543 DM.
func TestFieldExpansionCostCaptured(t *testing.T) {
	cases := []struct {
		additional int
		want       int64
	}{
		{1, 200},
		{2, 420},
		{3, 662},
		{4, 928},
		{5, 1221},
		{6, 1543},
	}
	for _, c := range cases {
		if got := FieldExpansionCost(0, c.additional); got != c.want {
			t.Errorf("FieldExpansionCost(0, %d) = %d, want %d", c.additional, got, c.want)
		}
	}
}

func TestFieldExpansionCostEdgeCases(t *testing.T) {
	if got := FieldExpansionCost(0, 0); got != 0 {
		t.Errorf("zero fields should be free, got %d", got)
	}
	if got := FieldExpansionCost(0, -3); got != 0 {
		t.Errorf("negative fields should be free, got %d", got)
	}
	// Buying n fields in one go equals buying them one at a time (no arbitrage):
	// the escalating curve is anchored to fields already purchased.
	var stepwise int64
	for i := 0; i < 6; i++ {
		stepwise += FieldExpansionCost(i, 1)
	}
	if batch := FieldExpansionCost(0, 6); batch != stepwise {
		t.Errorf("batch %d != stepwise %d", batch, stepwise)
	}
	// Each successive field is more expensive than the last.
	if FieldExpansionCost(5, 1) <= FieldExpansionCost(4, 1) {
		t.Error("per-field price should increase monotonically")
	}
}

// TestDiameterExpansionGains locks the captured Stardust diameter-expansion
// ranges (diameter +276..+414 km, fields +12..+18) and their roll mapping.
func TestDiameterExpansionGains(t *testing.T) {
	if DiameterExpansionMetalDebris != 100_000_000_000 ||
		DiameterExpansionCrystalDebris != 50_000_000_000 ||
		DiameterExpansionStardust != 1 {
		t.Fatalf("unexpected diameter cost: metal %d crystal %d stardust %d",
			DiameterExpansionMetalDebris, DiameterExpansionCrystalDebris, DiameterExpansionStardust)
	}

	cases := []struct {
		roll       float64
		wantKm     int
		wantFields int
	}{
		{0, DiameterExpansionMinKm, DiameterExpansionMinFields},
		{0.999999, DiameterExpansionMaxKm, DiameterExpansionMaxFields},
		{1, DiameterExpansionMaxKm, DiameterExpansionMaxFields},  // clamped
		{-1, DiameterExpansionMinKm, DiameterExpansionMinFields}, // clamped
	}
	for _, c := range cases {
		km, f := DiameterExpansionGains(c.roll, c.roll)
		if km != c.wantKm || f != c.wantFields {
			t.Errorf("DiameterExpansionGains(%v) = (%d,%d), want (%d,%d)", c.roll, km, f, c.wantKm, c.wantFields)
		}
	}

	// Sweep many rolls and assert both gains always stay within their range.
	for i := 0; i < 1000; i++ {
		km, f := DiameterExpansionGains(float64(i)/1000.0, float64(1000-i)/1000.0)
		if km < DiameterExpansionMinKm || km > DiameterExpansionMaxKm {
			t.Fatalf("diameter gain %d outside [%d,%d]", km, DiameterExpansionMinKm, DiameterExpansionMaxKm)
		}
		if f < DiameterExpansionMinFields || f > DiameterExpansionMaxFields {
			t.Fatalf("field gain %d outside [%d,%d]", f, DiameterExpansionMinFields, DiameterExpansionMaxFields)
		}
	}
}

package game

import "testing"

// TestFieldExpansionCostCaptured locks the price curve to the Planetarium
// capture (docs/screenshots_niburu/): +1..+6 = 200/420/662/928/1221/1543 DM.
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

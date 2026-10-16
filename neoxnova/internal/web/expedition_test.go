package web

import "testing"

// TestExpeditionSliderMappings locks the panel's slider curves: exp_time 1-10
// maps to 0.25-2.5 h (in minutes) and exp_speed 1-10 maps to 10-100 %.
func TestExpeditionSliderMappings(t *testing.T) {
	timeCases := map[int]int{1: 15, 4: 60, 10: 150}
	for in, want := range timeCases {
		if got := expeditionMinutes(in); got != want {
			t.Fatalf("expeditionMinutes(%d) = %d, want %d", in, got, want)
		}
	}
	speedCases := map[int]int{1: 10, 5: 50, 10: 100}
	for in, want := range speedCases {
		if got := expeditionSpeed(in); got != want {
			t.Fatalf("expeditionSpeed(%d) = %d, want %d", in, got, want)
		}
	}
	if got := expeditionMinutes(0); got != 15 {
		t.Fatalf("expeditionMinutes(0) = %d, want clamped 15", got)
	}
	if got := expeditionSpeed(99); got != 100 {
		t.Fatalf("expeditionSpeed(99) = %d, want clamped 100", got)
	}
}

// TestExpeditionHoldLabel locks the panel's exp_time caption: it shows the
// nominal selection plus the real, fleet-speed-compressed hold (0.25 h per step).
func TestExpeditionHoldLabel(t *testing.T) {
	cases := []struct {
		expTime int
		speed   float64
		want    string
	}{
		{4, 15, "1.00 h · real 4m 0s"},  // 1 h nominal at 15x
		{4, 5, "1.00 h · real 12m 0s"},  // 1 h nominal at 5x
		{4, 1, "1.00 h · real 1h 0m"},   // no compression
		{1, 500000, "0.25 h · real 1s"}, // floored at 1s
	}
	for _, c := range cases {
		if got := expeditionHoldLabel(c.expTime, c.speed); got != c.want {
			t.Fatalf("expeditionHoldLabel(%d, %g) = %q, want %q", c.expTime, c.speed, got, c.want)
		}
	}
}

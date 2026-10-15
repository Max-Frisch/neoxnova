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

package game

import "testing"

func TestCoordinateDistance(t *testing.T) {
	if d := CalculateCoordinateDistance(1, 1, 1, 2, 1, 1); d != 20000 {
		t.Fatalf("cross-galaxy distance = %v, want 20000", d)
	}
	if d := CalculateCoordinateDistance(1, 1, 1, 1, 2, 1); d != 2795 {
		t.Fatalf("cross-system distance = %v, want 2795", d)
	}
	if d := CalculateCoordinateDistance(1, 1, 1, 1, 1, 3); d != 1010 {
		t.Fatalf("same-system distance = %v, want 1010", d)
	}
}

func TestFlightDurationScalesWithFleetSpeed(t *testing.T) {
	const dist = 2795.0
	slow := CalculateFlightDuration(dist, 10000, 100, 5)
	fast := CalculateFlightDuration(dist, 10000, 100, 15)
	if slow <= fast {
		t.Fatalf("5x (%d) should be slower than 15x (%d)", slow, fast)
	}
	// 15x is roughly a third of 5x (allow rounding/off-by-one).
	if want := slow / 3; fast < want-2 || fast > want+2 {
		t.Fatalf("15x duration = %d, want ~%d", fast, want)
	}
}

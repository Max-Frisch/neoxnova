package game

import (
	"math"
	"testing"
)

func TestCoordinateDistanceSamples(t *testing.T) {
	cases := []struct {
		g1, s1, p1, g2, s2, p2 int
		want                   float64
	}{
		{1, 1, 1, 1, 1, 3, 1010},      // same system
		{1, 1, 1, 1, 2, 1, 2795},      // cross-system, same position
		{2, 188, 9, 2, 188, 16, 1035}, // live: Xusyty -> Japoqu
		{2, 188, 9, 2, 186, 9, 2890},  // live
		{2, 188, 9, 2, 186, 11, 3900}, // live
		{2, 188, 9, 2, 191, 10, 3990}, // live
		{1, 1, 1, 2, 1, 1, 28100},     // cross-galaxy (standard)
	}
	for _, c := range cases {
		if got := CalculateCoordinateDistance(c.g1, c.s1, c.p1, c.g2, c.s2, c.p2); got != c.want {
			t.Errorf("distance(%d:%d:%d -> %d:%d:%d) = %v, want %v",
				c.g1, c.s1, c.p1, c.g2, c.s2, c.p2, got, c.want)
		}
	}
}

// TestFuelMatchesLiveSamples checks the fuel model against the eight samples
// captured from niburuspace.com (distances/speeds in the session doc). The fit
// is within ~10%; the alternative reference formula was ~3x off.
func TestFuelMatchesLiveSamples(t *testing.T) {
	cases := []struct {
		base  int64
		dist  float64
		speed int
		want  int64
	}{
		{20, 2985, 100, 7},          // 1 Light Cargo
		{20 * 100, 2985, 50, 362},   // 100 Light Cargo @ 50%
		{250 * 10, 3990, 100, 1076}, // 10 Battleships
		{20, 28685, 100, 63},        // 1 Light Cargo, cross-galaxy
		{250 * 300, 2890, 100, 23313},
		{320 * 100, 3895, 100, 14665}, // 100 Galleon
		{320 * 200, 2985, 100, 22475}, // 200 Galleon
	}
	for _, c := range cases {
		got := CalculateDeuteriumConsumption(c.base, c.dist, c.speed)
		relErr := math.Abs(float64(got-c.want)) / float64(c.want)
		if relErr > 0.12 {
			t.Errorf("fuel(base=%d dist=%v speed=%d) = %d, want ~%d (%.0f%% off)",
				c.base, c.dist, c.speed, got, c.want, relErr*100)
		}
	}
}

func TestFleetMaxSpeed(t *testing.T) {
	// Light Fighter (12500, combustion) at combustion 10 -> +100% -> 25000.
	if got := FleetMaxSpeed(map[string]int64{"204": 1}, 10, 0, 0); got != 25000 {
		t.Fatalf("LF combustion 10 = %d, want 25000", got)
	}
	// Battleship (10000, hyperspace) at hyperspace 5 -> 15000.
	if got := FleetMaxSpeed(map[string]int64{"207": 1}, 0, 0, 5); got != 15000 {
		t.Fatalf("BS hyperspace 5 = %d, want 15000", got)
	}
	// Mixed fleet is limited by the slowest ship.
	if got := FleetMaxSpeed(map[string]int64{"204": 1, "207": 1}, 10, 0, 0); got != 10000 {
		t.Fatalf("mixed fleet = %d, want 10000 (BS limits)", got)
	}
}

func TestFlightDurationScalesWithFleetSpeed(t *testing.T) {
	const dist = 2795.0
	slow := CalculateFlightDuration(dist, 10000, 100, 5)
	fast := CalculateFlightDuration(dist, 10000, 100, 15)
	if slow <= fast {
		t.Fatalf("5x (%d) should be slower than 15x (%d)", slow, fast)
	}
	// Duration is linear in 1/fleetSpeed, so 15x is ~a third of 5x.
	if want := slow / 3; fast < want-2 || fast > want+2 {
		t.Fatalf("15x duration = %d, want ~%d", fast, want)
	}
}

func TestFlightDurationSampleRoughlyMatches(t *testing.T) {
	// Live: 2:188:9 -> 2:188:16 (distance 1035) at 15x fleet speed ~ 5 minutes.
	d := CalculateFlightDuration(1035, 10000, 100, 15)
	if d < 240 || d > 420 {
		t.Fatalf("one-way duration = %ds, want ~300s", d)
	}
}

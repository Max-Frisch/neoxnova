package game

import "testing"

func TestStorageCapacity(t *testing.T) {
	cases := []struct {
		level int
		speed float64
		want  int64
	}{
		{0, 10000, 100_000_000},
		{10, 10000, 53_550_000_000},
		{0, 50000, 500_000_000},
		{1, 1, 20_000},
		{0, 0, 10_000},  // non-positive speed falls back to 1x
		{-5, 1, 10_000}, // negative level clamps to 0
	}
	for _, c := range cases {
		if got := StorageCapacity(c.level, c.speed); got != c.want {
			t.Errorf("StorageCapacity(%d, %v) = %d, want %d", c.level, c.speed, got, c.want)
		}
	}
	if StorageCapacity(11, 10000) <= StorageCapacity(10, 10000) {
		t.Fatal("capacity must grow with storage level")
	}
}

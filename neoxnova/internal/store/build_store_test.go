package store

import (
	"math"
	"testing"

	"neoxnova/internal/game"
)

func TestAffordableQty(t *testing.T) {
	cases := []struct {
		name                      string
		metal, crystal, deuterium float64
		per                       game.Cost
		want                      int64
	}{
		{"metal-bound", 10000, 1e9, 1e9, game.Cost{Metal: 3000, Crystal: 1000}, 3},
		{"crystal-bound", 1e9, 5000, 1e9, game.Cost{Metal: 3000, Crystal: 1000}, 5},
		{"deut-bound", 1e9, 1e9, 2500, game.Cost{Metal: 1, Crystal: 1, Deuterium: 1000}, 2},
		{"free-unit-unbounded", 10, 10, 10, game.Cost{}, math.MaxInt64},
		{"cannot-afford-one", 100, 100, 100, game.Cost{Metal: 5000}, 0},
		{"exact", 3000, 1000, 0, game.Cost{Metal: 3000, Crystal: 1000}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := affordableQty(tc.metal, tc.crystal, tc.deuterium, tc.per); got != tc.want {
				t.Fatalf("affordableQty = %d, want %d", got, tc.want)
			}
		})
	}
}

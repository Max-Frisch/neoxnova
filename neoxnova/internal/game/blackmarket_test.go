package game

import "testing"

func TestMerchantRefund(t *testing.T) {
	base := Cost{Metal: 2000, Crystal: 2000, Deuterium: 0}
	cases := []struct {
		name string
		rate int
		want Cost
	}{
		{"half", 50, Cost{Metal: 1000, Crystal: 1000}},
		{"full", 100, Cost{Metal: 2000, Crystal: 2000}},
		{"none", 0, Cost{}},
		{"negative", -10, Cost{}},
		{"clamped", 250, Cost{Metal: 2000, Crystal: 2000}},
		{"floor", 33, Cost{Metal: 660, Crystal: 660}},
	}
	for _, tc := range cases {
		if got := MerchantRefund(base, tc.rate); got != tc.want {
			t.Errorf("%s: MerchantRefund(rate=%d) = %+v, want %+v", tc.name, tc.rate, got, tc.want)
		}
	}
}

func TestMerchantRefundDeuterium(t *testing.T) {
	base := Cost{Metal: 100, Crystal: 50, Deuterium: 25}
	if got := MerchantRefund(base, 50); got != (Cost{Metal: 50, Crystal: 25, Deuterium: 12}) {
		t.Errorf("MerchantRefund = %+v, want {50 25 12}", got)
	}
}

func TestDMMarketPrice(t *testing.T) {
	cases := []struct {
		name    string
		cost    Cost
		divisor int64
		want    int64
	}{
		{"exact", Cost{Metal: 1000, Crystal: 1000, Deuterium: 1000}, 1000, 3},
		{"roundsUp", Cost{Metal: 1000, Crystal: 1}, 1000, 2},
		{"zero", Cost{}, 1000, 0},
		{"zeroDivisor", Cost{Metal: 5}, 0, 5},
		{"negativeDivisor", Cost{Metal: 5}, -1, 5},
	}
	for _, tc := range cases {
		if got := DMMarketPrice(tc.cost, tc.divisor); got != tc.want {
			t.Errorf("%s: DMMarketPrice(%+v, %d) = %d, want %d", tc.name, tc.cost, tc.divisor, got, tc.want)
		}
	}
}

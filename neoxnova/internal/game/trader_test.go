package game

import (
	"errors"
	"testing"
)

// TestTradeQuoteRatio locks the captured M:C:D = 1:2:4 ratio and the fixed Dark
// Matter fee: selling crystal for deuterium costs 2 crystal per deuterium.
func TestTradeQuoteRatio(t *testing.T) {
	q, err := TradeQuote(ResourceCrystal, ResourceDeuterium, 100_000_000_000, 1000)
	if err != nil {
		t.Fatalf("TradeQuote: %v", err)
	}
	if q.SellCost != 200_000_000_000 {
		t.Errorf("SellCost = %d, want 200,000,000,000", q.SellCost)
	}
	if q.DarkMatter != TradeDarkMatterCost {
		t.Errorf("DarkMatter = %d, want %d", q.DarkMatter, TradeDarkMatterCost)
	}
}

// TestTradeQuoteDirection covers all six sell/buy pairs, including the rate
// direction (buying a cheap resource costs less than the sold one).
func TestTradeQuoteDirection(t *testing.T) {
	cases := []struct {
		sell, buy int
		amount    int64
		wantCost  int64
	}{
		{ResourceMetal, ResourceCrystal, 100, 200},     // crystal worth 2 metal
		{ResourceMetal, ResourceDeuterium, 100, 400},   // deut worth 4 metal
		{ResourceCrystal, ResourceMetal, 100, 50},      // metal worth 1/2 crystal
		{ResourceCrystal, ResourceDeuterium, 100, 200}, // deut worth 2 crystal
		{ResourceDeuterium, ResourceMetal, 100, 25},    // metal worth 1/4 deut
		{ResourceDeuterium, ResourceCrystal, 100, 50},  // crystal worth 1/2 deut
	}
	for _, c := range cases {
		q, err := TradeQuote(c.sell, c.buy, c.amount, TradeDarkMatterCost)
		if err != nil {
			t.Fatalf("TradeQuote(%d,%d): %v", c.sell, c.buy, err)
		}
		if q.SellCost != c.wantCost {
			t.Errorf("sell %d buy %d: cost = %d, want %d", c.sell, c.buy, q.SellCost, c.wantCost)
		}
	}
}

// TestTradeQuoteRoundsUp checks the cost always covers a whole unit of the sold
// resource (ceil), never a fractional amount.
func TestTradeQuoteRoundsUp(t *testing.T) {
	// 1 metal costs 1/4 deuterium => ceil to 1.
	q, err := TradeQuote(ResourceDeuterium, ResourceMetal, 1, TradeDarkMatterCost)
	if err != nil {
		t.Fatalf("TradeQuote: %v", err)
	}
	if q.SellCost != 1 {
		t.Errorf("SellCost = %d, want 1 (rounded up)", q.SellCost)
	}
	// 5 metal costs 1.25 deuterium => ceil to 2.
	q, err = TradeQuote(ResourceDeuterium, ResourceMetal, 5, TradeDarkMatterCost)
	if err != nil {
		t.Fatalf("TradeQuote: %v", err)
	}
	if q.SellCost != 2 {
		t.Errorf("SellCost = %d, want 2 (rounded up)", q.SellCost)
	}
}

// TestTradeQuoteRejections covers the invalid inputs and the fee check.
func TestTradeQuoteRejections(t *testing.T) {
	if _, err := TradeQuote(999, ResourceCrystal, 1, 1000); !errors.Is(err, ErrBadTrade) {
		t.Errorf("unknown sell = %v, want ErrBadTrade", err)
	}
	if _, err := TradeQuote(ResourceMetal, 999, 1, 1000); !errors.Is(err, ErrBadTrade) {
		t.Errorf("unknown buy = %v, want ErrBadTrade", err)
	}
	if _, err := TradeQuote(ResourceMetal, ResourceMetal, 1, 1000); !errors.Is(err, ErrBadTrade) {
		t.Errorf("same resource = %v, want ErrBadTrade", err)
	}
	if _, err := TradeQuote(ResourceMetal, ResourceCrystal, 0, 1000); !errors.Is(err, ErrBadTrade) {
		t.Errorf("zero amount = %v, want ErrBadTrade", err)
	}
	if _, err := TradeQuote(ResourceMetal, ResourceCrystal, -5, 1000); !errors.Is(err, ErrBadTrade) {
		t.Errorf("negative amount = %v, want ErrBadTrade", err)
	}
	if _, err := TradeQuote(ResourceMetal, ResourceCrystal, 1, TradeDarkMatterCost-1); !errors.Is(err, ErrInsufficientDarkMatter) {
		t.Errorf("broke dm = %v, want ErrInsufficientDarkMatter", err)
	}
}

// TestTradeResourceName keeps the ids mapping to the live labels.
func TestTradeResourceName(t *testing.T) {
	cases := map[int]string{
		ResourceMetal: "Metal", ResourceCrystal: "Crystal",
		ResourceDeuterium: "Deuterium", ResourceDarkMatter: "Dark Matter", 999: "",
	}
	for code, want := range cases {
		if got := TradeResourceName(code); got != want {
			t.Errorf("TradeResourceName(%d) = %q, want %q", code, got, want)
		}
	}
}

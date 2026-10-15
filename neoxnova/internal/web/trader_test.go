package web

import (
	"strings"
	"testing"

	"neoxnova/internal/game"
)

// TestTraderBodyRenders locks the Resource Trader page: the live 4/2/1 rate
// copy, the balance row and the sell/buy/amount form with the htmx preview.
func TestTraderBodyRenders(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := TraderData{
		Resources: []TraderResource{
			{game.ResourceMetal, "Metal", 1_000_000},
			{game.ResourceCrystal, "Crystal", 2_000_000},
			{game.ResourceDeuterium, "Deuterium", 3_000_000},
		},
		DarkMatter: 5_000,
		Fee:        game.TradeDarkMatterCost,
	}
	html := renderComponent(t, TraderBody(sh, d))

	for _, want := range []string{
		"Resource Trader",
		"4/2/1",
		"250",
		`action="/blackmarket/trader"`,
		`name="sell"`,
		`name="buy"`,
		`name="amount"`,
		`name="return" value="/blackmarket/trader"`,
		`hx-post="/blackmarket/trader/preview"`,
		`id="trader-quote"`,
		"Metal",
		"Crystal",
		"Deuterium",
		"Dark Matter",
		"Back to Black market",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("trader HTML missing %q", want)
		}
	}
}

// TestTraderQuotePartial covers the preview fragment for a valid quote and for
// a rejected one.
func TestTraderQuotePartial(t *testing.T) {
	offer, err := game.TradeQuote(game.ResourceCrystal, game.ResourceDeuterium, 100, game.TradeDarkMatterCost)
	if err != nil {
		t.Fatalf("TradeQuote: %v", err)
	}
	html := renderComponent(t, TraderQuotePartial(&offer, ""))
	for _, want := range []string{"Cost", "200", "Crystal", "250", "Dark Matter", "receive", "100", "Deuterium"} {
		if !strings.Contains(html, want) {
			t.Errorf("quote partial missing %q", want)
		}
	}

	html = renderComponent(t, TraderQuotePartial(nil, game.ErrBadTrade.Error()))
	if !strings.Contains(html, "invalid trade") || !strings.Contains(html, "trader-quote--err") {
		t.Errorf("error partial = %q, want the error message and error class", html)
	}
}

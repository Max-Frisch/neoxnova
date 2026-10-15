package game

import "errors"

// Resource Trader (Black market -> Resource Trader). Rules captured from
// captures/Resource_Trader_Crys_for_Deut_example.har (the private ops repo,
// 2026-10-14): the account sold 100,000,000,000 deuterium-dealing crystal for
// deuterium. Crystal fell by exactly 200,000,000,000 and deuterium rose by
// 100,000,000,000, so the merchant values the resources M:C:D = 1:2:4 (the
// live 4/2/1 amount ratio). The form charges a per-resource fee on the bought
// side: selling crystal, Metal costs 0.5 and Deuterium 2 crystal per unit
// (trader.js `ress = metal*0.5 + deuterium*2`). The page states "The cost of
// the merchant is 250 Dark Matter!" for every trade.
const (
	// ResourceMetal/Crystal/Deuterium/DarkMatter are the live client's resource
	// ids (`resource=`/`trade[<id>]=`).
	ResourceMetal      = 901
	ResourceCrystal    = 902
	ResourceDeuterium  = 903
	ResourceDarkMatter = 921
)

// tradeValue is each resource's value in the captured M:C:D = 1:2:4 ratio: one
// deuterium buys two crystal or four metal.
var tradeValue = map[int]int64{
	ResourceMetal:     1,
	ResourceCrystal:   2,
	ResourceDeuterium: 4,
}

// TradeDarkMatterCost is the fixed Dark Matter fee the merchant charges per
// trade, captured verbatim from the live Resource Trader page.
const TradeDarkMatterCost int64 = 250

// ErrBadTrade is returned for an unknown resource, a same-resource trade or a
// non-positive amount.
var ErrBadTrade = errors.New("invalid trade")

// ErrInsufficientDarkMatter is returned when the player cannot afford the fixed
// merchant fee.
var ErrInsufficientDarkMatter = errors.New("insufficient dark matter")

// TradeOffer is the price of buying BuyAmount of BuyCode by selling SellCode:
// the amount of the sold resource and the fixed Dark Matter fee it costs.
type TradeOffer struct {
	SellCode   int
	BuyCode    int
	BuyAmount  int64
	SellCost   int64
	DarkMatter int64
}

// TradeQuote prices selling sellCode to buy buyAmount of buyCode. dm is the
// player's current Dark Matter balance, used to reject a quote whose fixed
// merchant fee cannot be paid. The sold cost rounds up to a whole unit.
func TradeQuote(sellCode, buyCode int, buyAmount, dm int64) (TradeOffer, error) {
	sellValue, ok := tradeValue[sellCode]
	if !ok {
		return TradeOffer{}, ErrBadTrade
	}
	buyValue, ok := tradeValue[buyCode]
	if !ok || buyCode == sellCode {
		return TradeOffer{}, ErrBadTrade
	}
	if buyAmount <= 0 {
		return TradeOffer{}, ErrBadTrade
	}
	if dm < TradeDarkMatterCost {
		return TradeOffer{}, ErrInsufficientDarkMatter
	}
	sellCost := (buyAmount*buyValue + sellValue - 1) / sellValue
	return TradeOffer{
		SellCode:   sellCode,
		BuyCode:    buyCode,
		BuyAmount:  buyAmount,
		SellCost:   sellCost,
		DarkMatter: TradeDarkMatterCost,
	}, nil
}

// TradeResourceName resolves a resource id to its display name.
func TradeResourceName(code int) string {
	switch code {
	case ResourceMetal:
		return "Metal"
	case ResourceCrystal:
		return "Crystal"
	case ResourceDeuterium:
		return "Deuterium"
	case ResourceDarkMatter:
		return "Dark Matter"
	}
	return ""
}

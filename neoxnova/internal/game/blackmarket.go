package game

// Black market economy math (B4). Two knobs live on the universes row and are
// upserted from config/game.json on boot:
//
//   - Merchant of Fleets (page=FleetDealer) scraps a unit batch and pays back
//     merchant_rate percent of its base cost — the live page labels this the
//     "Merchant rate: 50%" cut.
//   - Buildings/Technology markets (page=dmbuild/dmtech) build or research one
//     instant level for Dark Matter priced from the level's resource cost.

// MerchantRefund is the resources the Merchant of Fleets pays for scrapping a
// batch priced at base: ratePercent percent of each resource, floored to whole
// units. A rate <= 0 refunds nothing; a rate >= 100 refunds the full base cost.
func MerchantRefund(base Cost, ratePercent int) Cost {
	if ratePercent <= 0 {
		return Cost{}
	}
	if ratePercent > 100 {
		ratePercent = 100
	}
	r := int64(ratePercent)
	return Cost{
		Metal:     base.Metal * r / 100,
		Crystal:   base.Crystal * r / 100,
		Deuterium: base.Deuterium * r / 100,
	}
}

// DMMarketPrice is the Dark Matter an instant level costs in the Buildings /
// Technology markets: one Dark Matter per divisor resource points of the
// level's cost, rounded up. A non-positive divisor falls back to 1 resource per
// Dark Matter so a misconfigured universe cannot divide by zero.
func DMMarketPrice(cost Cost, divisor int64) int64 {
	if divisor <= 0 {
		divisor = 1
	}
	total := cost.Metal + cost.Crystal + cost.Deuterium
	if total <= 0 {
		return 0
	}
	return (total + divisor - 1) / divisor
}

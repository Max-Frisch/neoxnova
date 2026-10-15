package web

import (
	"strings"
	"testing"

	"neoxnova/internal/game"
)

// TestMerchantBodyRenders locks the Merchant of Fleets page: the unit/number
// form posts to its route, offers the htmx preview and names the merchant rate.
func TestMerchantBodyRenders(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := MerchantData{
		RatePercent: 50,
		DarkMatter:  1234,
		Units: []MerchantUnit{
			{Code: "202", Name: "Light Cargo", Image: assetURL("202"), Cost: game.Cost{Metal: 2000, Crystal: 2000}, Available: 10},
		},
	}
	html := renderComponent(t, MerchantBody(sh, d))
	for _, want := range []string{
		"Merchant of Fleets",
		`action="/blackmarket/merchant"`,
		`hx-post="/blackmarket/merchant/preview"`,
		`name="unit"`,
		`name="number"`,
		"Light Cargo",
		"50%",
		`href="/blackmarket"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("merchant HTML missing %q", want)
		}
	}
}

// TestMerchantQuotePartial renders the htmx fragment for both the priced and the
// error state.
func TestMerchantQuotePartial(t *testing.T) {
	refund := game.Cost{Metal: 1000, Crystal: 1000}
	html := renderComponent(t, MerchantQuotePartial(&refund, "Light Cargo", 2, 50, ""))
	for _, want := range []string{"Scrap", "Light Cargo", "1,000", "50%"} {
		if !strings.Contains(html, want) {
			t.Errorf("quote HTML missing %q", want)
		}
	}
	errHTML := renderComponent(t, MerchantQuotePartial(nil, "", 0, 0, "Unknown unit"))
	if !strings.Contains(errHTML, "trader-quote--err") || !strings.Contains(errHTML, "Unknown unit") {
		t.Errorf("error quote should render the error style + message: %q", errHTML)
	}
}

// TestDMMarketBodyRenders locks the shared DM market layout: priced buttons for
// unlocked items and a disabled, reason-labelled tile for locked ones.
func TestDMMarketBodyRenders(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := DMMarketData{
		Title:      "Buildings Market",
		Subtitle:   "Build buildings instantly with Dark Matter.",
		Action:     "/blackmarket/buildings",
		Divisor:    1000,
		DarkMatter: 5000,
		Items: []DMMarketItem{
			{Code: "metal_mine", Name: "Metal Mine", Image: assetURL("metal_mine"), Level: 0, NextLevel: 1, Cost: game.Cost{Metal: 60, Crystal: 15}, DarkMatter: 1, Unlocked: true},
			{Code: "nanite_factory", Name: "Nanite Factory", Image: assetURL("nanite_factory"), Level: 0, NextLevel: 1, DarkMatter: 1600, Unlocked: false, Reason: "Requires Robot Factory 10"},
		},
	}
	html := renderComponent(t, DMMarketBody(sh, d))
	for _, want := range []string{
		"Buildings Market",
		`action="/blackmarket/buildings"`,
		`name="code" value="metal_mine"`,
		"Metal Mine",
		"Nanite Factory",
		"DM",
		"is-disabled",
		"Requires Robot Factory 10",
		`href="/blackmarket"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("DM market HTML missing %q", want)
		}
	}
}

// TestVoucherBodyRenders locks the Voucher System placeholder: the captured
// layout renders but redeeming is explicitly not available yet.
func TestVoucherBodyRenders(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	html := renderComponent(t, VoucherBody(sh))
	for _, want := range []string{
		"Voucher System",
		"Voucher code",
		"Redeem",
		"coming soon",
		`href="/blackmarket"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("voucher HTML missing %q", want)
		}
	}
}

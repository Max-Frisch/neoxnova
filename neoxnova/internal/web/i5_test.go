package web

import (
	"io/fs"
	"strings"
	"testing"
	"time"

	"neoxnova/internal/models"
)

// TestArsenalBodyTabs locks the I5 Arsenal tabs: the three tab headers render,
// the active tab is highlighted and each links back with its ?tab= key.
func TestArsenalBodyTabs(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := ArsenalData{Active: "arsenal", Tabs: arsenalTabs, Items: []ArsenalItem{
		{Code: 1, Name: "Laser weapons", Group: "weapon", Image: upgradeImage(1),
			Level: 0, Bonus: 0, NextBonus: 0.75, Available: 2},
	}}
	html := renderComponent(t, ArsenalBody(sh, d))

	for _, want := range []string{
		"Arsenal",
		"Market",
		"Sell Upgrades",
		`href="/arsenal?tab=market"`,
		`href="/arsenal?tab=sell"`,
		`class="tab is-active"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("arsenal HTML missing %q", want)
		}
	}
	if c := strings.Count(html, "tab is-active"); c != 1 {
		t.Errorf("active tab count = %d, want 1", c)
	}
}

// TestArsenalGridRendersUpgradeBox locks the Arsenal listing boxes: icon,
// current bonus with the next-activation bracket, held drawings, the Activate
// form when a drawing is held and "Absent" otherwise.
func TestArsenalGridRendersUpgradeBox(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := ArsenalData{Active: "arsenal", Tabs: arsenalTabs, Items: []ArsenalItem{
		{Code: 1, Name: "Laser weapons", Image: upgradeImage(1), Bonus: 0, NextBonus: 0.75, Available: 2},
		{Code: 5, Name: "Light armor", Image: upgradeImage(5), Bonus: 0, NextBonus: 0.6, Available: 0},
	}}
	html := renderComponent(t, ArsenalBody(sh, d))

	for _, want := range []string{
		"Laser weapons",
		"Light armor",
		`/static/niburu/icons/laser.jpg`,
		`/static/niburu/icons/d_light.jpg`,
		"Bonus:",
		"(+0.75)",
		`Available: <b>2</b>`,
		`action="/arsenal/activate"`,
		`name="code" value="1"`,
		"upgrade-absent",
		"Absent",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("arsenal grid HTML missing %q", want)
		}
	}
}

// TestArsenalSellTab locks the Sell Upgrades tab: the listing form and the
// caller's live auctions with a withdraw action.
func TestArsenalSellTab(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := ArsenalData{
		Active:   "sell",
		Tabs:     arsenalTabs,
		Listable: []ArsenalItem{{Code: 1, Name: "Laser weapons", Available: 3}},
		Mine: []models.MarketLot{
			{ID: 4, Upgrade: "Laser weapons", Amount: 1, TotalPriceAtm: 50, ExpiresAt: time.Now().Add(time.Hour)},
		},
	}
	html := renderComponent(t, ArsenalBody(sh, d))

	for _, want := range []string{
		"Sell Upgrades",
		`action="/market/list"`,
		"Laser weapons (3 held)",
		"Your auctions",
		`action="/market/remove"`,
		`name="lot_id" value="4"`,
		"Withdraw",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("sell tab HTML missing %q", want)
		}
	}
}

// TestArsenalMarketTab locks the Market tab: the live lots table with the buy
// action.
func TestArsenalMarketTab(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := ArsenalData{Active: "market", Tabs: arsenalTabs, Lots: []models.MarketLot{
		{ID: 9, Upgrade: "Jet engine", Amount: 2, TotalPriceAtm: 100, ExpiresAt: time.Now().Add(time.Hour)},
	}}
	html := renderComponent(t, ArsenalBody(sh, d))

	for _, want := range []string{
		"Total price",
		"Jet engine",
		`action="/market/buy"`,
		`name="lot_id" value="9"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("market tab HTML missing %q", want)
		}
	}
}

// TestMarketBodyReskin locks the restyled Black market page onto the panel and
// game-table classes while keeping the existing routes.
func TestMarketBodyReskin(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := MarketData{
		Listable: []ArsenalItem{{Code: 1, Name: "Laser weapons", Available: 1}},
		Mine:     []models.MarketLot{{ID: 2, Upgrade: "Laser weapons", Amount: 1, TotalPriceAtm: 30, ExpiresAt: time.Now().Add(time.Hour)}},
		Lots:     []models.MarketLot{{ID: 3, Upgrade: "Ion cannon", Amount: 2, TotalPriceAtm: 80, ExpiresAt: time.Now().Add(time.Hour)}},
	}
	html := renderComponent(t, MarketBody(sh, d))

	for _, want := range []string{
		"Black market",
		`class="panel market-panel"`,
		"game-table",
		`action="/market/list"`,
		`action="/market/buy"`,
		`action="/market/remove"`,
		"Ion cannon",
		"btn-warn",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("black market HTML missing %q", want)
		}
	}
}

// TestMessagesBodyGrid locks the I5 message category grid: all twelve live
// cells render, the backed ones carry real counts and links, the unbacked ones
// are disabled, and the report sections follow.
func TestMessagesBodyGrid(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := ReportsData{
		Active: "all",
		Categories: []MessageCategory{
			{Key: "spy", Label: "Spy messages", Color: "msg-spy", Count: 1, Href: "/reports/7?category=spy", Backed: true},
			{Key: "player", Label: "Player messages", Color: "msg-player"},
			{Key: "alliance", Label: "Alliance messages", Color: "msg-alliance"},
			{Key: "combat", Label: "Combat messages", Color: "msg-combat", Count: 2, Href: "/reports/7?category=combat", Backed: true},
			{Key: "system", Label: "System messages", Color: "msg-system"},
			{Key: "transport", Label: "Transport messages", Color: "msg-transport"},
			{Key: "expedition", Label: "Expedition messages", Color: "msg-expedition", Count: 3, Href: "/reports/7?category=expedition", Backed: true},
			{Key: "game", Label: "Game messages", Color: "msg-game"},
			{Key: "construction", Label: "Construction messages", Color: "msg-construction"},
			{Key: "all", Label: "All messages", Color: "msg-all", Count: 6, Href: "/reports/7?category=all", Backed: true},
			{Key: "archive", Label: "Archive", Color: "msg-archive"},
			{Key: "outbox", Label: "Outbox", Color: "msg-outbox"},
		},
	}
	html := renderComponent(t, MessagesBody(sh, d))

	for _, want := range []string{
		"Messages",
		"Spy messages", "Player messages", "Alliance messages", "Combat messages",
		"System messages", "Transport messages", "Expedition messages", "Game messages",
		"Construction messages", "All messages", "Archive", "Outbox",
		`href="/reports/7?category=spy"`,
		`href="/reports/7?category=combat"`,
		`href="/reports/7?category=expedition"`,
		`href="/reports/7?category=all"`,
		`>0/1<`,
		`>0/6<`,
		"is-disabled",
		"Combat reports",
		"Espionage reports",
		"Expedition reports",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("messages HTML missing %q", want)
		}
	}
}

// TestMessageCategoryValidation keeps the ?category= key inside the backed set.
func TestMessageCategoryValidation(t *testing.T) {
	cases := map[string]string{
		"":       "all",
		"spy":    "spy",
		"combat": "combat",
		"player": "all", // unbacked categories fall back to All
		"nope":   "all",
	}
	for in, want := range cases {
		if got := messageCategory(in); got != want {
			t.Errorf("messageCategory(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestMessageCategoryGrid wires real counts/links only into backed cells.
func TestMessageCategoryGrid(t *testing.T) {
	cats := messageCategoryGrid("/reports/7?category=", map[string]int{"spy": 1, "combat": 2, "expedition": 3, "all": 6})
	byKey := map[string]MessageCategory{}
	for _, c := range cats {
		byKey[c.Key] = c
	}
	if len(cats) != 12 {
		t.Fatalf("category count = %d, want 12", len(cats))
	}
	if c := byKey["spy"]; !c.Backed || c.Count != 1 || c.Href != "/reports/7?category=spy" {
		t.Errorf("spy cell = %+v", c)
	}
	if c := byKey["all"]; !c.Backed || c.Count != 6 {
		t.Errorf("all cell = %+v", c)
	}
	if c := byKey["player"]; c.Backed || c.Count != 0 || c.Href != "" {
		t.Errorf("player cell should be an unbacked placeholder: %+v", c)
	}
}

// TestRedirectBase guards the market `return` field against open redirects.
func TestRedirectBase(t *testing.T) {
	cases := map[string]string{
		"":                     "/market",
		"/arsenal?tab=sell":    "/arsenal?tab=sell",
		"//evil.example/x":     "/market",
		"https://evil.example": "/market",
		"javascript:alert(1)":  "/market",
	}
	for in, want := range cases {
		if got := redirectBase(in, "/market"); got != want {
			t.Errorf("redirectBase(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestUpgradeImagesComplete guarantees every catalog upgrade has a captured
// icon on disk, so the Arsenal grid never renders a gap.
func TestUpgradeImagesComplete(t *testing.T) {
	sub, err := fs.Sub(staticFiles, "static/niburu/icons")
	if err != nil {
		t.Fatalf("sub static/niburu/icons: %v", err)
	}
	for code := 1; code <= 19; code++ {
		url := upgradeImage(code)
		if url == "" {
			t.Errorf("upgrade #%d has no icon", code)
			continue
		}
		if _, err := fs.Stat(sub, strings.TrimPrefix(url, "/static/niburu/icons/")); err != nil {
			t.Errorf("missing upgrade icon for #%d: %s", code, url)
		}
	}
}

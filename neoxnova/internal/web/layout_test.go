package web

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"

	"neoxnova/internal/models"
)

// TestSkinAssetsServed verifies the I1 hand-authored shell assets are embedded
// and reachable through the /static/ file server.
func TestSkinAssetsServed(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{}).Mount(mux)
	for _, path := range []string{"/static/skin.css", "/static/app.js"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, rec.Code)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("GET %s returned empty body", path)
		}
	}
}

// renderComponent renders a templ component to a string for shell assertions.
func renderComponent(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

// TestShellRendersReskin locks the I1 shell contract: the top bar, the full
// left menu (backed links + disabled "coming soon" entries) and the resource
// bar including Dark Matter / Antimatter / Stardust all render.
func TestShellRendersReskin(t *testing.T) {
	sh := Shell{
		Title:            "Overview",
		Username:         "commander",
		ActivePlanetID:   7,
		ActivePlanetName: "Homeworld",
		ActiveCoords:     "3:125:12",
		ServerTime:       time.Date(2026, 10, 15, 6, 54, 21, 0, time.UTC),
		Planets:          []models.OwnedCelestial{{ID: 7, Name: "Homeworld", Galaxy: 3, System: 125, Position: 12}},
		Resources: &models.RealtimeResourceState{
			MetalCurrent:    500,
			MetalLimit:      1000,
			MetalHourlyRate: 12,
			CrystalCurrent:  250,
			CrystalLimit:    1000,
			DeutCurrent:     100,
			DeutLimit:       500,
			EnergyAvailable: 420,
			EnergyMax:       900,
			Stardust:        3,
			DarkMatter:      6478,
			Antimatter:      10355,
		},
	}
	html := renderComponent(t, Page(sh, templ.NopComponent))

	for _, want := range []string{
		"<title>Overview · Neo-XNova</title>",
		"id=\"server-clock\"",
		"15 Thu Oct 06:54:21",
		"/static/skin.css",
		"/static/app.js",
		"id=\"planet-select\"",
		"data-pct=\"50\"",
		"Metal",
		"Crystal",
		"Deuterium",
		"Energy",
		"Stardust",
		"Dark Matter",
		"Antimatter",
		"6,478",
		"10,355",
		"/overview/7",
		"/buildings/7",
		"/research/7",
		"/shipyard/7",
		"/fleet/7",
		"/technologies",
		"/market",
		"/galaxy",
		"/simulator",
		"is-disabled",
		"Coming soon",
		"Black market",
		"Online Bonus",
		"Antimatter Store",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("shell HTML missing %q", want)
		}
	}
}

// TestMenuActiveAliases covers the label→page-title matching used to highlight
// the current menu entry, including the Hangar/Black-market aliases.
func TestMenuActiveAliases(t *testing.T) {
	cases := []struct {
		title, label string
		want         bool
	}{
		{"Overview", "Overview", true},
		{"Shipyard", "Hangar", true},
		{"Market", "Black market", true},
		{"Galaxy 3:125", "Galaxy", true},
		{"Buildings", "Research", false},
	}
	for _, c := range cases {
		if got := menuActive(Shell{Title: c.title}, c.label); got != c.want {
			t.Errorf("menuActive(%q, %q) = %v, want %v", c.title, c.label, got, c.want)
		}
	}
}

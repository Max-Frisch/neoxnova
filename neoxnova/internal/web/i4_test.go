package web

import (
	"strconv"
	"strings"
	"testing"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
)

// TestTechnologiesBodyTabs locks the I4 technologies tabs: the five captured
// tab headers render, the active tab is highlighted, and each tab links back to
// the page with its ?tab= key.
func TestTechnologiesBodyTabs(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := TechnologiesData{
		Active: "research",
		Tabs:   techTabs,
		Groups: []TechGroup{{Name: "Research", Nodes: []TechNode{
			{Code: "energy_tech", Name: "Energy Technology", Level: 12, Leveled: true, Unlocked: true,
				Image:    assetURL("energy_tech"),
				Requires: []TechRequirement{{Name: "Research Lab", Have: 14, Need: 1, Met: true}}},
		}}},
	}
	html := renderComponent(t, TechnologiesBody(sh, d))

	for _, want := range []string{
		"Buildings",
		"Research",
		"Fleet",
		"Planetary Defense",
		"Requirements",
		`href="/technologies?tab=buildings"`,
		`href="/technologies?tab=requirements"`,
		`class="tab is-active"`,
		`/static/niburu/research/energy_tech.gif`,
		"Energy Technology",
		"Level 12",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("technologies HTML missing %q", want)
		}
	}
	if c := strings.Count(html, "tab is-active"); c != 1 {
		t.Errorf("active tab count = %d, want 1", c)
	}
}

// TestResourcesBodyBreakdown locks the I4 resources table: the per-source rows,
// the account bonus, storage capacity and the reconciled total/daily/weekly
// rows all render with the right labels.
func TestResourcesBodyBreakdown(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	bd := game.ProductionBreakdown(map[string]int{
		"metal_mine": 10, "crystal_mine": 8, "deuterium_synthesizer": 6, "solar_plant": 12,
	}, 40, 1.0, 0, game.ProductionBonus{Metal: 10}, true)

	html := renderComponent(t, ResourcesBody(sh, ResourcesData{Planet: "Homeworld", Breakdown: bd}))

	for _, want := range []string{
		`Resources produced on the planet "Homeworld"`,
		"Basic Production",
		"Metal Mine (Level 10)",
		"Crystal Mine (Level 8)",
		"Deuterium Refinery (Level 6)",
		"Solar Power plant (Level 12)",
		"Bonus (Officers/Bonus)",
		"Storage capacity",
		"Total",
		"Daily Production",
		"Weekly Production",
		`class="prod-pct"`,
		"res-metal",
		"res-energy",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("resources HTML missing %q", want)
		}
	}
}

// TestGalaxyBodyReskin locks the I4 galaxy contract: the galaxy/system
// steppers, the slot table with moon/debris/player columns, the fleet +
// espionage action links (mission preselected) and the footer stats.
func TestGalaxyBodyReskin(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	slots := make([]models.GalaxySlot, 21)
	for i := range slots {
		slots[i] = models.GalaxySlot{Position: i + 1}
	}
	slots[7] = models.GalaxySlot{Position: 8, Occupied: true, Type: models.TypePlanet, OwnerID: 9, OwnerName: "grenade"}
	slots[8] = models.GalaxySlot{Position: 9, Occupied: true, Type: models.TypePlanet, OwnerID: 10, OwnerName: "mirek", HasMoon: true}
	slots[9] = models.GalaxySlot{Position: 10, DebrisMetal: 5000, DebrisCrystal: 1200}
	slots[19] = models.GalaxySlot{Position: 20, Occupied: true, Type: models.TypePlanet, OwnerID: 7, OwnerName: "Homeworld", IsOwn: true}

	d := GalaxyData{
		Scan: models.GalaxyScanResponse{Galaxy: 2, System: 173, Slots: slots},
		Stats: GalaxyStats{
			ExistingPlanets: 3, FleetSlotsActive: 0, FleetSlotsTotal: 14,
			SpyProbes: 8, Recycler: 2,
		},
	}
	html := renderComponent(t, GalaxyBody(sh, d))

	for _, want := range []string{
		`href="/galaxy/1/173"`, // prev galaxy
		`href="/galaxy/2/174"`, // next system
		`class="galaxy-coord">173<`,
		"Saved",
		"View",
		"Planet Name (Activity)",
		"Player (State)",
		"Alliance",
		"/static/niburu/icons/moon.png",
		"grenade",
		"(Homeworld)",
		"mission=ESPIONAGE",
		"mission=ATTACK",
		"mission=RECYCLE",
		"Deep area of galaxy",
		"Existing planets",
		"Fleet Slots",
		"0/14",
		"Spy Probes",
		"Giga Recycler",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("galaxy HTML missing %q", want)
		}
	}
}

// TestGalaxyURLCamp keeps the stepper arrows inside the coordinate domains.
func TestGalaxyURLCamp(t *testing.T) {
	cases := []struct {
		g, s           int
		wantG, wantSys int
	}{
		{0, 0, 1, 1},
		{10, 600, 9, 499},
		{2, 173, 2, 173},
	}
	for _, c := range cases {
		got := galaxyURL(c.g, c.s)
		want := "/galaxy/" + strconv.Itoa(c.wantG) + "/" + strconv.Itoa(c.wantSys)
		if got != want {
			t.Errorf("galaxyURL(%d,%d) = %q, want %q", c.g, c.s, got, want)
		}
	}
}

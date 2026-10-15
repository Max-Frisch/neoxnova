package web

import (
	"strings"
	"testing"
	"time"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
)

// TestShipyardBodyReskin locks the I3 hangar contract: grouped sprite card
// bands, cost rows, duration, the Max button, the build form and the active
// queue cancel.
func TestShipyardBodyReskin(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	queue := &models.QueueEntrySummary{
		ID: 1, Code: "204", Quantity: 5,
		EndTime: time.Now().Add(time.Minute), RemainingSecs: 60,
	}
	d := ShipyardData{
		Queue: queue,
		Sections: []UnitSection{
			{Title: "Recently built", Items: []UnitItem{
				{Code: "210", Name: "Spy Probe", Count: 8, Unlocked: true,
					Image: assetURL("210"), Cost: models.CargoManifest{Crystal: 7500}, DurationSecs: 1, MaxQty: 42},
			}},
			{Title: "Small class", Items: []UnitItem{
				{Code: "202", Name: "Light Cargo", Count: 3, Unlocked: true,
					Image: assetURL("202"), Cost: models.CargoManifest{Metal: 2000, Crystal: 2000}, DurationSecs: 1, MaxQty: 100},
				{Code: "204", Name: "Light Fighter", Unlocked: true, Busy: true,
					Image: assetURL("204"), Cost: models.CargoManifest{Metal: 3000, Crystal: 1000}},
			}},
			{Title: "Medium class", Items: []UnitItem{
				{Code: "207", Name: "Battleship", Unlocked: false, Image: assetURL("207"),
					Prereqs: []Prereq{{Code: "shipyard", Name: "Shipyard", Required: 12, Current: 8, Image: assetURL("shipyard")}}},
			}},
			{Title: "Large class", Items: []UnitItem{
				{Code: "225", Name: "Galleon", Unlocked: false, Image: assetURL("225"),
					Prereqs: []Prereq{{Code: "shipyard", Name: "Shipyard", Required: 14, Current: 8, Image: assetURL("shipyard")}}},
			}},
			{Title: "Special", Items: []UnitItem{
				{Code: "208", Name: "Colony Ship", Unlocked: true, Image: assetURL("208"),
					Cost: models.CargoManifest{Metal: 250000, Crystal: 200000, Deuterium: 100000}, DurationSecs: 3600},
			}},
		},
	}
	html := renderComponent(t, ShipyardBody(sh, d))

	for _, want := range []string{
		"Recently built",
		"Small class",
		"Medium class",
		"Large class",
		"Special",
		`/static/niburu/ships/202.gif`,
		`/static/niburu/ships/207.gif`,
		"Duration: 00:00:01",
		`/shipyard/7/build`,
		`/shipyard/7/cancel`,
		`name="queue_id"`,
		"queue-cancel",
		`class="unit-max" data-target="q_202" data-max="100"`,
		"Necessary for the construction:",
		`/static/niburu/buildings/shipyard.gif`,
		"Busy",
		"Spy Probe (8)",
		"Light Cargo (3)",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("hangar HTML missing %q", want)
		}
	}
	if strings.Contains(html, "/static/niburu/defenses/401.gif") {
		t.Error("hangar HTML unexpectedly contains a defense sprite")
	}
}

// TestDefensesBodyReskin locks the I3 defenses grid: it posts to the defenses
// routes and renders defense sprites.
func TestDefensesBodyReskin(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := ShipyardData{
		Queue: &models.QueueEntrySummary{ID: 2, Code: "401", Quantity: 10, EndTime: time.Now().Add(time.Minute), RemainingSecs: 60},
		Sections: []UnitSection{
			{Title: "Small class", Items: []UnitItem{
				{Code: "401", Name: "Missile Launcher", Count: 25, Unlocked: true,
					Image: assetURL("401"), Cost: models.CargoManifest{Metal: 2000}, DurationSecs: 1, MaxQty: 9},
			}},
			{Title: "Large class", Items: []UnitItem{
				{Code: "407", Name: "Small Shield Dome", Unlocked: true, Busy: true,
					Image: assetURL("407"), Cost: models.CargoManifest{Metal: 1000000, Crystal: 1000000}},
			}},
		},
	}
	html := renderComponent(t, DefensesBody(sh, d))

	for _, want := range []string{
		`/static/niburu/defenses/401.gif`,
		`/static/niburu/defenses/407.gif`,
		`/defenses/7/build`,
		`/defenses/7/cancel`,
		`name="queue_id"`,
		"Missile Launcher (25)",
		`class="unit-max" data-target="q_401" data-max="9"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("defenses HTML missing %q", want)
		}
	}
}

// TestFleetBodyReskin locks the I3 fleet page: the dispatch form survives, the
// expedition link is present and recalls use the styled button.
func TestFleetBodyReskin(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := FleetData{
		TargetG: 1, TargetS: 1, TargetP: 1,
		Ships:    []FleetShip{{Code: "202", Name: "Light Cargo", Count: 4}},
		Fleets:   []models.FleetEventSummary{{FleetID: 11, Mission: "ATTACK", Phase: "OUTBOUND", RemainingSecs: 120}},
		Incoming: []models.IncomingFleet{{MissionText: "Hostile attack", RemainingSecs: 60}},
	}
	html := renderComponent(t, FleetBody(sh, d))

	for _, want := range []string{
		"/expedition/7",
		"Expedition command",
		`/fleet/7/dispatch`,
		`/fleet/7/recall`,
		`name="ship_202"`,
		"Light Cargo (4)",
		"queue-cancel",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("fleet HTML missing %q", want)
		}
	}
}

// TestUnitClassCatalogComplete guarantees every ship and defense the engine can
// build is reachable through exactly one band, so no unit silently disappears
// from the hangar/defenses pages.
func TestUnitClassCatalogComplete(t *testing.T) {
	check := func(name string, units map[string]game.UnitDef, classes []unitClassDef) {
		seen := map[string]int{}
		for _, c := range classes {
			for _, code := range c.Codes {
				if _, ok := units[code]; !ok {
					t.Errorf("%s band %q references unknown unit %q", name, c.Title, code)
				}
				seen[code]++
			}
		}
		for code := range units {
			switch seen[code] {
			case 0:
				t.Errorf("%s unit %q is missing from every band", name, code)
			case 1:
			default:
				t.Errorf("%s unit %q appears in %d bands", name, code, seen[code])
			}
		}
	}
	check("ship", game.Ships, shipClasses)
	check("defense", game.Defenses, defenseClasses)
}

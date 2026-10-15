package web

import (
	"strings"
	"testing"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
)

// TestPlanetariumBodyRenders locks the Planetarium page: the balances row, the
// three captured actions (fields / diameter / teleport) and the live preview
// hooks, one card per owned celestial.
func TestPlanetariumBodyRenders(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := PlanetariumData{
		DarkMatter:       5_000,
		Stardust:         3,
		PlanetsPerSystem: 20,
		MaxFields:        100,
		Celestials: []PlanetariumCelestial{
			{
				CelestialDetail: models.CelestialDetail{
					ID: 7, Name: "Homeworld", Type: models.TypePlanet,
					Galaxy: 3, System: 125, Position: 12,
					DiameterKm: 12_800, FieldsUsed: 10, FieldsMax: 163, FieldsBought: 0,
					DebrisMetal: 200_000_000_000, DebrisCrystal: 100_000_000_000,
				},
				Coords:      "3:125:12",
				NextFieldDM: game.FieldExpansionCost(0, 1),
			},
		},
	}
	html := renderComponent(t, PlanetariumBody(sh, d))

	for _, want := range []string{
		"Planetarium",
		"Dark Matter",
		"Stardust",
		"Homeworld",
		"3:125:12",
		`action="/blackmarket/planetarium/fields"`,
		`action="/blackmarket/planetarium/diameter"`,
		`action="/blackmarket/planetarium/relocate"`,
		`name="celestial_id" value="7"`,
		`name="fields"`,
		`name="galaxy"`,
		`name="system"`,
		`name="position"`,
		`hx-post="/blackmarket/planetarium/fields/preview"`,
		`hx-post="/blackmarket/planetarium/relocate/preview"`,
		`hx-target="next .planetarium-quote"`,
		"200",
		"Back to Black market",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("planetarium HTML missing %q", want)
		}
	}
}

// TestPlanetariumCardLocksAndNotes locks the relocation cooldown and post-
// teleport combat-lock notices.
func TestPlanetariumCardLocksAndNotes(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := PlanetariumData{PlanetsPerSystem: 20, MaxFields: 100, Celestials: []PlanetariumCelestial{
		{
			CelestialDetail: models.CelestialDetail{ID: 7, Name: "Homeworld", Type: models.TypePlanet},
			RelocCooldown:   "00:59:59",
			AttackLock:      "00:09:59",
		},
	}}
	html := renderComponent(t, PlanetariumBody(sh, d))
	for _, want := range []string{"inter-system cooldown 00:59:59", "combat-locked after relocation for 00:09:59"} {
		if !strings.Contains(html, want) {
			t.Errorf("planetarium card missing %q", want)
		}
	}
}

// TestPlanetariumQuotePartial covers the preview fragment for a valid price and
// for a rejected one.
func TestPlanetariumQuotePartial(t *testing.T) {
	html := renderComponent(t, PlanetariumQuotePartial("Cost 1,543 Dark Matter", ""))
	if !strings.Contains(html, "Cost 1,543 Dark Matter") {
		t.Errorf("quote partial = %q, want the price text", html)
	}

	html = renderComponent(t, PlanetariumQuotePartial("", "Not enough Dark Matter"))
	if !strings.Contains(html, "Not enough Dark Matter") || !strings.Contains(html, "trader-quote--err") {
		t.Errorf("error partial = %q, want the error message and error class", html)
	}
}

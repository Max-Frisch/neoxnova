package web

import (
	"strings"
	"testing"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
)

// TestEmpireBodyMatrix locks the Empire overview core matrix: the Σ column, the
// Resources / Production rows (incl. Dark Matter), the Buildings / Ships /
// Defenses matrices, the per-celestial Go links and the category filters.
func TestEmpireBodyMatrix(t *testing.T) {
	home := models.EmpireCelestial{
		ID: 1, Name: "Homeworld", Type: models.TypePlanet, Galaxy: 1, System: 1, Position: 1,
		IsHome: true, FieldsUsed: 10, FieldsMax: 163,
		Metal: 1000, Crystal: 500, Deuterium: 250, EnergyAvailable: 50, EnergyMax: 100,
		MetalPerHour: 120, CrystalPerHour: 60, DeuteriumPerHour: 30,
		Buildings: map[string]int{"metal_mine": 5}, Ships: map[string]int64{"202": 10}, Defenses: map[string]int64{"401": 3},
	}
	moon := models.EmpireCelestial{
		ID: 2, Name: "Lunar", Type: models.TypeMoon, Galaxy: 1, System: 1, Position: 2,
		FieldsUsed: 1, FieldsMax: 3,
		Metal: 2000, Crystal: 100, Deuterium: 10, EnergyAvailable: -5, EnergyMax: 20,
		MetalPerHour: 0, CrystalPerHour: 0, DeuteriumPerHour: 0, DarkMatterPerHour: 4752,
	}
	sh := Shell{ActivePlanetID: 1}
	html := renderComponent(t, EmpireBody(sh, buildEmpireData([]models.EmpireCelestial{home, moon})))

	for _, want := range []string{
		"Empire",
		`data-empire-filter="all"`,
		`data-empire-filter="building"`,
		`data-empire-filter="fleet"`,
		`data-empire-filter="defense"`,
		`class="empire-filter is-active"`,
		"Σ",
		"Resources",
		"Production per hour",
		"Dark Matter",
		"Buildings",
		"Ships",
		"Defenses",
		"Homeworld",
		"Lunar",
		"(Moon)",
		"1:1:1",
		"10 / 163",
		"Go",
		`href="/overview/1"`,
		`href="/overview/2"`,
		"Dark Matter",
		// Σ of Metal = 1000 + 2000.
		">3,000<",
		// Σ Dark Matter/h = 4752 (moon collider only).
		">4,752<",
		"Coming soon",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("empire HTML missing %q", want)
		}
	}

	// Every catalog building and ship/defense has a row (labels present once).
	cols := empireBuildingColumns()
	if len(cols) != len(game.Structures)+len(game.MoonOnlyStructures) {
		t.Fatalf("building columns = %d, want %d", len(cols), len(game.Structures)+len(game.MoonOnlyStructures))
	}
	for _, col := range []string{"Metal Mine", "Crystal Mine", "Moon base", "Jumpgate", "Light Conveyor"} {
		if !strings.Contains(html, ">"+col+"<") {
			t.Errorf("empire buildings matrix missing row %q", col)
		}
	}
	for _, col := range []string{"Light Cargo", "Battleship", "Missile Launcher", "Plasma Cannon"} {
		if !strings.Contains(html, ">"+col+"<") {
			t.Errorf("empire unit matrix missing row %q", col)
		}
	}
}

// TestEmpireEmptyState locks the no-celestial fallback (a user with no planets
// still gets a rendered page rather than an empty table).
func TestEmpireEmptyState(t *testing.T) {
	html := renderComponent(t, EmpireBody(Shell{}, buildEmpireData(nil)))
	if !strings.Contains(html, "Empire") {
		t.Errorf("empty empire HTML missing title")
	}
	if !strings.Contains(html, "do not own any celestial") {
		t.Errorf("empty empire HTML missing empty-state message")
	}
}

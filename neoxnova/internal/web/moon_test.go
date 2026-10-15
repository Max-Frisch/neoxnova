package web

import (
	"strings"
	"testing"

	"neoxnova/internal/store"
)

// TestJumpgateBodyReskin locks the Jumpgate panel contract: the ship picker, the
// destination dropdown and the jump form action.
func TestJumpgateBodyReskin(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := JumpgateData{
		Level:        3,
		CooldownSecs: 1350,
		Destinations: []store.JumpDestination{
			{MoonID: 9, Name: "Hub", Galaxy: 2, System: 188, Position: 9, JumpgateLevel: 2},
		},
		Ships: []JumpShipOption{{Code: "204", Name: "Light Fighter", Count: 12}},
	}
	html := renderComponent(t, JumpgateBody(sh, d))
	for _, want := range []string{
		`/jumpgate/7/jump`,
		`name="ship_204"`,
		`name="dst"`,
		"Hub",
		"Light Fighter",
		"resources can never be transferred",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("JumpgateBody missing %q", want)
		}
	}
}

// TestPhalanxBodyReskin locks the Phalanx scan form and the revealed fleets.
func TestPhalanxBodyReskin(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := PhalanxData{
		Level:   2,
		Target:  "3:125:12",
		Scanned: true,
		Fleets: []PhalanxFleet{
			{Mission: "Attack", Phase: "OUTBOUND", Origin: "3:125:12", Dest: "3:130:5", ETA: "00:05:00"},
		},
	}
	html := renderComponent(t, PhalanxBody(sh, d))
	for _, want := range []string{
		`/phalanx/7`,
		"3:125:12",
		"Attack",
		"Fleets in movement",
		"Scan range",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("PhalanxBody missing %q", want)
		}
	}
}

// TestBuildingsBodyMoonHelp locks the moon-specific fields helper text.
func TestBuildingsBodyMoonHelp(t *testing.T) {
	html := renderComponent(t, BuildingsBody(Shell{ActivePlanetID: 7}, BuildingsData{IsMoon: true, FieldsMax: 3}))
	if !strings.Contains(html, "structures on this moon") {
		t.Fatal("moon buildings page must use the moon fields help text")
	}
}

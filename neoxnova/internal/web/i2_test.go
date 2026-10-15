package web

import (
	"strings"
	"testing"
	"time"

	"neoxnova/internal/models"
)

// TestBuildingsBodyReskin locks the I2 buildings card-grid contract: sprites,
// cost rows, the locked "Necessary for the construction" block, the build form,
// the active-queue cancel button and the Buy-with-Dark-matter placeholder.
func TestBuildingsBodyReskin(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	queue := &models.QueueEntrySummary{
		ID: 1, Code: "nanite_factory", TargetLevel: 6,
		EndTime: time.Now().Add(6 * time.Minute), RemainingSecs: 365,
	}
	d := BuildingsData{
		FieldsUsed: 214, FieldsMax: 650, Queue: queue,
		Items: []BuildItem{
			{Code: "metal_mine", Name: "Metal Mine", Level: 5, Unlocked: true, Image: assetURL("metal_mine"),
				Cost: models.CargoManifest{Metal: 1000, Crystal: 500}, EnergyDelta: -80, DurationSecs: 65},
			{Code: "solar_plant", Name: "Solar Power Plant", Level: 5, Unlocked: true, Busy: true, Image: assetURL("solar_plant"),
				Cost: models.CargoManifest{Metal: 75}, EnergyDelta: 430},
			{Code: "nanite_factory", Name: "Nanite Factory", Level: 0, Unlocked: false,
				Image:   assetURL("nanite_factory"),
				Prereqs: []Prereq{{Code: "robotics_factory", Name: "Robot Factory", Required: 10, Current: 2, Image: assetURL("robotics_factory")}}},
		},
	}
	html := renderComponent(t, BuildingsBody(sh, d))

	for _, want := range []string{
		`/static/niburu/buildings/metal_mine.gif`,
		`/static/niburu/buildings/solar_plant.gif`,
		`/static/niburu/buildings/robotics_factory.gif`,
		"Build to the next level",
		`/buildings/7/build`,
		"Fields occupied",
		"Free: 436",
		"Buy with Dark matter.",
		"Need more 80 energy",
		"Produced 430 energy",
		"Necessary for the construction:",
		`/buildings/7/cancel`,
		`name="queue_id"`,
		"queue-cancel",
		"Busy",
		"Nanite Factory 6",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("buildings HTML missing %q", want)
		}
	}
}

// TestResearchBodyReskin locks the I2 research card-grid contract.
func TestResearchBodyReskin(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := ResearchData{
		Queue: &models.QueueEntrySummary{ID: 2, Code: "energy_tech", TargetLevel: 13, EndTime: time.Now().Add(time.Minute), RemainingSecs: 60},
		Items: []ResearchItem{
			{Code: "energy_tech", Name: "Energy Technology", Level: 12, Unlocked: true, Image: assetURL("energy_tech"),
				Cost: models.CargoManifest{Crystal: 800, Deuterium: 400}},
			{Code: "astrophysics", Name: "Astrophysics", Level: 0, Unlocked: false, Image: assetURL("astrophysics"),
				Prereqs: []Prereq{{Code: "espionage_tech", Name: "Spy Technology", Required: 4, Current: 3, Image: assetURL("espionage_tech")}}},
		},
	}
	html := renderComponent(t, ResearchBody(sh, d))

	for _, want := range []string{
		`/static/niburu/research/energy_tech.gif`,
		`/static/niburu/research/espionage_tech.gif`,
		"Research to the next level",
		`/research/7/start`,
		`/research/7/cancel`,
		`name="queue_id"`,
		"For research is needed:",
		"Energy Technology 12 Level",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("research HTML missing %q", want)
		}
	}
}

// TestAssetURLGroups verifies catalog codes resolve into the correct niburu/
// subdirectory.
func TestAssetURLGroups(t *testing.T) {
	cases := map[string]string{
		"metal_mine":     "/static/niburu/buildings/metal_mine.gif",
		"moon_base":      "/static/niburu/buildings/moon_base.gif",
		"phalanx_sensor": "/static/niburu/buildings/phalanx_sensor.gif",
		"jumpgate":       "/static/niburu/buildings/jumpgate.gif",
		"energy_tech":    "/static/niburu/research/energy_tech.gif",
		"202":            "/static/niburu/ships/202.gif",
		"401":            "/static/niburu/defenses/401.gif",
		"unknown_code":   "",
	}
	for code, want := range cases {
		if got := assetURL(code); got != want {
			t.Errorf("assetURL(%q) = %q, want %q", code, got, want)
		}
	}
}

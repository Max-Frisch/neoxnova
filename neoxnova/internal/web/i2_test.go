package web

import (
	"strings"
	"testing"
	"time"

	"neoxnova/internal/models"
)

// TestBuildingsBodyReskin locks the I2 buildings card-grid contract: sprites,
// cost rows, the locked "Necessary for the construction" block, the editable
// target-level build form (with its embedded cost/time table), the multi-row
// queue list with per-row cancel buttons and the Buy-with-Dark-matter placeholder.
func TestBuildingsBodyReskin(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := BuildingsData{
		FieldsUsed: 214, FieldsMax: 650,
		Queue: []models.QueueEntrySummary{
			{ID: 1, Code: "nanite_factory", TargetLevel: 6, Cost: models.CargoManifest{Metal: 1000, Crystal: 500}, EndTime: time.Now().Add(6 * time.Minute), RemainingSecs: 365},
			{ID: 2, Code: "metal_mine", TargetLevel: 8, EndTime: time.Now().Add(9 * time.Minute), RemainingSecs: 540},
		},
		Items: []BuildItem{
			{Code: "metal_mine", Name: "Metal Mine", Level: 5, NextLevel: 6, MaxLevel: 25, Unlocked: true, Image: assetURL("metal_mine"),
				Cost: models.CargoManifest{Metal: 1000, Crystal: 500}, EnergyDelta: -80, DurationSecs: 65,
				CostTable: []levelCost{{Level: 6, Metal: 1000, Crystal: 500, Secs: 65}}},
			{Code: "solar_plant", Name: "Solar Power Plant", Level: 5, NextLevel: 6, MaxLevel: 25, Unlocked: true, Image: assetURL("solar_plant"),
				Cost: models.CargoManifest{Metal: 75}, EnergyDelta: 430,
				CostTable: []levelCost{{Level: 6, Metal: 75, Secs: 10}}},
			{Code: "nanite_factory", Name: "Nanite Factory", Level: 0, NextLevel: 1, MaxLevel: 20, Unlocked: false,
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
		`name="target_level"`,
		`data-level-input`,
		`data-cost-table=`,
		`data-cost-total`,
		`data-duration-total`,
		"Fields occupied",
		"Free: 436",
		"Buy with Dark matter.",
		"Need more 80 energy",
		"Produced 430 energy",
		"Necessary for the construction:",
		`/buildings/7/cancel`,
		`name="queue_id"`,
		"queue-cancel",
		"queue-cost",
		"1,000 M · 500 C",
		"Nanite Factory 6",
		"Metal Mine 8",
		`id="buildings-panel"`,
		`hx-get="/buildings/7/status"`,
		`hx-trigger="every 5s"`,
		`name="sig"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("buildings HTML missing %q", want)
		}
	}
	if got := strings.Count(html, `class="queue-row"`); got != 2 {
		t.Errorf("queue-row count = %d, want 2", got)
	}
}

// TestResearchBodyReskin locks the I2 research card-grid contract.
func TestResearchBodyReskin(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	d := ResearchData{
		Queue: []models.QueueEntrySummary{
			{ID: 2, Code: "energy_tech", TargetLevel: 13, EndTime: time.Now().Add(time.Minute), RemainingSecs: 60},
			{ID: 3, Code: "energy_tech", TargetLevel: 14, EndTime: time.Now().Add(2 * time.Minute), RemainingSecs: 120},
		},
		Items: []ResearchItem{
			{Code: "energy_tech", Name: "Energy Technology", Level: 12, NextLevel: 15, MaxLevel: 34, Unlocked: true, Image: assetURL("energy_tech"),
				Cost: models.CargoManifest{Crystal: 800, Deuterium: 400}, DurationSecs: 300,
				CostTable: []levelCost{{Level: 15, Crystal: 800, Deuterium: 400, Secs: 300}}},
			{Code: "astrophysics", Name: "Astrophysics", Level: 0, NextLevel: 1, MaxLevel: 20, Unlocked: false, Image: assetURL("astrophysics"),
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
		`name="target_level"`,
		`data-level-input`,
		`data-cost-table=`,
		"For research is needed:",
		"Energy Technology 12 Level",
		"Energy Technology 13",
		"Energy Technology 14",
		`id="research-panel"`,
		`hx-get="/research/7/status"`,
		`hx-trigger="every 5s"`,
		`name="sig"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("research HTML missing %q", want)
		}
	}
	if got := strings.Count(html, `class="queue-row"`); got != 2 {
		t.Errorf("research queue-row count = %d, want 2", got)
	}
}

// TestPanelSig verifies the status-poll signature is deterministic but changes
// whenever the panel state changes.
func TestPanelSig(t *testing.T) {
	if panelSig("a|b") != panelSig("a|b") {
		t.Fatal("panelSig is not deterministic")
	}
	if panelSig("a|b") == panelSig("a|c") {
		t.Fatal("panelSig ignores state changes")
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

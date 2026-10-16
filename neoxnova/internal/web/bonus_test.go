package web

import (
	"strings"
	"testing"

	"neoxnova/internal/game"
)

// TestBonusBodyRender locks the Online Bonus page: the peaceful bonus list and
// the claim button (or its cooldown).
func TestBonusBodyRender(t *testing.T) {
	sh := Shell{ActivePlanetID: 7}
	base := BonusData{
		Level:       76,
		ProgressPct: 37.45,
		NextSeconds: 65873,
		Bonus:       game.PeacefulBonuses(76),
		Reward:      game.BonusButtonReward(76),
	}

	ready := base
	ready.Ready = true
	html := renderComponent(t, BonusBody(sh, ready))
	for _, want := range []string{
		"Online Bonus",
		"Peaceful level bonuses",
		"+76% the extraction of mines",
		"+15 fields on the planet",
		"Collect bonus",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("bonus HTML missing %q", want)
		}
	}

	cooling := base
	cooling.Ready, cooling.ReadySeconds = false, 600
	coolHTML := renderComponent(t, BonusBody(sh, cooling))
	if !strings.Contains(coolHTML, "Next bonus in") {
		t.Errorf("cooling HTML missing cooldown note")
	}
	if strings.Contains(coolHTML, "Collect bonus") {
		t.Errorf("cooling HTML should not show the claim button")
	}
}

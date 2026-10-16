package web

import (
	"strings"
	"testing"
	"time"

	"neoxnova/internal/game"
)

// TestSenateHub locks the Senate hub tiles: Governators links out, Officers is a
// disabled "coming soon" tile.
func TestSenateHub(t *testing.T) {
	sh := Shell{}
	html := renderComponent(t, SenateBody(sh, senateTiles))
	for _, want := range []string{
		"Senate",
		"Governators",
		"/senate/governators",
		"Officers",
		`class="bm-tile is-disabled"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("senate hub missing %q", want)
		}
	}
}

// TestGovernatorsBody locks the governator card contract: level, effect, the
// Dark Matter per-day price, the improve AP cost and the embedded live-preview
// tables.
func TestGovernatorsBody(t *testing.T) {
	def, _ := game.GovernatorByCode(game.GovCodeResource)
	it := GovernatorItem{
		Code: def.Code, Name: def.Name, Image: governatorImage(def.Code),
		Level: 10, MaxLevel: def.MaxLevel, BonusPct: def.BonusPct(10),
		Effects: def.Effects, NextLevel: 11, APCostNext: def.APCost(10, 11),
		APCum:         []int64{0, 100, 201},
		BonusPctTable: []int{10, 11, 12},
		DMPricePerDay: def.DMPricePerDay(10),
	}
	d := GovernatorsData{Items: []GovernatorItem{it}, AchievementPoints: 3849}
	sh := Shell{ActivePlanetID: 7}
	html := renderComponent(t, GovernatorsBody(sh, d))

	for _, want := range []string{
		"Ressource Upgrade",
		"[level 10/250]",
		"+20%",
		"the extraction of materials",
		"Dark Matter:",
		`id="dm_704"`,
		`/senate/governators/hire`,
		`/senate/governators/improve`,
		"To improve the level",
		`id="ap_704"`,
		`data-gov-level="10"`,
		`data-gov-max="250"`,
		`data-gov-bonus="`,
		`data-gov-effect`,
		`data-gov-sign="+"`,
		"Achievement points:",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("governators HTML missing %q", want)
		}
	}
}

// TestGovernatorCardActiveCountdown proves an active governator shows the
// activation countdown and the "Governor activated" state instead of Hire.
func TestGovernatorCardActiveCountdown(t *testing.T) {
	exp := time.Now().Add(2 * time.Hour)
	it := GovernatorItem{
		Code: game.GovCodeWeapons, Name: "Weapons Upgrade", Level: 0, MaxLevel: 65,
		BonusPct: 10, Effects: []game.GovernatorEffect{{Key: game.GovEffectAttack, Label: "Attack"}},
		NextLevel: 1, APCum: []int64{0, 50}, DMPricePerDay: 40000,
		Active: true, ExpiresAt: exp,
	}
	html := renderComponent(t, governatorCard(it))
	for _, want := range []string{
		"Activation time:",
		"Governor activated",
		"data-countdown=",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("active governator card missing %q", want)
		}
	}
	if strings.Contains(html, "/senate/governators/hire") {
		t.Error("active governator should not render the Hire form")
	}
}

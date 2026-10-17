package web

import (
	"strings"
	"testing"

	"neoxnova/internal/game"
)

// TestAcademyBody locks the skill-tree page contract: the point balance, the
// three branch headers, a skill card with its effect/level/next cost and the
// upgrade form, plus the three reset tiers per branch.
func TestAcademyBody(t *testing.T) {
	d := AcademyData{
		Points: 123,
		Branches: []AcademyBranchView{{
			Branch: game.AcademyAttack,
			Label:  "Attack",
			Skills: []AcademySkillView{{
				Code: game.SkillWeaponry, Name: "Weaponry", Tier: "I",
				Level: 5, MaxLevel: 50, EffectText: "+1% Attack",
				NextLevel: 6, CostNext: 3, PrereqMet: true, Affordable: true,
			}},
		}},
	}
	sh := Shell{}
	html := renderComponent(t, AcademyBody(sh, d))
	for _, want := range []string{
		"Academy points:",
		"123",
		"Attack branch",
		"Weaponry",
		"[level 5/50]",
		"+1% Attack",
		"/academy/upgrade",
		`name="code" value="1101"`,
		`name="target"`,
		"/academy/reset",
		"Reset 100%",
		"Reset 75%",
		"Reset 50%",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("academy HTML missing %q", want)
		}
	}
}

// TestAcademySkillMaxLevel proves an at-max skill renders the badge instead of
// the upgrade form.
func TestAcademySkillMaxLevel(t *testing.T) {
	html := renderComponent(t, academySkill(AcademySkillView{
		Code: game.SkillWeaponry, Name: "Weaponry", Tier: "I",
		Level: 50, MaxLevel: 50, EffectText: "+1% Attack", AtMax: true,
	}))
	if !strings.Contains(html, "Maximum level reached") {
		t.Error("at-max skill should show the maximum badge")
	}
	if strings.Contains(html, "/academy/upgrade") {
		t.Error("at-max skill should not render the upgrade form")
	}
}

// TestMenuAcademyLink proves the Academy menu entry links out (it used to be a
// disabled "coming soon" item).
func TestMenuAcademyLink(t *testing.T) {
	html := renderComponent(t, MainMenu(Shell{}))
	if !strings.Contains(html, `href="/academy"`) {
		t.Error("menu should link to /academy")
	}
}

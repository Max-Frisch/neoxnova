package game

import "testing"

func TestMissionDisplayFor(t *testing.T) {
	cases := map[string]struct {
		text    string
		hostile bool
	}{
		"ATTACK":       {"Attack", true},
		"TRANSPORT":    {"Transport", false},
		"ESPIONAGE":    {"Spy", false},
		"DESTROY_MOON": {"Destroy", true},
		"DEPLOY":       {"Station", false},
	}
	for mission, want := range cases {
		d := MissionDisplayFor(mission)
		if d.Text != want.text || d.Hostile != want.hostile {
			t.Fatalf("MissionDisplayFor(%q) = %+v, want text=%q hostile=%v", mission, d, want.text, want.hostile)
		}
		if d.Colour == "" {
			t.Fatalf("MissionDisplayFor(%q) has no colour", mission)
		}
	}
	// Unknown missions fall back to the raw name, neutral and non-hostile.
	if d := MissionDisplayFor("WEIRD"); d.Text != "WEIRD" || d.Hostile || d.Colour == "" {
		t.Fatalf("fallback = %+v", d)
	}
}

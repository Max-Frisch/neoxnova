package game

import "testing"

// Hostility is ownership-based: a foreign fleet is hostile whatever the mission
// (the live log marks enemy spy fleets hostile too). Colours come from the live
// overview "Fleet" event-log sample.
func TestMissionDisplayFor(t *testing.T) {
	cases := []struct {
		mission string
		hostile bool
		text    string
		colour  string
	}{
		{"ATTACK", true, "Attack", colourHostile},
		{"ATTACK", false, "Attack", colourOwn},
		{"ESPIONAGE", true, "Spy", colourHostileSpy},
		{"ESPIONAGE", false, "Spy", colourOwn},
		{"TRANSPORT", true, "Transport", colourHostile},
		{"TRANSPORT", false, "Transport", colourOwn},
		{"DEPLOY", false, "Station", colourOwn},
		{"DESTROY_MOON", true, "Destroy", colourHostile},
		{"EXPEDITION", false, "Expedition", colourOwn},
	}
	for _, c := range cases {
		d := MissionDisplayFor(c.mission, c.hostile)
		if d.Text != c.text || d.Colour != c.colour || d.Hostile != c.hostile {
			t.Fatalf("MissionDisplayFor(%q, %v) = %+v, want text=%q colour=%q hostile=%v",
				c.mission, c.hostile, d, c.text, c.colour, c.hostile)
		}
	}
	// Unknown missions fall back to the raw name and are never dropped; a foreign
	// one is still flagged/labelled as a threat.
	own := MissionDisplayFor("WEIRD", false)
	if own.Text != "WEIRD" || own.Hostile || own.Colour != colourNeutral {
		t.Fatalf("unknown own = %+v", own)
	}
	enemy := MissionDisplayFor("WEIRD", true)
	if enemy.Text != "WEIRD" || !enemy.Hostile || enemy.Colour != colourHostile {
		t.Fatalf("unknown hostile = %+v", enemy)
	}
}

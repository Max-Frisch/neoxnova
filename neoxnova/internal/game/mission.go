package game

// Mission display for incoming fleets. A defender sees every fleet inbound to a
// planet as a human-readable label plus a colour hint, mirroring the live
// overview "Fleet" event log. The labels match the server's type_mission_*
// strings (tools/explorer/data/_lang_FLEETphp).
//
// Hostility is about OWNERSHIP, not the mission: the live log marks every fleet
// that is not the viewer's own as a threat ("A hostile Fleets ... Mission: ..."),
// colouring enemy attacks red and enemy espionage orange, while the viewer's own
// inbound fleets render green. The palette below was taken from a live sample
// (2026-10-08: acc1 defending against acc2's attack + spy).
type MissionDisplay struct {
	Text    string
	Colour  string
	Hostile bool
}

const (
	colourHostile    = "#ff4d4d" // enemy fleet / attack
	colourHostileSpy = "#ff9900" // enemy espionage
	colourOwn        = "#4caf50" // the viewer's own inbound fleet
	colourNeutral    = "#9e9e9e" // unknown mission
)

// missionLabels maps a mission name (models.MissionType) to its live label.
var missionLabels = map[string]string{
	"ATTACK":       "Attack",
	"TRANSPORT":    "Transport",
	"DEPLOY":       "Station",
	"HOLD":         "Hold",
	"ESPIONAGE":    "Spy",
	"COLONIZE":     "Colonisation",
	"RECYCLE":      "Recycle",
	"DESTROY_MOON": "Destroy",
	"EXPEDITION":   "Expedition",
}

// MissionDisplayFor resolves a mission name plus the fleet's ownership (hostile
// = the fleet is NOT owned by the planet owner) to its defender-facing display.
// An unknown mission falls back to its raw name so the view never drops a fleet.
func MissionDisplayFor(mission string, hostile bool) MissionDisplay {
	label, ok := missionLabels[mission]
	if !ok {
		label = mission
		colour := colourNeutral
		if hostile {
			colour = colourHostile
		}
		return MissionDisplay{Text: label, Colour: colour, Hostile: hostile}
	}
	if !hostile {
		return MissionDisplay{Text: label, Colour: colourOwn}
	}
	if mission == "ESPIONAGE" {
		return MissionDisplay{Text: label, Colour: colourHostileSpy, Hostile: true}
	}
	return MissionDisplay{Text: label, Colour: colourHostile, Hostile: true}
}

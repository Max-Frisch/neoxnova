package game

// Mission display for incoming fleets. A defender sees the mission of every
// fleet inbound to a planet as a human-readable label plus a colour hint. The
// labels mirror the live server's type_mission_* strings
// (tools/explorer/data/_lang_FLEETphp): Attack / Transport / Station (deploy) /
// Hold / Spy / Colonisation / Recycle / Destroy / Expedition.
//
// The live theme's exact palette is not captured (no incoming-fleet sample), so
// the colours below are documented defaults; only the ATTACK (and moon-destroy)
// rows are flagged Hostile.
type MissionDisplay struct {
	Text    string
	Colour  string
	Hostile bool
}

// missionDisplays maps a mission name (models.MissionType) to its display.
var missionDisplays = map[string]MissionDisplay{
	"ATTACK":       {Text: "Attack", Colour: "#ff4d4d", Hostile: true},
	"TRANSPORT":    {Text: "Transport", Colour: "#4caf50"},
	"DEPLOY":       {Text: "Station", Colour: "#4c8dff"},
	"HOLD":         {Text: "Hold", Colour: "#26c6da"},
	"ESPIONAGE":    {Text: "Spy", Colour: "#ffd54f"},
	"COLONIZE":     {Text: "Colonisation", Colour: "#ab47bc"},
	"RECYCLE":      {Text: "Recycle", Colour: "#9e9e9e"},
	"DESTROY_MOON": {Text: "Destroy", Colour: "#b71c1c", Hostile: true},
	"EXPEDITION":   {Text: "Expedition", Colour: "#00bcd4"},
}

// MissionDisplayFor resolves a mission name to its defender-facing display. An
// unknown mission falls back to its raw name (neutral colour, not hostile) so
// the view never drops a fleet.
func MissionDisplayFor(mission string) MissionDisplay {
	if d, ok := missionDisplays[mission]; ok {
		return d
	}
	return MissionDisplay{Text: mission, Colour: "#9e9e9e"}
}

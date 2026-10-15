package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// GameSettings is the tunable game configuration loaded from config/game.json
// and upserted onto the universes row on boot. The zero-value fields are
// replaced by DefaultGameSettings before parsing so a partial file keeps the
// built-in defaults.
type GameSettings struct {
	GameSpeed          float64 `json:"game_speed"`
	ResourceSpeed      float64 `json:"resource_speed"`
	FleetSpeed         float64 `json:"fleet_speed"`
	BuildQueueSlots    int     `json:"build_queue_slots"`
	ResearchQueueSlots int     `json:"research_queue_slots"`
}

// DefaultGameSettings returns the built-in tuning, used when the config file is
// absent or omits a key. Speeds are the plain 1x defaults; the committed
// config/game.json carries the calibrated reference-server values.
func DefaultGameSettings() GameSettings {
	return GameSettings{
		GameSpeed:          1.0,
		ResourceSpeed:      1.0,
		FleetSpeed:         15.0,
		BuildQueueSlots:    10,
		ResearchQueueSlots: 4,
	}
}

// LoadGameSettings parses path over the defaults. A missing file is not an
// error (defaults are returned); unknown keys are rejected to catch typos. The
// returned settings are NOT yet validated against the DB — the caller upserts
// them onto the universes row.
func LoadGameSettings(path string) (GameSettings, error) {
	settings := DefaultGameSettings()

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return settings, fmt.Errorf("read game config %s: %w", path, err)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&settings); err != nil {
		return settings, fmt.Errorf("parse game config %s: %w", path, err)
	}
	return settings, nil
}

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGameJSON(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "game.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadGameSettingsDefaultsWhenAbsent(t *testing.T) {
	got, err := LoadGameSettings(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("absent file should not error: %v", err)
	}
	if got != DefaultGameSettings() {
		t.Fatalf("absent file = %+v, want defaults %+v", got, DefaultGameSettings())
	}
}

func TestLoadGameSettingsParsesFile(t *testing.T) {
	path := writeGameJSON(t, `{
		"game_speed": 4000,
		"resource_speed": 10000,
		"fleet_speed": 15,
		"build_queue_slots": 10,
		"research_queue_slots": 4,
		"merchant_rate": 40,
		"dm_market_divisor": 2000
	}`)
	got, err := LoadGameSettings(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := GameSettings{GameSpeed: 4000, ResourceSpeed: 10000, FleetSpeed: 15, BuildQueueSlots: 10, ResearchQueueSlots: 4, MerchantRate: 40, DMMarketDivisor: 2000}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

// A partial file must keep the built-in defaults for omitted keys.
func TestLoadGameSettingsPartialKeepsDefaults(t *testing.T) {
	path := writeGameJSON(t, `{"fleet_speed": 30}`)
	got, err := LoadGameSettings(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := DefaultGameSettings()
	want.FleetSpeed = 30
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestLoadGameSettingsRejectsUnknownField(t *testing.T) {
	path := writeGameJSON(t, `{"fleet_speeed": 15}`)
	if _, err := LoadGameSettings(path); err == nil {
		t.Fatal("unknown field should be rejected (typo guard)")
	}
}

func TestLoadGameSettingsRejectsMalformedJSON(t *testing.T) {
	path := writeGameJSON(t, `{"game_speed": }`)
	if _, err := LoadGameSettings(path); err == nil {
		t.Fatal("malformed JSON should error")
	}
}

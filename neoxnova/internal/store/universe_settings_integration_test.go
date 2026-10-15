package store

import (
	"context"
	"testing"

	"neoxnova/internal/config"
)

// C1 of docs/PLAYABILITY_FIXES_PLAN.md: the boot loader upserts config/game.json
// onto the universes row. Skipped unless DATABASE_URL is set.

// TestApplyGameSettingsUpsert writes distinct tuning values onto a seeded
// universe, reads them back, then restores the original row.
func TestApplyGameSettingsUpsert(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()

	var codeName string
	if err := db.QueryRowContext(ctx, `SELECT code_name FROM universes ORDER BY code_name LIMIT 1`).Scan(&codeName); err != nil {
		t.Skipf("no universe seeded: %v", err)
	}

	const cols = `game_speed, resource_speed, fleet_speed, build_queue_slots, research_queue_slots, merchant_rate, dm_market_divisor`

	var orig config.GameSettings
	if err := db.QueryRowContext(ctx, `SELECT `+cols+` FROM universes WHERE code_name = $1`, codeName).
		Scan(&orig.GameSpeed, &orig.ResourceSpeed, &orig.FleetSpeed, &orig.BuildQueueSlots, &orig.ResearchQueueSlots, &orig.MerchantRate, &orig.DMMarketDivisor); err != nil {
		t.Fatalf("read original settings: %v", err)
	}
	t.Cleanup(func() {
		if _, err := ApplyGameSettings(ctx, db, codeName, orig); err != nil {
			t.Errorf("restore original settings: %v", err)
		}
	})

	want := config.GameSettings{GameSpeed: 1234.50, ResourceSpeed: 4321.25, FleetSpeed: 7.50, BuildQueueSlots: 12, ResearchQueueSlots: 5, MerchantRate: 40, DMMarketDivisor: 2000}
	applied, err := ApplyGameSettings(ctx, db, codeName, want)
	if err != nil {
		t.Fatalf("apply settings: %v", err)
	}
	if !applied {
		t.Fatal("apply settings reported no row updated for a seeded universe")
	}

	var got config.GameSettings
	if err := db.QueryRowContext(ctx, `SELECT `+cols+` FROM universes WHERE code_name = $1`, codeName).
		Scan(&got.GameSpeed, &got.ResourceSpeed, &got.FleetSpeed, &got.BuildQueueSlots, &got.ResearchQueueSlots, &got.MerchantRate, &got.DMMarketDivisor); err != nil {
		t.Fatalf("read back settings: %v", err)
	}
	if got != want {
		t.Fatalf("read back %+v, want %+v", got, want)
	}
}

// TestApplyGameSettingsUnknownUniverse proves a missing row is not an error and
// reports "not applied" rather than crashing startup.
func TestApplyGameSettingsUnknownUniverse(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()

	applied, err := ApplyGameSettings(ctx, db, "no_such_universe_xyz", config.DefaultGameSettings())
	if err != nil {
		t.Fatalf("unknown universe should not error: %v", err)
	}
	if applied {
		t.Fatal("unknown universe reported applied = true")
	}
}

package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"neoxnova/internal/game"
)

// TestExecuteTrade verifies the Resource Trader exchange against a live
// Postgres: the ratio, the Dark Matter debit, the storage-cap clamp and the
// ownership / balance refusals. Skipped unless DATABASE_URL is set.
func TestExecuteTrade(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping DB integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	var universeID string
	if err := db.QueryRowContext(ctx, `SELECT universe_id::text FROM celestial_objects WHERE id = 1`).Scan(&universeID); err != nil {
		t.Skipf("seed missing celestial 1: %v", err)
	}

	upsertUser := func(name string, dm int64) int64 {
		var id int64
		if err := db.QueryRowContext(ctx, `
			INSERT INTO users (universe_id, username, email, password_hash, dark_matter)
			VALUES ($1, $2, $3, 'x', $4)
			ON CONFLICT (universe_id, username) DO UPDATE
				SET email = EXCLUDED.email, dark_matter = EXCLUDED.dark_matter
			RETURNING id
		`, universeID, name, name+"@example.com", dm).Scan(&id); err != nil {
			t.Fatalf("upsert user %s: %v", name, err)
		}
		return id
	}
	owner := upsertUser("trade_test_owner", 10_000)
	other := upsertUser("trade_test_other", 10_000)

	const (
		g, sys, pos = 9, 420, 5
		cap         = 10_000_000
	)
	var planetID int64
	// Production is zeroed so balances only change by the trade itself.
	if err := db.QueryRowContext(ctx, `
		INSERT INTO celestial_objects (universe_id, user_id, name, object_type, galaxy, system, position,
			metal, crystal, deuterium,
			metal_capacity, crystal_capacity, deuterium_capacity,
			metal_prod_hourly, crystal_prod_hourly, deuterium_prod_hourly,
			fields_max, base_fields_max, last_resource_calc_at)
		VALUES ($1, $2, 'TradeTest', 'PLANET', $3, $4, $5, 1000, 1000, 0, $6, $6, $6, 0, 0, 0, 500, 500, NOW())
		ON CONFLICT (universe_id, galaxy, system, position, object_type) DO UPDATE
			SET user_id = EXCLUDED.user_id, metal = 1000, crystal = 1000, deuterium = 0,
			    metal_capacity = $6, crystal_capacity = $6, deuterium_capacity = $6,
			    metal_prod_hourly = 0, crystal_prod_hourly = 0, deuterium_prod_hourly = 0,
			    last_resource_calc_at = NOW()
		RETURNING id
	`, universeID, owner, g, sys, pos, cap).Scan(&planetID); err != nil {
		t.Fatalf("insert planet: %v", err)
	}
	defer func() {
		db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID)
		db.ExecContext(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, owner, other)
	}()

	ts := NewTradeStore(db)

	readState := func() (metal, crystal, deuterium float64, dm int64) {
		t.Helper()
		if err := db.QueryRowContext(ctx, `
			SELECT metal, crystal, deuterium FROM celestial_objects WHERE id = $1
		`, planetID).Scan(&metal, &crystal, &deuterium); err != nil {
			t.Fatalf("read resources: %v", err)
		}
		if err := db.QueryRowContext(ctx, `SELECT dark_matter FROM users WHERE id = $1`, owner).Scan(&dm); err != nil {
			t.Fatalf("read dm: %v", err)
		}
		return
	}

	// A foreign owner cannot trade on the planet.
	foreign, _ := game.TradeQuote(game.ResourceCrystal, game.ResourceDeuterium, 100, 10_000)
	if err := ts.ExecuteTrade(ctx, planetID, other, foreign); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign trade = %v, want ErrNotFound", err)
	}

	// Sell 200 crystal to buy 100 deuterium at the 2:1 crystal:deut rate.
	q, err := game.TradeQuote(game.ResourceCrystal, game.ResourceDeuterium, 100, 10_000)
	if err != nil {
		t.Fatalf("TradeQuote: %v", err)
	}
	if q.SellCost != 200 {
		t.Fatalf("SellCost = %d, want 200", q.SellCost)
	}
	if err := ts.ExecuteTrade(ctx, planetID, owner, q); err != nil {
		t.Fatalf("ExecuteTrade: %v", err)
	}
	metal, crystal, deut, dm := readState()
	if metal != 1000 || crystal != 800 || deut != 100 {
		t.Fatalf("after trade = M %.0f C %.0f D %.0f, want 1000/800/100", metal, crystal, deut)
	}
	if dm != 10_000-game.TradeDarkMatterCost {
		t.Fatalf("dark matter = %d, want %d", dm, 10_000-game.TradeDarkMatterCost)
	}

	// The bought resource is clamped to its storage capacity while the full
	// cost is still charged.
	if _, err := db.ExecContext(ctx, `
		UPDATE celestial_objects SET crystal = 100000, deuterium = 0, deuterium_capacity = 150 WHERE id = $1`, planetID); err != nil {
		t.Fatalf("reset for cap: %v", err)
	}
	big, _ := game.TradeQuote(game.ResourceCrystal, game.ResourceDeuterium, 1000, 10_000)
	if err := ts.ExecuteTrade(ctx, planetID, owner, big); err != nil {
		t.Fatalf("capped ExecuteTrade: %v", err)
	}
	_, crystal, deut, _ = readState()
	if deut != 150 {
		t.Fatalf("deuterium = %.0f, want 150 (clamped to cap)", deut)
	}
	if crystal != 100000-2000 { // 1000 deut * 2 crystal
		t.Fatalf("crystal = %.0f, want 98000 (full cost still charged)", crystal)
	}

	// Insufficient sold resource is refused and changes nothing.
	if _, err := db.ExecContext(ctx, `
		UPDATE celestial_objects SET crystal = 10, deuterium = 0, deuterium_capacity = 1000000 WHERE id = $1`, planetID); err != nil {
		t.Fatalf("reset: %v", err)
	}
	broke, _ := game.TradeQuote(game.ResourceCrystal, game.ResourceDeuterium, 100, 10_000)
	if err := ts.ExecuteTrade(ctx, planetID, owner, broke); !errors.Is(err, ErrInsufficientResources) {
		t.Fatalf("broke trade = %v, want ErrInsufficientResources", err)
	}
	_, crystal, deut, dm = readState()
	if crystal != 10 || deut != 0 {
		t.Fatalf("resources changed after a refused trade: C %.0f D %.0f", crystal, deut)
	}
	if dm != 10_000-2*game.TradeDarkMatterCost {
		t.Fatalf("dark matter = %d after refused trade, want unchanged", dm)
	}

	// Insufficient Dark Matter is refused (the resources are affordable so the
	// fee check is what fails).
	if _, err := db.ExecContext(ctx, `
		UPDATE celestial_objects SET crystal = 1000, deuterium = 0 WHERE id = $1`, planetID); err != nil {
		t.Fatalf("reset for dm: %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET dark_matter = 0 WHERE id = $1`, owner); err != nil {
		t.Fatalf("zero dm: %v", err)
	}
	if err := ts.ExecuteTrade(ctx, planetID, owner, broke); !errors.Is(err, ErrInsufficientDarkMatter) {
		t.Fatalf("broke-dm trade = %v, want ErrInsufficientDarkMatter", err)
	}
}

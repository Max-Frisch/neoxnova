package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"neoxnova/internal/auth"
	"neoxnova/internal/game"
)

// B4 of docs/PLAYABILITY_FIXES_PLAN.md: Merchant of Fleets scraping and the DM
// Buildings/Technology markets, plus the 0021 universes knobs. Skipped unless
// DATABASE_URL is set.

// b4Fixture creates a fresh account with a homeworld and registers cleanup.
func b4Fixture(t *testing.T, db *sql.DB, prefix string) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	var code string
	if err := db.QueryRowContext(ctx, `SELECT code_name FROM universes ORDER BY code_name LIMIT 1`).Scan(&code); err != nil {
		t.Skipf("no universe seeded: %v", err)
	}
	uname := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	hash, err := auth.HashPassword("s3cret-password!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, planetID, err := NewAuthStore(db).CreateUser(ctx, code, uname, uname+"@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM planet_ships WHERE celestial_id = $1`, planetID)
		db.ExecContext(ctx, `DELETE FROM planet_defenses WHERE celestial_id = $1`, planetID)
		db.ExecContext(ctx, `DELETE FROM planet_structures WHERE celestial_id = $1`, planetID)
		db.ExecContext(ctx, `DELETE FROM user_technologies WHERE user_id = $1`, userID)
		db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID)
		db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})
	return userID, planetID
}

// seedTechRequirements inserts (or raises) whatever a technology needs so it
// passes RequiresMet: structures go on the planet, other techs on the account.
func seedTechRequirements(t *testing.T, db *sql.DB, planetID, userID int64, techCode string) {
	t.Helper()
	ctx := context.Background()
	for code, need := range game.Requirements(techCode) {
		if _, ok := game.Structures[code]; ok {
			if _, err := db.ExecContext(ctx, `
				INSERT INTO planet_structures (celestial_id, structure_code, level) VALUES ($1, $2, $3)
				ON CONFLICT (celestial_id, structure_code)
				DO UPDATE SET level = GREATEST(planet_structures.level, EXCLUDED.level)`,
				planetID, code, need); err != nil {
				t.Fatalf("seed structure %s: %v", code, err)
			}
			continue
		}
		if _, err := db.ExecContext(ctx, `
			INSERT INTO user_technologies (user_id, tech_code, level) VALUES ($1, $2, $3)
			ON CONFLICT (user_id, tech_code)
			DO UPDATE SET level = GREATEST(user_technologies.level, EXCLUDED.level)`,
			userID, code, need); err != nil {
			t.Fatalf("seed tech %s: %v", code, err)
		}
	}
}

// TestBlackMarketMigrationIdempotent re-runs 0021_blackmarket.sql and asserts the
// two universe knobs land with their locked types and defaults.
func TestBlackMarketMigrationIdempotent(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()

	sqlBytes, err := os.ReadFile(filepath.Join("..", "..", "migrations", "0021_blackmarket.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := db.ExecContext(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply migration (run %d): %v", i+1, err)
		}
	}
	for _, tc := range []struct{ col, wantType, wantDefault string }{
		{"merchant_rate", "smallint", "50"},
		{"dm_market_divisor", "integer", "1000"},
	} {
		var dataType, def string
		if err := db.QueryRowContext(ctx, `
			SELECT data_type, COALESCE(column_default, '')
			FROM information_schema.columns
			WHERE table_name = 'universes' AND column_name = $1
		`, tc.col).Scan(&dataType, &def); err != nil {
			t.Fatalf("column %s missing: %v", tc.col, err)
		}
		if dataType != tc.wantType {
			t.Errorf("%s type = %q, want %q", tc.col, dataType, tc.wantType)
		}
		if def != tc.wantDefault {
			t.Errorf("%s default = %q, want %q", tc.col, def, tc.wantDefault)
		}
	}
}

// TestScrapUnitsMerchant covers the Merchant of Fleets: refund at the configured
// rate, unit removal, capacity clamp and the refusals.
func TestScrapUnitsMerchant(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()
	userID, planetID := b4Fixture(t, db, "scrap")

	if _, err := db.ExecContext(ctx, `
		UPDATE celestial_objects
		SET metal = 0, crystal = 0, deuterium = 0,
		    metal_capacity = 1000000000, crystal_capacity = 1000000000, deuterium_capacity = 1000000000,
		    metal_prod_hourly = 0, crystal_prod_hourly = 0, deuterium_prod_hourly = 0,
		    last_resource_calc_at = NOW()
		WHERE id = $1`, planetID); err != nil {
		t.Fatalf("seed celestial: %v", err)
	}

	var rate int
	if err := db.QueryRowContext(ctx, `
		SELECT u.merchant_rate
		FROM universes u JOIN celestial_objects c ON c.universe_id = u.id
		WHERE c.id = $1`, planetID).Scan(&rate); err != nil {
		t.Fatalf("read merchant rate: %v", err)
	}

	ts := NewTradeStore(db)
	def := game.Ships["202"] // Light Cargo

	if _, err := db.ExecContext(ctx, `INSERT INTO planet_ships (celestial_id, ship_code, quantity) VALUES ($1, '202', 10)`, planetID); err != nil {
		t.Fatalf("seed ships: %v", err)
	}
	refund, gotRate, err := ts.ScrapUnits(ctx, planetID, userID, "202", 4)
	if err != nil {
		t.Fatalf("ScrapUnits: %v", err)
	}
	if gotRate != rate {
		t.Fatalf("rate = %d, want %d", gotRate, rate)
	}
	want := game.MerchantRefund(def.BaseCost.Mul(4), rate)
	if refund != want {
		t.Fatalf("refund = %+v, want %+v", refund, want)
	}
	var qty int64
	if err := db.QueryRowContext(ctx, `SELECT quantity FROM planet_ships WHERE celestial_id = $1 AND ship_code = '202'`, planetID).Scan(&qty); err != nil {
		t.Fatalf("read remaining: %v", err)
	}
	if qty != 6 {
		t.Fatalf("remaining ships = %d, want 6", qty)
	}
	var metal, crystal float64
	if err := db.QueryRowContext(ctx, `SELECT metal, crystal FROM celestial_objects WHERE id = $1`, planetID).Scan(&metal, &crystal); err != nil {
		t.Fatalf("read resources: %v", err)
	}
	if int64(metal) != want.Metal || int64(crystal) != want.Crystal {
		t.Fatalf("credited M %d C %d, want %d/%d", int64(metal), int64(crystal), want.Metal, want.Crystal)
	}

	// Scrapping more than held is refused.
	if _, _, err := ts.ScrapUnits(ctx, planetID, userID, "202", 100); !errors.Is(err, ErrInsufficientShips) {
		t.Fatalf("over-scrap = %v, want ErrInsufficientShips", err)
	}
	// An unknown unit is refused.
	if _, _, err := ts.ScrapUnits(ctx, planetID, userID, "999", 1); !errors.Is(err, ErrUnknownCode) {
		t.Fatalf("unknown unit = %v, want ErrUnknownCode", err)
	}
	// A foreign owner cannot scrap the planet's units.
	other, _ := b4Fixture(t, db, "scrap_other")
	if _, _, err := ts.ScrapUnits(ctx, planetID, other, "202", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign scrap = %v, want ErrNotFound", err)
	}

	// The defense line scrapes through planet_defenses.
	if _, err := db.ExecContext(ctx, `INSERT INTO planet_defenses (celestial_id, defense_code, quantity) VALUES ($1, '401', 5)`, planetID); err != nil {
		t.Fatalf("seed defenses: %v", err)
	}
	drefund, _, err := ts.ScrapUnits(ctx, planetID, userID, "401", 5)
	if err != nil {
		t.Fatalf("scrap defense: %v", err)
	}
	if wantD := game.MerchantRefund(game.Defenses["401"].BaseCost.Mul(5), rate); drefund != wantD {
		t.Fatalf("defense refund = %+v, want %+v", drefund, wantD)
	}
}

// TestInstantStructureDMMarket covers the Buildings Market: price, DM debit,
// level application and the refusals (insufficient DM, prerequisites, owner).
func TestInstantStructureDMMarket(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()
	userID, planetID := b4Fixture(t, db, "dmbuild")

	var divisor int64
	if err := db.QueryRowContext(ctx, `
		SELECT u.dm_market_divisor
		FROM universes u JOIN celestial_objects c ON c.universe_id = u.id
		WHERE c.id = $1`, planetID).Scan(&divisor); err != nil {
		t.Fatalf("read divisor: %v", err)
	}
	const startDM = int64(1_000_000_000)
	if _, err := db.ExecContext(ctx, `UPDATE users SET dark_matter = $2 WHERE id = $1`, userID, startDM); err != nil {
		t.Fatalf("fund dm: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM planet_structures WHERE celestial_id = $1 AND structure_code = 'metal_mine'`, planetID); err != nil {
		t.Fatalf("reset structure: %v", err)
	}

	bs := NewBuildStore(db)
	cost, price, err := bs.InstantStructure(ctx, planetID, userID, "metal_mine")
	if err != nil {
		t.Fatalf("InstantStructure: %v", err)
	}
	if want := game.DMMarketPrice(cost, divisor); price != want {
		t.Fatalf("price = %d, want %d", price, want)
	}
	var lvl int
	if err := db.QueryRowContext(ctx, `SELECT level FROM planet_structures WHERE celestial_id = $1 AND structure_code = 'metal_mine'`, planetID).Scan(&lvl); err != nil {
		t.Fatalf("read level: %v", err)
	}
	if lvl != 1 {
		t.Fatalf("metal_mine level = %d, want 1", lvl)
	}
	var dm int64
	if err := db.QueryRowContext(ctx, `SELECT dark_matter FROM users WHERE id = $1`, userID).Scan(&dm); err != nil {
		t.Fatalf("read dm: %v", err)
	}
	if dm != startDM-price {
		t.Fatalf("dark matter = %d, want %d", dm, startDM-price)
	}

	// Insufficient Dark Matter is refused.
	if _, err := db.ExecContext(ctx, `UPDATE users SET dark_matter = 0 WHERE id = $1`, userID); err != nil {
		t.Fatalf("zero dm: %v", err)
	}
	if _, _, err := bs.InstantStructure(ctx, planetID, userID, "metal_mine"); !errors.Is(err, ErrInsufficientDarkMatter) {
		t.Fatalf("broke build = %v, want ErrInsufficientDarkMatter", err)
	}
	// Prerequisites (Nanite Factory needs Robotics 10 + Shipyard 8).
	if _, _, err := bs.InstantStructure(ctx, planetID, userID, "nanite_factory"); !errors.Is(err, ErrPrerequisites) {
		t.Fatalf("prereq build = %v, want ErrPrerequisites", err)
	}
	// A foreign owner cannot build on the planet.
	other, _ := b4Fixture(t, db, "dmbuild_other")
	if _, _, err := bs.InstantStructure(ctx, planetID, other, "metal_mine"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign build = %v, want ErrNotFound", err)
	}
}

// TestInstantTechDMMarket covers the Technology Market: price, DM debit, level
// application and the refusals.
func TestInstantTechDMMarket(t *testing.T) {
	db := openQueueTestDB(t)
	ctx := context.Background()
	userID, planetID := b4Fixture(t, db, "dmtech")

	var divisor int64
	if err := db.QueryRowContext(ctx, `
		SELECT u.dm_market_divisor
		FROM universes u JOIN celestial_objects c ON c.universe_id = u.id
		WHERE c.id = $1`, planetID).Scan(&divisor); err != nil {
		t.Fatalf("read divisor: %v", err)
	}
	const startDM = int64(1_000_000_000)
	if _, err := db.ExecContext(ctx, `UPDATE users SET dark_matter = $2 WHERE id = $1`, userID, startDM); err != nil {
		t.Fatalf("fund dm: %v", err)
	}

	// Seed Spy Technology's prerequisites so a fresh planet can research it.
	const techCode = "espionage_tech"
	seedTechRequirements(t, db, planetID, userID, techCode)

	bs := NewBuildStore(db)
	cost, price, err := bs.InstantTech(ctx, planetID, userID, techCode)
	if err != nil {
		t.Fatalf("InstantTech: %v", err)
	}
	if want := game.DMMarketPrice(cost, divisor); price != want {
		t.Fatalf("price = %d, want %d", price, want)
	}
	var lvl int
	if err := db.QueryRowContext(ctx, `SELECT level FROM user_technologies WHERE user_id = $1 AND tech_code = $2`, userID, techCode).Scan(&lvl); err != nil {
		t.Fatalf("read level: %v", err)
	}
	if lvl != 1 {
		t.Fatalf("espionage_tech level = %d, want 1", lvl)
	}
	var dm int64
	if err := db.QueryRowContext(ctx, `SELECT dark_matter FROM users WHERE id = $1`, userID).Scan(&dm); err != nil {
		t.Fatalf("read dm: %v", err)
	}
	if dm != startDM-price {
		t.Fatalf("dark matter = %d, want %d", dm, startDM-price)
	}

	// Prerequisites (Astrophysics needs Spy Tech 4 + Research Lab 3).
	if _, _, err := bs.InstantTech(ctx, planetID, userID, "astrophysics"); !errors.Is(err, ErrPrerequisites) {
		t.Fatalf("prereq research = %v, want ErrPrerequisites", err)
	}
	// Insufficient Dark Matter is refused.
	if _, err := db.ExecContext(ctx, `UPDATE users SET dark_matter = 0 WHERE id = $1`, userID); err != nil {
		t.Fatalf("zero dm: %v", err)
	}
	if _, _, err := bs.InstantTech(ctx, planetID, userID, techCode); !errors.Is(err, ErrInsufficientDarkMatter) {
		t.Fatalf("broke research = %v, want ErrInsufficientDarkMatter", err)
	}
}

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

// TestArsenalLifecycle exercises the upgrade + market tables against a live
// Postgres: activation consumes drawings, listing escrows them, buying moves
// Antimatter and drawings atomically, and expiry returns unsold drawings.
// Skipped unless DATABASE_URL is set.
//
//	$env:DATABASE_URL = "postgres://postgres:password@localhost:5432/neoxnova?sslmode=disable"
//	go test ./internal/store/ -run TestArsenalLifecycle -v
func TestArsenalLifecycle(t *testing.T) {
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

	upsertUser := func(name string, atm int64) int64 {
		var id int64
		if err := db.QueryRowContext(ctx, `
			INSERT INTO users (universe_id, username, email, password_hash, antimatter)
			VALUES ($1, $2, $3, 'x', $4)
			ON CONFLICT (universe_id, username) DO UPDATE
				SET email = EXCLUDED.email, antimatter = EXCLUDED.antimatter
			RETURNING id
		`, universeID, name, name+"@example.com", atm).Scan(&id); err != nil {
			t.Fatalf("upsert user %s: %v", name, err)
		}
		return id
	}
	seller := upsertUser("arsenal_seller", 1000)
	buyer := upsertUser("arsenal_buyer", 1000)

	defer func() {
		db.ExecContext(ctx, `DELETE FROM market_purchases WHERE buyer_account_id IN ($1,$2) OR seller_account_id IN ($1,$2)`, seller, buyer)
		db.ExecContext(ctx, `DELETE FROM market_lots WHERE seller_account_id IN ($1,$2)`, seller, buyer)
		db.ExecContext(ctx, `DELETE FROM upgrade_items WHERE account_id IN ($1,$2)`, seller, buyer)
		db.ExecContext(ctx, `DELETE FROM account_upgrades WHERE account_id IN ($1,$2)`, seller, buyer)
		db.ExecContext(ctx, `DELETE FROM users WHERE id IN ($1,$2)`, seller, buyer)
	}()

	as := NewArsenalStore(db)

	grant := func(accountID int64, code, qty int) {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin grant: %v", err)
		}
		if err := AddUpgradeItems(ctx, tx, accountID, code, qty); err != nil {
			tx.Rollback()
			t.Fatalf("grant items: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit grant: %v", err)
		}
	}
	grant(seller, 1, 3)

	// Activation consumes a drawing even on the guaranteed first levels.
	res, err := as.Activate(ctx, seller, 1, 0.5)
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if !res.Success || res.Level != 1 || res.Chance != 1 {
		t.Fatalf("activation = %+v, want success level 1 chance 1", res)
	}
	if res.Bonus != game.Upgrades[1].PerLevel {
		t.Fatalf("bonus = %v, want %v", res.Bonus, game.Upgrades[1].PerLevel)
	}

	list, err := as.ArsenalList(ctx, seller)
	if err != nil {
		t.Fatalf("arsenal list: %v", err)
	}
	var laser = list[0]
	if laser.Code != 1 || laser.Level != 1 || laser.Available != 2 {
		t.Fatalf("laser after activate = %+v, want level 1 avail 2", laser)
	}

	// Listing escrows two drawings at 100 Antimatter each.
	lot, err := as.ListUpgrade(ctx, seller, 1, 2, 100)
	if err != nil {
		t.Fatalf("list upgrade: %v", err)
	}
	if lot.TotalPriceAtm != 200 {
		t.Fatalf("lot total = %d, want 200", lot.TotalPriceAtm)
	}

	market, err := as.ListMarket(ctx)
	if err != nil {
		t.Fatalf("market: %v", err)
	}
	var found bool
	for _, l := range market {
		if l.ID == lot.ID && l.Upgrade == "Laser weapons" {
			found = true
		}
	}
	if !found {
		t.Fatalf("listed lot %d missing from market", lot.ID)
	}

	// Buying transfers Antimatter and drawings atomically.
	if _, err := as.BuyLot(ctx, buyer, lot.ID); err != nil {
		t.Fatalf("buy lot: %v", err)
	}
	if _, err := as.BuyLot(ctx, buyer, lot.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rebuy = %v, want ErrNotFound", err)
	}
	var sellerAtm, buyerAtm int64
	if err := db.QueryRowContext(ctx, `SELECT antimatter FROM users WHERE id = $1`, seller).Scan(&sellerAtm); err != nil {
		t.Fatalf("read seller atm: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT antimatter FROM users WHERE id = $1`, buyer).Scan(&buyerAtm); err != nil {
		t.Fatalf("read buyer atm: %v", err)
	}
	if sellerAtm != 1200 || buyerAtm != 800 {
		t.Fatalf("antimatter seller=%d buyer=%d, want 1200/800", sellerAtm, buyerAtm)
	}
	var buyerQty int
	if err := db.QueryRowContext(ctx, `SELECT qty FROM upgrade_items WHERE account_id = $1 AND upgrade_code = 1`, buyer).Scan(&buyerQty); err != nil {
		t.Fatalf("read buyer items: %v", err)
	}
	if buyerQty != 2 {
		t.Fatalf("buyer drawings = %d, want 2", buyerQty)
	}

	// Buying your own lot and overspending are refused.
	own, err := as.ListUpgrade(ctx, buyer, 1, 1, 100)
	if err != nil {
		t.Fatalf("list own lot: %v", err)
	}
	if _, err := as.BuyLot(ctx, buyer, own.ID); !errors.Is(err, ErrCannotBuyOwnLot) {
		t.Fatalf("buy own = %v, want ErrCannotBuyOwnLot", err)
	}

	// The seller's "Your Auctions" tab lists only their own lots; cancelling one
	// reclaims the drawings, and another account cannot cancel it.
	ownLots, err := as.ListOwnLots(ctx, buyer)
	if err != nil {
		t.Fatalf("list own lots: %v", err)
	}
	var hasOwn bool
	for _, l := range ownLots {
		if l.ID == own.ID {
			hasOwn = true
		}
	}
	if !hasOwn {
		t.Fatalf("own lot %d missing from Your Auctions", own.ID)
	}
	if err := as.RemoveLot(ctx, seller, own.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign remove = %v, want ErrNotFound", err)
	}
	if err := as.RemoveLot(ctx, buyer, own.ID); err != nil {
		t.Fatalf("remove own lot: %v", err)
	}
	if q := itemQty(t, db, buyer, 1); q != 2 {
		t.Fatalf("drawings after remove = %d, want 2", q)
	}

	grant(seller, 1, 1)
	expensive, err := as.ListUpgrade(ctx, seller, 1, 1, 100000)
	if err != nil {
		t.Fatalf("list expensive lot: %v", err)
	}
	if _, err := as.BuyLot(ctx, buyer, expensive.ID); !errors.Is(err, ErrInsufficientAntimatter) {
		t.Fatalf("buy expensive = %v, want ErrInsufficientAntimatter", err)
	}

	// A past-expiry lot is returned to its seller by the engine.
	var expiredID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO market_lots (seller_account_id, upgrade_code, amount, price_atm, expires_at)
		VALUES ($1, 1, 1, 50, NOW() - interval '1 hour')
		RETURNING id
	`, seller).Scan(&expiredID); err != nil {
		t.Fatalf("insert expired lot: %v", err)
	}
	if err := as.ExpireLot(ctx, expiredID); err != nil {
		t.Fatalf("expire lot: %v", err)
	}
	var stillThere bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM market_lots WHERE id = $1)`, expiredID).Scan(&stillThere); err != nil {
		t.Fatalf("check expired lot: %v", err)
	}
	if stillThere {
		t.Fatal("expired lot was not deleted")
	}

	// Listing more drawings than held is refused.
	if _, err := as.ListUpgrade(ctx, seller, 1, 25, 100); !errors.Is(err, ErrInsufficientUpgrades) {
		t.Fatalf("oversell = %v, want ErrInsufficientUpgrades", err)
	}
}

// itemQty reads how many un-activated drawings an account holds for one upgrade.
func itemQty(t *testing.T, db *sql.DB, accountID int64, code int) int {
	t.Helper()
	var qty int
	err := db.QueryRow(`SELECT qty FROM upgrade_items WHERE account_id = $1 AND upgrade_code = $2`, accountID, code).Scan(&qty)
	if errors.Is(err, sql.ErrNoRows) {
		return 0
	}
	if err != nil {
		t.Fatalf("read item qty: %v", err)
	}
	return qty
}

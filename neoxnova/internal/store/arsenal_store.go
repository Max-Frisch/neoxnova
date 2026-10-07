package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
)

// ArsenalStore persists account upgrades (owned levels), un-activated drawings
// and the auction market. All mutating operations are transactional and lock
// the rows they touch, so concurrent activations/buys are safe.
type ArsenalStore struct {
	db *sql.DB
}

func NewArsenalStore(db *sql.DB) *ArsenalStore {
	return &ArsenalStore{db: db}
}

// ArsenalList returns every catalog upgrade enriched with the account's owned
// level/value and the number of un-activated drawings it holds. Catalog order
// (1..19) is preserved.
func (s *ArsenalStore) ArsenalList(ctx context.Context, accountID int64) ([]models.ArsenalUpgrade, error) {
	owned := map[int]models.ArsenalUpgrade{}
	rows, err := s.db.QueryContext(ctx, `SELECT upgrade_code, level, value FROM account_upgrades WHERE account_id = $1`, accountID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var code, level int
		var value float64
		if err := rows.Scan(&code, &level, &value); err != nil {
			rows.Close()
			return nil, err
		}
		owned[code] = models.ArsenalUpgrade{Level: level, Bonus: value}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	items := map[int]int{}
	rows, err = s.db.QueryContext(ctx, `SELECT upgrade_code, qty FROM upgrade_items WHERE account_id = $1`, accountID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var code, qty int
		if err := rows.Scan(&code, &qty); err != nil {
			rows.Close()
			return nil, err
		}
		items[code] = qty
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]models.ArsenalUpgrade, 0, len(game.Upgrades))
	for code := 1; code <= 19; code++ {
		def, ok := game.UpgradeByCode(code)
		if !ok {
			continue
		}
		o := owned[code]
		out = append(out, models.ArsenalUpgrade{
			Code:      def.Code,
			Key:       def.Key,
			Name:      def.Name,
			Group:     def.Group,
			Class:     def.Class,
			Level:     o.Level,
			Bonus:     o.Bonus,
			NextBonus: def.PerLevel,
			Available: items[code],
		})
	}
	return out, nil
}

// Activate consumes one drawing and resolves an activation. roll must be in
// [0,1); passing it in keeps the store deterministic for tests while callers
// own randomness (matching the engine's seeded approach).
func (s *ArsenalStore) Activate(ctx context.Context, accountID int64, code int, roll float64) (models.ArsenalActivateResult, error) {
	def, ok := game.UpgradeByCode(code)
	if !ok {
		return models.ArsenalActivateResult{}, ErrUnknownCode
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return models.ArsenalActivateResult{}, err
	}
	defer tx.Rollback()

	if err := consumeUpgradeItems(ctx, tx, accountID, code, 1); err != nil {
		return models.ArsenalActivateResult{}, err
	}

	var level int
	var value float64
	err = tx.QueryRowContext(ctx, `
		SELECT level, value FROM account_upgrades
		WHERE account_id = $1 AND upgrade_code = $2
		FOR UPDATE
	`, accountID, code).Scan(&level, &value)
	if errors.Is(err, sql.ErrNoRows) {
		level, value = 0, 0
	} else if err != nil {
		return models.ArsenalActivateResult{}, err
	}

	newLevel, newValue, success, chance := game.Activate(def, level, value, roll)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO account_upgrades (account_id, upgrade_code, level, value)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (account_id, upgrade_code)
		DO UPDATE SET level = EXCLUDED.level, value = EXCLUDED.value
	`, accountID, code, newLevel, newValue); err != nil {
		return models.ArsenalActivateResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return models.ArsenalActivateResult{}, err
	}
	return models.ArsenalActivateResult{
		Code: def.Code, Name: def.Name, Success: success,
		Chance: chance, Level: newLevel, Bonus: newValue,
	}, nil
}

// AddUpgradeItems credits un-activated drawings to an account. It runs on the
// caller's transaction so a drop can be committed atomically with the
// expedition/combat that produced it.
func AddUpgradeItems(ctx context.Context, tx *sql.Tx, accountID int64, code, qty int) error {
	if qty <= 0 {
		return nil
	}
	if _, ok := game.UpgradeByCode(code); !ok {
		return ErrUnknownCode
	}
	return addUpgradeItemsTx(ctx, tx, accountID, code, qty)
}

// ListUpgrade lists `amount` drawings on the market at `rate` Antimatter each.
// The lot's stored total price is rate*amount.
func (s *ArsenalStore) ListUpgrade(ctx context.Context, accountID int64, code, amount int, rate int64) (models.MarketLot, error) {
	def, ok := game.UpgradeByCode(code)
	if !ok {
		return models.MarketLot{}, ErrUnknownCode
	}
	if amount < 1 || amount > 25 {
		return models.MarketLot{}, ErrInvalidQuantity
	}
	if rate < 1 || rate > 1_000_000 {
		return models.MarketLot{}, ErrInvalidQuantity
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return models.MarketLot{}, err
	}
	defer tx.Rollback()

	if err := consumeUpgradeItems(ctx, tx, accountID, code, amount); err != nil {
		return models.MarketLot{}, err
	}

	total := rate * int64(amount)
	var lot models.MarketLot
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO market_lots (seller_account_id, upgrade_code, amount, price_atm)
		VALUES ($1, $2, $3, $4)
		RETURNING id, expires_at
	`, accountID, code, amount, total).Scan(&lot.ID, &lot.ExpiresAt); err != nil {
		return models.MarketLot{}, err
	}

	if err := tx.Commit(); err != nil {
		return models.MarketLot{}, err
	}
	lot.UpgradeCode, lot.Upgrade, lot.Amount, lot.TotalPriceAtm = def.Code, def.Name, amount, total
	return lot, nil
}

// ListMarket returns every live (unexpired) lot, cheapest first.
func (s *ArsenalStore) ListMarket(ctx context.Context) ([]models.MarketLot, error) {
	return s.queryLots(ctx, `
		SELECT id, upgrade_code, amount, price_atm, expires_at
		FROM market_lots
		WHERE expires_at > NOW()
		ORDER BY price_atm ASC, id ASC
	`)
}

// ListOwnLots returns an account's live lots (the "Your Auctions" tab).
func (s *ArsenalStore) ListOwnLots(ctx context.Context, accountID int64) ([]models.MarketLot, error) {
	return s.queryLots(ctx, `
		SELECT id, upgrade_code, amount, price_atm, expires_at
		FROM market_lots
		WHERE seller_account_id = $1 AND expires_at > NOW()
		ORDER BY id ASC
	`, accountID)
}

func (s *ArsenalStore) queryLots(ctx context.Context, query string, args ...any) ([]models.MarketLot, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.MarketLot{}
	for rows.Next() {
		var lot models.MarketLot
		if err := rows.Scan(&lot.ID, &lot.UpgradeCode, &lot.Amount, &lot.TotalPriceAtm, &lot.ExpiresAt); err != nil {
			return nil, err
		}
		if def, ok := game.UpgradeByCode(lot.UpgradeCode); ok {
			lot.Upgrade = def.Name
		}
		out = append(out, lot)
	}
	return out, rows.Err()
}

// BuyLot atomically charges the buyer Antimatter, credits the seller, transfers
// the drawings and removes the lot.
func (s *ArsenalStore) BuyLot(ctx context.Context, buyerID, lotID int64) (models.MarketLot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return models.MarketLot{}, err
	}
	defer tx.Rollback()

	var lot models.MarketLot
	var sellerID int64
	err = tx.QueryRowContext(ctx, `
		SELECT seller_account_id, upgrade_code, amount, price_atm, expires_at
		FROM market_lots WHERE id = $1 FOR UPDATE
	`, lotID).Scan(&sellerID, &lot.UpgradeCode, &lot.Amount, &lot.TotalPriceAtm, &lot.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.MarketLot{}, ErrNotFound
	} else if err != nil {
		return models.MarketLot{}, err
	}
	if time.Now().After(lot.ExpiresAt) {
		// Return the drawings and drop the dead lot in this same transaction.
		if err := addUpgradeItemsTx(ctx, tx, sellerID, lot.UpgradeCode, lot.Amount); err != nil {
			return models.MarketLot{}, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM market_lots WHERE id = $1`, lotID); err != nil {
			return models.MarketLot{}, err
		}
		if err := tx.Commit(); err != nil {
			return models.MarketLot{}, err
		}
		return models.MarketLot{}, ErrLotExpired
	}
	if sellerID == buyerID {
		return models.MarketLot{}, ErrCannotBuyOwnLot
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE users SET antimatter = antimatter - $1 WHERE id = $2 AND antimatter >= $1
	`, lot.TotalPriceAtm, buyerID)
	if err != nil {
		return models.MarketLot{}, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return models.MarketLot{}, err
	} else if n == 0 {
		return models.MarketLot{}, ErrInsufficientAntimatter
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET antimatter = antimatter + $1 WHERE id = $2`, lot.TotalPriceAtm, sellerID); err != nil {
		return models.MarketLot{}, err
	}
	if err := addUpgradeItemsTx(ctx, tx, buyerID, lot.UpgradeCode, lot.Amount); err != nil {
		return models.MarketLot{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO market_purchases (lot_id, buyer_account_id, seller_account_id, upgrade_code, amount, price_atm)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, lotID, buyerID, sellerID, lot.UpgradeCode, lot.Amount, lot.TotalPriceAtm); err != nil {
		return models.MarketLot{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM market_lots WHERE id = $1`, lotID); err != nil {
		return models.MarketLot{}, err
	}

	if err := tx.Commit(); err != nil {
		return models.MarketLot{}, err
	}
	lot.ID = lotID
	if def, ok := game.UpgradeByCode(lot.UpgradeCode); ok {
		lot.Upgrade = def.Name
	}
	return lot, nil
}

// RemoveLot cancels one of the caller's own live lots and returns its drawings.
// Lots owned by anyone else are reported as not found (ownership concealed).
func (s *ArsenalStore) RemoveLot(ctx context.Context, accountID, lotID int64) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var sellerID int64
	var code, amount int
	err = tx.QueryRowContext(ctx, `
		SELECT seller_account_id, upgrade_code, amount
		FROM market_lots WHERE id = $1 FOR UPDATE
	`, lotID).Scan(&sellerID, &code, &amount)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if sellerID != accountID {
		return ErrNotFound
	}
	if err := addUpgradeItemsTx(ctx, tx, sellerID, code, amount); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM market_lots WHERE id = $1`, lotID); err != nil {
		return err
	}
	return tx.Commit()
}

// ExpireLot returns an expired lot's drawings to the seller and deletes it. It
// is idempotent: a lot claimed by a buyer is simply skipped.
func (s *ArsenalStore) ExpireLot(ctx context.Context, lotID int64) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var sellerID int64
	var code, amount int
	var expiresAt time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT seller_account_id, upgrade_code, amount, expires_at
		FROM market_lots WHERE id = $1 FOR UPDATE
	`, lotID).Scan(&sellerID, &code, &amount, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	if time.Now().Before(expiresAt) {
		return nil
	}
	if err := addUpgradeItemsTx(ctx, tx, sellerID, code, amount); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM market_lots WHERE id = $1`, lotID); err != nil {
		return err
	}
	return tx.Commit()
}

// consumeUpgradeItems removes qty drawings, locking the row first. It fails
// with ErrInsufficientUpgrades when the account holds too few.
func consumeUpgradeItems(ctx context.Context, tx *sql.Tx, accountID int64, code, qty int) error {
	var have int
	err := tx.QueryRowContext(ctx, `
		SELECT qty FROM upgrade_items
		WHERE account_id = $1 AND upgrade_code = $2
		FOR UPDATE
	`, accountID, code).Scan(&have)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && have < qty) {
		return ErrInsufficientUpgrades
	} else if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE upgrade_items SET qty = qty - $3
		WHERE account_id = $1 AND upgrade_code = $2 AND qty >= $3
	`, accountID, code, qty); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM upgrade_items WHERE account_id = $1 AND upgrade_code = $2 AND qty <= 0
	`, accountID, code); err != nil {
		return err
	}
	return nil
}

// addUpgradeItemsTx upserts a drawing count onto an account.
func addUpgradeItemsTx(ctx context.Context, tx *sql.Tx, accountID int64, code, qty int) error {
	if qty <= 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO upgrade_items (account_id, upgrade_code, qty)
		VALUES ($1, $2, $3)
		ON CONFLICT (account_id, upgrade_code)
		DO UPDATE SET qty = upgrade_items.qty + EXCLUDED.qty
	`, accountID, code, qty)
	return err
}

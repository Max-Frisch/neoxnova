package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"neoxnova/internal/game"
)

// TradeStore performs Resource Trader exchanges (Black market -> Resource
// Trader). Trades are transactional, accrue the celestial's resources first and
// debit the merchant's Dark Matter fee, so concurrent trades serialize on the
// celestial row lock.
type TradeStore struct {
	db *sql.DB
}

func NewTradeStore(db *sql.DB) *TradeStore {
	return &TradeStore{db: db}
}

// tradeColumns maps a tradable resource id to its celestial_objects column.
// Only whitelisted identifiers ever reach the dynamically-built UPDATE.
var tradeColumns = map[int]string{
	game.ResourceMetal:     "metal",
	game.ResourceCrystal:   "crystal",
	game.ResourceDeuterium: "deuterium",
}

// ExecuteTrade runs one priced exchange for a celestial the caller owns: it
// accrues production, checks the balance of the sold resource, debits it and
// the fixed Dark Matter fee, and credits the bought resource clamped to its
// storage capacity (excess above the cap is lost, matching the live clamp).
func (s *TradeStore) ExecuteTrade(ctx context.Context, celestialID, userID int64, quote game.TradeOffer) error {
	sellCol, ok := tradeColumns[quote.SellCode]
	if !ok {
		return ErrUnknownCode
	}
	buyCol, ok := tradeColumns[quote.BuyCode]
	if !ok || quote.BuyCode == quote.SellCode {
		return ErrUnknownCode
	}
	if quote.BuyAmount <= 0 || quote.SellCost < 0 {
		return ErrInvalidQuantity
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Accrue and row-lock first so the balance we read is final for this trade.
	if err := tx.QueryRowContext(ctx, `SELECT * FROM update_celestial_resources($1)`, celestialID).
		Scan(new(float64), new(float64), new(float64), new(any)); err != nil {
		return err
	}

	var (
		owner                         sql.NullInt64
		metal, crystal, deuterium     float64
		metalCap, crystalCap, deutCap int64
	)
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, metal, crystal, deuterium,
		       metal_capacity, crystal_capacity, deuterium_capacity
		FROM celestial_objects WHERE id = $1 FOR UPDATE
	`, celestialID).Scan(&owner, &metal, &crystal, &deuterium, &metalCap, &crystalCap, &deutCap)
	if err == sql.ErrNoRows || (err == nil && (!owner.Valid || owner.Int64 != userID)) {
		return ErrNotFound
	} else if err != nil {
		return err
	}

	balance := map[int]float64{
		game.ResourceMetal:     metal,
		game.ResourceCrystal:   crystal,
		game.ResourceDeuterium: deuterium,
	}
	caps := map[int]int64{
		game.ResourceMetal:     metalCap,
		game.ResourceCrystal:   crystalCap,
		game.ResourceDeuterium: deutCap,
	}
	if balance[quote.SellCode] < float64(quote.SellCost) {
		return ErrInsufficientResources
	}

	// Debit the sold resource, then credit the bought one clamped to capacity.
	newSell := balance[quote.SellCode] - float64(quote.SellCost)
	newBuy := math.Min(float64(caps[quote.BuyCode]), balance[quote.BuyCode]+float64(quote.BuyAmount))
	for col, val := range map[string]float64{sellCol: newSell, buyCol: newBuy} {
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`UPDATE celestial_objects SET %s = $1 WHERE id = $2`, col), val, celestialID); err != nil {
			return err
		}
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE users SET dark_matter = dark_matter - $1 WHERE id = $2 AND dark_matter >= $1
	`, quote.DarkMatter, userID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrInsufficientDarkMatter
	}
	return tx.Commit()
}

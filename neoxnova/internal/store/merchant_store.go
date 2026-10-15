package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"neoxnova/internal/game"
)

// ScrapUnits sells quantity of one ship or defense the caller owns on a
// celestial to the Merchant of Fleets (Black market -> Merchant of Fleets). It
// accrues production and row-locks the celestial, removes the units, credits the
// refund (merchant_rate percent of the batch's base cost, clamped to storage)
// and rebuilds the cached rates so solar-satellite energy stays consistent. It
// returns the resources paid and the merchant rate used.
func (s *TradeStore) ScrapUnits(ctx context.Context, celestialID, userID int64, unitCode string, quantity int64) (game.Cost, int, error) {
	def, ok := game.LookupUnit(unitCode)
	if !ok {
		return game.Cost{}, 0, ErrUnknownCode
	}
	if quantity < 1 {
		return game.Cost{}, 0, ErrInvalidQuantity
	}

	// Ships and defenses live in separate tables; the names are fixed literals.
	table, codeColumn := "planet_ships", "ship_code"
	if _, isShip := game.Ships[unitCode]; !isShip {
		table, codeColumn = "planet_defenses", "defense_code"
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return game.Cost{}, 0, err
	}
	defer tx.Rollback()

	// Accrue and row-lock first so the balances we read are final.
	if err := tx.QueryRowContext(ctx, `SELECT * FROM update_celestial_resources($1)`, celestialID).
		Scan(new(float64), new(float64), new(float64), new(any)); err != nil {
		return game.Cost{}, 0, err
	}

	var (
		owner                         sql.NullInt64
		metal, crystal, deuterium     float64
		metalCap, crystalCap, deutCap int64
		rate                          int
	)
	err = tx.QueryRowContext(ctx, `
		SELECT c.user_id, c.metal, c.crystal, c.deuterium,
		       c.metal_capacity, c.crystal_capacity, c.deuterium_capacity, u.merchant_rate
		FROM celestial_objects c
		JOIN universes u ON u.id = c.universe_id
		WHERE c.id = $1
		FOR UPDATE OF c
	`, celestialID).Scan(&owner, &metal, &crystal, &deuterium, &metalCap, &crystalCap, &deutCap, &rate)
	if err == sql.ErrNoRows || (err == nil && (!owner.Valid || owner.Int64 != userID)) {
		return game.Cost{}, 0, ErrNotFound
	} else if err != nil {
		return game.Cost{}, 0, err
	}

	var held int64
	q := fmt.Sprintf(`SELECT quantity FROM %s WHERE celestial_id = $1 AND %s = $2 FOR UPDATE`, table, codeColumn)
	switch err := tx.QueryRowContext(ctx, q, celestialID, unitCode).Scan(&held); {
	case err == sql.ErrNoRows:
		return game.Cost{}, 0, ErrInsufficientShips
	case err != nil:
		return game.Cost{}, 0, err
	}
	if held < quantity {
		return game.Cost{}, 0, ErrInsufficientShips
	}

	refund := game.MerchantRefund(def.BaseCost.Mul(quantity), rate)

	upd := fmt.Sprintf(`UPDATE %s SET quantity = quantity - $1 WHERE celestial_id = $2 AND %s = $3`, table, codeColumn)
	if _, err := tx.ExecContext(ctx, upd, quantity, celestialID, unitCode); err != nil {
		return game.Cost{}, 0, err
	}

	// Credit the refund clamped to storage capacity (excess above the cap is lost).
	m := math.Min(float64(metalCap), metal+float64(refund.Metal))
	cr := math.Min(float64(crystalCap), crystal+float64(refund.Crystal))
	d := math.Min(float64(deutCap), deuterium+float64(refund.Deuterium))
	if _, err := tx.ExecContext(ctx, `
		UPDATE celestial_objects SET metal = $1, crystal = $2, deuterium = $3 WHERE id = $4
	`, m, cr, d, celestialID); err != nil {
		return game.Cost{}, 0, err
	}

	if err := NewBuildStore(s.db).RecomputeCelestial(ctx, tx, celestialID); err != nil {
		return game.Cost{}, 0, err
	}

	if err := tx.Commit(); err != nil {
		return game.Cost{}, 0, err
	}
	return refund, rate, nil
}

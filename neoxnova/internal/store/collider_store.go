package store

import (
	"context"
	"database/sql"
	"errors"

	"neoxnova/internal/game"
)

// ColliderStatus is the Moon Collider state shown on its page: the current
// level and Dark-Matter-per-hour output, plus the next level's price and output.
type ColliderStatus struct {
	MoonID            int64
	Level             int
	ProductionPerHour int64 // current level (0 at level 0)
	NextLevel         int
	NextProduction    int64
	NextCostAtm       int64
	Antimatter        int64
}

// ColliderStatusFor returns the Collider state for one owned moon. It returns
// ErrNotFound for a missing celestial, a non-moon, or one the user does not own.
func (s *MoonStore) ColliderStatusFor(ctx context.Context, userID, moonID int64) (ColliderStatus, error) {
	var (
		owner sql.NullInt64
		otype string
		level int
		anti  int64
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT c.user_id, c.object_type::text, c.collider_level,
		       COALESCE(u.antimatter, 0)
		FROM celestial_objects c
		LEFT JOIN users u ON u.id = c.user_id
		WHERE c.id = $1
	`, moonID).Scan(&owner, &otype, &level, &anti)
	if errors.Is(err, sql.ErrNoRows) {
		return ColliderStatus{}, ErrNotFound
	} else if err != nil {
		return ColliderStatus{}, err
	}
	if !owner.Valid || owner.Int64 != userID || otype != "MOON" {
		return ColliderStatus{}, ErrNotFound
	}
	return colliderStatusFrom(moonID, level, anti), nil
}

func colliderStatusFrom(moonID int64, level int, anti int64) ColliderStatus {
	next := level + 1
	return ColliderStatus{
		MoonID:            moonID,
		Level:             level,
		ProductionPerHour: game.ColliderProduction(level),
		NextLevel:         next,
		NextProduction:    game.ColliderProduction(next),
		NextCostAtm:       game.ColliderCost(next),
		Antimatter:        anti,
	}
}

// UpgradeCollider raises a moon's Collider one level by spending Antimatter
// (account-wide users.antimatter). It banks accrued Dark Matter at the old rate
// first, then applies the level and recomputes the celestial + account DM rate.
// Returns ErrNotFound for a missing/non-owned/non-moon celestial and
// ErrInsufficientAntimatter when the account cannot cover the price. Safe under
// concurrent calls (the celestial row is locked).
func (s *MoonStore) UpgradeCollider(ctx context.Context, userID, moonID int64) (ColliderStatus, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return ColliderStatus{}, err
	}
	defer tx.Rollback()

	var (
		owner sql.NullInt64
		otype string
		level int
	)
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, object_type::text, collider_level
		FROM celestial_objects WHERE id = $1 FOR UPDATE
	`, moonID).Scan(&owner, &otype, &level)
	if errors.Is(err, sql.ErrNoRows) {
		return ColliderStatus{}, ErrNotFound
	} else if err != nil {
		return ColliderStatus{}, err
	}
	if !owner.Valid || owner.Int64 != userID || otype != "MOON" {
		return ColliderStatus{}, ErrNotFound
	}

	// Bank Dark Matter at the old rate before the rate changes (also locks the
	// user row so the antimatter check and debit are concurrent-safe).
	if err := accrueUserDarkMatterTx(ctx, tx, userID); err != nil {
		return ColliderStatus{}, err
	}

	cost := game.ColliderCost(level + 1)
	var anti int64
	if err := tx.QueryRowContext(ctx, `SELECT antimatter FROM users WHERE id = $1`, userID).Scan(&anti); err != nil {
		return ColliderStatus{}, err
	}
	if anti < cost {
		return ColliderStatus{}, ErrInsufficientAntimatter
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET antimatter = antimatter - $1 WHERE id = $2`, cost, userID); err != nil {
		return ColliderStatus{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE celestial_objects SET collider_level = $1 WHERE id = $2
	`, level+1, moonID); err != nil {
		return ColliderStatus{}, err
	}
	// RecomputeCelestial rewrites collider_dm_prod_hourly and re-sums the
	// account's DM/h rate, so the new level's output begins accruing now.
	if err := s.builds.RecomputeCelestial(ctx, tx, moonID); err != nil {
		return ColliderStatus{}, err
	}
	var newAnti int64
	if err := tx.QueryRowContext(ctx, `SELECT antimatter FROM users WHERE id = $1`, userID).Scan(&newAnti); err != nil {
		return ColliderStatus{}, err
	}
	if err := tx.Commit(); err != nil {
		return ColliderStatus{}, err
	}
	return colliderStatusFrom(moonID, level+1, newAnti), nil
}

// accrueUserDarkMatterTx banks the account's Dark Matter inside a transaction.
func accrueUserDarkMatterTx(ctx context.Context, tx *sql.Tx, userID int64) error {
	var dm int64
	var rate float64
	return tx.QueryRowContext(ctx,
		`SELECT out_dark_matter, out_prod_hourly FROM update_user_dark_matter($1)`,
		userID).Scan(&dm, &rate)
}

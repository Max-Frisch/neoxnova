package store

import (
	"context"
	"database/sql"
)

// LoadAccountUpgrades returns an account's owned Arsenal upgrade values keyed by
// upgrade code. The value is the accumulated bonus percent (account_upgrades.value).
// Only positive values are returned. It runs on the caller's transaction so it
// composes with combat/production resolution.
func LoadAccountUpgrades(ctx context.Context, tx *sql.Tx, accountID int64) (map[int]float64, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT upgrade_code, value FROM account_upgrades
		WHERE account_id = $1 AND value > 0
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int]float64{}
	for rows.Next() {
		var code int
		var value float64
		if err := rows.Scan(&code, &value); err != nil {
			return nil, err
		}
		out[code] = value
	}
	return out, rows.Err()
}

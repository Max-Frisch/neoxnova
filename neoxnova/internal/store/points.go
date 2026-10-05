package store

import (
	"context"
	"database/sql"

	"neoxnova/internal/game"
)

// PlayerPoints estimates a player's score the way the reference server ranks
// players: the summed resource cost of current structures, technologies, ships
// and defenses. It is an approximation (in-flight fleets are not counted), which
// is enough to enforce the ~4:1 noob-protection gate.
func PlayerPoints(ctx context.Context, tx *sql.Tx, userID int64) (int64, error) {
	var total int64
	add := func(c game.Cost) { total += c.Metal + c.Crystal + c.Deuterium }

	rows, err := tx.QueryContext(ctx, `
		SELECT ps.structure_code, ps.level
		FROM planet_structures ps
		JOIN celestial_objects c ON c.id = ps.celestial_id
		WHERE c.user_id = $1 AND ps.level > 0
	`, userID)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var code string
		var level int
		if err := rows.Scan(&code, &level); err != nil {
			rows.Close()
			return 0, err
		}
		if cost, ok := game.StructureCost(code, level); ok {
			add(cost)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	rows, err = tx.QueryContext(ctx, `SELECT tech_code, level FROM user_technologies WHERE user_id = $1 AND level > 0`, userID)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var code string
		var level int
		if err := rows.Scan(&code, &level); err != nil {
			rows.Close()
			return 0, err
		}
		if cost, ok := game.TechCost(code, level); ok {
			add(cost)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, q := range []struct{ table, col string }{
		{"planet_ships", "ship_code"},
		{"planet_defenses", "defense_code"},
	} {
		rows, err = tx.QueryContext(ctx, `
			SELECT u.`+q.col+`, u.quantity
			FROM `+q.table+` u
			JOIN celestial_objects c ON c.id = u.celestial_id
			WHERE c.user_id = $1 AND u.quantity > 0
		`, userID)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var code string
			var qty int64
			if err := rows.Scan(&code, &qty); err != nil {
				rows.Close()
				return 0, err
			}
			if cost, ok := game.UnitCost(code, qty); ok {
				add(cost)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return 0, err
		}
	}

	return total, nil
}

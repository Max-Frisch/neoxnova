package store

import (
	"context"
	"database/sql"

	"neoxnova/internal/config"
)

// ApplyGameSettings writes the boot-time tuning from config/game.json onto the
// universe row, making the DB the runtime source of truth. It returns false
// with no error when the universe has not been seeded yet, so a fresh database
// does not crash startup (the scheduler tolerates a missing universe too).
func ApplyGameSettings(ctx context.Context, db *sql.DB, codeName string, s config.GameSettings) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE universes
		SET game_speed = $2, resource_speed = $3, fleet_speed = $4,
		    build_queue_slots = $5, research_queue_slots = $6
		WHERE code_name = $1
	`, codeName, s.GameSpeed, s.ResourceSpeed, s.FleetSpeed, s.BuildQueueSlots, s.ResearchQueueSlots)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

package engine

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	"neoxnova/internal/cache"
	"neoxnova/internal/game"
	"neoxnova/internal/store"
)

// EventEngine is a durable, at-least-once scheduler. Postgres is the source of
// truth: every tick it claims due rows with FOR UPDATE SKIP LOCKED (safe for
// multiple instances) and resolves them idempotently. Redis is an optional
// wake-up accelerator only — correctness never depends on it.
type EventEngine struct {
	rdb    *redis.Client
	db     *sql.DB
	builds *store.BuildStore
}

func NewEventEngine(rdb *redis.Client, db *sql.DB) *EventEngine {
	return &EventEngine{rdb: rdb, db: db, builds: store.NewBuildStore(db)}
}

// StartScheduler runs the resolution loop until the context is cancelled.
func (e *EventEngine) StartScheduler(ctx context.Context, universeID string) {
	log.Printf("[SCHEDULER] Started durable event scheduler for universe %s", universeID)

	wake := make(chan struct{}, 1)
	go e.watchWake(ctx, cache.WakeKey(universeID), wake)

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[SCHEDULER] Shutting down event scheduler gracefully.")
			return
		case <-ticker.C:
			e.processDue(ctx, universeID)
		case <-wake:
			e.processDue(ctx, universeID)
		}
	}
}

// watchWake blocks on Redis for a low-latency nudge from API handlers. It is
// best-effort: if Redis is unavailable the ticker still drives the scheduler.
func (e *EventEngine) watchWake(ctx context.Context, key string, out chan<- struct{}) {
	if e.rdb == nil {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		_, err := e.rdb.BLPop(ctx, 2*time.Second, key).Result()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if err != redis.Nil {
				time.Sleep(2 * time.Second)
			}
			continue
		}
		select {
		case out <- struct{}{}:
		default:
		}
	}
}

func (e *EventEngine) processDue(ctx context.Context, universeID string) {
	for _, id := range e.dueIDs(ctx, `
		SELECT id FROM fleets
		WHERE phase IN ('OUTBOUND', 'HOLDING', 'RETURNING') AND arrival_time <= NOW()
		ORDER BY arrival_time LIMIT 200`) {
		e.resolveFleetEvent(ctx, id)
	}
	for _, id := range e.dueIDs(ctx, `
		SELECT id FROM construction_queues
		WHERE status = 'IN_PROGRESS' AND end_time <= NOW()
		ORDER BY end_time LIMIT 200`) {
		e.resolveConstruction(ctx, id)
	}
	for _, id := range e.dueIDs(ctx, `
		SELECT id FROM shipyard_queues
		WHERE status = 'IN_PROGRESS' AND end_time <= NOW()
		ORDER BY end_time LIMIT 200`) {
		e.resolveShipyard(ctx, id)
	}
	for _, id := range e.dueIDs(ctx, `
		SELECT id FROM research_queues
		WHERE status = 'IN_PROGRESS' AND end_time <= NOW()
		ORDER BY end_time LIMIT 200`) {
		e.resolveResearch(ctx, id)
	}
	_ = universeID
}

// dueIDs collects candidate ids without locking; each resolver re-locks and
// re-checks its own row, so a candidate may be skipped if another worker won.
func (e *EventEngine) dueIDs(ctx context.Context, query string) []int64 {
	rows, err := e.db.QueryContext(ctx, query)
	if err != nil {
		log.Printf("[ERROR] Scheduler due-query failed: %v", err)
		return nil
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			log.Printf("[ERROR] Scheduler row scan failed: %v", err)
			return ids
		}
		ids = append(ids, id)
	}
	return ids
}

// resolveFleetEvent resolves one due fleet atomically.
func (e *EventEngine) resolveFleetEvent(ctx context.Context, fleetID int64) {
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		log.Printf("[ERROR] Tx begin error: %v", err)
		return
	}
	defer tx.Rollback()

	query := `
		SELECT id, user_id, mission, phase, origin_id, target_id,
		       cargo_metal, cargo_crystal, cargo_deuterium,
		       holding_end_time, return_time
		FROM fleets
		WHERE id = $1 AND phase IN ('OUTBOUND', 'HOLDING', 'RETURNING') AND arrival_time <= NOW()
		FOR UPDATE SKIP LOCKED;
	`
	var (
		id, userID, originID                int64
		targetID                            sql.NullInt64
		cargoMetal, cargoCrystal, cargoDeut int64
		mission, phase                      string
		holdingEndTime, returnTime          sql.NullTime
	)

	err = tx.QueryRowContext(ctx, query, fleetID).Scan(
		&id, &userID, &mission, &phase, &originID, &targetID,
		&cargoMetal, &cargoCrystal, &cargoDeut,
		&holdingEndTime, &returnTime,
	)
	if err == sql.ErrNoRows {
		return // claimed/completed elsewhere
	} else if err != nil {
		log.Printf("[ERROR] Fleet lock acquisition failed for fleet #%d: %v", fleetID, err)
		return
	}

	switch phase {
	case "OUTBOUND":
		switch mission {
		case "EXPEDITION":
			if !holdingEndTime.Valid {
				log.Printf("[ERROR] Fleet #%d has null holding_end_time for expedition", id)
				return
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE fleets SET phase = 'HOLDING', arrival_time = holding_end_time WHERE id = $1
			`, id); err != nil {
				log.Printf("[ERROR] Failed to transition fleet #%d to HOLDING: %v", id, err)
				return
			}

		case "DEPLOY":
			if !targetID.Valid {
				log.Printf("[ERROR] Fleet #%d has no target celestial for DEPLOY", id)
				return
			}
			if err := e.accrueResources(ctx, tx, targetID.Int64); err != nil {
				log.Printf("[ERROR] Failed to accrue resources for fleet #%d: %v", id, err)
				return
			}
			if err := e.creditCargo(ctx, tx, targetID.Int64, cargoMetal, cargoCrystal, cargoDeut); err != nil {
				log.Printf("[ERROR] Failed to unload cargo for fleet #%d: %v", id, err)
				return
			}
			if err := e.stationShips(ctx, tx, targetID.Int64, id); err != nil {
				log.Printf("[ERROR] Failed to station ships for fleet #%d: %v", id, err)
				return
			}
			if _, err := tx.ExecContext(ctx, `UPDATE fleets SET phase = 'RESOLVED' WHERE id = $1`, id); err != nil {
				log.Printf("[ERROR] Failed to resolve fleet #%d: %v", id, err)
				return
			}

		case "TRANSPORT":
			if !targetID.Valid {
				log.Printf("[ERROR] Fleet #%d has no target celestial for TRANSPORT", id)
				return
			}
			if err := e.accrueResources(ctx, tx, targetID.Int64); err != nil {
				log.Printf("[ERROR] Failed to accrue resources for fleet #%d: %v", id, err)
				return
			}
			if err := e.creditCargo(ctx, tx, targetID.Int64, cargoMetal, cargoCrystal, cargoDeut); err != nil {
				log.Printf("[ERROR] Failed to unload transport cargo for fleet #%d: %v", id, err)
				return
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE fleets
				SET phase = 'RETURNING', arrival_time = return_time,
				    cargo_metal = 0, cargo_crystal = 0, cargo_deuterium = 0
				WHERE id = $1
			`, id); err != nil {
				log.Printf("[ERROR] Failed to set fleet #%d to RETURNING: %v", id, err)
				return
			}

		default:
			if _, err := tx.ExecContext(ctx, `
				UPDATE fleets SET phase = 'RETURNING', arrival_time = return_time WHERE id = $1
			`, id); err != nil {
				log.Printf("[ERROR] Failed to set fleet #%d to RETURNING: %v", id, err)
				return
			}
		}

	case "HOLDING":
		if _, err := tx.ExecContext(ctx, `
			UPDATE fleets SET phase = 'RETURNING', arrival_time = return_time WHERE id = $1
		`, id); err != nil {
			log.Printf("[ERROR] Failed to transition fleet #%d from HOLDING to RETURNING: %v", id, err)
			return
		}

	case "RETURNING":
		if err := e.accrueResources(ctx, tx, originID); err != nil {
			log.Printf("[ERROR] Failed to accrue resources for fleet #%d: %v", id, err)
			return
		}
		if err := e.creditCargo(ctx, tx, originID, cargoMetal, cargoCrystal, cargoDeut); err != nil {
			log.Printf("[ERROR] Failed to deposit cargo for fleet #%d: %v", id, err)
			return
		}
		if err := e.stationShips(ctx, tx, originID, id); err != nil {
			log.Printf("[ERROR] Failed to restore ships for fleet #%d: %v", id, err)
			return
		}
		if _, err := tx.ExecContext(ctx, `UPDATE fleets SET phase = 'RESOLVED' WHERE id = $1`, id); err != nil {
			log.Printf("[ERROR] Failed to resolve returning fleet #%d: %v", id, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[ERROR] Tx commit failed for fleet #%d: %v", fleetID, err)
		return
	}
	log.Printf("[EVENT RESOLVED] Fleet #%d phase '%s' processed", fleetID, phase)
}

// resolveConstruction completes a due building level.
func (e *EventEngine) resolveConstruction(ctx context.Context, queueID int64) {
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		log.Printf("[ERROR] Tx begin error: %v", err)
		return
	}
	defer tx.Rollback()

	var celestialID int64
	var code string
	var targetLevel int
	err = tx.QueryRowContext(ctx, `
		SELECT celestial_id, structure_code, target_level
		FROM construction_queues
		WHERE id = $1 AND status = 'IN_PROGRESS' AND end_time <= NOW()
		FOR UPDATE SKIP LOCKED
	`, queueID).Scan(&celestialID, &code, &targetLevel)
	if err == sql.ErrNoRows {
		return
	} else if err != nil {
		log.Printf("[ERROR] Construction lock failed for queue #%d: %v", queueID, err)
		return
	}

	if err := e.accrueResources(ctx, tx, celestialID); err != nil {
		log.Printf("[ERROR] Failed to accrue before construction #%d: %v", queueID, err)
		return
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO planet_structures (celestial_id, structure_code, level)
		VALUES ($1, $2, $3)
		ON CONFLICT (celestial_id, structure_code)
		DO UPDATE SET level = EXCLUDED.level
	`, celestialID, code, targetLevel); err != nil {
		log.Printf("[ERROR] Failed to apply structure for queue #%d: %v", queueID, err)
		return
	}
	if err := e.builds.RecomputeCelestial(ctx, tx, celestialID); err != nil {
		log.Printf("[ERROR] Failed to recompute celestial %d: %v", celestialID, err)
		return
	}
	if _, err := tx.ExecContext(ctx, `UPDATE construction_queues SET status = 'COMPLETED' WHERE id = $1`, queueID); err != nil {
		log.Printf("[ERROR] Failed to complete construction queue #%d: %v", queueID, err)
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[ERROR] Tx commit failed for construction #%d: %v", queueID, err)
		return
	}
	log.Printf("[EVENT RESOLVED] Construction queue #%d: %s -> level %d on celestial %d", queueID, code, targetLevel, celestialID)
}

// resolveShipyard completes a due ship batch.
func (e *EventEngine) resolveShipyard(ctx context.Context, queueID int64) {
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		log.Printf("[ERROR] Tx begin error: %v", err)
		return
	}
	defer tx.Rollback()

	var celestialID, quantity int64
	var unitCode string
	err = tx.QueryRowContext(ctx, `
		SELECT celestial_id, unit_code, quantity_total
		FROM shipyard_queues
		WHERE id = $1 AND status = 'IN_PROGRESS' AND end_time <= NOW()
		FOR UPDATE SKIP LOCKED
	`, queueID).Scan(&celestialID, &unitCode, &quantity)
	if err == sql.ErrNoRows {
		return
	} else if err != nil {
		log.Printf("[ERROR] Shipyard lock failed for queue #%d: %v", queueID, err)
		return
	}

	table := "planet_ships"
	codeColumn := "ship_code"
	if _, ok := game.Ships[unitCode]; !ok {
		table = "planet_defenses"
		codeColumn = "defense_code"
	}

	// Table/column names are selected from fixed literals above, never user input.
	insert := "INSERT INTO " + table + " (celestial_id, " + codeColumn + ", quantity) VALUES ($1, $2, $3) " +
		"ON CONFLICT (celestial_id, " + codeColumn + ") DO UPDATE SET quantity = " + table + ".quantity + EXCLUDED.quantity"
	if _, err := tx.ExecContext(ctx, insert, celestialID, unitCode, quantity); err != nil {
		log.Printf("[ERROR] Failed to station ships for queue #%d: %v", queueID, err)
		return
	}
	if _, err := tx.ExecContext(ctx, `UPDATE shipyard_queues SET status = 'COMPLETED', quantity_completed = quantity_total WHERE id = $1`, queueID); err != nil {
		log.Printf("[ERROR] Failed to complete shipyard queue #%d: %v", queueID, err)
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[ERROR] Tx commit failed for shipyard #%d: %v", queueID, err)
		return
	}
	log.Printf("[EVENT RESOLVED] Shipyard queue #%d: %d x %s stationed on celestial %d", queueID, quantity, unitCode, celestialID)
}

// resolveResearch completes a due technology.
func (e *EventEngine) resolveResearch(ctx context.Context, queueID int64) {
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		log.Printf("[ERROR] Tx begin error: %v", err)
		return
	}
	defer tx.Rollback()

	var userID int64
	var techCode string
	var targetLevel int
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, tech_code, target_level
		FROM research_queues
		WHERE id = $1 AND status = 'IN_PROGRESS' AND end_time <= NOW()
		FOR UPDATE SKIP LOCKED
	`, queueID).Scan(&userID, &techCode, &targetLevel)
	if err == sql.ErrNoRows {
		return
	} else if err != nil {
		log.Printf("[ERROR] Research lock failed for queue #%d: %v", queueID, err)
		return
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_technologies (user_id, tech_code, level)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, tech_code)
		DO UPDATE SET level = EXCLUDED.level
	`, userID, techCode, targetLevel); err != nil {
		log.Printf("[ERROR] Failed to apply research for queue #%d: %v", queueID, err)
		return
	}
	if _, err := tx.ExecContext(ctx, `UPDATE research_queues SET status = 'COMPLETED' WHERE id = $1`, queueID); err != nil {
		log.Printf("[ERROR] Failed to complete research queue #%d: %v", queueID, err)
		return
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[ERROR] Tx commit failed for research #%d: %v", queueID, err)
		return
	}
	log.Printf("[EVENT RESOLVED] Research queue #%d: %s -> level %d for user %d", queueID, techCode, targetLevel, userID)
}

// accrueResources applies the zero-cron accumulator before crediting a deposit.
func (e *EventEngine) accrueResources(ctx context.Context, tx *sql.Tx, celestialID int64) error {
	_, err := tx.ExecContext(ctx, `SELECT * FROM update_celestial_resources($1)`, celestialID)
	return err
}

// creditCargo adds a cargo manifest to a celestial object.
func (e *EventEngine) creditCargo(ctx context.Context, tx *sql.Tx, celestialID, metal, crystal, deut int64) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE celestial_objects
		SET metal = metal + $1, crystal = crystal + $2, deuterium = deuterium + $3
		WHERE id = $4
	`, metal, crystal, deut, celestialID)
	return err
}

// stationShips moves a fleet's ship complement into a celestial's hangar.
func (e *EventEngine) stationShips(ctx context.Context, tx *sql.Tx, celestialID, fleetID int64) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO planet_ships (celestial_id, ship_code, quantity)
		SELECT $1, ship_code, count FROM fleet_ships WHERE fleet_id = $2
		ON CONFLICT (celestial_id, ship_code)
		DO UPDATE SET quantity = planet_ships.quantity + EXCLUDED.quantity
	`, celestialID, fleetID)
	return err
}

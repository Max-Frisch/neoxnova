package engine

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

type EventEngine struct {
	rdb *redis.Client
	db  *sql.DB
}

func NewEventEngine(rdb *redis.Client, db *sql.DB) *EventEngine {
	return &EventEngine{rdb: rdb, db: db}
}

// StartEventLoop runs an autonomous tick loop checking Redis ZSET
func (e *EventEngine) StartEventLoop(ctx context.Context, universeID string) {
	ticker := time.NewTicker(100 * time.Millisecond) // 10Hz tick loop
	zsetKey := fmt.Sprintf("universe:%s:fleet_events", universeID)

	log.Printf("[EVENT ENGINE] Started time-wheel worker for universe %s", universeID)

	// Thread-safe atomic Lua script: Pops matching events whose arrival <= nowMs
	luaScript := redis.NewScript(`
		local matches = redis.call('ZRANGEBYSCORE', KEYS[1], ARGV[1], ARGV[2], 'LIMIT', 0, tonumber(ARGV[3]))
		if #matches > 0 then
			for _, member in ipairs(matches) do
				redis.call('ZREM', KEYS[1], member)
			end
			return matches
		end
		return {}
	`)

	for {
		select {
		case <-ctx.Done():
			log.Println("[EVENT ENGINE] Shutting down event loop worker gracefully.")
			return
		case <-ticker.C:
			nowMs := float64(time.Now().UnixMilli())

			// Execute script pipeline passing current universe timestamp bounds
			res, err := luaScript.Run(ctx, e.rdb, []string{zsetKey}, "-inf", fmt.Sprintf("%f", nowMs), "100").Result()
			if err != nil && err != redis.Nil {
				log.Printf("[ERROR] Failed to query event ZSET layer: %v", err)
				continue
			}

			// Map interface slice string assertions safely
			slice, ok := res.([]interface{})
			if ok && len(slice) > 0 {
				for _, item := range slice {
					fleetID, isString := item.(string)
					if isString {
						go e.resolveFleetEvent(ctx, zsetKey, fleetID)
					}
				}
			}
		}
	}
}

// resolveFleetEvent resolves the fleet arrival atomically
func (e *EventEngine) resolveFleetEvent(ctx context.Context, zsetKey, fleetID string) {
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		log.Printf("[ERROR] Tx begin error: %v", err)
		return
	}
	defer tx.Rollback()

	// 1. Acquire exclusive row lock. Rejects any concurrent recall API request
	query := `
		SELECT id, user_id, mission, phase, origin_id, target_id, 
		       cargo_metal, cargo_crystal, cargo_deuterium,
		       holding_end_time, return_time
		FROM fleets
		WHERE id = $1 AND phase IN ('OUTBOUND', 'HOLDING', 'RETURNING')
		FOR UPDATE;
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
		// Fleet was already recalled or resolved under concurrent lock
		return
	} else if err != nil {
		log.Printf("[ERROR] Fleet lock acquisition failed for fleet #%s: %v", fleetID, err)
		return
	}

	// 2. State transition logic based on phase and mission
	switch phase {
	case "OUTBOUND":
		switch mission {
		case "EXPEDITION":
			// Transition to HOLDING phase at deep space slot
			if !holdingEndTime.Valid {
				log.Printf("[ERROR] Fleet #%d has null holding_end_time for expedition", id)
				return
			}
			_, err = tx.ExecContext(ctx, `
				UPDATE fleets 
				SET phase = 'HOLDING', arrival_time = holding_end_time 
				WHERE id = $1
			`, id)
			if err != nil {
				log.Printf("[ERROR] Failed to transition fleet #%d to HOLDING: %v", id, err)
				return
			}

			// Re-enqueue event into Redis ZSET for holding phase expiration!
			e.rdb.ZAdd(ctx, zsetKey, redis.Z{
				Score:  float64(holdingEndTime.Time.UnixMilli()),
				Member: fleetID,
			})

		case "DEPLOY":
			// Arrived at target colony: unload cargo and station ships there.
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
			// Unload cargo at the target, then return empty to origin.
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

			// Re-enqueue return journey into Redis ZSET!
			if returnTime.Valid {
				e.rdb.ZAdd(ctx, zsetKey, redis.Z{
					Score:  float64(returnTime.Time.UnixMilli()),
					Member: fleetID,
				})
			}

		default:
			// For generic missions (Attack return, Espionage, Recycle): transition to RETURNING
			_, err = tx.ExecContext(ctx, `
				UPDATE fleets 
				SET phase = 'RETURNING', arrival_time = return_time 
				WHERE id = $1
			`, id)
			if err != nil {
				log.Printf("[ERROR] Failed to set fleet #%d to RETURNING: %v", id, err)
				return
			}

			if returnTime.Valid {
				e.rdb.ZAdd(ctx, zsetKey, redis.Z{
					Score:  float64(returnTime.Time.UnixMilli()),
					Member: fleetID,
				})
			}
		}

	case "HOLDING":
		// Holding period expired (e.g. Expedition exploration completed) -> Return to origin
		_, err = tx.ExecContext(ctx, `
			UPDATE fleets 
			SET phase = 'RETURNING', arrival_time = return_time 
			WHERE id = $1
		`, id)
		if err != nil {
			log.Printf("[ERROR] Failed to transition fleet #%d from HOLDING to RETURNING: %v", id, err)
			return
		}

		if returnTime.Valid {
			e.rdb.ZAdd(ctx, zsetKey, redis.Z{
				Score:  float64(returnTime.Time.UnixMilli()),
				Member: fleetID,
			})
		}

	case "RETURNING":
		// Deposit cargo and restore ships back to the origin planet.
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
		log.Printf("[ERROR] Tx commit failed for fleet #%s: %v", fleetID, err)
		return
	}

	log.Printf("[EVENT RESOLVED] Fleet #%s phase '%s' processed cleanly without hanging", fleetID, phase)
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

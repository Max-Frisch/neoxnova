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
		id, userID, originID, targetID      int64
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
			// Arrived at target colony: Unload cargo and station ships at target planet
			_, err = tx.ExecContext(ctx, `
				SELECT update_celestial_resources($1);
				UPDATE celestial_objects 
				SET metal = metal + $2, crystal = crystal + $3, deuterium = deuterium + $4
				WHERE id = $1;

				INSERT INTO planet_ships (celestial_id, ship_code, quantity)
				SELECT $1, ship_code, count FROM fleet_ships WHERE fleet_id = $5
				ON CONFLICT (celestial_id, ship_code)
				DO UPDATE SET quantity = planet_ships.quantity + EXCLUDED.quantity;

				UPDATE fleets SET phase = 'RESOLVED' WHERE id = $5;
			`, targetID, cargoMetal, cargoCrystal, cargoDeut, id)
			if err != nil {
				log.Printf("[ERROR] Failed to deploy fleet #%d: %v", id, err)
				return
			}

		case "TRANSPORT":
			// Unload cargo at target, then return empty to origin
			_, err = tx.ExecContext(ctx, `
				SELECT update_celestial_resources($1);
				UPDATE celestial_objects 
				SET metal = metal + $2, crystal = crystal + $3, deuterium = deuterium + $4
				WHERE id = $1;

				UPDATE fleets 
				SET phase = 'RETURNING', arrival_time = return_time,
				    cargo_metal = 0, cargo_crystal = 0, cargo_deuterium = 0
				WHERE id = $5;
			`, targetID, cargoMetal, cargoCrystal, cargoDeut, id)
			if err != nil {
				log.Printf("[ERROR] Failed to transport cargo for fleet #%d: %v", id, err)
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
		// Deposit cargo AND restore ships back to origin planet
		_, err = tx.ExecContext(ctx, `
			SELECT update_celestial_resources($1);
			UPDATE celestial_objects 
			SET metal = metal + $2, crystal = crystal + $3, deuterium = deuterium + $4
			WHERE id = $1;

			INSERT INTO planet_ships (celestial_id, ship_code, quantity)
			SELECT $1, ship_code, count FROM fleet_ships WHERE fleet_id = $5
			ON CONFLICT (celestial_id, ship_code)
			DO UPDATE SET quantity = planet_ships.quantity + EXCLUDED.quantity;

			UPDATE fleets SET phase = 'RESOLVED' WHERE id = $5;
		`, originID, cargoMetal, cargoCrystal, cargoDeut, id)
		if err != nil {
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

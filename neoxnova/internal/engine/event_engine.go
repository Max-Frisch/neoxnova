package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math/rand"
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
	rdb     *redis.Client
	db      *sql.DB
	builds  *store.BuildStore
	arsenal *store.ArsenalStore
}

func NewEventEngine(rdb *redis.Client, db *sql.DB) *EventEngine {
	return &EventEngine{rdb: rdb, db: db, builds: store.NewBuildStore(db), arsenal: store.NewArsenalStore(db)}
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
	for _, id := range e.dueIDs(ctx, `
		SELECT id FROM market_lots
		WHERE expires_at <= NOW()
		ORDER BY expires_at LIMIT 200`) {
		if err := e.arsenal.ExpireLot(ctx, id); err != nil {
			log.Printf("[ERROR] Failed to expire market lot #%d: %v", id, err)
		}
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

		case "HOLD":
			// A hold mission parks at the destination for its stay window, then
			// returns. With no stay window it simply turns around on arrival.
			if holdingEndTime.Valid {
				if _, err := tx.ExecContext(ctx, `
					UPDATE fleets SET phase = 'HOLDING', arrival_time = holding_end_time WHERE id = $1
				`, id); err != nil {
					log.Printf("[ERROR] Failed to transition HOLD fleet #%d to HOLDING: %v", id, err)
					return
				}
			} else if _, err := tx.ExecContext(ctx, `
				UPDATE fleets SET phase = 'RETURNING', arrival_time = return_time WHERE id = $1
			`, id); err != nil {
				log.Printf("[ERROR] Failed to turn HOLD fleet #%d around: %v", id, err)
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

		case "ATTACK":
			if !targetID.Valid {
				// Should be blocked at dispatch; if it slips through, send the
				// fleet home instead of leaving it OUTBOUND forever.
				log.Printf("[ERROR] Fleet #%d has no target celestial for ATTACK; returning", id)
				if _, err := tx.ExecContext(ctx, `UPDATE fleets SET phase = 'RETURNING', arrival_time = return_time WHERE id = $1`, id); err != nil {
					log.Printf("[ERROR] Failed to return target-less fleet #%d: %v", id, err)
					return
				}
				break
			}
			if err := e.resolveAttack(ctx, tx, id, userID, targetID.Int64); err != nil {
				log.Printf("[ERROR] Failed to resolve ATTACK for fleet #%d: %v", id, err)
				return
			}

		case "RECYCLE":
			if !targetID.Valid {
				log.Printf("[ERROR] Fleet #%d has no target debris field for RECYCLE; returning", id)
				if _, err := tx.ExecContext(ctx, `UPDATE fleets SET phase = 'RETURNING', arrival_time = return_time WHERE id = $1`, id); err != nil {
					log.Printf("[ERROR] Failed to return target-less fleet #%d: %v", id, err)
					return
				}
				break
			}
			if err := e.resolveRecycle(ctx, tx, id, targetID.Int64); err != nil {
				log.Printf("[ERROR] Failed to resolve RECYCLE for fleet #%d: %v", id, err)
				return
			}

		case "COLONIZE":
			if err := e.resolveColonize(ctx, tx, id, userID); err != nil {
				log.Printf("[ERROR] Failed to resolve COLONIZE for fleet #%d: %v", id, err)
				return
			}

		case "ESPIONAGE":
			if !targetID.Valid {
				log.Printf("[ERROR] Fleet #%d has no target celestial for ESPIONAGE; returning", id)
				if _, err := tx.ExecContext(ctx, `UPDATE fleets SET phase = 'RETURNING', arrival_time = return_time WHERE id = $1`, id); err != nil {
					log.Printf("[ERROR] Failed to return target-less fleet #%d: %v", id, err)
					return
				}
				break
			}
			if err := e.resolveEspionage(ctx, tx, id, userID, targetID.Int64); err != nil {
				log.Printf("[ERROR] Failed to resolve ESPIONAGE for fleet #%d: %v", id, err)
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

// resolveAttack runs a battle between the attacking fleet and the target
// celestial's defenders, then persists losses, debris, defense repair and (on an
// attacker win) the loot carried home by the survivors. Pure resolution lives in
// internal/game/combat.go; this function is only the persistence/edge layer.
func (e *EventEngine) resolveAttack(ctx context.Context, tx *sql.Tx, fleetID, attackerID, targetID int64) error {
	if err := e.accrueResources(ctx, tx, targetID); err != nil {
		return err
	}

	var (
		defOwner                  sql.NullInt64
		universeID                string
		galaxy, system, position  int
		metal, crystal, deuterium int64
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT user_id, universe_id::text, galaxy, system, position,
		       metal::bigint, crystal::bigint, deuterium::bigint
		FROM celestial_objects WHERE id = $1 FOR UPDATE
	`, targetID).Scan(&defOwner, &universeID, &galaxy, &system, &position, &metal, &crystal, &deuterium); err != nil {
		return err
	}

	atkShips, err := loadFleetShips(ctx, tx, fleetID)
	if err != nil {
		return err
	}
	atkTechs, err := loadCombatTechs(ctx, tx, attackerID)
	if err != nil {
		return err
	}
	atkAcademy, err := loadAcademy(ctx, tx, attackerID)
	if err != nil {
		return err
	}
	atkUpgrades, err := store.LoadAccountUpgrades(ctx, tx, attackerID)
	if err != nil {
		return err
	}
	defShips, err := loadPlanetUnits(ctx, tx, "planet_ships", "ship_code", targetID)
	if err != nil {
		return err
	}
	defDefs, err := loadPlanetUnits(ctx, tx, "planet_defenses", "defense_code", targetID)
	if err != nil {
		return err
	}
	var defTechs game.CombatTechs
	var defAcademy map[string]int
	var defUpgrades map[int]float64
	if defOwner.Valid {
		if defTechs, err = loadCombatTechs(ctx, tx, defOwner.Int64); err != nil {
			return err
		}
		if defAcademy, err = loadAcademy(ctx, tx, defOwner.Int64); err != nil {
			return err
		}
		if defUpgrades, err = store.LoadAccountUpgrades(ctx, tx, defOwner.Int64); err != nil {
			return err
		}
	}

	attacker := game.Combatant{Units: atkShips, Techs: atkTechs, Academy: atkAcademy, Upgrades: atkUpgrades}
	defender := game.Combatant{Units: mergeCounts(defShips, defDefs), Techs: defTechs, Academy: defAcademy, Upgrades: defUpgrades}
	res := game.Resolve(attacker, defender, game.CombatSeed(attacker, defender))

	// Persist a player-readable report (full per-round detail as JSONB).
	if reportJSON, mErr := json.Marshal(res); mErr == nil {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO combat_reports (
				universe_id, fleet_id, attacker_id, defender_id, target_id,
				galaxy, system, position, result, rounds,
				debris_metal, debris_crystal, moon_chance, report
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		`, universeID, fleetID, attackerID, defOwner, targetID,
			galaxy, system, position, res.Winner, res.Rounds,
			res.DebrisMetal, res.DebrisCrystal, res.MoonChance, reportJSON); err != nil {
			return err
		}
	} else {
		log.Printf("[WARN] Failed to marshal combat report for fleet #%d: %v", fleetID, mErr)
	}

	// Defender: ships are permanently lost, defenses repair ~61% (2026-10-05).
	if err := applyRemaining(ctx, tx, "planet_ships", "ship_code", targetID, defShips, res.Defender.Remaining, 0); err != nil {
		return err
	}
	if err := applyRemaining(ctx, tx, "planet_defenses", "defense_code", targetID, defDefs, res.Defender.Remaining, 61); err != nil {
		return err
	}
	// Attacker survivors return home.
	if err := overwriteFleetShips(ctx, tx, fleetID, res.Attacker.Remaining); err != nil {
		return err
	}

	// Debris field (ships only) accumulates at the target's coordinates.
	if res.DebrisMetal > 0 || res.DebrisCrystal > 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO celestial_objects (universe_id, name, object_type, galaxy, system, position, metal, crystal)
			VALUES ($1, 'Debris Field', 'DEBRIS_FIELD', $2, $3, $4, $5, $6)
			ON CONFLICT (universe_id, galaxy, system, position, object_type)
			DO UPDATE SET metal = celestial_objects.metal + EXCLUDED.metal,
			              crystal = celestial_objects.crystal + EXCLUDED.crystal
		`, universeID, galaxy, system, position, res.DebrisMetal, res.DebrisCrystal); err != nil {
			return err
		}
	}

	survivors := totalCount(res.Attacker.Remaining)
	if res.Winner == "attacker" && survivors > 0 {
		loot := game.Loot(
			game.Cost{Metal: metal, Crystal: crystal, Deuterium: deuterium},
			game.FleetCargo(res.Attacker.Remaining),
		)
		if loot.Metal+loot.Crystal+loot.Deuterium > 0 {
			if _, err := tx.ExecContext(ctx, `
				UPDATE celestial_objects
				SET metal = metal - $1, crystal = crystal - $2, deuterium = deuterium - $3
				WHERE id = $4
			`, loot.Metal, loot.Crystal, loot.Deuterium, targetID); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `
			UPDATE fleets SET phase = 'RETURNING', arrival_time = return_time,
			       cargo_metal = $1, cargo_crystal = $2, cargo_deuterium = $3
			WHERE id = $4
		`, loot.Metal, loot.Crystal, loot.Deuterium, fleetID)
		return err
	}

	if survivors == 0 {
		// Wiped: the fleet was destroyed and there is nothing to return.
		_, err := tx.ExecContext(ctx, `
			UPDATE fleets SET phase = 'RESOLVED', cargo_metal = 0, cargo_crystal = 0, cargo_deuterium = 0
			WHERE id = $1
		`, fleetID)
		return err
	}

	// Lost or drew, but survivors limp home empty-handed.
	_, err = tx.ExecContext(ctx, `
		UPDATE fleets SET phase = 'RETURNING', arrival_time = return_time,
		       cargo_metal = 0, cargo_crystal = 0, cargo_deuterium = 0
		WHERE id = $1
	`, fleetID)
	return err
}

// resolveRecycle collects a debris field into the recyclers' cargo holds (metal
// then crystal, capped by capacity) and sends the fleet home with the haul. A
// vanished field simply returns the fleet empty.
func (e *EventEngine) resolveRecycle(ctx context.Context, tx *sql.Tx, fleetID, targetID int64) error {
	var metal, crystal int64
	err := tx.QueryRowContext(ctx, `
		SELECT metal::bigint, crystal::bigint FROM celestial_objects WHERE id = $1 FOR UPDATE
	`, targetID).Scan(&metal, &crystal)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	ships, err := loadFleetShips(ctx, tx, fleetID)
	if err != nil {
		return err
	}
	capacity := game.FleetCargo(ships)
	take := func(v int64) int64 {
		w := v
		if w > capacity {
			w = capacity
		}
		if w < 0 {
			w = 0
		}
		capacity -= w
		return w
	}
	lootMetal := take(metal)
	lootCrystal := take(crystal)

	if lootMetal > 0 || lootCrystal > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE celestial_objects SET metal = metal - $1, crystal = crystal - $2 WHERE id = $3
		`, lootMetal, lootCrystal, targetID); err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE fleets SET phase = 'RETURNING', arrival_time = return_time,
		       cargo_metal = $1, cargo_crystal = $2, cargo_deuterium = 0
		WHERE id = $3
	`, lootMetal, lootCrystal, fleetID)
	return err
}

// espionageIntel is the JSON payload stored on an espionage report. Niburu
// returns every section regardless of the espionage-tech difference.
type espionageIntel struct {
	Target     string           `json:"target"`
	Galaxy     int              `json:"galaxy"`
	System     int              `json:"system"`
	Position   int              `json:"position"`
	Metal      int64            `json:"metal"`
	Crystal    int64            `json:"crystal"`
	Deuterium  int64            `json:"deuterium"`
	Fleet      map[string]int64 `json:"fleet"`
	Defense    map[string]int64 `json:"defense"`
	Buildings  map[string]int   `json:"buildings"`
	Research   map[string]int   `json:"research"`
	ProbesSent int              `json:"probes_sent"`
	ProbesLost int              `json:"probes_lost"`
}

// resolveEspionage delivers a spy report and resolves counter-espionage. Per the
// live niburu capture (docs/ESPIONAGE_LIVE_2026-10-06.md) the report is always
// full; the espionage-tech difference only affects whether the probes are shot
// down (ships-only detection). Caught probes are destroyed but the report is
// still delivered. Survivors fly home.
func (e *EventEngine) resolveEspionage(ctx context.Context, tx *sql.Tx, fleetID, attackerID, targetID int64) error {
	if err := e.accrueResources(ctx, tx, targetID); err != nil {
		return err
	}

	var (
		defOwner                 sql.NullInt64
		universeID               string
		targetName               string
		galaxy, system, position int
		metal, crystal, deut     int64
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT user_id, universe_id::text, name, galaxy, system, position,
		       metal::bigint, crystal::bigint, deuterium::bigint
		FROM celestial_objects WHERE id = $1 FOR UPDATE
	`, targetID).Scan(&defOwner, &universeID, &targetName, &galaxy, &system, &position, &metal, &crystal, &deut); err != nil {
		return err
	}

	atkShips, err := loadFleetShips(ctx, tx, fleetID)
	if err != nil {
		return err
	}
	probes := atkShips["210"]
	atkEsp, err := loadTechLevel(ctx, tx, attackerID, "espionage_tech")
	if err != nil {
		return err
	}
	defEsp := 0
	research := map[string]int{}
	if defOwner.Valid {
		if defEsp, err = loadTechLevel(ctx, tx, defOwner.Int64, "espionage_tech"); err != nil {
			return err
		}
		if research, err = loadTechLevels(ctx, tx, defOwner.Int64); err != nil {
			return err
		}
	}
	defShips, err := loadPlanetUnits(ctx, tx, "planet_ships", "ship_code", targetID)
	if err != nil {
		return err
	}
	defDefs, err := loadPlanetUnits(ctx, tx, "planet_defenses", "defense_code", targetID)
	if err != nil {
		return err
	}
	buildings, err := loadStructureLevels(ctx, tx, targetID)
	if err != nil {
		return err
	}

	score := game.EspionageScore(int(probes), atkEsp, defEsp)

	// Counter-espionage: ships-only detection, seeded by fleet id so a retried
	// resolution is stable.
	probesLost := 0
	chance := game.CounterEspionageChance(atkEsp, defEsp, int(probes), int(totalCount(defShips)))
	if probes > 0 && chance > 0 {
		if rand.New(rand.NewSource(fleetID)).Float64() < chance {
			probesLost = int(probes)
		}
	}

	intel := espionageIntel{
		Target: targetName, Galaxy: galaxy, System: system, Position: position,
		Metal: metal, Crystal: crystal, Deuterium: deut,
		Fleet: defShips, Defense: defDefs, Buildings: buildings, Research: research,
		ProbesSent: int(probes), ProbesLost: probesLost,
	}
	reportJSON, err := json.Marshal(intel)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO espionage_reports (
			universe_id, fleet_id, attacker_id, defender_id, target_id,
			galaxy, system, position, probes_sent, probes_lost, score, report
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, universeID, fleetID, attackerID, defOwner, targetID,
		galaxy, system, position, int(probes), probesLost, score, reportJSON); err != nil {
		return err
	}

	remaining := map[string]int64{}
	for code, n := range atkShips {
		if code == "210" {
			n -= int64(probesLost)
		}
		if n > 0 {
			remaining[code] = n
		}
	}
	if err := overwriteFleetShips(ctx, tx, fleetID, remaining); err != nil {
		return err
	}
	if totalCount(remaining) == 0 {
		_, err := tx.ExecContext(ctx, `UPDATE fleets SET phase = 'RESOLVED' WHERE id = $1`, fleetID)
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE fleets SET phase = 'RETURNING', arrival_time = return_time WHERE id = $1`, fleetID)
	return err
}

// resolveColonize founds a new planet at the fleet's target coordinates,
// consuming one Colony Ship and stationing the rest of the fleet there. If the
// destination is already occupied or the fleet carries no colony ship, the
// fleet simply returns home.
func (e *EventEngine) resolveColonize(ctx context.Context, tx *sql.Tx, fleetID, userID int64) error {
	var (
		universeID string
		g, s, p    int
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT universe_id::text, target_galaxy, target_system, target_position
		FROM fleets WHERE id = $1
	`, fleetID).Scan(&universeID, &g, &s, &p); err != nil {
		return err
	}

	var colonyShips int64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(count, 0) FROM fleet_ships WHERE fleet_id = $1 AND ship_code = '208'
	`, fleetID).Scan(&colonyShips); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	var planetsPerSystem int
	if err := tx.QueryRowContext(ctx, `SELECT planets_per_system FROM universes WHERE id = $1`, universeID).Scan(&planetsPerSystem); err != nil {
		return err
	}

	// Reject occupied or slot-locked positions, and the expedition slot.
	var blocked bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM celestial_objects
			WHERE universe_id = $1 AND galaxy = $2 AND system = $3 AND position = $4
			  AND object_type IN ('PLANET', 'MOON')
		) OR EXISTS (
			SELECT 1 FROM coordinate_locks
			WHERE universe_id = $1 AND galaxy = $2 AND system = $3 AND position = $4
			  AND locked_until > NOW()
		)
	`, universeID, g, s, p).Scan(&blocked); err != nil {
		return err
	}
	returnHome := func() error {
		_, err := tx.ExecContext(ctx, `UPDATE fleets SET phase = 'RETURNING', arrival_time = return_time WHERE id = $1`, fleetID)
		return err
	}
	if colonyShips < 1 || blocked || p < 1 || p > planetsPerSystem {
		return returnHome()
	}

	// Roll the planet's fields/temperature from the slot's captured ranges.
	spec, ok := game.RollPlanet(game.FieldSlotSeed(g, s, p), p)
	if !ok {
		return returnHome()
	}

	var username string
	_ = tx.QueryRowContext(ctx, `SELECT username FROM users WHERE id = $1`, userID).Scan(&username)

	var newID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO celestial_objects (
			universe_id, user_id, name, object_type, galaxy, system, position,
			diameter_km, fields_used, fields_max, base_fields_max, temp_min, temp_max,
			metal, crystal, deuterium
		) VALUES ($1, $2, $3, 'PLANET', $4, $5, $6, $7, 0, $8, $9, $10, $11, 500, 500, 0)
		RETURNING id
	`, universeID, userID, username+"'s Colony", g, s, p, spec.Diameter, spec.FieldsMax, spec.FieldsMax, spec.TempMin, spec.TempMax).Scan(&newID); err != nil {
		return err
	}

	// Consume exactly one colony ship (the count>0 constraint forbids writing 0).
	if _, err := tx.ExecContext(ctx, `
		UPDATE fleet_ships SET count = count - 1 WHERE fleet_id = $1 AND ship_code = '208' AND count > 1
	`, fleetID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM fleet_ships WHERE fleet_id = $1 AND ship_code = '208' AND count = 1
	`, fleetID); err != nil {
		return err
	}
	if err := e.stationShips(ctx, tx, newID, fleetID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE fleets SET phase = 'RESOLVED' WHERE id = $1`, fleetID)
	return err
}

func loadFleetShips(ctx context.Context, tx *sql.Tx, fleetID int64) (map[string]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT ship_code, count FROM fleet_ships WHERE fleet_id = $1`, fleetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var code string
		var n int64
		if err := rows.Scan(&code, &n); err != nil {
			return nil, err
		}
		out[code] += n
	}
	return out, rows.Err()
}

func loadPlanetUnits(ctx context.Context, tx *sql.Tx, table, codeColumn string, celestialID int64) (map[string]int64, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT `+codeColumn+`, quantity FROM `+table+` WHERE celestial_id = $1 AND quantity > 0`, celestialID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var code string
		var n int64
		if err := rows.Scan(&code, &n); err != nil {
			return nil, err
		}
		out[code] += n
	}
	return out, rows.Err()
}

func loadCombatTechs(ctx context.Context, tx *sql.Tx, userID int64) (game.CombatTechs, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT tech_code, level FROM user_technologies
		WHERE user_id = $1 AND tech_code IN ('weapons_tech', 'shielding_tech', 'armour_tech')
	`, userID)
	if err != nil {
		return game.CombatTechs{}, err
	}
	defer rows.Close()
	var t game.CombatTechs
	for rows.Next() {
		var code string
		var level int
		if err := rows.Scan(&code, &level); err != nil {
			return t, err
		}
		switch code {
		case "weapons_tech":
			t.Weapons = level
		case "shielding_tech":
			t.Shield = level
		case "armour_tech":
			t.Armour = level
		}
	}
	return t, rows.Err()
}

func loadAcademy(ctx context.Context, tx *sql.Tx, userID int64) (map[string]int, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT skill_code, level FROM user_academy_skills WHERE user_id = $1 AND level > 0
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var code string
		var level int
		if err := rows.Scan(&code, &level); err != nil {
			return nil, err
		}
		out[code] = level
	}
	return out, rows.Err()
}

// loadTechLevel returns one technology level for a user (0 when unresearched).
func loadTechLevel(ctx context.Context, tx *sql.Tx, userID int64, code string) (int, error) {
	var level int
	err := tx.QueryRowContext(ctx,
		`SELECT level FROM user_technologies WHERE user_id = $1 AND tech_code = $2`, userID, code).Scan(&level)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return level, err
}

// loadTechLevels returns all researched technology levels for a user.
func loadTechLevels(ctx context.Context, tx *sql.Tx, userID int64) (map[string]int, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT tech_code, level FROM user_technologies WHERE user_id = $1 AND level > 0`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var code string
		var level int
		if err := rows.Scan(&code, &level); err != nil {
			return nil, err
		}
		out[code] = level
	}
	return out, rows.Err()
}

// loadStructureLevels returns the built structure levels on a celestial.
func loadStructureLevels(ctx context.Context, tx *sql.Tx, celestialID int64) (map[string]int, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT structure_code, level FROM planet_structures WHERE celestial_id = $1 AND level > 0`, celestialID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var code string
		var level int
		if err := rows.Scan(&code, &level); err != nil {
			return nil, err
		}
		out[code] = level
	}
	return out, rows.Err()
}

func mergeCounts(a, b map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(a)+len(b))
	for code, n := range a {
		out[code] += n
	}
	for code, n := range b {
		out[code] += n
	}
	return out
}

func totalCount(m map[string]int64) int64 {
	var total int64
	for _, n := range m {
		total += n
	}
	return total
}

// applyRemaining rewrites a celestial's stored quantity of each code to reflect
// survivors, optionally restoring repairPct% of the destroyed count (defenses).
func applyRemaining(ctx context.Context, tx *sql.Tx, table, codeColumn string, celestialID int64, initial, remaining map[string]int64, repairPct int64) error {
	for code, start := range initial {
		rem := remaining[code]
		if repairPct > 0 {
			destroyed := start - rem
			if destroyed > 0 {
				rem += repairPct * destroyed / 100
			}
		}
		if rem <= 0 {
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM `+table+` WHERE celestial_id = $1 AND `+codeColumn+` = $2`, celestialID, code); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE `+table+` SET quantity = $1 WHERE celestial_id = $2 AND `+codeColumn+` = $3`, rem, celestialID, code); err != nil {
			return err
		}
	}
	return nil
}

func overwriteFleetShips(ctx context.Context, tx *sql.Tx, fleetID int64, remaining map[string]int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM fleet_ships WHERE fleet_id = $1`, fleetID); err != nil {
		return err
	}
	for code, n := range remaining {
		if n <= 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO fleet_ships (fleet_id, ship_code, count) VALUES ($1, $2, $3)
		`, fleetID, code, n); err != nil {
			return err
		}
	}
	return nil
}

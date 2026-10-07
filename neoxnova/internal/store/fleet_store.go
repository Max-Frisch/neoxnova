package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
)

type FleetStore struct {
	db *sql.DB
}

func NewFleetStore(db *sql.DB) *FleetStore {
	return &FleetStore{db: db}
}

type DispatchResult struct {
	FleetID      int64
	Arrival      time.Time
	DurationSecs int64
	FuelBurn     int64
}

// Dispatch atomically verifies ships, deducts fuel, and persists the new fleet mission.
func (s *FleetStore) Dispatch(ctx context.Context, req models.FleetDispatchRequest) (DispatchResult, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return DispatchResult{}, err
	}
	defer tx.Rollback()

	var oG, oS, oP int
	var universeID string
	var userID int64
	var originDeut, fleetSpeed float64
	var attackLockedUntil sql.NullTime
	originQuery := `SELECT c.universe_id, c.user_id, c.galaxy, c.system, c.position, c.deuterium, u.fleet_speed,
	                       c.attack_locked_until
	                FROM celestial_objects c
	                JOIN universes u ON u.id = c.universe_id
	                WHERE c.id = $1 FOR UPDATE OF c`
	err = tx.QueryRowContext(ctx, originQuery, req.OriginPlanetID).Scan(
		&universeID, &userID, &oG, &oS, &oP, &originDeut, &fleetSpeed, &attackLockedUntil,
	)
	if err != nil {
		return DispatchResult{}, ErrNotFound
	}

	// A planet recently teleported cannot launch attacks for a short window
	// (Planetarium rule: 15 minutes).
	if req.Mission == models.MissionAttack && attackLockedUntil.Valid && time.Now().Before(attackLockedUntil.Time) {
		return DispatchResult{}, ErrAttackLocked
	}

	// Resolve the destination celestial (if one exists) so the event engine can
	// later unload cargo/ships into it. Empty space targets stay NULL.
	var targetID sql.NullInt64
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM celestial_objects
		WHERE universe_id = $1 AND galaxy = $2 AND system = $3 AND position = $4 AND object_type = $5
	`, universeID, req.Target.Galaxy, req.Target.System, req.Target.Position, req.Target.Type).Scan(&targetID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return DispatchResult{}, err
	}

	// Missions that act on a celestial must have one at the destination; only
	// COLONIZE (and deliberate HOLD/EXPEDITION) may target empty space. Without
	// this, the fleet would never resolve (see AGENTS "empty-space targets").
	switch req.Mission {
	case models.MissionAttack, models.MissionTransport, models.MissionDeploy,
		models.MissionRecycle, models.MissionEspionage:
		if !targetID.Valid {
			return DispatchResult{}, ErrNoTarget
		}
	}

	// Noob protection: attacks are only allowed within a ~4:1 points ratio in
	// either direction (learned from the reference server).
	if req.Mission == models.MissionAttack && targetID.Valid {
		var defenderOwner sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT user_id FROM celestial_objects WHERE id = $1`, targetID.Int64).Scan(&defenderOwner); err != nil {
			return DispatchResult{}, err
		}
		if defenderOwner.Valid && defenderOwner.Int64 != userID {
			atkPoints, err := PlayerPoints(ctx, tx, userID)
			if err != nil {
				return DispatchResult{}, err
			}
			defPoints, err := PlayerPoints(ctx, tx, defenderOwner.Int64)
			if err != nil {
				return DispatchResult{}, err
			}
			if atkPoints > defPoints*NoobProtectionRatio || defPoints > atkPoints*NoobProtectionRatio {
				return DispatchResult{}, ErrNoobProtection
			}
		}
	}

	for shipCode, count := range req.Ships {
		var available int64
		err := tx.QueryRowContext(ctx, `
			SELECT quantity FROM planet_ships WHERE celestial_id = $1 AND ship_code = $2 FOR UPDATE
		`, req.OriginPlanetID, shipCode).Scan(&available)
		if err != nil || available < count {
			return DispatchResult{}, &InsufficientShipsError{
				ShipCode:  shipCode,
				Available: available,
				Requested: count,
			}
		}

		_, err = tx.ExecContext(ctx, `
			UPDATE planet_ships SET quantity = quantity - $1
			WHERE celestial_id = $2 AND ship_code = $3
		`, count, req.OriginPlanetID, shipCode)
		if err != nil {
			return DispatchResult{}, err
		}
	}

	// Engine technologies speed up their class of ships (+10%/level); Arsenal
	// engine upgrades add an extra additive percent on top.
	var combustion, impulse, hyperspace int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(level) FILTER (WHERE tech_code = 'combustion_drive'), 0),
		       COALESCE(MAX(level) FILTER (WHERE tech_code = 'impulse_drive'), 0),
		       COALESCE(MAX(level) FILTER (WHERE tech_code = 'hyperspace_drive'), 0)
		FROM user_technologies WHERE user_id = $1
	`, userID).Scan(&combustion, &impulse, &hyperspace); err != nil {
		return DispatchResult{}, err
	}
	upgrades, err := LoadAccountUpgrades(ctx, tx, userID)
	if err != nil {
		return DispatchResult{}, err
	}

	dist := game.CalculateCoordinateDistance(oG, oS, oP, req.Target.Galaxy, req.Target.System, req.Target.Position)
	baseSpeed := game.FleetMaxSpeed(req.Ships, combustion, impulse, hyperspace, upgrades)
	durationSecs := game.CalculateFlightDuration(dist, baseSpeed, req.SpeedPercent, fleetSpeed)

	fuelBurn := game.CalculateDeuteriumConsumption(game.FleetFuelBase(req.Ships), dist, req.SpeedPercent)

	if originDeut < float64(fuelBurn) {
		return DispatchResult{}, &InsufficientFuelError{Needed: fuelBurn}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE celestial_objects SET deuterium = deuterium - $1 WHERE id = $2
	`, fuelBurn, req.OriginPlanetID)
	if err != nil {
		return DispatchResult{}, err
	}

	now := time.Now()
	arrivalTime := now.Add(time.Duration(durationSecs) * time.Second)
	var holdingEnd sql.NullTime
	if (req.Mission == models.MissionExpedition || req.Mission == models.MissionHold) && req.HoldingHours > 0 {
		holdingEnd = sql.NullTime{Time: arrivalTime.Add(time.Duration(req.HoldingHours) * time.Hour), Valid: true}
	}
	returnTime := arrivalTime.Add(time.Duration(durationSecs) * time.Second)
	if holdingEnd.Valid {
		returnTime = holdingEnd.Time.Add(time.Duration(durationSecs) * time.Second)
	}

	var fleetID int64
	insertQuery := `
		INSERT INTO fleets (
			universe_id, user_id, mission, phase, origin_id, target_id,
			origin_galaxy, origin_system, origin_position, origin_type,
			target_galaxy, target_system, target_position, target_type,
			start_time, arrival_time, holding_end_time, return_time,
			flight_speed_pct, deuterium_consumption,
			cargo_metal, cargo_crystal, cargo_deuterium
		) VALUES (
			$1, $2, $3, 'OUTBOUND', $4, $5,
			$6, $7, $8, 'PLANET',
			$9, $10, $11, $12,
			$13, $14, $15, $16,
			$17, $18,
			$19, $20, $21
		) RETURNING id
	`
	err = tx.QueryRowContext(ctx, insertQuery,
		universeID, userID, req.Mission, req.OriginPlanetID, targetID,
		oG, oS, oP,
		req.Target.Galaxy, req.Target.System, req.Target.Position, req.Target.Type,
		now, arrivalTime, holdingEnd, returnTime,
		float64(req.SpeedPercent)/100.0, fuelBurn,
		req.Cargo.Metal, req.Cargo.Crystal, req.Cargo.Deuterium,
	).Scan(&fleetID)
	if err != nil {
		return DispatchResult{}, err
	}

	for shipCode, count := range req.Ships {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO fleet_ships (fleet_id, ship_code, count) VALUES ($1, $2, $3)
		`, fleetID, shipCode, count)
		if err != nil {
			return DispatchResult{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return DispatchResult{}, err
	}

	return DispatchResult{
		FleetID:      fleetID,
		Arrival:      arrivalTime,
		DurationSecs: durationSecs,
		FuelBurn:     fuelBurn,
	}, nil
}

type RecallResult struct {
	Arrival time.Time
	Elapsed time.Duration
}

// Recall reverses the trajectory of an outbound fleet while holding a row lock.
func (s *FleetStore) Recall(ctx context.Context, fleetID int64) (RecallResult, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return RecallResult{}, err
	}
	defer tx.Rollback()

	var phase string
	var startTime, arrivalTime time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT phase, start_time, arrival_time
		FROM fleets
		WHERE id = $1 FOR UPDATE
	`, fleetID).Scan(&phase, &startTime, &arrivalTime)
	if errors.Is(err, sql.ErrNoRows) {
		return RecallResult{}, ErrNotFound
	} else if err != nil {
		return RecallResult{}, err
	}

	now := time.Now()
	if phase != string(models.PhaseOutbound) || now.After(arrivalTime) {
		return RecallResult{}, ErrCannotRecall
	}

	elapsed := now.Sub(startTime)
	newReturnArrival := now.Add(elapsed)

	_, err = tx.ExecContext(ctx, `
		UPDATE fleets
		SET phase = 'RETURNING', arrival_time = $1, return_time = $1
		WHERE id = $2
	`, newReturnArrival, fleetID)
	if err != nil {
		return RecallResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return RecallResult{}, err
	}

	return RecallResult{Arrival: newReturnArrival, Elapsed: elapsed}, nil
}

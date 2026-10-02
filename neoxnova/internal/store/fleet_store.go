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
	originQuery := `SELECT c.universe_id, c.user_id, c.galaxy, c.system, c.position, c.deuterium, u.fleet_speed
	                FROM celestial_objects c
	                JOIN universes u ON u.id = c.universe_id
	                WHERE c.id = $1 FOR UPDATE OF c`
	err = tx.QueryRowContext(ctx, originQuery, req.OriginPlanetID).Scan(
		&universeID, &userID, &oG, &oS, &oP, &originDeut, &fleetSpeed,
	)
	if err != nil {
		return DispatchResult{}, ErrNotFound
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

	dist := game.CalculateCoordinateDistance(oG, oS, oP, req.Target.Galaxy, req.Target.System, req.Target.Position)
	baseSpeed := 10000 // default battleship baseline
	durationSecs := game.CalculateFlightDuration(dist, baseSpeed, req.SpeedPercent, fleetSpeed)

	var totalShipCount int64
	for _, c := range req.Ships {
		totalShipCount += c
	}
	fuelBurn := game.CalculateDeuteriumConsumption(totalShipCount, 500, dist)

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
	if req.Mission == models.MissionExpedition && req.HoldingHours > 0 {
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

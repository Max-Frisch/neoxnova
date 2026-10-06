package store

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
)

type PlanetStore struct {
	db *sql.DB
}

func NewPlanetStore(db *sql.DB) *PlanetStore {
	return &PlanetStore{db: db}
}

// SlotLockDuration is how long a position stays locked after a planet is
// abandoned before it can be colonised again (global per slot).
var SlotLockDuration = 24 * time.Hour

// RelocationCooldown is how long a planet must wait between relocations.
var RelocationCooldown = time.Hour

// AttackLockoutAfterRelocation is how long a teleported planet is barred from
// launching ATTACK missions (Planetarium rule).
var AttackLockoutAfterRelocation = 15 * time.Minute

// maxFieldPurchase caps a single Dark-Matter field purchase.
const maxFieldPurchase = 100

// AbandonPlanet deletes a planet the user owns and locks its slot. The
// homeworld (the user's oldest planet) and the last remaining planet cannot be
// abandoned, and no fleets may be in flight to or from it.
func (s *PlanetStore) AbandonPlanet(ctx context.Context, planetID, userID int64) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var (
		owner       sql.NullInt64
		universe    string
		g, sys, pos int
		objectType  string
	)
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, universe_id::text, galaxy, system, position, object_type
		FROM celestial_objects WHERE id = $1 FOR UPDATE
	`, planetID).Scan(&owner, &universe, &g, &sys, &pos, &objectType)
	if err == sql.ErrNoRows {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if !owner.Valid || owner.Int64 != userID || objectType != "PLANET" {
		return ErrNotFound
	}

	var planetCount int
	var homeID sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*), MIN(id) FROM celestial_objects WHERE user_id = $1 AND object_type = 'PLANET'
	`, userID).Scan(&planetCount, &homeID); err != nil {
		return err
	}
	if planetCount <= 1 {
		return ErrLastPlanet
	}
	if homeID.Valid && planetID == homeID.Int64 {
		return ErrCannotAbandonHome
	}

	var fleetBusy bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM fleets
			WHERE phase IN ('OUTBOUND', 'HOLDING', 'RETURNING')
			  AND (origin_id = $1 OR target_id = $1)
		)
	`, planetID).Scan(&fleetBusy); err != nil {
		return err
	}
	if fleetBusy {
		return ErrFleetInbound
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO coordinate_locks (universe_id, galaxy, system, position, locked_until, reason)
		VALUES ($1, $2, $3, $4, $5, 'abandoned')
		ON CONFLICT (universe_id, galaxy, system, position)
		DO UPDATE SET locked_until = EXCLUDED.locked_until, reason = EXCLUDED.reason
	`, universe, g, sys, pos, time.Now().Add(SlotLockDuration)); err != nil {
		return err
	}
	return tx.Commit()
}

// RelocatePlanet moves an owned planet to new coordinates for a distance-priced
// Dark Matter fee and returns the fee charged. Unlike abandonment it does not
// lock the origin slot; the planet is instead put on a per-planet cooldown for
// inter-system/galaxy teleports (the reference server allows unlimited
// same-system teleports, so those bypass and do not set the cooldown). The
// destination must be inside the planet domain, unoccupied and unlocked, the
// planet must have no active fleets (fleet targets snapshot coordinates), and
// the owner must hold enough Dark Matter.
func (s *PlanetStore) RelocatePlanet(ctx context.Context, planetID, userID int64, toGalaxy, toSystem, toPosition int) (int64, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var (
		owner            sql.NullInt64
		universe         string
		g, sys, pos      int
		objectType       string
		nextAt           sql.NullTime
		planetsPerSystem int
	)
	err = tx.QueryRowContext(ctx, `
		SELECT c.user_id, c.universe_id::text, c.galaxy, c.system, c.position, c.object_type,
		       c.relocation_next_at, u.planets_per_system
		FROM celestial_objects c
		JOIN universes u ON u.id = c.universe_id
		WHERE c.id = $1
		FOR UPDATE OF c
	`, planetID).Scan(&owner, &universe, &g, &sys, &pos, &objectType, &nextAt, &planetsPerSystem)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	} else if err != nil {
		return 0, err
	}
	if !owner.Valid || owner.Int64 != userID || objectType != "PLANET" {
		return 0, ErrNotFound
	}

	if !game.ValidRelocationTarget(toGalaxy, toSystem, toPosition, planetsPerSystem) {
		return 0, ErrInvalidCoordinates
	}
	if toGalaxy == g && toSystem == sys && toPosition == pos {
		return 0, ErrSameCoordinates
	}
	leavesSystem := game.RelocationLeavesSystem(g, sys, toGalaxy, toSystem)
	if leavesSystem && nextAt.Valid && time.Now().Before(nextAt.Time) {
		return 0, ErrRelocationCooldown
	}

	// The destination must be free (no planet/moon) and not slot-locked.
	var blocked bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM celestial_objects
			WHERE universe_id = $1 AND galaxy = $2 AND system = $3 AND position = $4
			  AND object_type IN ('PLANET', 'MOON') AND id <> $5
		) OR EXISTS (
			SELECT 1 FROM coordinate_locks
			WHERE universe_id = $1 AND galaxy = $2 AND system = $3 AND position = $4
			  AND locked_until > NOW()
		)
	`, universe, toGalaxy, toSystem, toPosition, planetID).Scan(&blocked); err != nil {
		return 0, err
	}
	if blocked {
		return 0, ErrTargetOccupied
	}

	var fleetBusy bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM fleets
			WHERE phase IN ('OUTBOUND', 'HOLDING', 'RETURNING')
			  AND (origin_id = $1 OR target_id = $1)
		)
	`, planetID).Scan(&fleetBusy); err != nil {
		return 0, err
	}
	if fleetBusy {
		return 0, ErrFleetInbound
	}

	cost := game.RelocationCost(g, sys, pos, toGalaxy, toSystem, toPosition)

	res, err := tx.ExecContext(ctx, `
		UPDATE users SET dark_matter = dark_matter - $1 WHERE id = $2 AND dark_matter >= $1
	`, cost, userID)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if affected == 0 {
		return 0, ErrInsufficientDarkMatter
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE celestial_objects
		SET galaxy = $1, system = $2, position = $3,
		    relocation_next_at = CASE
		        WHEN $4 THEN NOW() + make_interval(secs => $5::double precision)
		        ELSE relocation_next_at END,
		    attack_locked_until = NOW() + make_interval(secs => $6::double precision)
		WHERE id = $7
	`, toGalaxy, toSystem, toPosition, leavesSystem, int64(RelocationCooldown.Seconds()),
		int64(AttackLockoutAfterRelocation.Seconds()), planetID); err != nil {
		if isUniqueViolation(err) {
			return 0, ErrTargetOccupied
		}
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return cost, nil
}

// FieldsResult reports a successful Dark-Matter field purchase.
type FieldsResult struct {
	Cost         int64
	FieldsMax    int64
	FieldsBought int64
}

// ExpandFields buys `additional` extra fields for a planet or moon with Dark
// Matter. The price escalates per field already bought with Dark Matter (see
// game.FieldExpansionCost) and is independent of the planet's base size.
// Purchased fields are folded into base_fields_max so that RecomputeCelestial
// (fields_max = base_fields_max + 7*terraformer) preserves them across builds.
func (s *PlanetStore) ExpandFields(ctx context.Context, planetID, userID int64, additional int) (FieldsResult, error) {
	if additional < 1 || additional > maxFieldPurchase {
		return FieldsResult{}, ErrInvalidQuantity
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return FieldsResult{}, err
	}
	defer tx.Rollback()

	var (
		owner      sql.NullInt64
		objectType string
		bought     int64
	)
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, object_type, fields_bought
		FROM celestial_objects WHERE id = $1 FOR UPDATE
	`, planetID).Scan(&owner, &objectType, &bought)
	if err == sql.ErrNoRows {
		return FieldsResult{}, ErrNotFound
	} else if err != nil {
		return FieldsResult{}, err
	}
	if !owner.Valid || owner.Int64 != userID || (objectType != "PLANET" && objectType != "MOON") {
		return FieldsResult{}, ErrNotFound
	}

	cost := game.FieldExpansionCost(int(bought), additional)

	res, err := tx.ExecContext(ctx, `
		UPDATE users SET dark_matter = dark_matter - $1 WHERE id = $2 AND dark_matter >= $1
	`, cost, userID)
	if err != nil {
		return FieldsResult{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return FieldsResult{}, err
	}
	if affected == 0 {
		return FieldsResult{}, ErrInsufficientDarkMatter
	}

	var newMax, newBought int64
	if err := tx.QueryRowContext(ctx, `
		UPDATE celestial_objects
		SET base_fields_max = base_fields_max + $1,
		    fields_max = fields_max + $1,
		    fields_bought = fields_bought + $1
		WHERE id = $2
		RETURNING fields_max, fields_bought
	`, int64(additional), planetID).Scan(&newMax, &newBought); err != nil {
		return FieldsResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return FieldsResult{}, err
	}
	return FieldsResult{Cost: cost, FieldsMax: newMax, FieldsBought: newBought}, nil
}

// UpdateResources runs the continuous resource accumulator and returns the fresh state.
func (s *PlanetStore) UpdateResources(ctx context.Context, planetID string) (float64, float64, float64, time.Time, error) {
	var metal, crystal, deut float64
	var lastCalc time.Time
	err := s.db.QueryRowContext(ctx, "SELECT * FROM update_celestial_resources($1)", planetID).Scan(
		&metal, &crystal, &deut, &lastCalc,
	)
	return metal, crystal, deut, lastCalc, err
}

// GetResourceState returns the real-time resource state, calculated on-the-fly.
func (s *PlanetStore) GetResourceState(ctx context.Context, planetID string) (models.RealtimeResourceState, error) {
	outMetal, outCrystal, outDeut, outLastCalc, err := s.UpdateResources(ctx, planetID)
	if err != nil {
		return models.RealtimeResourceState{}, err
	}

	var metalCap, crystalCap, deutCap int64
	var metalRate, crystalRate, deutRate float64
	var energyAvailable, energyMax int
	query := `SELECT metal_capacity, crystal_capacity, deuterium_capacity,
	                 metal_prod_hourly, crystal_prod_hourly, deuterium_prod_hourly,
	                 energy_used, energy_max
	          FROM celestial_objects WHERE id = $1`
	err = s.db.QueryRowContext(ctx, query, planetID).Scan(
		&metalCap, &crystalCap, &deutCap,
		&metalRate, &crystalRate, &deutRate,
		&energyAvailable, &energyMax,
	)
	if err != nil {
		return models.RealtimeResourceState{}, err
	}

	return models.RealtimeResourceState{
		MetalCurrent:      outMetal,
		MetalLimit:        metalCap,
		MetalHourlyRate:   metalRate,
		CrystalCurrent:    outCrystal,
		CrystalLimit:      crystalCap,
		CrystalHourlyRate: crystalRate,
		DeutCurrent:       outDeut,
		DeutLimit:         deutCap,
		DeutHourlyRate:    deutRate,
		EnergyAvailable:   energyMax - energyAvailable,
		EnergyMax:         energyMax,
		LastCalculatedAt:  outLastCalc,
	}, nil
}

// GetOverview builds the full planet overview payload, including active fleets.
func (s *PlanetStore) GetOverview(ctx context.Context, planetID string) (models.PlanetOverviewResponse, error) {
	outMetal, outCrystal, outDeut, outLastCalc, err := s.UpdateResources(ctx, planetID)
	if err != nil {
		return models.PlanetOverviewResponse{}, err
	}

	var planetName string
	var galaxy, system, position, diameter, fieldsUsed, fieldsMax, tempMin, tempMax int
	var metalCap, crystalCap, deutCap int64
	var metalRate, crystalRate, deutRate float64
	var energyUsed, energyMax int
	var userID int64

	query := `SELECT user_id, name, galaxy, system, position, diameter_km,
	                 fields_used, fields_max, temp_min, temp_max,
	                 metal_capacity, crystal_capacity, deuterium_capacity,
	                 metal_prod_hourly, crystal_prod_hourly, deuterium_prod_hourly,
	                 energy_used, energy_max
	          FROM celestial_objects WHERE id = $1`
	err = s.db.QueryRowContext(ctx, query, planetID).Scan(
		&userID, &planetName, &galaxy, &system, &position, &diameter,
		&fieldsUsed, &fieldsMax, &tempMin, &tempMax,
		&metalCap, &crystalCap, &deutCap,
		&metalRate, &crystalRate, &deutRate,
		&energyUsed, &energyMax,
	)
	if err != nil {
		return models.PlanetOverviewResponse{}, err
	}

	activeFleets, _ := s.activeFleets(ctx, userID)
	planetID64, _ := strconv.ParseInt(planetID, 10, 64)

	return models.PlanetOverviewResponse{
		ServerTime:       time.Now(),
		ActivePlanetID:   planetID64,
		ActivePlanetName: planetName,
		Coordinates: models.Coordinates{
			Galaxy:   galaxy,
			System:   system,
			Position: position,
			Type:     models.TypePlanet,
		},
		DiameterKm: diameter,
		FieldsUsed: fieldsUsed,
		FieldsMax:  fieldsMax,
		TempRange:  [2]int{tempMin, tempMax},
		Resources: models.RealtimeResourceState{
			MetalCurrent:      outMetal,
			MetalLimit:        metalCap,
			MetalHourlyRate:   metalRate,
			CrystalCurrent:    outCrystal,
			CrystalLimit:      crystalCap,
			CrystalHourlyRate: crystalRate,
			DeutCurrent:       outDeut,
			DeutLimit:         deutCap,
			DeutHourlyRate:    deutRate,
			EnergyAvailable:   energyMax - energyUsed,
			EnergyMax:         energyMax,
			LastCalculatedAt:  outLastCalc,
		},
		ActiveFleets: activeFleets,
	}, nil
}

// GetCoordinates returns the display name and absolute coordinate vector of a planet.
func (s *PlanetStore) GetCoordinates(ctx context.Context, planetID string) (name string, galaxy, system, position int, err error) {
	err = s.db.QueryRowContext(ctx,
		"SELECT name, galaxy, system, position FROM celestial_objects WHERE id = $1",
		planetID,
	).Scan(&name, &galaxy, &system, &position)
	return
}

func (s *PlanetStore) activeFleets(ctx context.Context, userID int64) ([]models.FleetEventSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, mission, phase, origin_galaxy, origin_system, origin_position, origin_type,
		       target_galaxy, target_system, target_position, target_type,
		       start_time, arrival_time, cargo_metal, cargo_crystal, cargo_deuterium
		FROM fleets
		WHERE user_id = $1 AND phase IN ('OUTBOUND', 'HOLDING', 'RETURNING')
		ORDER BY arrival_time ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fleets := make([]models.FleetEventSummary, 0)
	for rows.Next() {
		var f models.FleetEventSummary
		var oG, oS, oP, tG, tS, tP int
		var oT, tT, missionStr, phaseStr string
		err := rows.Scan(
			&f.FleetID, &missionStr, &phaseStr,
			&oG, &oS, &oP, &oT,
			&tG, &tS, &tP, &tT,
			&f.DepartureTime, &f.ArrivalTime,
			&f.Cargo.Metal, &f.Cargo.Crystal, &f.Cargo.Deuterium,
		)
		if err != nil {
			continue
		}
		f.Mission = models.MissionType(missionStr)
		f.Phase = models.FleetPhase(phaseStr)
		f.Origin = models.Coordinates{Galaxy: oG, System: oS, Position: oP, Type: models.CelestialType(oT)}
		f.Destination = models.Coordinates{Galaxy: tG, System: tS, Position: tP, Type: models.CelestialType(tT)}
		f.RemainingSecs = int64(time.Until(f.ArrivalTime).Seconds())
		if f.RemainingSecs < 0 {
			f.RemainingSecs = 0
		}
		fleets = append(fleets, f)
	}
	return fleets, nil
}

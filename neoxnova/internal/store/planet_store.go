package store

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"neoxnova/internal/models"
)

type PlanetStore struct {
	db *sql.DB
}

func NewPlanetStore(db *sql.DB) *PlanetStore {
	return &PlanetStore{db: db}
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

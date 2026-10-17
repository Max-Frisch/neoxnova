package store

import (
	"context"
	"time"

	"github.com/lib/pq"

	"neoxnova/internal/models"
)

// EmpireOverview loads the Empire matrix (live page=imperium): every owned
// planet and moon in creation (id) order, each with its on-demand accrued
// stock, cached hourly production (including the moon Collider's Dark Matter/h),
// field usage and the full building / ship / defense counts.
//
// Resources are accrued per celestial with the same update_celestial_resources
// function the overview page uses, so the matrix always shows live stock. The
// three matrices are then loaded with one grouped query each (`= ANY(ids)`)
// rather than one query per celestial.
func (s *PlanetStore) EmpireOverview(ctx context.Context, userID int64) ([]models.EmpireCelestial, error) {
	ids, err := s.ownedCelestialIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []models.EmpireCelestial{}, nil
	}

	current := make(map[int64][3]float64, len(ids))
	for _, id := range ids {
		var metal, crystal, deut float64
		var lastCalc time.Time
		if err := s.db.QueryRowContext(ctx,
			"SELECT * FROM update_celestial_resources($1)", id,
		).Scan(&metal, &crystal, &deut, &lastCalc); err != nil {
			return nil, err
		}
		current[id] = [3]float64{metal, crystal, deut}
	}

	out, index, err := s.empireCelestials(ctx, userID, current)
	if err != nil {
		return nil, err
	}
	if err := s.loadEmpireBuildings(ctx, ids, out, index); err != nil {
		return nil, err
	}
	if err := s.loadEmpireUnitTable(ctx, ids, out, index, "planet_ships", "ship_code", func(c *models.EmpireCelestial) *map[string]int64 { return &c.Ships }); err != nil {
		return nil, err
	}
	if err := s.loadEmpireUnitTable(ctx, ids, out, index, "planet_defenses", "defense_code", func(c *models.EmpireCelestial) *map[string]int64 { return &c.Defenses }); err != nil {
		return nil, err
	}
	return out, nil
}

// ownedCelestialIDs lists an account's planets and moons in creation order.
func (s *PlanetStore) ownedCelestialIDs(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM celestial_objects
		WHERE user_id = $1 AND object_type IN ('PLANET', 'MOON')
		ORDER BY id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// empireCelestials builds the per-celestial rows (minus the matrices), flagging
// the oldest planet as the homeworld and merging in the accrued stock.
func (s *PlanetStore) empireCelestials(ctx context.Context, userID int64, current map[int64][3]float64) ([]models.EmpireCelestial, map[int64]int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, COALESCE(c.name, ''), c.object_type::text, c.galaxy, c.system, c.position,
		       c.fields_used, c.fields_max,
		       c.metal_prod_hourly, c.crystal_prod_hourly, c.deuterium_prod_hourly,
		       c.energy_used, c.energy_max, c.collider_dm_prod_hourly
		FROM celestial_objects c
		WHERE c.user_id = $1 AND c.object_type IN ('PLANET', 'MOON')
		ORDER BY c.id
	`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var homeID int64
	out := make([]models.EmpireCelestial, 0)
	for rows.Next() {
		var c models.EmpireCelestial
		var otype string
		var energyUsed, energyMax int
		var dmPerHour float64
		if err := rows.Scan(
			&c.ID, &c.Name, &otype, &c.Galaxy, &c.System, &c.Position,
			&c.FieldsUsed, &c.FieldsMax,
			&c.MetalPerHour, &c.CrystalPerHour, &c.DeuteriumPerHour,
			&energyUsed, &energyMax, &dmPerHour,
		); err != nil {
			return nil, nil, err
		}
		c.Type = models.CelestialType(otype)
		c.EnergyMax = energyMax
		c.EnergyAvailable = energyMax - energyUsed
		c.DarkMatterPerHour = dmPerHour
		if cur, ok := current[c.ID]; ok {
			c.Metal, c.Crystal, c.Deuterium = cur[0], cur[1], cur[2]
		}
		if c.Type == models.TypePlanet && (homeID == 0 || c.ID < homeID) {
			homeID = c.ID
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	index := make(map[int64]int, len(out))
	for i := range out {
		out[i].IsHome = out[i].ID == homeID
		index[out[i].ID] = i
	}
	return out, index, nil
}

// loadEmpireBuildings fills each celestial's building levels with one grouped
// query over planet_structures.
func (s *PlanetStore) loadEmpireBuildings(ctx context.Context, ids []int64, out []models.EmpireCelestial, index map[int64]int) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT celestial_id, structure_code, level
		FROM planet_structures
		WHERE celestial_id = ANY($1) AND level > 0
	`, pq.Array(ids))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var code string
		var level int
		if err := rows.Scan(&id, &code, &level); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok {
			continue
		}
		if out[i].Buildings == nil {
			out[i].Buildings = map[string]int{}
		}
		out[i].Buildings[code] = level
	}
	return rows.Err()
}

// loadEmpireUnitTable fills each celestial's ship or defense counts with one
// grouped query over the given table/code column; pick returns the map field to
// populate for a celestial.
func (s *PlanetStore) loadEmpireUnitTable(ctx context.Context, ids []int64, out []models.EmpireCelestial, index map[int64]int, table, codeColumn string, pick func(*models.EmpireCelestial) *map[string]int64) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT celestial_id, `+codeColumn+`, quantity
		FROM `+table+`
		WHERE celestial_id = ANY($1) AND quantity > 0
	`, pq.Array(ids))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var code string
		var qty int64
		if err := rows.Scan(&id, &code, &qty); err != nil {
			return err
		}
		i, ok := index[id]
		if !ok {
			continue
		}
		m := pick(&out[i])
		if *m == nil {
			*m = map[string]int64{}
		}
		(*m)[code] = qty
	}
	return rows.Err()
}

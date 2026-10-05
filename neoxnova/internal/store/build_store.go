package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/lib/pq"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
)

type BuildStore struct {
	db *sql.DB
}

func NewBuildStore(db *sql.DB) *BuildStore {
	return &BuildStore{db: db}
}

// QueueResult is returned by every successful enqueue operation.
type QueueResult struct {
	QueueID  int64
	EndTime  time.Time
	Duration time.Duration
	Cost     game.Cost
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

func scanLevels(rows *sql.Rows) (map[string]int, error) {
	defer rows.Close()
	levels := make(map[string]int)
	for rows.Next() {
		var code string
		var level int
		if err := rows.Scan(&code, &level); err != nil {
			return nil, err
		}
		levels[code] = level
	}
	return levels, rows.Err()
}

func (s *BuildStore) planetContext(ctx context.Context, tx *sql.Tx, planetID int64) (userID int64, tempMax, fieldsUsed, fieldsMax int, gameSpeed float64, metal, crystal, deuterium float64, err error) {
	var lastCalc time.Time
	err = tx.QueryRowContext(ctx, `SELECT * FROM update_celestial_resources($1)`, planetID).Scan(
		&metal, &crystal, &deuterium, &lastCalc,
	)
	if err != nil {
		return
	}
	err = tx.QueryRowContext(ctx, `
		SELECT c.user_id, c.temp_max, c.fields_used, c.fields_max, u.game_speed
		FROM celestial_objects c
		JOIN universes u ON u.id = c.universe_id
		WHERE c.id = $1
	`, planetID).Scan(&userID, &tempMax, &fieldsUsed, &fieldsMax, &gameSpeed)
	return
}

func (s *BuildStore) structureLevels(ctx context.Context, tx *sql.Tx, planetID int64) (map[string]int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT structure_code, level FROM planet_structures WHERE celestial_id = $1`, planetID)
	if err != nil {
		return nil, err
	}
	return scanLevels(rows)
}

func (s *BuildStore) techLevels(ctx context.Context, tx *sql.Tx, userID int64) (map[string]int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT tech_code, level FROM user_technologies WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	return scanLevels(rows)
}

// EnqueueStructure starts construction of the next level of a building.
func (s *BuildStore) EnqueueStructure(ctx context.Context, planetID int64, code string) (QueueResult, error) {
	if _, ok := game.Structures[code]; !ok {
		return QueueResult{}, ErrUnknownCode
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return QueueResult{}, err
	}
	defer tx.Rollback()

	_, _, fieldsUsed, fieldsMax, gameSpeed, metal, crystal, deuterium, err := s.planetContext(ctx, tx, planetID)
	if errors.Is(err, sql.ErrNoRows) {
		return QueueResult{}, ErrNotFound
	} else if err != nil {
		return QueueResult{}, err
	}

	levels, err := s.structureLevels(ctx, tx, planetID)
	if err != nil {
		return QueueResult{}, err
	}
	def := game.Structures[code]
	if !game.RequiresMet(def.Requires, levels) {
		return QueueResult{}, ErrPrerequisites
	}
	if fieldsUsed+1 > fieldsMax {
		return QueueResult{}, ErrNoFields
	}

	var busy bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM construction_queues WHERE celestial_id = $1 AND status = 'IN_PROGRESS')`, planetID).Scan(&busy); err != nil {
		return QueueResult{}, err
	}
	if busy {
		return QueueResult{}, ErrQueueBusy
	}

	targetLevel := levels[code] + 1
	cost, _ := game.StructureCost(code, targetLevel)
	if metal < float64(cost.Metal) || crystal < float64(cost.Crystal) || deuterium < float64(cost.Deuterium) {
		return QueueResult{}, ErrInsufficientResources
	}

	duration := game.StructureDuration(code, targetLevel, levels["robotics_factory"], levels["nanite_factory"], gameSpeed)
	start := time.Now()
	end := start.Add(duration)

	if _, err := tx.ExecContext(ctx, `
		UPDATE celestial_objects
		SET metal = metal - $1, crystal = crystal - $2, deuterium = deuterium - $3
		WHERE id = $4
	`, cost.Metal, cost.Crystal, cost.Deuterium, planetID); err != nil {
		return QueueResult{}, err
	}

	var queueID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO construction_queues (celestial_id, structure_code, target_level, start_time, end_time, status)
		VALUES ($1, $2, $3, $4, $5, 'IN_PROGRESS')
		RETURNING id
	`, planetID, code, targetLevel, start, end).Scan(&queueID)
	if isUniqueViolation(err) {
		return QueueResult{}, ErrQueueBusy
	} else if err != nil {
		return QueueResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return QueueResult{}, err
	}
	return QueueResult{QueueID: queueID, EndTime: end, Duration: duration, Cost: cost}, nil
}

// EnqueueShipyard starts building a batch of ships.
func (s *BuildStore) EnqueueShipyard(ctx context.Context, planetID int64, unitCode string, quantity int64) (QueueResult, error) {
	unitDef, ok := game.LookupUnit(unitCode)
	if !ok {
		return QueueResult{}, ErrUnknownCode
	}
	if quantity < 1 {
		return QueueResult{}, ErrInvalidQuantity
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return QueueResult{}, err
	}
	defer tx.Rollback()

	_, _, _, _, gameSpeed, metal, crystal, deuterium, err := s.planetContext(ctx, tx, planetID)
	if errors.Is(err, sql.ErrNoRows) {
		return QueueResult{}, ErrNotFound
	} else if err != nil {
		return QueueResult{}, err
	}

	levels, err := s.structureLevels(ctx, tx, planetID)
	if err != nil {
		return QueueResult{}, err
	}
	if !game.RequiresMet(unitDef.Requires, levels) {
		return QueueResult{}, ErrPrerequisites
	}

	var busy bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM shipyard_queues WHERE celestial_id = $1 AND status = 'IN_PROGRESS')`, planetID).Scan(&busy); err != nil {
		return QueueResult{}, err
	}
	if busy {
		return QueueResult{}, ErrQueueBusy
	}

	cost, _ := game.UnitCost(unitCode, quantity)
	if metal < float64(cost.Metal) || crystal < float64(cost.Crystal) || deuterium < float64(cost.Deuterium) {
		return QueueResult{}, ErrInsufficientResources
	}

	duration := game.UnitDuration(unitCode, quantity, levels["shipyard"], levels["nanite_factory"], gameSpeed)
	start := time.Now()
	end := start.Add(duration)
	perUnit := duration.Seconds() / float64(quantity)

	if _, err := tx.ExecContext(ctx, `
		UPDATE celestial_objects
		SET metal = metal - $1, crystal = crystal - $2, deuterium = deuterium - $3
		WHERE id = $4
	`, cost.Metal, cost.Crystal, cost.Deuterium, planetID); err != nil {
		return QueueResult{}, err
	}

	var queueID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO shipyard_queues (celestial_id, unit_code, quantity_total, quantity_completed, build_time_per_unit, start_time, end_time, status)
		VALUES ($1, $2, $3, 0, $4, $5, $6, 'IN_PROGRESS')
		RETURNING id
	`, planetID, unitCode, quantity, perUnit, start, end).Scan(&queueID)
	if isUniqueViolation(err) {
		return QueueResult{}, ErrQueueBusy
	} else if err != nil {
		return QueueResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return QueueResult{}, err
	}
	return QueueResult{QueueID: queueID, EndTime: end, Duration: duration, Cost: cost}, nil
}

// EnqueueResearch starts an empire-wide technology. Costs are drawn from the
// supplied planet, whose research lab gates the target level.
func (s *BuildStore) EnqueueResearch(ctx context.Context, planetID int64, techCode string) (QueueResult, error) {
	if _, ok := game.Techs[techCode]; !ok {
		return QueueResult{}, ErrUnknownCode
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return QueueResult{}, err
	}
	defer tx.Rollback()

	userID, _, _, _, gameSpeed, metal, crystal, deuterium, err := s.planetContext(ctx, tx, planetID)
	if errors.Is(err, sql.ErrNoRows) {
		return QueueResult{}, ErrNotFound
	} else if err != nil {
		return QueueResult{}, err
	}

	structLevels, err := s.structureLevels(ctx, tx, planetID)
	if err != nil {
		return QueueResult{}, err
	}
	techs, err := s.techLevels(ctx, tx, userID)
	if err != nil {
		return QueueResult{}, err
	}
	combined := make(map[string]int, len(structLevels)+len(techs))
	for k, v := range structLevels {
		combined[k] = v
	}
	for k, v := range techs {
		combined[k] = v
	}
	if !game.RequiresMet(game.Techs[techCode].Requires, combined) {
		return QueueResult{}, ErrPrerequisites
	}

	targetLevel := techs[techCode] + 1
	if structLevels["research_lab"] < targetLevel {
		return QueueResult{}, ErrPrerequisites
	}

	var busy bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM research_queues WHERE user_id = $1 AND status = 'IN_PROGRESS')`, userID).Scan(&busy); err != nil {
		return QueueResult{}, err
	}
	if busy {
		return QueueResult{}, ErrQueueBusy
	}

	cost, _ := game.TechCost(techCode, targetLevel)
	if metal < float64(cost.Metal) || crystal < float64(cost.Crystal) || deuterium < float64(cost.Deuterium) {
		return QueueResult{}, ErrInsufficientResources
	}

	duration := game.TechDuration(techCode, targetLevel, structLevels["research_lab"], gameSpeed)
	start := time.Now()
	end := start.Add(duration)

	if _, err := tx.ExecContext(ctx, `
		UPDATE celestial_objects
		SET metal = metal - $1, crystal = crystal - $2, deuterium = deuterium - $3
		WHERE id = $4
	`, cost.Metal, cost.Crystal, cost.Deuterium, planetID); err != nil {
		return QueueResult{}, err
	}

	var queueID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO research_queues (user_id, celestial_id, tech_code, target_level, start_time, end_time, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'IN_PROGRESS')
		RETURNING id
	`, userID, planetID, techCode, targetLevel, start, end).Scan(&queueID)
	if isUniqueViolation(err) {
		return QueueResult{}, ErrQueueBusy
	} else if err != nil {
		return QueueResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return QueueResult{}, err
	}
	return QueueResult{QueueID: queueID, EndTime: end, Duration: duration, Cost: cost}, nil
}

// RecomputeCelestial rewrites the cached production/energy/field columns from
// the current structure levels. Callers should bank accrued resources first
// (update_celestial_resources) so the new rate is not applied retroactively.
func (s *BuildStore) RecomputeCelestial(ctx context.Context, tx *sql.Tx, celestialID int64) error {
	var tempMax int
	var resourceSpeed float64
	var baseFields int64
	if err := tx.QueryRowContext(ctx, `
		SELECT c.temp_max, u.resource_speed, c.base_fields_max
		FROM celestial_objects c
		JOIN universes u ON u.id = c.universe_id
		WHERE c.id = $1
	`, celestialID).Scan(&tempMax, &resourceSpeed, &baseFields); err != nil {
		return err
	}
	var satCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(quantity, 0) FROM planet_ships WHERE celestial_id = $1 AND ship_code = '212'
	`, celestialID).Scan(&satCount); err != nil {
		return err
	}
	levels, err := s.structureLevels(ctx, tx, celestialID)
	if err != nil {
		return err
	}
	eco := game.RecomputeProduction(levels, tempMax, resourceSpeed, satCount)

	fieldsUsed := 0
	for _, lvl := range levels {
		fieldsUsed += lvl
	}
	// Terraformer adds 7 fields per level on top of the colonisation roll.
	fieldsMax := baseFields + 7*int64(levels["terraformer"])

	_, err = tx.ExecContext(ctx, `
		UPDATE celestial_objects
		SET metal_prod_hourly = $1, crystal_prod_hourly = $2, deuterium_prod_hourly = $3,
		    energy_used = $4, energy_max = $5, fields_used = $6, fields_max = $7
		WHERE id = $8
	`, eco.MetalPerHour, eco.CrystalPerHour, eco.DeutPerHour, int(eco.EnergyUsed), int(eco.EnergyMax), fieldsUsed, fieldsMax, celestialID)
	return err
}

// GetBuildings returns structure levels, the active construction item and the
// next-level costs for every known structure.
func (s *BuildStore) GetBuildings(ctx context.Context, planetID string) (models.BuildingsResponse, error) {
	id, err := strconv.ParseInt(planetID, 10, 64)
	if err != nil {
		return models.BuildingsResponse{}, ErrNotFound
	}

	if _, err := s.db.ExecContext(ctx, `SELECT * FROM update_celestial_resources($1)`, id); err != nil {
		return models.BuildingsResponse{}, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT structure_code, level FROM planet_structures WHERE celestial_id = $1`, id)
	if err != nil {
		return models.BuildingsResponse{}, err
	}
	levels, err := scanLevels(rows)
	if err != nil {
		return models.BuildingsResponse{}, err
	}

	resp := models.BuildingsResponse{PlanetID: id, Levels: levels, NextCosts: map[string]models.CargoManifest{}}
	for code := range game.Structures {
		if c, ok := game.StructureCost(code, levels[code]+1); ok {
			resp.NextCosts[code] = models.CargoManifest{Metal: c.Metal, Crystal: c.Crystal, Deuterium: c.Deuterium}
		}
	}

	var q models.QueueEntrySummary
	err = s.db.QueryRowContext(ctx, `
		SELECT id, structure_code, target_level, start_time, end_time
		FROM construction_queues
		WHERE celestial_id = $1 AND status = 'IN_PROGRESS'
		LIMIT 1
	`, id).Scan(&q.ID, &q.Code, &q.TargetLevel, &q.StartTime, &q.EndTime)
	if err == nil {
		q.RemainingSecs = int64(time.Until(q.EndTime).Seconds())
		if q.RemainingSecs < 0 {
			q.RemainingSecs = 0
		}
		resp.Queue = &q
	} else if !errors.Is(err, sql.ErrNoRows) {
		return models.BuildingsResponse{}, err
	}

	return resp, nil
}

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

func (s *BuildStore) planetContext(ctx context.Context, tx *sql.Tx, planetID int64) (userID int64, objectType string, tempMax, fieldsUsed, fieldsMax int, gameSpeed float64, metal, crystal, deuterium float64, err error) {
	var lastCalc time.Time
	err = tx.QueryRowContext(ctx, `SELECT * FROM update_celestial_resources($1)`, planetID).Scan(
		&metal, &crystal, &deuterium, &lastCalc,
	)
	if err != nil {
		return
	}
	err = tx.QueryRowContext(ctx, `
		SELECT c.user_id, c.object_type::text, c.temp_max, c.fields_used, c.fields_max, u.game_speed
		FROM celestial_objects c
		JOIN universes u ON u.id = c.universe_id
		WHERE c.id = $1
	`, planetID).Scan(&userID, &objectType, &tempMax, &fieldsUsed, &fieldsMax, &gameSpeed)
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

// combinedLevels merges a planet's structure levels with its owner's research
// levels. Prerequisite checks must see both: many ships/defenses require a drive
// or weapons technology (e.g. Light Fighter needs combustion_drive), and some
// structures need techs too (e.g. Terraformer needs energy_tech).
func (s *BuildStore) combinedLevels(ctx context.Context, tx *sql.Tx, planetID, userID int64) (map[string]int, error) {
	levels, err := s.structureLevels(ctx, tx, planetID)
	if err != nil {
		return nil, err
	}
	techs, err := s.techLevels(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	for code, lvl := range techs {
		if lvl > levels[code] {
			levels[code] = lvl
		}
	}
	return levels, nil
}

// researchLabLevels returns the research-lab levels of the user's other
// celestials (excluding the origin planet), used to compute how many labs the
// Intergalactic Research Network connects.
func (s *BuildStore) researchLabLevels(ctx context.Context, tx *sql.Tx, userID, originID int64) ([]int, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT ps.level
		FROM planet_structures ps
		JOIN celestial_objects c ON c.id = ps.celestial_id
		WHERE c.user_id = $1 AND ps.structure_code = 'research_lab' AND c.id <> $2
	`, userID, originID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var labs []int
	for rows.Next() {
		var lvl int
		if err := rows.Scan(&lvl); err != nil {
			return nil, err
		}
		labs = append(labs, lvl)
	}
	return labs, rows.Err()
}

// EnqueueStructure starts construction of the next level of a building.
func (s *BuildStore) EnqueueStructure(ctx context.Context, planetID int64, code string) (QueueResult, error) {
	if _, ok := game.Structures[code]; !ok {
		if _, moonOK := game.MoonOnlyStructures[code]; !moonOK {
			return QueueResult{}, ErrUnknownCode
		}
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return QueueResult{}, err
	}
	defer tx.Rollback()

	userID, objectType, _, fieldsUsed, fieldsMax, gameSpeed, metal, crystal, deuterium, err := s.planetContext(ctx, tx, planetID)
	if errors.Is(err, sql.ErrNoRows) {
		return QueueResult{}, ErrNotFound
	} else if err != nil {
		return QueueResult{}, err
	}

	// A moon may only build its moon-legal set (moon-only buildings plus the
	// moon-legal planet buildings); a planet may not build moon-only buildings.
	if objectType == "MOON" {
		if _, ok := game.MoonStructure(code); !ok {
			return QueueResult{}, ErrUnknownCode
		}
	}

	levels, err := s.combinedLevels(ctx, tx, planetID, userID)
	if err != nil {
		return QueueResult{}, err
	}
	def := game.Structures[code]
	if objectType == "MOON" {
		def, _ = game.MoonStructure(code)
	}
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

	userID, _, _, _, _, gameSpeed, metal, crystal, deuterium, err := s.planetContext(ctx, tx, planetID)
	if errors.Is(err, sql.ErrNoRows) {
		return QueueResult{}, ErrNotFound
	} else if err != nil {
		return QueueResult{}, err
	}

	levels, err := s.combinedLevels(ctx, tx, planetID, userID)
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
// supplied planet. The research lab only *unlocks* technologies (via each tech's
// Requires) and shortens research time; it does not cap the target level. Time
// uses that planet's effective lab level (its own lab plus IRN-connected colony
// labs) and its local University, which is not shared account-wide.
func (s *BuildStore) EnqueueResearch(ctx context.Context, planetID int64, techCode string) (QueueResult, error) {
	if _, ok := game.Techs[techCode]; !ok {
		return QueueResult{}, ErrUnknownCode
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return QueueResult{}, err
	}
	defer tx.Rollback()

	userID, _, _, _, _, gameSpeed, metal, crystal, deuterium, err := s.planetContext(ctx, tx, planetID)
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
	colonyLabs, err := s.researchLabLevels(ctx, tx, userID, planetID)
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

	labLevel := game.EffectiveResearchLabLevel(structLevels["research_lab"], techs["intergalactic_research_network"], colonyLabs)
	duration := game.TechDuration(techCode, targetLevel, labLevel, structLevels["university"], gameSpeed)
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

// CancelScope selects which build queue a cancel applies to.
type CancelScope string

const (
	CancelConstruction CancelScope = "construction"
	CancelShipyard     CancelScope = "shipyard"
	CancelResearch     CancelScope = "research"
)

// CancelQueue removes the single active queue entry for the given scope and
// refunds its full cost to the celestial the work was charged against. It is
// multi-instance safe: the row is locked with FOR UPDATE SKIP LOCKED and the
// condition re-checked inside the transaction, so a scheduler resolving the
// same queue concurrently wins the race and this call simply finds nothing to
// cancel (ErrNotFound). An entry already due (end_time <= NOW()) is treated as
// gone for the same reason.
//
// ownerID is the celestial id for CancelConstruction/CancelShipyard and the
// user id for CancelResearch. The refund is credited to the celestial the work
// drew from (research stores the planet it was started on).
func (s *BuildStore) CancelQueue(ctx context.Context, scope CancelScope, ownerID int64) (game.Cost, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return game.Cost{}, err
	}
	defer tx.Rollback()

	var (
		table       string
		queueID     int64
		celestialID int64
		cost        game.Cost
	)

	switch scope {
	case CancelConstruction:
		var code string
		var targetLevel int
		err = tx.QueryRowContext(ctx, `
			SELECT id, structure_code, target_level
			FROM construction_queues
			WHERE celestial_id = $1 AND status = 'IN_PROGRESS' AND end_time > NOW()
			FOR UPDATE SKIP LOCKED
		`, ownerID).Scan(&queueID, &code, &targetLevel)
		if err != nil {
			return game.Cost{}, cancelLookupErr(err)
		}
		table, celestialID = "construction_queues", ownerID
		cost, _ = game.StructureCost(code, targetLevel)

	case CancelShipyard:
		var code string
		var quantity int64
		err = tx.QueryRowContext(ctx, `
			SELECT id, unit_code, quantity_total
			FROM shipyard_queues
			WHERE celestial_id = $1 AND status = 'IN_PROGRESS' AND end_time > NOW()
			FOR UPDATE SKIP LOCKED
		`, ownerID).Scan(&queueID, &code, &quantity)
		if err != nil {
			return game.Cost{}, cancelLookupErr(err)
		}
		table, celestialID = "shipyard_queues", ownerID
		cost, _ = game.UnitCost(code, quantity)

	case CancelResearch:
		var code string
		var targetLevel int
		err = tx.QueryRowContext(ctx, `
			SELECT id, celestial_id, tech_code, target_level
			FROM research_queues
			WHERE user_id = $1 AND status = 'IN_PROGRESS' AND end_time > NOW()
			FOR UPDATE SKIP LOCKED
		`, ownerID).Scan(&queueID, &celestialID, &code, &targetLevel)
		if err != nil {
			return game.Cost{}, cancelLookupErr(err)
		}
		table = "research_queues"
		cost, _ = game.TechCost(code, targetLevel)

	default:
		return game.Cost{}, ErrNotFound
	}

	// Bank accrued output first, then refund. update_celestial_resources takes
	// the celestial row lock, so this stays consistent with the scheduler.
	if _, err := tx.ExecContext(ctx, `SELECT * FROM update_celestial_resources($1)`, celestialID); err != nil {
		return game.Cost{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE celestial_objects
		SET metal = metal + $1, crystal = crystal + $2, deuterium = deuterium + $3
		WHERE id = $4
	`, cost.Metal, cost.Crystal, cost.Deuterium, celestialID); err != nil {
		return game.Cost{}, err
	}
	// table is one of the fixed literals selected above, never user input.
	if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET status = 'CANCELLED' WHERE id = $1`, queueID); err != nil {
		return game.Cost{}, err
	}
	if err := tx.Commit(); err != nil {
		return game.Cost{}, err
	}
	return cost, nil
}

// cancelLookupErr maps a no-row lookup to ErrNotFound so callers can render
// "nothing to cancel" without distinguishing a missing queue from a lost race.
func cancelLookupErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// RecomputeCelestial rewrites the cached production/energy/field columns from
// the current structure levels. Callers should bank accrued resources first
// (update_celestial_resources) so the new rate is not applied retroactively.
func (s *BuildStore) RecomputeCelestial(ctx context.Context, tx *sql.Tx, celestialID int64) error {
	var tempMax int
	var resourceSpeed float64
	var baseFields int64
	var owner sql.NullInt64
	var objectType string
	if err := tx.QueryRowContext(ctx, `
		SELECT c.temp_max, u.resource_speed, c.base_fields_max, c.user_id, c.object_type
		FROM celestial_objects c
		JOIN universes u ON u.id = c.universe_id
		WHERE c.id = $1
	`, celestialID).Scan(&tempMax, &resourceSpeed, &baseFields, &owner, &objectType); err != nil {
		return err
	}
	// A planet that has never built a solar satellite has no row here, so wrap
	// the lookup in a sub-select: COALESCE over a missing row still yields zero
	// rows and would fail the scans below.
	var satCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE((SELECT quantity FROM planet_ships WHERE celestial_id = $1 AND ship_code = '212'), 0)
	`, celestialID).Scan(&satCount); err != nil {
		return err
	}
	levels, err := s.structureLevels(ctx, tx, celestialID)
	if err != nil {
		return err
	}
	bonus := game.ProductionBonus{}
	if owner.Valid {
		upgrades, err := LoadAccountUpgrades(ctx, tx, owner.Int64)
		if err != nil {
			return err
		}
		techs, err := s.techLevels(ctx, tx, owner.Int64)
		if err != nil {
			return err
		}
		bonus = game.ProductionBonusFor(upgrades).Plus(game.ResearchProductionBonus(techs))
	}
	eco := game.RecomputeProduction(levels, tempMax, resourceSpeed, satCount, bonus, objectType == "PLANET")

	fieldsUsed := 0
	for _, lvl := range levels {
		fieldsUsed += lvl
	}
	// A moon's capacity comes from its Moon base (1 + 3 per level); the
	// colonisation roll + Terraformer only apply to planets.
	fieldsMax := baseFields + 7*int64(levels["terraformer"])
	if objectType == "MOON" {
		fieldsMax = int64(game.MoonFieldsMax(levels["moon_base"]))
	}

	metalCap := game.StorageCapacity(levels["metal_storage"], resourceSpeed)
	crystalCap := game.StorageCapacity(levels["crystal_storage"], resourceSpeed)
	deutCap := game.StorageCapacity(levels["deuterium_storage"], resourceSpeed)

	_, err = tx.ExecContext(ctx, `
		UPDATE celestial_objects
		SET metal_prod_hourly = $1, crystal_prod_hourly = $2, deuterium_prod_hourly = $3,
		    energy_used = $4, energy_max = $5, fields_used = $6, fields_max = $7,
		    metal_capacity = $8, crystal_capacity = $9, deuterium_capacity = $10
		WHERE id = $11
	`, eco.MetalPerHour, eco.CrystalPerHour, eco.DeutPerHour, int(eco.EnergyUsed), int(eco.EnergyMax), fieldsUsed, fieldsMax,
		metalCap, crystalCap, deutCap, celestialID)
	return err
}

// RecomputeAccountProduction banks every owned planet's accrued resources and
// rewrites its cached production/energy/field/capacity columns with the
// account's current bonuses (Arsenal upgrades plus extraction research). It runs
// in one transaction so the whole account flips atomically, and is called when
// an account-wide production source (an Arsenal upgrade or Mineral/
// Semi-Crystals/Fuel Research) changes.
func (s *BuildStore) RecomputeAccountProduction(ctx context.Context, accountID int64) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM celestial_objects WHERE user_id = $1 AND object_type = 'PLANET'
	`, accountID)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `SELECT * FROM update_celestial_resources($1)`, id); err != nil {
			return err
		}
		if err := s.RecomputeCelestial(ctx, tx, id); err != nil {
			return err
		}
	}
	return tx.Commit()
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

	var objectType string
	_ = s.db.QueryRowContext(ctx, `SELECT object_type FROM celestial_objects WHERE id = $1`, id).Scan(&objectType)

	resp := models.BuildingsResponse{PlanetID: id, Levels: levels, NextCosts: map[string]models.CargoManifest{}}
	codes := make([]string, 0, len(game.Structures))
	if objectType == "MOON" {
		for _, d := range game.MoonStructureDefs() {
			codes = append(codes, d.Code)
		}
	} else {
		for code := range game.Structures {
			codes = append(codes, code)
		}
	}
	for _, code := range codes {
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

// GetResearch returns empire-wide technology levels, the active research item
// and the next-level cost for every known tech (for the research page).
func (s *BuildStore) GetResearch(ctx context.Context, planetID string) (models.ResearchResponse, error) {
	id, err := strconv.ParseInt(planetID, 10, 64)
	if err != nil {
		return models.ResearchResponse{}, ErrNotFound
	}
	if _, err := s.db.ExecContext(ctx, `SELECT * FROM update_celestial_resources($1)`, id); err != nil {
		return models.ResearchResponse{}, err
	}

	var userID int64
	if err := s.db.QueryRowContext(ctx, `SELECT user_id FROM celestial_objects WHERE id = $1`, id).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.ResearchResponse{}, ErrNotFound
		}
		return models.ResearchResponse{}, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT tech_code, level FROM user_technologies WHERE user_id = $1`, userID)
	if err != nil {
		return models.ResearchResponse{}, err
	}
	levels, err := scanLevels(rows)
	if err != nil {
		return models.ResearchResponse{}, err
	}

	resp := models.ResearchResponse{PlanetID: id, Levels: levels, NextCosts: map[string]models.CargoManifest{}}
	for code := range game.Techs {
		if c, ok := game.TechCost(code, levels[code]+1); ok {
			resp.NextCosts[code] = models.CargoManifest{Metal: c.Metal, Crystal: c.Crystal, Deuterium: c.Deuterium}
		}
	}

	var q models.QueueEntrySummary
	err = s.db.QueryRowContext(ctx, `
		SELECT id, tech_code, target_level, start_time, end_time
		FROM research_queues
		WHERE user_id = $1 AND status = 'IN_PROGRESS'
		LIMIT 1
	`, userID).Scan(&q.ID, &q.Code, &q.TargetLevel, &q.StartTime, &q.EndTime)
	if err == nil {
		q.RemainingSecs = int64(time.Until(q.EndTime).Seconds())
		if q.RemainingSecs < 0 {
			q.RemainingSecs = 0
		}
		resp.Queue = &q
	} else if !errors.Is(err, sql.ErrNoRows) {
		return models.ResearchResponse{}, err
	}

	return resp, nil
}

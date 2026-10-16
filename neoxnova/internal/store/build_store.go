package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
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

// QueueResult is returned by every successful enqueue operation. For chained
// multi-order enqueues (structures/research targeting a level) it reports the
// last order inserted plus how many levels/orders were queued and the final
// target reached.
type QueueResult struct {
	QueueID     int64
	EndTime     time.Time
	Duration    time.Duration
	Cost        game.Cost
	Queued      int
	Quantity    int64
	TargetLevel int
}

// affordableQty is the largest batch of a unit payable from the given resources;
// 0 when at least one unit's cost cannot be met. A zero-cost component is
// ignored (no limit from it).
func affordableQty(metal, crystal, deuterium float64, per game.Cost) int64 {
	limit := int64(math.MaxInt64)
	if per.Metal > 0 {
		if v := int64(metal) / per.Metal; v < limit {
			limit = v
		}
	}
	if per.Crystal > 0 {
		if v := int64(crystal) / per.Crystal; v < limit {
			limit = v
		}
	}
	if per.Deuterium > 0 {
		if v := int64(deuterium) / per.Deuterium; v < limit {
			limit = v
		}
	}
	if limit < 1 {
		return 0
	}
	return limit
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

func scanCounts(rows *sql.Rows) (map[string]int, error) {
	defer rows.Close()
	counts := make(map[string]int)
	for rows.Next() {
		var code string
		var n int64
		if err := rows.Scan(&code, &n); err != nil {
			return nil, err
		}
		counts[code] = int(n)
	}
	return counts, rows.Err()
}

// universeQueueSlots returns the per-scope chain-length caps for a celestial's
// universe. A cap of 0 means "unlimited" for that scope.
func (s *BuildStore) universeQueueSlots(ctx context.Context, tx *sql.Tx, planetID int64) (buildSlots, researchSlots int, err error) {
	err = tx.QueryRowContext(ctx, `
		SELECT u.build_queue_slots, u.research_queue_slots
		FROM celestial_objects c
		JOIN universes u ON u.id = c.universe_id
		WHERE c.id = $1
	`, planetID).Scan(&buildSlots, &researchSlots)
	return
}

// queuedStructureCounts returns how many IN_PROGRESS construction orders each
// structure code has on a celestial.
func (s *BuildStore) queuedStructureCounts(ctx context.Context, tx *sql.Tx, planetID int64) (map[string]int, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT structure_code, COUNT(*) FROM construction_queues
		WHERE celestial_id = $1 AND status = 'IN_PROGRESS'
		GROUP BY structure_code
	`, planetID)
	if err != nil {
		return nil, err
	}
	return scanCounts(rows)
}

// queuedResearchCounts returns how many IN_PROGRESS research orders each tech
// code has for a user.
func (s *BuildStore) queuedResearchCounts(ctx context.Context, tx *sql.Tx, userID int64) (map[string]int, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT tech_code, COUNT(*) FROM research_queues
		WHERE user_id = $1 AND status = 'IN_PROGRESS'
		GROUP BY tech_code
	`, userID)
	if err != nil {
		return nil, err
	}
	return scanCounts(rows)
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

// EnqueueStructure queues one or more levels of a building. targetLevel <= 0
// (or below the next unqueued level) queues a single level; otherwise levels
// are chained from the next unqueued level up to targetLevel, stopping early
// when the scope's slot cap is reached or the next level is unaffordable — a
// *partial* enqueue that still returns success. Each order freezes its cost
// (deducted up front) and its end_time chains behind the previous order of the
// scope, so the scheduler builds them back-to-back in order.
func (s *BuildStore) EnqueueStructure(ctx context.Context, planetID int64, code string, targetLevel int) (QueueResult, error) {
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

	buildSlots, _, err := s.universeQueueSlots(ctx, tx, planetID)
	if err != nil {
		return QueueResult{}, err
	}
	queued, err := s.queuedStructureCounts(ctx, tx, planetID)
	if err != nil {
		return QueueResult{}, err
	}
	queuedTotal := 0
	for _, n := range queued {
		queuedTotal += n
	}

	current := levels[code]
	// The goal is an absolute level; "next level" is the level after any already
	// queued orders of this code.
	if next := current + queued[code] + 1; targetLevel < next {
		targetLevel = next
	}

	var lastEnd sql.NullTime
	if err := tx.QueryRowContext(ctx, `
		SELECT MAX(end_time) FROM construction_queues
		WHERE celestial_id = $1 AND status = 'IN_PROGRESS'
	`, planetID).Scan(&lastEnd); err != nil {
		return QueueResult{}, err
	}

	result := QueueResult{TargetLevel: current}
	for {
		if buildSlots > 0 && queuedTotal >= buildSlots {
			if result.Queued == 0 {
				return QueueResult{}, ErrQueueFull
			}
			break
		}
		next := current + queued[code] + 1
		if next > targetLevel {
			break
		}
		// Every level of every structure occupies a field (fields_used is the sum
		// of all levels), so the projected use includes every queued order.
		if fieldsUsed+queuedTotal+1 > fieldsMax {
			if result.Queued == 0 {
				return QueueResult{}, ErrNoFields
			}
			break
		}
		cost, ok := game.StructureCost(code, next)
		if !ok {
			return QueueResult{}, ErrUnknownCode
		}
		total := result.Cost.Add(cost)
		if metal < float64(total.Metal) || crystal < float64(total.Crystal) || deuterium < float64(total.Deuterium) {
			if result.Queued == 0 {
				return QueueResult{}, ErrInsufficientResources
			}
			break
		}
		projRobotics := levels["robotics_factory"] + queued["robotics_factory"]
		projNanite := levels["nanite_factory"] + queued["nanite_factory"]
		duration := game.StructureDuration(code, next, projRobotics, projNanite, gameSpeed)

		start := time.Now()
		if lastEnd.Valid && lastEnd.Time.After(start) {
			start = lastEnd.Time
		}
		end := start.Add(duration)

		var queueID int64
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO construction_queues (celestial_id, structure_code, target_level, start_time, end_time, status)
			VALUES ($1, $2, $3, $4, $5, 'IN_PROGRESS')
			RETURNING id
		`, planetID, code, next, start, end).Scan(&queueID); err != nil {
			return QueueResult{}, err
		}

		queued[code]++
		queuedTotal++
		result = QueueResult{
			QueueID:     queueID,
			EndTime:     end,
			Duration:    duration,
			Cost:        total,
			Queued:      result.Queued + 1,
			TargetLevel: next,
		}
		lastEnd = sql.NullTime{Time: end, Valid: true}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE celestial_objects
		SET metal = metal - $1, crystal = crystal - $2, deuterium = deuterium - $3
		WHERE id = $4
	`, result.Cost.Metal, result.Cost.Crystal, result.Cost.Deuterium, planetID); err != nil {
		return QueueResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return QueueResult{}, err
	}
	return result, nil
}

// shipCodes / defenseCodes are the full unit catalog split by production line.
func shipCodes() []string {
	out := make([]string, 0, len(game.Ships))
	for code := range game.Ships {
		out = append(out, code)
	}
	return out
}

func defenseCodes() []string {
	out := make([]string, 0, len(game.Defenses))
	for code := range game.Defenses {
		out = append(out, code)
	}
	return out
}

// EnqueueShipyard appends a batch of ships or defenses. Ships and defenses are
// separate production lines: the batch chains behind the last in-progress batch
// of the *same kind* (unlimited — the shipyard scope has no slot cap).
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

	// Partial builds: when the requested batch is unaffordable, build as many
	// units as the current resources allow instead of failing outright.
	unitCost, _ := game.UnitCost(unitCode, 1)
	if affordable := affordableQty(metal, crystal, deuterium, unitCost); affordable < quantity {
		quantity = affordable
	}
	if quantity < 1 {
		return QueueResult{}, ErrInsufficientResources
	}
	cost, _ := game.UnitCost(unitCode, quantity)

	kindCodes := defenseCodes()
	if _, isShip := game.Ships[unitCode]; isShip {
		kindCodes = shipCodes()
	}
	var tail sql.NullTime
	if err := tx.QueryRowContext(ctx, `
		SELECT MAX(end_time) FROM shipyard_queues
		WHERE celestial_id = $1 AND status = 'IN_PROGRESS' AND unit_code = ANY($2)
	`, planetID, pq.Array(kindCodes)).Scan(&tail); err != nil {
		return QueueResult{}, err
	}

	queued, err := s.queuedStructureCounts(ctx, tx, planetID)
	if err != nil {
		return QueueResult{}, err
	}
	projShipyard := levels["shipyard"] + queued["shipyard"]
	projNanite := levels["nanite_factory"] + queued["nanite_factory"]
	duration := game.UnitDuration(unitCode, quantity, projShipyard, projNanite, gameSpeed)
	perUnit := duration.Seconds() / float64(quantity)

	start := time.Now()
	if tail.Valid && tail.Time.After(start) {
		start = tail.Time
	}
	end := start.Add(duration)

	if _, err := tx.ExecContext(ctx, `
		UPDATE celestial_objects
		SET metal = metal - $1, crystal = crystal - $2, deuterium = deuterium - $3
		WHERE id = $4
	`, cost.Metal, cost.Crystal, cost.Deuterium, planetID); err != nil {
		return QueueResult{}, err
	}

	var queueID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO shipyard_queues (celestial_id, unit_code, quantity_total, quantity_completed, build_time_per_unit, start_time, end_time, status)
		VALUES ($1, $2, $3, 0, $4, $5, $6, 'IN_PROGRESS')
		RETURNING id
	`, planetID, unitCode, quantity, perUnit, start, end).Scan(&queueID); err != nil {
		return QueueResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return QueueResult{}, err
	}
	return QueueResult{QueueID: queueID, EndTime: end, Duration: duration, Cost: cost, Queued: 1, Quantity: quantity}, nil
}

// EnqueueResearch queues one or more levels of an empire-wide technology. Costs
// are drawn from the supplied planet. targetLevel <= 0 (or below the next
// unqueued level) queues a single level; otherwise levels chain up to
// targetLevel, stopping when the account's research slot cap is reached or the
// next level is unaffordable (a partial enqueue). The research lab only
// *unlocks* technologies (via each tech's Requires) and shortens research time;
// it does not cap the target level. Durations use the projected lab (the
// planet's own lab plus IRN-connected colony labs) and its local University,
// both advanced by any already-queued orders.
func (s *BuildStore) EnqueueResearch(ctx context.Context, planetID int64, techCode string, targetLevel int) (QueueResult, error) {
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

	_, researchSlots, err := s.universeQueueSlots(ctx, tx, planetID)
	if err != nil {
		return QueueResult{}, err
	}
	queued, err := s.queuedResearchCounts(ctx, tx, userID)
	if err != nil {
		return QueueResult{}, err
	}
	queuedTotal := 0
	for _, n := range queued {
		queuedTotal += n
	}
	queuedStructs, err := s.queuedStructureCounts(ctx, tx, planetID)
	if err != nil {
		return QueueResult{}, err
	}

	current := techs[techCode]
	if next := current + queued[techCode] + 1; targetLevel < next {
		targetLevel = next
	}

	var lastEnd sql.NullTime
	if err := tx.QueryRowContext(ctx, `
		SELECT MAX(end_time) FROM research_queues
		WHERE user_id = $1 AND status = 'IN_PROGRESS'
	`, userID).Scan(&lastEnd); err != nil {
		return QueueResult{}, err
	}

	result := QueueResult{TargetLevel: current}
	for {
		if researchSlots > 0 && queuedTotal >= researchSlots {
			if result.Queued == 0 {
				return QueueResult{}, ErrQueueFull
			}
			break
		}
		next := current + queued[techCode] + 1
		if next > targetLevel {
			break
		}
		cost, ok := game.TechCost(techCode, next)
		if !ok {
			return QueueResult{}, ErrUnknownCode
		}
		total := result.Cost.Add(cost)
		if metal < float64(total.Metal) || crystal < float64(total.Crystal) || deuterium < float64(total.Deuterium) {
			if result.Queued == 0 {
				return QueueResult{}, ErrInsufficientResources
			}
			break
		}
		projIRN := techs["intergalactic_research_network"] + queued["intergalactic_research_network"]
		projLab := structLevels["research_lab"] + queuedStructs["research_lab"]
		projUni := structLevels["university"] + queuedStructs["university"]
		labLevel := game.EffectiveResearchLabLevel(projLab, projIRN, colonyLabs)
		duration := game.TechDuration(techCode, next, labLevel, projUni, gameSpeed)

		start := time.Now()
		if lastEnd.Valid && lastEnd.Time.After(start) {
			start = lastEnd.Time
		}
		end := start.Add(duration)

		var queueID int64
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO research_queues (user_id, celestial_id, tech_code, target_level, start_time, end_time, status)
			VALUES ($1, $2, $3, $4, $5, $6, 'IN_PROGRESS')
			RETURNING id
		`, userID, planetID, techCode, next, start, end).Scan(&queueID); err != nil {
			return QueueResult{}, err
		}

		queued[techCode]++
		queuedTotal++
		result = QueueResult{
			QueueID:     queueID,
			EndTime:     end,
			Duration:    duration,
			Cost:        total,
			Queued:      result.Queued + 1,
			TargetLevel: next,
		}
		lastEnd = sql.NullTime{Time: end, Valid: true}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE celestial_objects
		SET metal = metal - $1, crystal = crystal - $2, deuterium = deuterium - $3
		WHERE id = $4
	`, result.Cost.Metal, result.Cost.Crystal, result.Cost.Deuterium, planetID); err != nil {
		return QueueResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return QueueResult{}, err
	}
	return result, nil
}

// CancelScope selects which build queue a cancel applies to.
type CancelScope string

const (
	CancelConstruction CancelScope = "construction"
	CancelShipyard     CancelScope = "shipyard"
	CancelResearch     CancelScope = "research"
)

// CancelQueue cancels a single queued order identified by its row id and
// refunds its frozen cost to the celestial the work was charged against. It is
// multi-instance safe: the row is locked with FOR UPDATE and its status
// re-checked, so a scheduler resolving the same order concurrently wins the race
// and this call finds status != IN_PROGRESS or an already-due row (ErrNotFound).
//
// ownerID is the celestial id for CancelConstruction/CancelShipyard and the
// user id for CancelResearch; the row must belong to it or nothing is cancelled.
// The refund is credited to the celestial the work drew from (research stores
// the planet it was started on).
//
// Cancelling an order closes the gap in its chain: every later order is pulled
// forward, and later orders *of the same code* shift down one level, the
// level-cost difference being refunded.
func (s *BuildStore) CancelQueue(ctx context.Context, scope CancelScope, queueID, ownerID int64) (game.Cost, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return game.Cost{}, err
	}
	defer tx.Rollback()

	var (
		celestialID int64
		code        string
		targetLevel int
		quantity    int64
		start       time.Time
		cost        game.Cost
	)

	switch scope {
	case CancelConstruction:
		err = tx.QueryRowContext(ctx, `
			SELECT celestial_id, structure_code, target_level, start_time
			FROM construction_queues
			WHERE id = $1 AND celestial_id = $2 AND status = 'IN_PROGRESS' AND end_time > NOW()
			FOR UPDATE
		`, queueID, ownerID).Scan(&celestialID, &code, &targetLevel, &start)
		if err != nil {
			return game.Cost{}, cancelLookupErr(err)
		}
		cost, _ = game.StructureCost(code, targetLevel)

	case CancelShipyard:
		err = tx.QueryRowContext(ctx, `
			SELECT celestial_id, unit_code, quantity_total, start_time
			FROM shipyard_queues
			WHERE id = $1 AND celestial_id = $2 AND status = 'IN_PROGRESS' AND end_time > NOW()
			FOR UPDATE
		`, queueID, ownerID).Scan(&celestialID, &code, &quantity, &start)
		if err != nil {
			return game.Cost{}, cancelLookupErr(err)
		}
		cost, _ = game.UnitCost(code, quantity)

	case CancelResearch:
		err = tx.QueryRowContext(ctx, `
			SELECT celestial_id, tech_code, target_level, start_time
			FROM research_queues
			WHERE id = $1 AND user_id = $2 AND status = 'IN_PROGRESS' AND end_time > NOW()
			FOR UPDATE
		`, queueID, ownerID).Scan(&celestialID, &code, &targetLevel, &start)
		if err != nil {
			return game.Cost{}, cancelLookupErr(err)
		}
		cost, _ = game.TechCost(code, targetLevel)

	default:
		return game.Cost{}, ErrNotFound
	}

	// Bank accrued output first, then refund. update_celestial_resources takes
	// the celestial row lock, so this stays consistent with the scheduler.
	if err := s.creditCelestial(ctx, tx, celestialID, cost); err != nil {
		return game.Cost{}, err
	}
	switch scope {
	case CancelConstruction:
		_, err = tx.ExecContext(ctx, `UPDATE construction_queues SET status = 'CANCELLED' WHERE id = $1`, queueID)
	case CancelShipyard:
		_, err = tx.ExecContext(ctx, `UPDATE shipyard_queues SET status = 'CANCELLED' WHERE id = $1`, queueID)
	case CancelResearch:
		_, err = tx.ExecContext(ctx, `UPDATE research_queues SET status = 'CANCELLED' WHERE id = $1`, queueID)
	}
	if err != nil {
		return game.Cost{}, err
	}

	switch scope {
	case CancelConstruction:
		if err := s.renumberConstructionTx(ctx, tx, celestialID, queueID, code, start); err != nil {
			return game.Cost{}, err
		}
	case CancelShipyard:
		kindCodes := defenseCodes()
		if _, isShip := game.Ships[code]; isShip {
			kindCodes = shipCodes()
		}
		if err := s.renumberShipyardTx(ctx, tx, celestialID, queueID, kindCodes, start); err != nil {
			return game.Cost{}, err
		}
	case CancelResearch:
		if err := s.renumberResearchTx(ctx, tx, ownerID, queueID, code, start); err != nil {
			return game.Cost{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return game.Cost{}, err
	}
	return cost, nil
}

// creditCelestial banks accrued resources and credits cost to a celestial.
func (s *BuildStore) creditCelestial(ctx context.Context, tx *sql.Tx, celestialID int64, cost game.Cost) error {
	if _, err := tx.ExecContext(ctx, `SELECT * FROM update_celestial_resources($1)`, celestialID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE celestial_objects
		SET metal = metal + $1, crystal = crystal + $2, deuterium = deuterium + $3
		WHERE id = $4
	`, cost.Metal, cost.Crystal, cost.Deuterium, celestialID)
	return err
}

// renumberConstructionTx repairs a celestial's construction chain after one
// order is cancelled: every later order is pulled forward to start right after
// the (now shorter) chain, and later orders of the cancelled building shift down
// one level, the price difference being refunded. Each order's duration is
// recomputed with the projected robotics/nanite factory (the planet's completed
// level plus every queued upgrade ahead of that order), matching enqueue.
func (s *BuildStore) renumberConstructionTx(ctx context.Context, tx *sql.Tx, celestialID, cancelledID int64, code string, cancelledStart time.Time) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, structure_code, target_level
		FROM construction_queues
		WHERE celestial_id = $1 AND status = 'IN_PROGRESS'
		ORDER BY id
	`, celestialID)
	if err != nil {
		return err
	}
	type order struct {
		id     int64
		code   string
		target int
	}
	var all []order
	for rows.Next() {
		var o order
		if err := rows.Scan(&o.id, &o.code, &o.target); err != nil {
			rows.Close()
			return err
		}
		all = append(all, o)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(all) == 0 {
		return nil
	}

	var gameSpeed float64
	if err := tx.QueryRowContext(ctx, `
		SELECT u.game_speed FROM celestial_objects c
		JOIN universes u ON u.id = c.universe_id WHERE c.id = $1
	`, celestialID).Scan(&gameSpeed); err != nil {
		return err
	}
	levels, err := s.structureLevels(ctx, tx, celestialID)
	if err != nil {
		return err
	}
	baseRobotics := levels["robotics_factory"]
	baseNanite := levels["nanite_factory"]

	cursor := cancelledStart
	if cursor.Before(time.Now()) {
		cursor = time.Now()
	}
	var seenRobotics, seenNanite int
	for i := range all {
		o := all[i]
		projRobotics := baseRobotics + seenRobotics
		projNanite := baseNanite + seenNanite
		if o.code == "robotics_factory" {
			seenRobotics++
		}
		if o.code == "nanite_factory" {
			seenNanite++
		}
		// Orders before the cancelled one are already scheduled correctly.
		if o.id <= cancelledID {
			continue
		}
		if o.code == code {
			next := o.target - 1
			if next < 1 {
				next = 1
			}
			if next != o.target {
				oldCost, _ := game.StructureCost(o.code, o.target)
				newCost, _ := game.StructureCost(o.code, next)
				if err := s.creditCelestial(ctx, tx, celestialID, game.Cost{
					Metal:     oldCost.Metal - newCost.Metal,
					Crystal:   oldCost.Crystal - newCost.Crystal,
					Deuterium: oldCost.Deuterium - newCost.Deuterium,
				}); err != nil {
					return err
				}
				o.target = next
			}
		}
		dur := game.StructureDuration(o.code, o.target, projRobotics, projNanite, gameSpeed)
		start := cursor
		end := start.Add(dur)
		if _, err := tx.ExecContext(ctx, `
			UPDATE construction_queues SET target_level = $2, start_time = $3, end_time = $4 WHERE id = $1
		`, o.id, o.target, start, end); err != nil {
			return err
		}
		cursor = end
	}
	return nil
}

// renumberResearchTx repairs an account's research chain after one order is
// cancelled: every later order is pulled forward to start right after the (now
// shorter) chain, and later orders of the cancelled tech shift down one level,
// the price difference being refunded to the planet that paid for it.
func (s *BuildStore) renumberResearchTx(ctx context.Context, tx *sql.Tx, userID, cancelledID int64, code string, cancelledStart time.Time) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, celestial_id, tech_code, target_level, start_time, end_time
		FROM research_queues
		WHERE user_id = $1 AND status = 'IN_PROGRESS' AND id > $2
		ORDER BY id
	`, userID, cancelledID)
	if err != nil {
		return err
	}
	type order struct {
		id          int64
		celestialID int64
		code        string
		target      int
		start       time.Time
		end         time.Time
	}
	var tail []order
	for rows.Next() {
		var o order
		if err := rows.Scan(&o.id, &o.celestialID, &o.code, &o.target, &o.start, &o.end); err != nil {
			rows.Close()
			return err
		}
		tail = append(tail, o)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if len(tail) == 0 {
		return nil
	}

	var gameSpeed float64
	if err := tx.QueryRowContext(ctx, `
		SELECT u.game_speed FROM celestial_objects c
		JOIN universes u ON u.id = c.universe_id WHERE c.id = $1
	`, tail[0].celestialID).Scan(&gameSpeed); err != nil {
		return err
	}
	type lab struct{ level, university int }
	labCache := map[int64]lab{}
	effectiveLab := func(celestialID int64) (lab, error) {
		if l, ok := labCache[celestialID]; ok {
			return l, nil
		}
		lvl, uni, err := s.effectiveResearchLab(ctx, tx, celestialID)
		if err != nil {
			return lab{}, err
		}
		l := lab{level: lvl, university: uni}
		labCache[celestialID] = l
		return l, nil
	}

	cursor := cancelledStart
	if cursor.Before(time.Now()) {
		cursor = time.Now()
	}
	for i := range tail {
		o := tail[i]
		dur := o.end.Sub(o.start)
		if o.code == code {
			next := o.target - 1
			if next < 1 {
				next = 1
			}
			if next != o.target {
				oldCost, _ := game.TechCost(o.code, o.target)
				newCost, _ := game.TechCost(o.code, next)
				if err := s.creditCelestial(ctx, tx, o.celestialID, game.Cost{
					Metal:     oldCost.Metal - newCost.Metal,
					Crystal:   oldCost.Crystal - newCost.Crystal,
					Deuterium: oldCost.Deuterium - newCost.Deuterium,
				}); err != nil {
					return err
				}
				o.target = next
				l, err := effectiveLab(o.celestialID)
				if err != nil {
					return err
				}
				dur = game.TechDuration(o.code, next, l.level, l.university, gameSpeed)
			}
		}
		start := cursor
		end := start.Add(dur)
		if _, err := tx.ExecContext(ctx, `
			UPDATE research_queues SET target_level = $2, start_time = $3, end_time = $4 WHERE id = $1
		`, o.id, o.target, start, end); err != nil {
			return err
		}
		cursor = end
	}
	return nil
}

// renumberShipyardTx pulls a celestial's later ship or defense batches forward
// after one is cancelled, keeping each batch's own duration.
func (s *BuildStore) renumberShipyardTx(ctx context.Context, tx *sql.Tx, celestialID, cancelledID int64, kindCodes []string, cancelledStart time.Time) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, start_time, end_time
		FROM shipyard_queues
		WHERE celestial_id = $1 AND status = 'IN_PROGRESS' AND unit_code = ANY($2) AND id > $3
		ORDER BY id
	`, celestialID, pq.Array(kindCodes), cancelledID)
	if err != nil {
		return err
	}
	type order struct {
		id    int64
		start time.Time
		end   time.Time
	}
	var tail []order
	for rows.Next() {
		var o order
		if err := rows.Scan(&o.id, &o.start, &o.end); err != nil {
			rows.Close()
			return err
		}
		tail = append(tail, o)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	cursor := cancelledStart
	if cursor.Before(time.Now()) {
		cursor = time.Now()
	}
	for i := range tail {
		o := tail[i]
		dur := o.end.Sub(o.start)
		start := cursor
		end := start.Add(dur)
		if _, err := tx.ExecContext(ctx, `
			UPDATE shipyard_queues SET start_time = $2, end_time = $3 WHERE id = $1
		`, o.id, start, end); err != nil {
			return err
		}
		cursor = end
	}
	return nil
}

// effectiveResearchLab returns the research-lab level (its own lab plus
// IRN-connected colony labs) and the local University level of a celestial,
// used to recompute a shifted research order's duration.
func (s *BuildStore) effectiveResearchLab(ctx context.Context, tx *sql.Tx, celestialID int64) (int, int, error) {
	var userID int64
	if err := tx.QueryRowContext(ctx, `SELECT user_id FROM celestial_objects WHERE id = $1`, celestialID).Scan(&userID); err != nil {
		return 0, 0, err
	}
	structLevels, err := s.structureLevels(ctx, tx, celestialID)
	if err != nil {
		return 0, 0, err
	}
	techs, err := s.techLevels(ctx, tx, userID)
	if err != nil {
		return 0, 0, err
	}
	colonyLabs, err := s.researchLabLevels(ctx, tx, userID, celestialID)
	if err != nil {
		return 0, 0, err
	}
	lvl := game.EffectiveResearchLabLevel(structLevels["research_lab"], techs["intergalactic_research_network"], colonyLabs)
	return lvl, structLevels["university"], nil
}

// EffectiveResearchLab exposes the effective research-lab level (its own lab
// plus IRN-connected colony labs) and the local University level of a celestial,
// used by the web research page's live cost/time preview.
func (s *BuildStore) EffectiveResearchLab(ctx context.Context, celestialID int64) (int, int, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted, ReadOnly: true})
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	return s.effectiveResearchLab(ctx, tx, celestialID)
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
		activeGov, err := loadActiveGovernators(ctx, tx, owner.Int64)
		if err != nil {
			return err
		}
		bonus = game.ProductionBonusFor(upgrades).
			Plus(game.ResearchProductionBonus(techs)).
			Plus(game.GovernatorProductionBonus(activeGov))
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

	rows, err = s.db.QueryContext(ctx, `
		SELECT id, structure_code, target_level, start_time, end_time
		FROM construction_queues
		WHERE celestial_id = $1 AND status = 'IN_PROGRESS'
		ORDER BY end_time, id
	`, id)
	if err != nil {
		return models.BuildingsResponse{}, err
	}
	if resp.Queue, err = scanQueueSummaries(rows, true); err != nil {
		return models.BuildingsResponse{}, err
	}
	for i := range resp.Queue {
		if c, ok := game.StructureCost(resp.Queue[i].Code, resp.Queue[i].TargetLevel); ok {
			resp.Queue[i].Cost = models.CargoManifest{Metal: c.Metal, Crystal: c.Crystal, Deuterium: c.Deuterium}
		}
	}

	return resp, nil
}

// scanQueueSummaries scans (id, code, value, start, end) rows into queue
// summaries. hasTarget selects whether the middle column is a target level
// (structures/research) or a quantity (units).
func scanQueueSummaries(rows *sql.Rows, hasTarget bool) ([]models.QueueEntrySummary, error) {
	defer rows.Close()
	var out []models.QueueEntrySummary
	for rows.Next() {
		var (
			q        models.QueueEntrySummary
			target   int
			quantity int64
		)
		var err error
		if hasTarget {
			err = rows.Scan(&q.ID, &q.Code, &target, &q.StartTime, &q.EndTime)
			q.TargetLevel = target
		} else {
			err = rows.Scan(&q.ID, &q.Code, &quantity, &q.StartTime, &q.EndTime)
			q.Quantity = quantity
		}
		if err != nil {
			return nil, err
		}
		q.RemainingSecs = int64(time.Until(q.EndTime).Seconds())
		if q.RemainingSecs < 0 {
			q.RemainingSecs = 0
		}
		out = append(out, q)
	}
	return out, rows.Err()
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

	rows, err = s.db.QueryContext(ctx, `
		SELECT id, tech_code, target_level, start_time, end_time
		FROM research_queues
		WHERE user_id = $1 AND status = 'IN_PROGRESS'
		ORDER BY end_time, id
	`, userID)
	if err != nil {
		return models.ResearchResponse{}, err
	}
	if resp.Queue, err = scanQueueSummaries(rows, true); err != nil {
		return models.ResearchResponse{}, err
	}
	for i := range resp.Queue {
		if c, ok := game.TechCost(resp.Queue[i].Code, resp.Queue[i].TargetLevel); ok {
			resp.Queue[i].Cost = models.CargoManifest{Metal: c.Metal, Crystal: c.Crystal, Deuterium: c.Deuterium}
		}
	}

	return resp, nil
}

// GetShipyardQueue returns the chained ship/defense production queue for a
// celestial (ships and defenses share the table; the caller separates them by
// code). It is the read model behind the shipyard/defenses pages.
func (s *BuildStore) GetShipyardQueue(ctx context.Context, planetID string) (models.ShipyardResponse, error) {
	id, err := strconv.ParseInt(planetID, 10, 64)
	if err != nil {
		return models.ShipyardResponse{}, ErrNotFound
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, unit_code, quantity_total, start_time, end_time
		FROM shipyard_queues
		WHERE celestial_id = $1 AND status = 'IN_PROGRESS'
		ORDER BY end_time, id
	`, id)
	if err != nil {
		return models.ShipyardResponse{}, err
	}
	queue, err := scanQueueSummaries(rows, false)
	if err != nil {
		return models.ShipyardResponse{}, err
	}
	for i := range queue {
		if c, ok := game.UnitCost(queue[i].Code, queue[i].Quantity); ok {
			queue[i].Cost = models.CargoManifest{Metal: c.Metal, Crystal: c.Crystal, Deuterium: c.Deuterium}
		}
	}
	return models.ShipyardResponse{PlanetID: id, Queue: queue}, nil
}

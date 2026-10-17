package store

import (
	"context"
	"database/sql"
	"log"

	"neoxnova/internal/game"
)

// AcademyStore persists the Academy skill tree (see internal/game/academy.go).
// Skill levels live in user_academy_skills (migration 0004); the spendable
// Academy-point balance lives on users.academy_points (migration 0026).
type AcademyStore struct {
	db *sql.DB
}

func NewAcademyStore(db *sql.DB) *AcademyStore {
	return &AcademyStore{db: db}
}

// academyQuerier is satisfied by both *sql.DB and *sql.Tx so the skill loader
// can run on the request DB or inside a build/research transaction.
type academyQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// loadAcademyLevels reads the account's non-zero skill levels keyed by code.
func loadAcademyLevels(ctx context.Context, q academyQuerier, userID int64) (map[string]int, error) {
	rows, err := q.QueryContext(ctx, `
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

// Skills returns the account's owned skill levels (omitting level 0).
func (s *AcademyStore) Skills(ctx context.Context, userID int64) (map[string]int, error) {
	return loadAcademyLevels(ctx, s.db, userID)
}

// Points returns the account's spendable Academy points.
func (s *AcademyStore) Points(ctx context.Context, userID int64) (int64, error) {
	var p int64
	err := s.db.QueryRowContext(ctx, `SELECT academy_points FROM users WHERE id = $1`, userID).Scan(&p)
	return p, err
}

// AcademyUpgradeResult reports an upgrade outcome. Partial is set when the
// requested target was unaffordable and the level was locked in at the highest
// affordable one instead.
type AcademyUpgradeResult struct {
	Code      string
	Level     int
	Requested int
	Spent     int64
	Partial   bool
}

// Upgrade raises a skill toward target with the account's Academy points. A
// prerequisite gate is enforced; when the target is unaffordable the level locks
// in at the highest affordable one (partial). If even the next level cannot be
// paid for it returns ErrInsufficientAcademyPoints and changes nothing.
func (s *AcademyStore) Upgrade(ctx context.Context, userID int64, code string, target int) (AcademyUpgradeResult, error) {
	def, ok := game.AcademySkillByCode(code)
	if !ok {
		return AcademyUpgradeResult{}, ErrUnknownCode
	}
	if target > def.MaxLevel {
		target = def.MaxLevel
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return AcademyUpgradeResult{}, err
	}
	defer tx.Rollback()

	levels, err := loadAcademyLevels(ctx, tx, userID)
	if err != nil {
		return AcademyUpgradeResult{}, err
	}
	if !game.AcademyPrereqMet(def, levels) {
		return AcademyUpgradeResult{}, ErrAcademyPrereq
	}
	level := levels[code]
	if target <= level {
		return AcademyUpgradeResult{}, ErrAcademyLevel
	}

	var points int64
	if err := tx.QueryRowContext(ctx, `
		SELECT academy_points FROM users WHERE id = $1 FOR UPDATE
	`, userID).Scan(&points); err != nil {
		return AcademyUpgradeResult{}, err
	}
	newLevel := game.AcademyAffordableLevel(def, level, target, points)
	if newLevel <= level {
		return AcademyUpgradeResult{}, ErrInsufficientAcademyPoints
	}
	cost := game.AcademyCostRange(def, level, newLevel)

	if _, err := tx.ExecContext(ctx, `
		UPDATE users SET academy_points = academy_points - $1 WHERE id = $2
	`, cost, userID); err != nil {
		return AcademyUpgradeResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_academy_skills (user_id, skill_code, level)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, skill_code) DO UPDATE SET level = EXCLUDED.level
	`, userID, code, newLevel); err != nil {
		return AcademyUpgradeResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AcademyUpgradeResult{}, err
	}
	if academyAffectsProduction(code) {
		s.recompute(ctx, userID)
	}
	return AcademyUpgradeResult{Code: code, Level: newLevel, Requested: target, Spent: cost, Partial: newLevel < target}, nil
}

// academyAffectsProduction reports whether a skill changes the cached
// production/energy/storage columns, so its level change must recompute.
func academyAffectsProduction(code string) bool {
	switch code {
	case game.SkillProductionSpeed, game.SkillEnergetics, game.SkillCosmoDepot:
		return true
	default:
		return false
	}
}

// Reset tiers, from the captured reset panel. A full reset costs Antimatter and
// refunds 100%; the DM tier refunds 75%; the free tier refunds 50%.
const (
	AcademyResetFullPct    = 100
	AcademyResetPartialPct = 75
	AcademyResetFreePct    = 50
	academyResetAntimatter = 10000
	academyResetDarkMatter = 200000
)

// AcademyResetResult reports a branch reset: the points refunded and the
// currency spent.
type AcademyResetResult struct {
	Branch    game.AcademyBranch
	RefundPct int
	Refunded  int64
	Spent     int64 // Academy points returned to the pool
	Cost      int64 // price paid (Antimatter or Dark Matter)
	Currency  string
}

// ResetBranch zeroes every skill in a branch and refunds a percentage of the
// points spent in it. tier selects the captured refund/price combination:
// AcademyResetFullPct (100% refund, Antimatter), AcademyResetPartialPct (75%,
// Dark Matter) or AcademyResetFreePct (50%, free). Returns ErrNothingToReset when
// the branch is untouched, or the relevant insufficient-funds error.
func (s *AcademyStore) ResetBranch(ctx context.Context, userID int64, branch game.AcademyBranch, percent int) (AcademyResetResult, error) {
	var antimatterCost, dmCost int64
	switch percent {
	case AcademyResetFullPct:
		antimatterCost = academyResetAntimatter
	case AcademyResetPartialPct:
		dmCost = academyResetDarkMatter
	case AcademyResetFreePct:
	default:
		return AcademyResetResult{}, ErrUnknownCode
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return AcademyResetResult{}, err
	}
	defer tx.Rollback()

	levels, err := loadAcademyLevels(ctx, tx, userID)
	if err != nil {
		return AcademyResetResult{}, err
	}
	var spent int64
	codes := make([]string, 0, len(levels))
	for code, lvl := range levels {
		def, ok := game.AcademySkillByCode(code)
		if !ok || def.Branch != branch || lvl <= 0 {
			continue
		}
		spent += game.AcademyCostRange(def, 0, lvl)
		codes = append(codes, code)
	}
	if len(codes) == 0 {
		return AcademyResetResult{}, ErrNothingToReset
	}

	var antimatter, dm int64
	if err := tx.QueryRowContext(ctx, `
		SELECT antimatter, dark_matter FROM users WHERE id = $1 FOR UPDATE
	`, userID).Scan(&antimatter, &dm); err != nil {
		return AcademyResetResult{}, err
	}
	result := AcademyResetResult{Branch: branch, RefundPct: percent, Spent: spent}
	switch {
	case antimatterCost > 0:
		if antimatter < antimatterCost {
			return AcademyResetResult{}, ErrInsufficientAntimatter
		}
		result.Currency, result.Cost = "Antimatter", antimatterCost
		if _, err := tx.ExecContext(ctx, `UPDATE users SET antimatter = antimatter - $1 WHERE id = $2`, antimatterCost, userID); err != nil {
			return AcademyResetResult{}, err
		}
	case dmCost > 0:
		if dm < dmCost {
			return AcademyResetResult{}, ErrInsufficientDarkMatter
		}
		result.Currency, result.Cost = "Dark Matter", dmCost
		if err := debitDarkMatter(ctx, tx, userID, dmCost); err != nil {
			return AcademyResetResult{}, err
		}
	}
	refund := int64(float64(spent) * float64(percent) / 100.0)
	result.Refunded = refund
	if _, err := tx.ExecContext(ctx, `
		UPDATE users SET academy_points = academy_points + $1 WHERE id = $2
	`, refund, userID); err != nil {
		return AcademyResetResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM user_academy_skills WHERE user_id = $1 AND skill_code = ANY($2)
	`, userID, codes); err != nil {
		return AcademyResetResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AcademyResetResult{}, err
	}
	if branch == game.AcademyEconomy {
		s.recompute(ctx, userID)
	}
	return result, nil
}

// recompute refreshes an account's cached production/energy/storage after an
// economy skill changes. Best-effort: a failure is logged, not fatal.
func (s *AcademyStore) recompute(ctx context.Context, userID int64) {
	if err := NewBuildStore(s.db).RecomputeAccountProduction(ctx, userID); err != nil {
		log.Printf("[WARN] Failed to recompute production for account %d after an academy change: %v", userID, err)
	}
}

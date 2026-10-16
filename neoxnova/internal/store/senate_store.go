package store

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"neoxnova/internal/game"
)

// SenateStore persists Governators (see internal/game/senate.go). A governator
// has a permanently-bought level (Achievement Points) and an optional, timed
// activation bought with Dark Matter.
type SenateStore struct {
	db *sql.DB
}

func NewSenateStore(db *sql.DB) *SenateStore {
	return &SenateStore{db: db}
}

// govQuerier is satisfied by both *sql.DB and *sql.Tx so the active-governator
// loader can run on the request DB or inside a build transaction.
type govQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// GovernatorState is an account's owned level and (optional) activation expiry.
type GovernatorState struct {
	Level     int
	ExpiresAt sql.NullTime
}

// GovernatorStates returns every governator the account has interacted with,
// keyed by code. Missing codes are absent (level 0, inactive).
func (s *SenateStore) GovernatorStates(ctx context.Context, userID int64) (map[int]GovernatorState, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT governator_code, level, expires_at FROM user_governators WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]GovernatorState{}
	for rows.Next() {
		var code int
		var st GovernatorState
		if err := rows.Scan(&code, &st.Level, &st.ExpiresAt); err != nil {
			return nil, err
		}
		out[code] = st
	}
	return out, rows.Err()
}

// AchievementPoints returns the account's spendable Achievement Points.
func (s *SenateStore) AchievementPoints(ctx context.Context, userID int64) (int64, error) {
	var ap int64
	err := s.db.QueryRowContext(ctx, `SELECT achievement_points FROM users WHERE id = $1`, userID).Scan(&ap)
	return ap, err
}

// ActiveGovernators returns the codes of every governator whose activation is
// live (level > 0 and expires_at in the future) mapped to its level.
func (s *SenateStore) ActiveGovernators(ctx context.Context, userID int64) (map[int]int, error) {
	return loadActiveGovernators(ctx, s.db, userID)
}

func loadActiveGovernators(ctx context.Context, q govQuerier, userID int64) (map[int]int, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT governator_code, level FROM user_governators
		WHERE user_id = $1 AND level > 0 AND expires_at > NOW()
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var code, level int
		if err := rows.Scan(&code, &level); err != nil {
			return nil, err
		}
		out[code] = level
	}
	return out, rows.Err()
}

// isProductionGovernator reports whether a governator changes the cached
// production/energy columns, so its activation change must trigger a recompute.
func isProductionGovernator(code int) bool {
	return code == game.GovCodeResource || code == game.GovCodeEnergy
}

// ImproveGovernator permanently raises a governator's level to target, paying
// the summed Achievement-Point cost of the levels in between. The action is
// irreversible and does not activate the bonus. Returns the AP spent.
func (s *SenateStore) ImproveGovernator(ctx context.Context, userID int64, code, target int) (int64, error) {
	def, ok := game.GovernatorByCode(code)
	if !ok {
		return 0, ErrUnknownCode
	}
	if target > def.MaxLevel {
		target = def.MaxLevel
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	level := 0
	err = tx.QueryRowContext(ctx, `
		SELECT level FROM user_governators WHERE user_id = $1 AND governator_code = $2 FOR UPDATE
	`, userID, code).Scan(&level)
	if errors.Is(err, sql.ErrNoRows) {
		level = 0
	} else if err != nil {
		return 0, err
	}
	if target <= level {
		return 0, ErrGovernatorLevel
	}
	cost := def.APCost(level, target)

	res, err := tx.ExecContext(ctx, `
		UPDATE users SET achievement_points = achievement_points - $1
		WHERE id = $2 AND achievement_points >= $1
	`, cost, userID)
	if err != nil {
		return 0, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return 0, err
	} else if n == 0 {
		return 0, ErrInsufficientAchievement
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_governators (user_id, governator_code, level)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, governator_code) DO UPDATE SET level = EXCLUDED.level
	`, userID, code, target); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if isProductionGovernator(code) {
		s.recompute(ctx, userID)
	}
	return cost, nil
}

// HireGovernator activates a governator for `days` days at its current level,
// paying the Dark Matter per-day price. A live activation is extended rather
// than restarted. Returns the Dark Matter spent.
func (s *SenateStore) HireGovernator(ctx context.Context, userID int64, code, days int) (int64, error) {
	def, ok := game.GovernatorByCode(code)
	if !ok {
		return 0, ErrUnknownCode
	}
	if days < 1 || days > game.MaxGovernatorDays {
		return 0, ErrInvalidQuantity
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	level := 0
	var exp sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT level, expires_at FROM user_governators
		WHERE user_id = $1 AND governator_code = $2 FOR UPDATE
	`, userID, code).Scan(&level, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		level, exp = 0, sql.NullTime{}
	} else if err != nil {
		return 0, err
	}

	perDay := def.DMPricePerDay(level)
	total := perDay * int64(days)
	if err := debitDarkMatter(ctx, tx, userID, total); err != nil {
		return 0, err
	}

	base := time.Now()
	if exp.Valid && exp.Time.After(base) {
		base = exp.Time
	}
	newExp := base.Add(time.Duration(days) * 24 * time.Hour)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_governators (user_id, governator_code, level, activated_at, expires_at)
		VALUES ($1, $2, $3, NOW(), $4)
		ON CONFLICT (user_id, governator_code)
		DO UPDATE SET activated_at = NOW(), expires_at = EXCLUDED.expires_at
	`, userID, code, level, newExp); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if isProductionGovernator(code) {
		s.recompute(ctx, userID)
	}
	return total, nil
}

// ExpireGovernators clears lapsed activations (keeping the bought level) and
// refreshes the cached production of any account whose Resource/Energy bonus
// lapsed. Bounded per call; safe to run repeatedly.
func (s *SenateStore) ExpireGovernators(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		UPDATE user_governators SET activated_at = NULL, expires_at = NULL
		WHERE ctid IN (
			SELECT ctid FROM user_governators WHERE expires_at <= NOW() LIMIT 200
		)
		RETURNING user_id, governator_code
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	users := map[int64]bool{}
	for rows.Next() {
		var uid int64
		var code int
		if err := rows.Scan(&uid, &code); err != nil {
			return err
		}
		if isProductionGovernator(code) {
			users[uid] = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for uid := range users {
		s.recompute(ctx, uid)
	}
	return nil
}

// recompute refreshes an account's cached production/energy after an activated
// production governator changes. Best-effort: a failure is logged, not fatal.
func (s *SenateStore) recompute(ctx context.Context, userID int64) {
	if err := NewBuildStore(s.db).RecomputeAccountProduction(ctx, userID); err != nil {
		log.Printf("[WARN] Failed to recompute production for account %d after a governator change: %v", userID, err)
	}
}

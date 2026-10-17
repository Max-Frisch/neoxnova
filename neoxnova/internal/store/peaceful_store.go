package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"neoxnova/internal/game"
)

// PeacefulStore tracks an account's peaceful (online) level and the Online Bonus
// button. Peaceful seconds accrue from activity gaps; the level and its bonuses
// are derived from that total (game.PeacefulLevelState).
type PeacefulStore struct {
	db *sql.DB
}

func NewPeacefulStore(db *sql.DB) *PeacefulStore {
	return &PeacefulStore{db: db}
}

// AccrueOnlineTime records the gap since the account was last seen, capped so an
// offline stretch cannot bank unlimited time. The row is only touched once at
// least peacefulAccrualMinGap has elapsed, so htmx polls don't write every few
// seconds.
const (
	peacefulAccrualMinGap = 15  // seconds between writes
	peacefulAccrualCap    = 300 // max seconds credited for a single gap
)

func (s *PeacefulStore) AccrueOnlineTime(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET peaceful_seconds = peaceful_seconds + LEAST(
		        GREATEST(EXTRACT(EPOCH FROM (NOW() - COALESCE(peaceful_seen_at, NOW()))), 0),
		        $2
		    )::bigint,
		    peaceful_seen_at = NOW()
		WHERE id = $1
		  AND (peaceful_seen_at IS NULL OR NOW() - peaceful_seen_at >= $3 * interval '1 second')
	`, userID, peacefulAccrualCap, peacefulAccrualMinGap)
	return err
}

// PeacefulStatus is the derived progression snapshot backing the overview and
// the Online Bonus page.
type PeacefulStatus struct {
	Level             int
	ProgressPct       float64
	NextSeconds       int64
	Seconds           int64
	BonusReady        bool
	BonusReadySeconds int64
	LastBonusClickAt  sql.NullTime
}

// Status derives the caller's peaceful level and bonus-button availability.
func (s *PeacefulStore) Status(ctx context.Context, userID int64) (PeacefulStatus, error) {
	var seconds int64
	var readyAt, lastClick sql.NullTime
	if err := s.db.QueryRowContext(ctx, `
		SELECT peaceful_seconds, bonus_button_ready_at, bonus_last_click_at
		FROM users WHERE id = $1
	`, userID).Scan(&seconds, &readyAt, &lastClick); err != nil {
		return PeacefulStatus{}, err
	}
	st := PeacefulStatus{Seconds: seconds, LastBonusClickAt: lastClick}
	st.Level, st.ProgressPct, st.NextSeconds = game.PeacefulLevelState(float64(seconds))
	now := time.Now()
	if !readyAt.Valid || !readyAt.Time.After(now) {
		st.BonusReady = true
	} else {
		st.BonusReadySeconds = int64(readyAt.Time.Sub(now).Seconds())
	}
	return st, nil
}

// ClaimBonusButton grants one Online Bonus payout if the button is off cooldown:
// Dark Matter plus peaceful and combat experience. It is safe under concurrent
// clicks (the user row is locked and availability re-checked).
func (s *PeacefulStore) ClaimBonusButton(ctx context.Context, userID int64) (game.BonusReward, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return game.BonusReward{}, err
	}
	defer tx.Rollback()

	var seconds int64
	var readyAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT peaceful_seconds, bonus_button_ready_at FROM users WHERE id = $1 FOR UPDATE
	`, userID).Scan(&seconds, &readyAt)
	if errors.Is(err, sql.ErrNoRows) {
		return game.BonusReward{}, ErrNotFound
	} else if err != nil {
		return game.BonusReward{}, err
	}

	now := time.Now()
	if readyAt.Valid && readyAt.Time.After(now) {
		return game.BonusReward{}, ErrBonusNotReady
	}

	level, _, _ := game.PeacefulLevelState(float64(seconds))
	reward := game.BonusButtonReward(level)
	next := now.Add(game.BonusButtonInterval(game.PeacefulPremium{}))
	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET dark_matter = dark_matter + $2,
		    peaceful_seconds = peaceful_seconds + $3,
		    academy_points = academy_points + $4,
		    bonus_button_ready_at = $5,
		    bonus_last_click_at = NOW()
		WHERE id = $1
	`, userID, reward.DarkMatter, reward.PeacefulSeconds, reward.AcademyPoints, next); err != nil {
		return game.BonusReward{}, err
	}
	if err := tx.Commit(); err != nil {
		return game.BonusReward{}, err
	}
	return reward, nil
}

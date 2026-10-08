package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
)

// AuthStore handles users and opaque sessions.
type AuthStore struct {
	db *sql.DB
}

func NewAuthStore(db *sql.DB) *AuthStore {
	return &AuthStore{db: db}
}

// CreateUser registers a user in the universe identified by its code name
// (e.g. "universe_6_niburu") and returns the new id.
func (s *AuthStore) CreateUser(ctx context.Context, universeCode, username, email, passwordHash string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO users (universe_id, username, email, password_hash)
		SELECT id, $2, $3, $4 FROM universes WHERE code_name = $1
		RETURNING id
	`, universeCode, username, email, passwordHash).Scan(&id)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return 0, ErrUserExists
		}
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound // unknown universe
		}
		return 0, err
	}
	return id, nil
}

// UserAuth returns the id and password hash for a login (username OR email).
func (s *AuthStore) UserAuth(ctx context.Context, login string) (int64, string, error) {
	var id int64
	var hash string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, password_hash FROM users WHERE username = $1 OR email = $1 LIMIT 1
	`, login).Scan(&id, &hash)
	if err == sql.ErrNoRows {
		return 0, "", ErrInvalidCredentials
	}
	return id, hash, err
}

// AuthRole returns a user's role ('PLAYER', 'MODERATOR', 'ADMIN').
func (s *AuthStore) AuthRole(ctx context.Context, userID int64) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx, `SELECT auth_role FROM users WHERE id = $1`, userID).Scan(&role)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return role, err
}

// UserByID returns a user's public profile fields.
func (s *AuthStore) UserByID(ctx context.Context, userID int64) (username, email string, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT username, email FROM users WHERE id = $1
	`, userID).Scan(&username, &email)
	if err == sql.ErrNoRows {
		return "", "", ErrNotFound
	}
	return username, email, err
}

// UserExists reports whether a username/email is taken.
func (s *AuthStore) UserExists(ctx context.Context, username, email string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM users WHERE username = $1 OR email = $2)
	`, username, email).Scan(&exists)
	return exists, err
}

// CreateSession stores a new session token hash.
func (s *AuthStore) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time, userAgent, ip string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at, user_agent, ip)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''))
	`, userID, tokenHash, expiresAt, userAgent, ip)
	return err
}

// SessionUser resolves a non-expired session to its user id and refreshes
// last_seen_at. Returns ErrNotFound for missing/expired sessions.
func (s *AuthStore) SessionUser(ctx context.Context, tokenHash string) (int64, error) {
	var userID int64
	err := s.db.QueryRowContext(ctx, `
		UPDATE sessions SET last_seen_at = NOW()
		WHERE token_hash = $1 AND expires_at > NOW()
		RETURNING user_id
	`, tokenHash).Scan(&userID)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	return userID, err
}

// DeleteSession revokes one session.
func (s *AuthStore) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

// DeleteExpiredSessions prunes stale rows (best-effort housekeeping).
func (s *AuthStore) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= NOW()`)
	return err
}

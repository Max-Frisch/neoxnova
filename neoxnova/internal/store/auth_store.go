package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"math/rand"
	"time"

	"github.com/lib/pq"

	"neoxnova/internal/game"
)

// AuthStore handles users and opaque sessions.
type AuthStore struct {
	db *sql.DB
}

func NewAuthStore(db *sql.DB) *AuthStore {
	return &AuthStore{db: db}
}

// Homeworld onboarding constants. A freshly registered player is granted a
// single planet at a random slot (galaxy 1..max_galaxies, system 1..max_systems,
// position in the 6..16 band), a generous 700 starting field allowance, and a
// structure-free resource loadout. Base production (see game.Base*PerHour) keeps
// the planet earning even before the first mine is built.
const (
	homeworldFields       = 700
	homeworldStartMetal   = 100000
	homeworldStartCrystal = 50000
	homeworldStartDeut    = 20000
	homeworldStartCap     = 100000000
	homeworldMinPosition  = 6
	homeworldMaxPosition  = 16
	homeworldSlotAttempts = 50
)

// CreateUser registers a user in the universe identified by its code name
// (e.g. "universe_6_niburu"), provisions a starter homeworld, and returns the
// new user id and homeworld celestial id.
func (s *AuthStore) CreateUser(ctx context.Context, universeCode, username, email, passwordHash string) (userID, planetID int64, err error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	var (
		resourceSpeed float64
		maxGalaxies   int
		maxSystems    int
	)
	err = tx.QueryRowContext(ctx, `
		SELECT resource_speed, max_galaxies, max_systems
		FROM universes WHERE code_name = $1
	`, universeCode).Scan(&resourceSpeed, &maxGalaxies, &maxSystems)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, ErrNotFound // unknown universe
	}
	if err != nil {
		return 0, 0, err
	}
	if maxGalaxies < 1 {
		maxGalaxies = 1
	}
	if maxSystems < 1 {
		maxSystems = 1
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO users (universe_id, username, email, password_hash)
		SELECT id, $2, $3, $4 FROM universes WHERE code_name = $1
		RETURNING id
	`, universeCode, username, email, passwordHash).Scan(&userID)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return 0, 0, ErrUserExists
		}
		return 0, 0, err
	}

	if planetID, err = createHomeworld(ctx, tx, universeCode, userID, username, resourceSpeed, maxGalaxies, maxSystems); err != nil {
		return 0, 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return userID, planetID, nil
}

// createHomeworld inserts the starter planet for a new user, retrying random
// slots until a free, unlocked one is found.
func createHomeworld(ctx context.Context, tx *sql.Tx, universeCode string, userID int64, username string, resourceSpeed float64, maxGalaxies, maxSystems int) (int64, error) {
	name := username + "'s Homeworld"
	if len(name) > 64 {
		name = name[:64]
	}

	metalProd := game.BaseMetalPerHour * resourceSpeed
	crystalProd := game.BaseCrystalPerHour * resourceSpeed
	deutProd := game.BaseDeutPerHour * resourceSpeed

	for attempt := 0; attempt < homeworldSlotAttempts; attempt++ {
		g := 1 + rand.Intn(maxGalaxies)
		sys := 1 + rand.Intn(maxSystems)
		p := homeworldMinPosition + rand.Intn(homeworldMaxPosition-homeworldMinPosition+1)

		spec, ok := game.RollPlanet(game.FieldSlotSeed(g, sys, p), p)
		if !ok {
			continue
		}

		var planetID int64
		err := tx.QueryRowContext(ctx, `
			INSERT INTO celestial_objects (
				universe_id, user_id, name, object_type, galaxy, system, position,
				diameter_km, fields_used, fields_max, base_fields_max, temp_min, temp_max,
				metal, crystal, deuterium,
				metal_capacity, crystal_capacity, deuterium_capacity,
				metal_prod_hourly, crystal_prod_hourly, deuterium_prod_hourly,
				energy_used, energy_max
			)
			SELECT u.id, $2, $3, 'PLANET', $4, $5, $6,
			       $7, 0, $8::int, $8::bigint, $9, $10,
			       $11, $12, $13,
			       $14::bigint, $14::bigint, $14::bigint,
			       $15, $16, $17, 0, 0
			FROM universes u WHERE u.code_name = $1
			ON CONFLICT (universe_id, galaxy, system, position, object_type) DO NOTHING
			RETURNING id
		`, universeCode, userID, name, g, sys, p,
			int(math.Round(1000*math.Sqrt(homeworldFields))), homeworldFields, spec.TempMin, spec.TempMax,
			homeworldStartMetal, homeworldStartCrystal, homeworldStartDeut,
			homeworldStartCap,
			metalProd, crystalProd, deutProd).Scan(&planetID)
		if errors.Is(err, sql.ErrNoRows) {
			continue // slot taken by a concurrent insert; roll another
		}
		if err != nil {
			return 0, err
		}

		// Skip slots that are still under a post-abandon lock.
		var locked bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM coordinate_locks
				WHERE universe_id = (SELECT id FROM universes WHERE code_name = $1)
				  AND galaxy = $2 AND system = $3 AND position = $4
				  AND locked_until > NOW()
			)
		`, universeCode, g, sys, p).Scan(&locked); err != nil {
			return 0, err
		}
		if locked {
			if _, err := tx.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID); err != nil {
				return 0, err
			}
			continue
		}
		return planetID, nil
	}
	return 0, ErrNotFound // no free slot found (universe effectively full)
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

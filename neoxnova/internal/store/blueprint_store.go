package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"neoxnova/internal/blueprint"
	"neoxnova/internal/game"
)

// BlueprintStore persists auto-build blueprints and loads the state snapshots
// the pure planner needs.
type BlueprintStore struct {
	db *sql.DB
}

func NewBlueprintStore(db *sql.DB) *BlueprintStore {
	return &BlueprintStore{db: db}
}

// Blueprint is a stored auto-build blueprint row.
type Blueprint struct {
	ID           int64           `json:"id"`
	UniverseID   string          `json:"-"`
	UserID       int64           `json:"user_id"`
	CelestialID  sql.NullInt64   `json:"celestial_id"`
	Name         string          `json:"name"`
	Scope        string          `json:"scope"`
	Spec         json.RawMessage `json:"spec"`
	Enabled      bool            `json:"enabled"`
	Priority     int             `json:"priority"`
	PausedUntil  sql.NullTime    `json:"-"`
	LastActionAt sql.NullTime    `json:"-"`
	LastError    sql.NullString  `json:"last_error"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

const blueprintColumns = `id, universe_id::text, user_id, celestial_id, name, scope,
	spec, enabled, priority, paused_until, last_action_at, last_error, created_at, updated_at`

func scanBlueprint(row interface{ Scan(...any) error }) (Blueprint, error) {
	var b Blueprint
	err := row.Scan(&b.ID, &b.UniverseID, &b.UserID, &b.CelestialID, &b.Name, &b.Scope,
		&b.Spec, &b.Enabled, &b.Priority, &b.PausedUntil, &b.LastActionAt, &b.LastError,
		&b.CreatedAt, &b.UpdatedAt)
	return b, err
}

// Get returns a blueprint by id.
func (s *BlueprintStore) Get(ctx context.Context, id int64) (Blueprint, error) {
	b, err := scanBlueprint(s.db.QueryRowContext(ctx, `SELECT `+blueprintColumns+` FROM build_blueprints WHERE id = $1`, id))
	if err == sql.ErrNoRows {
		return Blueprint{}, ErrNotFound
	}
	return b, err
}

// GetPlanet returns the enabled planet blueprint for a celestial.
func (s *BlueprintStore) GetPlanet(ctx context.Context, celestialID int64) (Blueprint, error) {
	b, err := scanBlueprint(s.db.QueryRowContext(ctx, `
		SELECT `+blueprintColumns+` FROM build_blueprints
		WHERE celestial_id = $1 AND enabled
		LIMIT 1
	`, celestialID))
	if err == sql.ErrNoRows {
		return Blueprint{}, ErrNotFound
	}
	return b, err
}

// GetAccount returns the enabled account-scope blueprint for a user.
func (s *BlueprintStore) GetAccount(ctx context.Context, userID int64) (Blueprint, error) {
	b, err := scanBlueprint(s.db.QueryRowContext(ctx, `
		SELECT `+blueprintColumns+` FROM build_blueprints
		WHERE user_id = $1 AND celestial_id IS NULL AND enabled
		LIMIT 1
	`, userID))
	if err == sql.ErrNoRows {
		return Blueprint{}, ErrNotFound
	}
	return b, err
}

// GetForCelestial returns the newest blueprint for a celestial, enabled or not.
func (s *BlueprintStore) GetForCelestial(ctx context.Context, celestialID int64) (Blueprint, error) {
	b, err := scanBlueprint(s.db.QueryRowContext(ctx, `
		SELECT `+blueprintColumns+` FROM build_blueprints
		WHERE celestial_id = $1 ORDER BY id DESC LIMIT 1
	`, celestialID))
	if err == sql.ErrNoRows {
		return Blueprint{}, ErrNotFound
	}
	return b, err
}

// DeleteForCelestial removes every blueprint attached to a celestial.
func (s *BlueprintStore) DeleteForCelestial(ctx context.Context, celestialID int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM build_blueprints WHERE celestial_id = $1`, celestialID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CelestialOwner returns the user id owning a celestial (0 when unowned).
func (s *BlueprintStore) CelestialOwner(ctx context.Context, celestialID int64) (int64, error) {
	var owner sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM celestial_objects WHERE id = $1`, celestialID).Scan(&owner)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return owner.Int64, nil
}

// ListForUser returns every blueprint owned by a user (newest first).
func (s *BlueprintStore) ListForUser(ctx context.Context, userID int64) ([]Blueprint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+blueprintColumns+` FROM build_blueprints WHERE user_id = $1 ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Blueprint
	for rows.Next() {
		b, err := scanBlueprint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// Save inserts (ID == 0) or updates a blueprint. Enabling a blueprint disables
// any other enabled blueprint in the same scope, keeping the partial unique
// indexes satisfied.
func (s *BlueprintStore) Save(ctx context.Context, b Blueprint) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if b.Enabled {
		if b.CelestialID.Valid {
			if _, err := tx.ExecContext(ctx, `
				UPDATE build_blueprints SET enabled = false, updated_at = NOW()
				WHERE celestial_id = $1 AND enabled AND id <> $2
			`, b.CelestialID.Int64, b.ID); err != nil {
				return 0, err
			}
		} else {
			if _, err := tx.ExecContext(ctx, `
				UPDATE build_blueprints SET enabled = false, updated_at = NOW()
				WHERE user_id = $1 AND celestial_id IS NULL AND enabled AND id <> $2
			`, b.UserID, b.ID); err != nil {
				return 0, err
			}
		}
	}

	if b.ID > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE build_blueprints
			SET name = $2, scope = $3, spec = $4, enabled = $5, priority = $6,
			    paused_until = $7, updated_at = NOW()
			WHERE id = $1
		`, b.ID, b.Name, b.Scope, b.Spec, b.Enabled, b.Priority, b.PausedUntil); err != nil {
			return 0, err
		}
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		return b.ID, nil
	}

	var id int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO build_blueprints (universe_id, user_id, celestial_id, name, scope, spec, enabled, priority, paused_until)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`, b.UniverseID, b.UserID, b.CelestialID, b.Name, b.Scope, b.Spec, b.Enabled, b.Priority, b.PausedUntil).Scan(&id); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// Delete removes a blueprint.
func (s *BlueprintStore) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM build_blueprints WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// EnabledRef identifies one enabled blueprint for the scheduler sweep.
type EnabledRef struct {
	ID          int64
	CelestialID sql.NullInt64
	UserID      int64
	Scope       string
}

// ListEnabled returns every enabled planet blueprint (account blueprints are
// merged into their planets when advancing, so they are not swept directly).
func (s *BlueprintStore) ListEnabled(ctx context.Context, limit int) ([]EnabledRef, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, celestial_id, user_id, scope
		FROM build_blueprints
		WHERE enabled AND celestial_id IS NOT NULL
		ORDER BY id
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EnabledRef
	for rows.Next() {
		var r EnabledRef
		if err := rows.Scan(&r.ID, &r.CelestialID, &r.UserID, &r.Scope); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Touch records the outcome of one advance attempt.
func (s *BlueprintStore) Touch(ctx context.Context, id int64, lastError string) error {
	var e any
	if lastError != "" {
		e = lastError
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE build_blueprints
		SET last_action_at = NOW(), last_error = $2, updated_at = NOW()
		WHERE id = $1
	`, id, e)
	return err
}

// LoadState builds the planner snapshot for a celestial: structure/tech levels,
// ship/defense counts, current resources, energy, fields and queue occupancy.
func (s *BlueprintStore) LoadState(ctx context.Context, celestialID int64) (blueprint.State, error) {
	st := blueprint.State{
		Structures: map[string]int{},
		Techs:      map[string]int{},
		Ships:      map[string]int64{},
		Defenses:   map[string]int64{},
	}

	var userID sql.NullInt64
	var gameSpeed float64
	if err := s.db.QueryRowContext(ctx, `
		SELECT c.user_id, u.game_speed
		FROM celestial_objects c JOIN universes u ON u.id = c.universe_id
		WHERE c.id = $1
	`, celestialID).Scan(&userID, &gameSpeed); err != nil {
		if err == sql.ErrNoRows {
			return st, ErrNotFound
		}
		return st, err
	}
	st.GameSpeed = gameSpeed

	var metal, crystal, deuterium float64
	var lastCalc time.Time
	if err := s.db.QueryRowContext(ctx, `SELECT * FROM update_celestial_resources($1)`, celestialID).Scan(&metal, &crystal, &deuterium, &lastCalc); err != nil {
		return st, err
	}
	st.Resources = game.Cost{Metal: int64(metal), Crystal: int64(crystal), Deuterium: int64(deuterium)}

	if err := s.db.QueryRowContext(ctx, `
		SELECT energy_used, energy_max, fields_used, fields_max
		FROM celestial_objects WHERE id = $1
	`, celestialID).Scan(&st.EnergyUsed, &st.EnergyMax, &st.FieldsUsed, &st.FieldsMax); err != nil {
		return st, err
	}

	if err := loadLevels(ctx, s.db, `SELECT structure_code, level FROM planet_structures WHERE celestial_id = $1`, celestialID, st.Structures); err != nil {
		return st, err
	}
	if err := loadLevels(ctx, s.db, `SELECT tech_code, level FROM user_technologies WHERE user_id = $1`, userID, st.Techs); err != nil {
		return st, err
	}
	if err := loadCounts(ctx, s.db, `SELECT ship_code, quantity FROM planet_ships WHERE celestial_id = $1`, celestialID, st.Ships); err != nil {
		return st, err
	}
	if err := loadCounts(ctx, s.db, `SELECT defense_code, quantity FROM planet_defenses WHERE celestial_id = $1`, celestialID, st.Defenses); err != nil {
		return st, err
	}

	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM construction_queues WHERE celestial_id = $1 AND status = 'IN_PROGRESS')`, celestialID).Scan(&st.BuildBusy); err != nil {
		return st, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM shipyard_queues WHERE celestial_id = $1 AND status = 'IN_PROGRESS')`, celestialID).Scan(&st.ShipBusy); err != nil {
		return st, err
	}
	if userID.Valid {
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM research_queues WHERE user_id = $1 AND status = 'IN_PROGRESS')`, userID.Int64).Scan(&st.ResearchBusy); err != nil {
			return st, err
		}
	}
	return st, nil
}

func loadLevels(ctx context.Context, db *sql.DB, query string, arg any, out map[string]int) error {
	rows, err := db.QueryContext(ctx, query, arg)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var level int
		if err := rows.Scan(&code, &level); err != nil {
			return err
		}
		out[code] = level
	}
	return rows.Err()
}

func loadCounts(ctx context.Context, db *sql.DB, query string, arg any, out map[string]int64) error {
	rows, err := db.QueryContext(ctx, query, arg)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var code string
		var n int64
		if err := rows.Scan(&code, &n); err != nil {
			return err
		}
		out[code] = n
	}
	return rows.Err()
}

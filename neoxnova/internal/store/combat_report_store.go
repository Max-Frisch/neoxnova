package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// CombatReportStore persists and serves battle reports.
type CombatReportStore struct {
	db *sql.DB
}

func NewCombatReportStore(db *sql.DB) *CombatReportStore {
	return &CombatReportStore{db: db}
}

// CombatReport is a stored battle outcome plus its full JSON detail payload.
type CombatReport struct {
	ID            int64           `json:"id"`
	FleetID       sql.NullInt64   `json:"-"`
	AttackerID    sql.NullInt64   `json:"attacker_id"`
	DefenderID    sql.NullInt64   `json:"defender_id"`
	TargetID      sql.NullInt64   `json:"target_id"`
	Galaxy        int             `json:"galaxy"`
	System        int             `json:"system"`
	Position      int             `json:"position"`
	Result        string          `json:"result"`
	Rounds        int             `json:"rounds"`
	DebrisMetal   int64           `json:"debris_metal"`
	DebrisCrystal int64           `json:"debris_crystal"`
	MoonChance    int             `json:"moon_chance"`
	Report        json.RawMessage `json:"report"`
	CreatedAt     time.Time       `json:"created_at"`
}

const combatReportColumns = `id, fleet_id, attacker_id, defender_id, target_id,
	galaxy, system, position, result, rounds, debris_metal, debris_crystal,
	moon_chance, report, created_at`

func scanCombatReport(row interface{ Scan(...any) error }) (CombatReport, error) {
	var r CombatReport
	err := row.Scan(
		&r.ID, &r.FleetID, &r.AttackerID, &r.DefenderID, &r.TargetID,
		&r.Galaxy, &r.System, &r.Position, &r.Result, &r.Rounds,
		&r.DebrisMetal, &r.DebrisCrystal, &r.MoonChance, &r.Report, &r.CreatedAt,
	)
	return r, err
}

// Get returns a single report by id.
func (s *CombatReportStore) Get(ctx context.Context, id int64) (CombatReport, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+combatReportColumns+` FROM combat_reports WHERE id = $1`, id)
	r, err := scanCombatReport(row)
	if err == sql.ErrNoRows {
		return CombatReport{}, ErrNotFound
	}
	return r, err
}

// ListForCelestial returns the newest reports whose attacker OR defender is the
// given celestial's owner, OR that target the celestial itself.
func (s *CombatReportStore) ListForCelestial(ctx context.Context, celestialID int64, limit int) ([]CombatReport, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+combatReportColumns+`
		FROM combat_reports
		WHERE target_id = $1
		   OR attacker_id = (SELECT user_id FROM celestial_objects WHERE id = $1)
		   OR defender_id = (SELECT user_id FROM celestial_objects WHERE id = $1)
		ORDER BY id DESC
		LIMIT $2
	`, celestialID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CombatReport
	for rows.Next() {
		r, err := scanCombatReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

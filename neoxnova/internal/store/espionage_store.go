package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// EspionageReportStore persists and serves spy reports.
type EspionageReportStore struct {
	db *sql.DB
}

func NewEspionageReportStore(db *sql.DB) *EspionageReportStore {
	return &EspionageReportStore{db: db}
}

// EspionageReport is a stored spy report plus its full intel JSON payload.
type EspionageReport struct {
	ID         int64           `json:"id"`
	FleetID    sql.NullInt64   `json:"-"`
	AttackerID sql.NullInt64   `json:"attacker_id"`
	DefenderID sql.NullInt64   `json:"defender_id"`
	TargetID   sql.NullInt64   `json:"target_id"`
	Galaxy     int             `json:"galaxy"`
	System     int             `json:"system"`
	Position   int             `json:"position"`
	ProbesSent int             `json:"probes_sent"`
	ProbesLost int             `json:"probes_lost"`
	Score      int             `json:"score"`
	Report     json.RawMessage `json:"report"`
	CreatedAt  time.Time       `json:"created_at"`
}

const espionageReportColumns = `id, fleet_id, attacker_id, defender_id, target_id,
	galaxy, system, position, probes_sent, probes_lost, score, report, created_at`

func scanEspionageReport(row interface{ Scan(...any) error }) (EspionageReport, error) {
	var r EspionageReport
	err := row.Scan(
		&r.ID, &r.FleetID, &r.AttackerID, &r.DefenderID, &r.TargetID,
		&r.Galaxy, &r.System, &r.Position, &r.ProbesSent, &r.ProbesLost,
		&r.Score, &r.Report, &r.CreatedAt,
	)
	return r, err
}

// Get returns a single report by id.
func (s *EspionageReportStore) Get(ctx context.Context, id int64) (EspionageReport, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+espionageReportColumns+` FROM espionage_reports WHERE id = $1`, id)
	r, err := scanEspionageReport(row)
	if err == sql.ErrNoRows {
		return EspionageReport{}, ErrNotFound
	}
	return r, err
}

// ListForCelestial returns the newest reports that target the celestial, were
// made by its owner (outgoing), or were made against its owner (incoming).
func (s *EspionageReportStore) ListForCelestial(ctx context.Context, celestialID int64, limit int) ([]EspionageReport, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+espionageReportColumns+`
		FROM espionage_reports
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
	var out []EspionageReport
	for rows.Next() {
		r, err := scanEspionageReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

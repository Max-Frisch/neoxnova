package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// ExpeditionReportStore persists and serves expedition outcome messages.
type ExpeditionReportStore struct {
	db *sql.DB
}

func NewExpeditionReportStore(db *sql.DB) *ExpeditionReportStore {
	return &ExpeditionReportStore{db: db}
}

// ExpeditionReport is one persisted expedition outcome plus its message text
// and full result payload.
type ExpeditionReport struct {
	ID               int64           `json:"id"`
	FleetID          sql.NullInt64   `json:"-"`
	UserID           sql.NullInt64   `json:"user_id"`
	Galaxy           int             `json:"galaxy"`
	System           int             `json:"system"`
	Position         int             `json:"position"`
	Outcome          string          `json:"outcome"`
	NPC              sql.NullString  `json:"npc"`
	CombatReportID   sql.NullInt64   `json:"combat_report_id"`
	CargoMetal       int64           `json:"cargo_metal"`
	CargoCrystal     int64           `json:"cargo_crystal"`
	CargoDeuterium   int64           `json:"cargo_deuterium"`
	DarkMatter       int64           `json:"dark_matter"`
	Ships            json.RawMessage `json:"ships"`
	UpgradeCode      int             `json:"upgrade_code"`
	ReturnAdjustSecs int64           `json:"return_adjust_secs"`
	Title            string          `json:"title"`
	Message          string          `json:"message"`
	Detail           json.RawMessage `json:"detail"`
	CreatedAt        time.Time       `json:"created_at"`
}

const expeditionReportColumns = `id, fleet_id, user_id, target_galaxy, target_system,
	target_position, outcome, npc, combat_report_id, cargo_metal, cargo_crystal,
	cargo_deuterium, dark_matter, ships, upgrade_code, return_adjust_secs, title,
	message, detail, created_at`

func scanExpeditionReport(row interface{ Scan(...any) error }) (ExpeditionReport, error) {
	var r ExpeditionReport
	err := row.Scan(
		&r.ID, &r.FleetID, &r.UserID, &r.Galaxy, &r.System, &r.Position,
		&r.Outcome, &r.NPC, &r.CombatReportID, &r.CargoMetal, &r.CargoCrystal,
		&r.CargoDeuterium, &r.DarkMatter, &r.Ships, &r.UpgradeCode,
		&r.ReturnAdjustSecs, &r.Title, &r.Message, &r.Detail, &r.CreatedAt,
	)
	return r, err
}

// Get returns a single expedition report by id.
func (s *ExpeditionReportStore) Get(ctx context.Context, id int64) (ExpeditionReport, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+expeditionReportColumns+` FROM expedition_reports WHERE id = $1`, id)
	r, err := scanExpeditionReport(row)
	if err == sql.ErrNoRows {
		return ExpeditionReport{}, ErrNotFound
	}
	return r, err
}

// ListForCelestial returns the newest expedition reports belonging to the
// owner of the given celestial (expeditions target empty deep space, so there
// is no target celestial to key on).
func (s *ExpeditionReportStore) ListForCelestial(ctx context.Context, celestialID int64, limit int) ([]ExpeditionReport, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+expeditionReportColumns+`
		FROM expedition_reports
		WHERE user_id = (SELECT user_id FROM celestial_objects WHERE id = $1)
		ORDER BY id DESC
		LIMIT $2
	`, celestialID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExpeditionReport
	for rows.Next() {
		r, err := scanExpeditionReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

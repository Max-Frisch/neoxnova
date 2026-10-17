package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
)

// MoonStore holds the moon-only operations: the Jumpgate (ship-only, cross-moon
// teleport) and the Phalanx Sensor (same-galaxy fleet scan). See docs/MOONS.md.
type MoonStore struct {
	db     *sql.DB
	builds *BuildStore
}

func NewMoonStore(db *sql.DB) *MoonStore {
	return &MoonStore{db: db, builds: NewBuildStore(db)}
}

// JumpDestination is one selectable destination moon for a Jumpgate jump.
type JumpDestination struct {
	MoonID        int64
	Name          string
	Galaxy        int
	System        int
	Position      int
	JumpgateLevel int
}

// JumpgateStatus is the state shown on the Jumpgate page for a source moon.
type JumpgateStatus struct {
	Level        int
	ReadyAt      time.Time
	CooldownSecs int64
	Destinations []JumpDestination
}

// JumpgateStatusFor returns the source moon's gate level, its recharge window
// and every other owned moon that also has a gate (the valid destinations).
func (s *MoonStore) JumpgateStatusFor(ctx context.Context, userID, moonID int64) (JumpgateStatus, error) {
	var (
		owner   sql.NullInt64
		otype   string
		level   int
		readyAt sql.NullTime
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT c.user_id, c.object_type::text,
		       COALESCE(ps.level, 0), c.jumpgate_next_at
		FROM celestial_objects c
		LEFT JOIN planet_structures ps
		       ON ps.celestial_id = c.id AND ps.structure_code = 'jumpgate'
		WHERE c.id = $1
	`, moonID).Scan(&owner, &otype, &level, &readyAt)
	if errors.Is(err, sql.ErrNoRows) {
		return JumpgateStatus{}, ErrNotFound
	} else if err != nil {
		return JumpgateStatus{}, err
	}
	if !owner.Valid || owner.Int64 != userID || otype != "MOON" {
		return JumpgateStatus{}, ErrNotFound
	}

	st := JumpgateStatus{Level: level}
	if readyAt.Valid {
		st.ReadyAt = readyAt.Time
		if secs := int64(time.Until(readyAt.Time).Seconds()); secs > 0 {
			st.CooldownSecs = secs
		}
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.name, c.galaxy, c.system, c.position, ps.level
		FROM celestial_objects c
		JOIN planet_structures ps
		  ON ps.celestial_id = c.id AND ps.structure_code = 'jumpgate' AND ps.level >= 1
		WHERE c.user_id = $1 AND c.object_type = 'MOON' AND c.id <> $2
		ORDER BY c.galaxy, c.system, c.position
	`, userID, moonID)
	if err != nil {
		return JumpgateStatus{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var d JumpDestination
		if err := rows.Scan(&d.MoonID, &d.Name, &d.Galaxy, &d.System, &d.Position, &d.JumpgateLevel); err != nil {
			return JumpgateStatus{}, err
		}
		st.Destinations = append(st.Destinations, d)
	}
	return st, rows.Err()
}

// JumpFleet teleports `ships` from one owned moon to another. Both moons need a
// Jumpgate; only ships travel (resources can never be transferred), and the
// source gate is put on game.JumpgateCooldown(level). The cooldown is stored on
// the source moon.
func (s *MoonStore) JumpFleet(ctx context.Context, userID, srcMoonID, dstMoonID int64, ships map[string]int64) error {
	if srcMoonID == dstMoonID {
		return ErrNoJumpgate
	}
	if len(ships) == 0 {
		return ErrInvalidQuantity
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	gateLevel := func(moonID int64) (int, *time.Time, error) {
		var owner sql.NullInt64
		var otype string
		var level int
		var readyAt sql.NullTime
		err := tx.QueryRowContext(ctx, `
			SELECT c.user_id, c.object_type::text,
			       COALESCE(ps.level, 0), c.jumpgate_next_at
			FROM celestial_objects c
			LEFT JOIN planet_structures ps
			       ON ps.celestial_id = c.id AND ps.structure_code = 'jumpgate'
			WHERE c.id = $1 FOR UPDATE OF c
		`, moonID).Scan(&owner, &otype, &level, &readyAt)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil, ErrNotFound
		} else if err != nil {
			return 0, nil, err
		}
		if !owner.Valid || owner.Int64 != userID || otype != "MOON" {
			return 0, nil, ErrNotFound
		}
		if level < 1 {
			return 0, nil, ErrNoJumpgate
		}
		var r *time.Time
		if readyAt.Valid {
			t := readyAt.Time
			r = &t
		}
		return level, r, nil
	}

	srcLevel, readyAt, err := gateLevel(srcMoonID)
	if err != nil {
		return err
	}
	if _, _, err := gateLevel(dstMoonID); err != nil {
		return err
	}
	if readyAt != nil && time.Now().Before(*readyAt) {
		return ErrJumpgateCooldown
	}

	for code, count := range ships {
		if count <= 0 {
			return ErrInvalidQuantity
		}
		var available int64
		err := tx.QueryRowContext(ctx, `
			SELECT quantity FROM planet_ships
			WHERE celestial_id = $1 AND ship_code = $2 FOR UPDATE
		`, srcMoonID, code).Scan(&available)
		if err != nil || available < count {
			return &InsufficientShipsError{ShipCode: code, Available: available, Requested: count}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE planet_ships SET quantity = quantity - $1
			WHERE celestial_id = $2 AND ship_code = $3
		`, count, srcMoonID, code); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO planet_ships (celestial_id, ship_code, quantity)
			VALUES ($1, $2, $3)
			ON CONFLICT (celestial_id, ship_code)
			DO UPDATE SET quantity = planet_ships.quantity + EXCLUDED.quantity
		`, dstMoonID, code, count); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE celestial_objects SET jumpgate_next_at = $1 WHERE id = $2
	`, time.Now().Add(game.JumpgateCooldown(srcLevel)), srcMoonID); err != nil {
		return err
	}
	return tx.Commit()
}

// PhalanxScan returns the active fleets moving to or from a target coordinate,
// as seen by a Phalanx Sensor on sensorMoonID. It enforces the measured rules:
// the sensor must exist, be online (10 min after a teleport), and the target
// must be in the same galaxy within level^2-1 systems. The scan is passive — it
// reveals composition, so the full fleet summaries are returned.
func (s *MoonStore) PhalanxScan(ctx context.Context, userID, sensorMoonID int64, target models.Coordinates) ([]models.FleetEventSummary, error) {
	var (
		owner       sql.NullInt64
		otype       string
		galaxy      int
		system      int
		level       int
		offlineTill sql.NullTime
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT c.user_id, c.object_type::text, c.galaxy, c.system,
		       COALESCE(ps.level, 0), c.phalanx_offline_until
		FROM celestial_objects c
		LEFT JOIN planet_structures ps
		       ON ps.celestial_id = c.id AND ps.structure_code = 'phalanx_sensor'
		WHERE c.id = $1
	`, sensorMoonID).Scan(&owner, &otype, &galaxy, &system, &level, &offlineTill)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	if !owner.Valid || owner.Int64 != userID || otype != "MOON" || level < 1 {
		return nil, ErrPhalanxNoSensor
	}
	if offlineTill.Valid && time.Now().Before(offlineTill.Time) {
		return nil, ErrPhalanxOffline
	}
	if !game.PhalanxInRange(galaxy, system, target.Galaxy, target.System, level) {
		return nil, ErrPhalanxOutOfRange
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, mission, phase, origin_galaxy, origin_system, origin_position, origin_type,
		       target_galaxy, target_system, target_position, target_type,
		       start_time, arrival_time, cargo_metal, cargo_crystal, cargo_deuterium
		FROM fleets
		WHERE phase IN ('OUTBOUND', 'HOLDING', 'RETURNING')
		  AND (
		    (target_galaxy = $1 AND target_system = $2 AND target_position = $3)
		    OR
		    (origin_galaxy = $1 AND origin_system = $2 AND origin_position = $3)
		  )
		ORDER BY arrival_time ASC
	`, target.Galaxy, target.System, target.Position)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fleets := make([]models.FleetEventSummary, 0)
	for rows.Next() {
		var f models.FleetEventSummary
		var oG, oS, oP, tG, tS, tP int
		var oT, tT, missionStr, phaseStr string
		if err := rows.Scan(
			&f.FleetID, &missionStr, &phaseStr,
			&oG, &oS, &oP, &oT,
			&tG, &tS, &tP, &tT,
			&f.DepartureTime, &f.ArrivalTime,
			&f.Cargo.Metal, &f.Cargo.Crystal, &f.Cargo.Deuterium,
		); err != nil {
			return nil, err
		}
		f.Mission = models.MissionType(missionStr)
		f.Phase = models.FleetPhase(phaseStr)
		f.Origin = models.Coordinates{Galaxy: oG, System: oS, Position: oP, Type: models.CelestialType(oT)}
		f.Destination = models.Coordinates{Galaxy: tG, System: tS, Position: tP, Type: models.CelestialType(tT)}
		if secs := int64(time.Until(f.ArrivalTime).Seconds()); secs > 0 {
			f.RemainingSecs = secs
		}
		fleets = append(fleets, f)
	}
	return fleets, rows.Err()
}

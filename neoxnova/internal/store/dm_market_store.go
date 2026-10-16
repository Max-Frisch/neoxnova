package store

import (
	"context"
	"database/sql"
	"errors"

	"neoxnova/internal/game"
)

// universeDMDivisor reads the resources-per-Dark-Matter divisor used to price
// the Buildings/Technology markets for a celestial's universe.
func (s *BuildStore) universeDMDivisor(ctx context.Context, tx *sql.Tx, planetID int64) (int64, error) {
	var divisor int64
	err := tx.QueryRowContext(ctx, `
		SELECT u.dm_market_divisor
		FROM celestial_objects c
		JOIN universes u ON u.id = c.universe_id
		WHERE c.id = $1
	`, planetID).Scan(&divisor)
	return divisor, err
}

// StructureLevels returns a celestial's building levels (exported for the DM
// Buildings Market page, which renders the next-level price).
func (s *BuildStore) StructureLevels(ctx context.Context, planetID int64) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT structure_code, level FROM planet_structures WHERE celestial_id = $1`, planetID)
	if err != nil {
		return nil, err
	}
	return scanLevels(rows)
}

// TechLevels returns a user's researched levels (exported for the DM Technology
// Market page).
func (s *BuildStore) TechLevels(ctx context.Context, userID int64) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tech_code, level FROM user_technologies WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	return scanLevels(rows)
}

// InstantStructure builds the next level of a building immediately for Dark
// Matter (Black market -> Buildings Market; page=dmbuild). It respects moon
// legality, prerequisites and the field budget, debits the DM price and applies
// the level in one transaction (no queue). Returns the level's resource cost and
// the Dark Matter paid.
func (s *BuildStore) InstantStructure(ctx context.Context, planetID, userID int64, code string) (game.Cost, int64, error) {
	if _, ok := game.Structures[code]; !ok {
		if _, moonOK := game.MoonOnlyStructures[code]; !moonOK {
			return game.Cost{}, 0, ErrUnknownCode
		}
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return game.Cost{}, 0, err
	}
	defer tx.Rollback()

	owner, objectType, _, fieldsUsed, fieldsMax, _, _, _, _, err := s.planetContext(ctx, tx, planetID)
	if errors.Is(err, sql.ErrNoRows) {
		return game.Cost{}, 0, ErrNotFound
	} else if err != nil {
		return game.Cost{}, 0, err
	}
	if owner != userID {
		return game.Cost{}, 0, ErrNotFound
	}
	if objectType == "MOON" {
		if _, ok := game.MoonStructure(code); !ok {
			return game.Cost{}, 0, ErrUnknownCode
		}
	}

	levels, err := s.combinedLevels(ctx, tx, planetID, userID)
	if err != nil {
		return game.Cost{}, 0, err
	}
	def := game.Structures[code]
	if objectType == "MOON" {
		def, _ = game.MoonStructure(code)
	}
	if !game.RequiresMet(def.Requires, levels) {
		return game.Cost{}, 0, ErrPrerequisites
	}
	if fieldsUsed+1 > fieldsMax {
		return game.Cost{}, 0, ErrNoFields
	}

	target := levels[code] + 1
	cost, ok := game.StructureCost(code, target)
	if !ok {
		return game.Cost{}, 0, ErrUnknownCode
	}
	divisor, err := s.universeDMDivisor(ctx, tx, planetID)
	if err != nil {
		return game.Cost{}, 0, err
	}
	price := game.DMMarketPrice(cost, divisor)

	if err := debitDarkMatter(ctx, tx, userID, price); err != nil {
		return game.Cost{}, 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO planet_structures (celestial_id, structure_code, level)
		VALUES ($1, $2, $3)
		ON CONFLICT (celestial_id, structure_code) DO UPDATE SET level = EXCLUDED.level
	`, planetID, code, target); err != nil {
		return game.Cost{}, 0, err
	}
	if err := s.RecomputeCelestial(ctx, tx, planetID); err != nil {
		return game.Cost{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return game.Cost{}, 0, err
	}
	return cost, price, nil
}

// InstantTech researches the next level of a technology immediately for Dark
// Matter (Black market -> Technology Market; page=dmtech). It respects
// prerequisites, debits the DM price and applies the level in one transaction.
// Extraction research refreshes the account's cached production afterwards, as
// the scheduler does. Returns the level's resource cost and the Dark Matter paid.
func (s *BuildStore) InstantTech(ctx context.Context, planetID, userID int64, techCode string) (game.Cost, int64, error) {
	def, ok := game.Techs[techCode]
	if !ok {
		return game.Cost{}, 0, ErrUnknownCode
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return game.Cost{}, 0, err
	}
	defer tx.Rollback()

	owner, _, _, _, _, _, _, _, _, err := s.planetContext(ctx, tx, planetID)
	if errors.Is(err, sql.ErrNoRows) {
		return game.Cost{}, 0, ErrNotFound
	} else if err != nil {
		return game.Cost{}, 0, err
	}
	if owner != userID {
		return game.Cost{}, 0, ErrNotFound
	}

	levels, err := s.combinedLevels(ctx, tx, planetID, userID)
	if err != nil {
		return game.Cost{}, 0, err
	}
	if !game.RequiresMet(def.Requires, levels) {
		return game.Cost{}, 0, ErrPrerequisites
	}

	target := levels[techCode] + 1
	cost, ok := game.TechCost(techCode, target)
	if !ok {
		return game.Cost{}, 0, ErrUnknownCode
	}
	if req, gated := game.TechEnergyRequirement(techCode, target); gated {
		var energyMax float64
		if err := tx.QueryRowContext(ctx, `SELECT energy_max FROM celestial_objects WHERE id = $1`, planetID).Scan(&energyMax); err != nil {
			return game.Cost{}, 0, err
		}
		if energyMax < float64(req) {
			return game.Cost{}, 0, ErrEnergyRequirement
		}
	}
	divisor, err := s.universeDMDivisor(ctx, tx, planetID)
	if err != nil {
		return game.Cost{}, 0, err
	}
	price := game.DMMarketPrice(cost, divisor)

	if err := debitDarkMatter(ctx, tx, userID, price); err != nil {
		return game.Cost{}, 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_technologies (user_id, tech_code, level)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, tech_code) DO UPDATE SET level = EXCLUDED.level
	`, userID, techCode, target); err != nil {
		return game.Cost{}, 0, err
	}
	if err := tx.Commit(); err != nil {
		return game.Cost{}, 0, err
	}
	if game.IsProductionResearch(techCode) {
		_ = s.RecomputeAccountProduction(ctx, userID)
	}
	return cost, price, nil
}

// debitDarkMatter subtracts price from a user's balance, failing with
// ErrInsufficientDarkMatter when they cannot afford it.
func debitDarkMatter(ctx context.Context, tx *sql.Tx, userID, price int64) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE users SET dark_matter = dark_matter - $1 WHERE id = $2 AND dark_matter >= $1
	`, price, userID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrInsufficientDarkMatter
	}
	return nil
}

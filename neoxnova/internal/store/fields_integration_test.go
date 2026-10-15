package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"neoxnova/internal/game"
)

// TestExpandFields verifies Dark-Matter field expansion against a live Postgres:
// the escalating price, the Dark-Matter debit, and the base_fields_max fold.
// Skipped unless DATABASE_URL is set.
func TestExpandFields(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping DB integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	var universeID string
	if err := db.QueryRowContext(ctx, `SELECT universe_id::text FROM celestial_objects WHERE id = 1`).Scan(&universeID); err != nil {
		t.Skipf("seed missing celestial 1: %v", err)
	}

	upsertUser := func(name string, dm int64) int64 {
		var id int64
		if err := db.QueryRowContext(ctx, `
			INSERT INTO users (universe_id, username, email, password_hash, dark_matter)
			VALUES ($1, $2, $3, 'x', $4)
			ON CONFLICT (universe_id, username) DO UPDATE
				SET email = EXCLUDED.email, dark_matter = EXCLUDED.dark_matter
			RETURNING id
		`, universeID, name, name+"@example.com", dm).Scan(&id); err != nil {
			t.Fatalf("upsert user %s: %v", name, err)
		}
		return id
	}
	owner := upsertUser("fields_test_owner", 100000)
	other := upsertUser("fields_test_other", 100000)

	var pid int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO celestial_objects (universe_id, user_id, name, object_type, galaxy, system, position, base_fields_max, fields_max)
		VALUES ($1, $2, 'FieldsTest', 'PLANET', 9, 410, 5, 500, 500)
		ON CONFLICT (universe_id, galaxy, system, position, object_type) DO UPDATE
			SET user_id = EXCLUDED.user_id, base_fields_max = 500, fields_max = 500, fields_bought = 0
		RETURNING id
	`, universeID, owner).Scan(&pid); err != nil {
		t.Fatalf("insert planet: %v", err)
	}
	defer func() {
		db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, pid)
		db.ExecContext(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, owner, other)
	}()

	ps := NewPlanetStore(db)

	for _, n := range []int{0, -1, 101} {
		if _, err := ps.ExpandFields(ctx, pid, owner, n); !errors.Is(err, ErrInvalidQuantity) {
			t.Fatalf("ExpandFields(%d) = %v, want ErrInvalidQuantity", n, err)
		}
	}
	if _, err := ps.ExpandFields(ctx, pid, other, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign ExpandFields = %v, want ErrNotFound", err)
	}

	// First field costs 200 DM.
	res, err := ps.ExpandFields(ctx, pid, owner, 1)
	if err != nil {
		t.Fatalf("expand 1: %v", err)
	}
	if res.Cost != 200 || res.FieldsMax != 501 || res.FieldsBought != 1 {
		t.Fatalf("expand 1 => %+v, want cost 200 max 501 bought 1", res)
	}
	// The second field escalates to 220 DM (1.1x).
	res, err = ps.ExpandFields(ctx, pid, owner, 1)
	if err != nil {
		t.Fatalf("expand 2: %v", err)
	}
	if res.Cost != 220 || res.FieldsBought != 2 {
		t.Fatalf("expand 2 => %+v, want cost 220 bought 2", res)
	}
	// A batch of 3 with 2 already bought.
	batchCost := game.FieldExpansionCost(2, 3)
	res, err = ps.ExpandFields(ctx, pid, owner, 3)
	if err != nil {
		t.Fatalf("expand 3: %v", err)
	}
	if res.Cost != batchCost || res.FieldsMax != 505 || res.FieldsBought != 5 {
		t.Fatalf("expand 3 => %+v, want cost %d max 505 bought 5", res, batchCost)
	}

	// Dark Matter debited exactly; base_fields_max carries the purchase.
	var dm, base int64
	if err := db.QueryRowContext(ctx, `SELECT dark_matter FROM users WHERE id = $1`, owner).Scan(&dm); err != nil {
		t.Fatalf("read dm: %v", err)
	}
	if want := int64(100000 - 200 - 220 - batchCost); dm != want {
		t.Fatalf("dm = %d, want %d", dm, want)
	}
	if err := db.QueryRowContext(ctx, `SELECT base_fields_max FROM celestial_objects WHERE id = $1`, pid).Scan(&base); err != nil {
		t.Fatalf("read base: %v", err)
	}
	if base != 505 {
		t.Fatalf("base_fields_max = %d, want 505", base)
	}

	// Insufficient Dark Matter is refused and does not change the field count.
	if _, err := db.ExecContext(ctx, `UPDATE users SET dark_matter = 0 WHERE id = $1`, owner); err != nil {
		t.Fatalf("zero dm: %v", err)
	}
	if _, err := ps.ExpandFields(ctx, pid, owner, 1); !errors.Is(err, ErrInsufficientDarkMatter) {
		t.Fatalf("broke ExpandFields = %v, want ErrInsufficientDarkMatter", err)
	}
	var bought int64
	if err := db.QueryRowContext(ctx, `SELECT fields_bought FROM celestial_objects WHERE id = $1`, pid).Scan(&bought); err != nil {
		t.Fatalf("read bought: %v", err)
	}
	if bought != 5 {
		t.Fatalf("fields_bought changed to %d after a failed purchase", bought)
	}
}

// TestExpandDiameter verifies the Stardust "increase in diameter" sink against a
// live Postgres: the Stardust + debris-field debit, the diameter/field gain, the
// moon support and the insufficient-resource refusals. Skipped without
// DATABASE_URL.
func TestExpandDiameter(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping DB integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	var universeID string
	if err := db.QueryRowContext(ctx, `SELECT universe_id::text FROM celestial_objects WHERE id = 1`).Scan(&universeID); err != nil {
		t.Skipf("seed missing celestial 1: %v", err)
	}

	upsertUser := func(name string, stardust int64) int64 {
		var id int64
		if err := db.QueryRowContext(ctx, `
			INSERT INTO users (universe_id, username, email, password_hash, stardust)
			VALUES ($1, $2, $3, 'x', $4)
			ON CONFLICT (universe_id, username) DO UPDATE
				SET email = EXCLUDED.email, stardust = EXCLUDED.stardust
			RETURNING id
		`, universeID, name, name+"@example.com", stardust).Scan(&id); err != nil {
			t.Fatalf("upsert user %s: %v", name, err)
		}
		return id
	}
	owner := upsertUser("diameter_test_owner", 5)
	other := upsertUser("diameter_test_other", 5)

	const (
		g, sys, pos = 9, 411, 5
		startDia    = 10000
		startFields = 200
	)
	var planetID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO celestial_objects (universe_id, user_id, name, object_type, galaxy, system, position,
		                               diameter_km, base_fields_max, fields_max)
		VALUES ($1, $2, 'DiameterTest', 'PLANET', $3, $4, $5, $6, $7::bigint, $7::int)
		ON CONFLICT (universe_id, galaxy, system, position, object_type) DO UPDATE
			SET user_id = EXCLUDED.user_id, diameter_km = $6, base_fields_max = $7::bigint, fields_max = $7::int
		RETURNING id
	`, universeID, owner, g, sys, pos, startDia, startFields).Scan(&planetID); err != nil {
		t.Fatalf("insert planet: %v", err)
	}
	// A debris field at the planet's own coordinates (shared with any moon).
	seedDebris := func(metal, crystal int64) {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO celestial_objects (universe_id, name, object_type, galaxy, system, position, metal, crystal)
			VALUES ($1, 'Debris Field', 'DEBRIS_FIELD', $2, $3, $4, $5, $6)
			ON CONFLICT (universe_id, galaxy, system, position, object_type)
			DO UPDATE SET metal = EXCLUDED.metal, crystal = EXCLUDED.crystal
		`, universeID, g, sys, pos, metal, crystal); err != nil {
			t.Fatalf("seed debris: %v", err)
		}
	}
	seedDebris(300_000_000_000, 200_000_000_000)
	defer func() {
		db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE universe_id = $1 AND galaxy = $2 AND system = $3 AND position = $4`, universeID, g, sys, pos)
		db.ExecContext(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, owner, other)
	}()

	ps := NewPlanetStore(db)

	// Foreign owner cannot expand.
	if _, err := ps.ExpandDiameter(ctx, planetID, other, 300, 15); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign ExpandDiameter = %v, want ErrNotFound", err)
	}

	// Happy path.
	res, err := ps.ExpandDiameter(ctx, planetID, owner, 300, 15)
	if err != nil {
		t.Fatalf("ExpandDiameter: %v", err)
	}
	if res.DiameterGain != 300 || res.FieldsGain != 15 || res.StardustSpent != 1 {
		t.Fatalf("result gains = %+v, want dia 300 fields 15 stardust 1", res)
	}
	if res.DiameterKm != startDia+300 || res.FieldsMax != startFields+15 {
		t.Fatalf("result totals = %+v, want dia %d fields %d", res, startDia+300, startFields+15)
	}

	var (
		stardust         int64
		dia              int
		base, maxFields  int64
		dMetal, dCrystal int64
	)
	if err := db.QueryRowContext(ctx, `SELECT stardust FROM users WHERE id = $1`, owner).Scan(&stardust); err != nil {
		t.Fatalf("read stardust: %v", err)
	}
	if want := int64(5 - 1); stardust != want {
		t.Fatalf("stardust = %d, want %d", stardust, want)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT diameter_km, base_fields_max, fields_max FROM celestial_objects WHERE id = $1
	`, planetID).Scan(&dia, &base, &maxFields); err != nil {
		t.Fatalf("read planet: %v", err)
	}
	if dia != startDia+300 || base != startFields+15 || maxFields != startFields+15 {
		t.Fatalf("planet = dia %d base %d max %d, want dia %d base %d max %d",
			dia, base, maxFields, startDia+300, startFields+15, startFields+15)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT metal::bigint, crystal::bigint FROM celestial_objects
		WHERE universe_id = $1 AND galaxy = $2 AND system = $3 AND position = $4 AND object_type = 'DEBRIS_FIELD'
	`, universeID, g, sys, pos).Scan(&dMetal, &dCrystal); err != nil {
		t.Fatalf("read debris: %v", err)
	}
	if dMetal != 300_000_000_000-game.DiameterExpansionMetalDebris ||
		dCrystal != 200_000_000_000-game.DiameterExpansionCrystalDebris {
		t.Fatalf("debris = %d/%d, want %d/%d", dMetal, dCrystal,
			300_000_000_000-game.DiameterExpansionMetalDebris,
			200_000_000_000-game.DiameterExpansionCrystalDebris)
	}

	// Insufficient Stardust is refused and changes nothing.
	if _, err := db.ExecContext(ctx, `UPDATE users SET stardust = 0 WHERE id = $1`, owner); err != nil {
		t.Fatalf("zero stardust: %v", err)
	}
	if _, err := ps.ExpandDiameter(ctx, planetID, owner, 300, 15); !errors.Is(err, ErrInsufficientStardust) {
		t.Fatalf("broke-stardust ExpandDiameter = %v, want ErrInsufficientStardust", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT diameter_km FROM celestial_objects WHERE id = $1`, planetID).Scan(&dia); err != nil {
		t.Fatalf("read dia: %v", err)
	}
	if dia != startDia+300 {
		t.Fatalf("diameter changed to %d after a refused expansion", dia)
	}

	// Insufficient debris rolls the Stardust back and changes nothing.
	if _, err := db.ExecContext(ctx, `UPDATE users SET stardust = 2 WHERE id = $1`, owner); err != nil {
		t.Fatalf("restore stardust: %v", err)
	}
	seedDebris(game.DiameterExpansionMetalDebris, 1) // enough metal, no crystal
	if _, err := ps.ExpandDiameter(ctx, planetID, owner, 300, 15); !errors.Is(err, ErrInsufficientDebris) {
		t.Fatalf("broke-debris ExpandDiameter = %v, want ErrInsufficientDebris", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT stardust FROM users WHERE id = $1`, owner).Scan(&stardust); err != nil {
		t.Fatalf("read stardust: %v", err)
	}
	if stardust != 2 {
		t.Fatalf("stardust = %d after a refused debris expansion, want 2 (rolled back)", stardust)
	}

	// Moons share their planet's coordinates and use the same sink.
	seedDebris(300_000_000_000, 200_000_000_000)
	var moonID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO celestial_objects (universe_id, user_id, name, object_type, galaxy, system, position,
		                               diameter_km, base_fields_max, fields_max)
		VALUES ($1, $2, 'DiameterTestMoon', 'MOON', $3, $4, $5, 5000, 50::bigint, 50::int)
		ON CONFLICT (universe_id, galaxy, system, position, object_type) DO UPDATE
			SET user_id = EXCLUDED.user_id, diameter_km = 5000, base_fields_max = 50::bigint, fields_max = 50::int
		RETURNING id
	`, universeID, owner, g, sys, pos).Scan(&moonID); err != nil {
		t.Fatalf("insert moon: %v", err)
	}
	res, err = ps.ExpandDiameter(ctx, moonID, owner, 300, 15)
	if err != nil {
		t.Fatalf("moon ExpandDiameter: %v", err)
	}
	if res.DiameterKm != 5300 || res.FieldsMax != 65 {
		t.Fatalf("moon result = %+v, want dia 5300 fields 65", res)
	}
}

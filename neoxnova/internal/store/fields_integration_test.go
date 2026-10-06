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

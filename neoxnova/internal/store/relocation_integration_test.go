package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"neoxnova/internal/game"
)

// TestRelocatePlanet verifies the relocation rules against a live Postgres:
// distance-priced Dark Matter debit, the origin slot is *not* locked, the target
// must be in-domain/free, and the per-planet cooldown holds. Skipped unless
// DATABASE_URL is set.
//
//	$env:DATABASE_URL = "postgres://postgres:password@localhost:5432/neoxnova?sslmode=disable"
//	go test ./internal/store/ -run TestRelocatePlanet -v
func TestRelocatePlanet(t *testing.T) {
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

	// Two throwaway owners in galaxy 9, well away from the seed data.
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
	upsertPlanet := func(name string, owner int64, g, s, p int) int64 {
		var id int64
		if err := db.QueryRowContext(ctx, `
			INSERT INTO celestial_objects (universe_id, user_id, name, object_type, galaxy, system, position, base_fields_max)
			VALUES ($1, $2, $3, 'PLANET', $4, $5, $6, 300)
			ON CONFLICT (universe_id, galaxy, system, position, object_type) DO UPDATE
				SET user_id = EXCLUDED.user_id
			RETURNING id
		`, universeID, owner, name, g, s, p).Scan(&id); err != nil {
			t.Fatalf("upsert planet %s: %v", name, err)
		}
		return id
	}

	owner := upsertUser("reloc_test_owner", 100000)
	other := upsertUser("reloc_test_other", 0)
	planet := upsertPlanet("Mover", owner, 9, 400, 5)
	occupied := upsertPlanet("Blocker", other, 9, 401, 5)

	defer func() {
		db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id IN ($1, $2)`, planet, occupied)
		db.ExecContext(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, owner, other)
		db.ExecContext(ctx, `DELETE FROM coordinate_locks WHERE universe_id=$1 AND galaxy=9 AND system IN (400,401,402,403)`, universeID)
	}()

	ps := NewPlanetStore(db)

	// Out-of-domain destinations are rejected before any state changes.
	for _, tc := range []struct{ g, s, p int }{{9, 400, 21}, {0, 400, 5}, {10, 400, 5}, {9, 500, 5}} {
		if _, err := ps.RelocatePlanet(ctx, planet, owner, tc.g, tc.s, tc.p); !errors.Is(err, ErrInvalidCoordinates) {
			t.Fatalf("relocate to (%d,%d,%d) = %v, want ErrInvalidCoordinates", tc.g, tc.s, tc.p, err)
		}
	}

	// Same coordinates and an occupied destination are refused.
	if _, err := ps.RelocatePlanet(ctx, planet, owner, 9, 400, 5); !errors.Is(err, ErrSameCoordinates) {
		t.Fatalf("same-coordinate relocate = %v, want ErrSameCoordinates", err)
	}
	if _, err := ps.RelocatePlanet(ctx, planet, owner, 9, 401, 5); !errors.Is(err, ErrTargetOccupied) {
		t.Fatalf("occupied relocate = %v, want ErrTargetOccupied", err)
	}

	// Wrong owner sees a 404 (ownership concealed).
	if _, err := ps.RelocatePlanet(ctx, planet, other, 9, 402, 5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign relocate = %v, want ErrNotFound", err)
	}

	// Not enough Dark Matter is refused and does not move the planet.
	if _, err := db.ExecContext(ctx, `UPDATE users SET dark_matter = 0 WHERE id = $1`, owner); err != nil {
		t.Fatalf("zero dm: %v", err)
	}
	if _, err := ps.RelocatePlanet(ctx, planet, owner, 9, 402, 5); !errors.Is(err, ErrInsufficientDarkMatter) {
		t.Fatalf("broke relocate = %v, want ErrInsufficientDarkMatter", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET dark_matter = 100000 WHERE id = $1`, owner); err != nil {
		t.Fatalf("restore dm: %v", err)
	}

	// Successful relocation: exact fee debited, coordinates moved, cooldown set.
	wantCost := game.RelocationCost(9, 400, 5, 9, 402, 5)
	var dmBefore int64
	if err := db.QueryRowContext(ctx, `SELECT dark_matter FROM users WHERE id = $1`, owner).Scan(&dmBefore); err != nil {
		t.Fatalf("read dm: %v", err)
	}
	cost, err := ps.RelocatePlanet(ctx, planet, owner, 9, 402, 5)
	if err != nil {
		t.Fatalf("relocate: %v", err)
	}
	if cost != wantCost {
		t.Fatalf("charged %d DM, want %d", cost, wantCost)
	}
	var dmAfter int64
	if err := db.QueryRowContext(ctx, `SELECT dark_matter FROM users WHERE id = $1`, owner).Scan(&dmAfter); err != nil {
		t.Fatalf("read dm after: %v", err)
	}
	if dmBefore-dmAfter != cost {
		t.Fatalf("dm changed by %d, want %d", dmBefore-dmAfter, cost)
	}

	var g, s, p int
	var nextAt, attackLocked sql.NullTime
	if err := db.QueryRowContext(ctx, `
		SELECT galaxy, system, position, relocation_next_at, attack_locked_until
		FROM celestial_objects WHERE id = $1
	`, planet).Scan(&g, &s, &p, &nextAt, &attackLocked); err != nil {
		t.Fatalf("read moved planet: %v", err)
	}
	if g != 9 || s != 402 || p != 5 {
		t.Fatalf("planet at (%d,%d,%d), want (9,402,5)", g, s, p)
	}
	if !nextAt.Valid {
		t.Fatal("relocation cooldown was not recorded")
	}
	if !attackLocked.Valid || !attackLocked.Time.After(time.Now()) {
		t.Fatalf("post-teleport attack lockout not set: %v", attackLocked)
	}

	// Relocating does NOT lock the origin slot (unlike abandonment).
	var originLocks int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM coordinate_locks
		WHERE universe_id=$1 AND galaxy=9 AND system=400 AND position=5
	`, universeID).Scan(&originLocks); err != nil {
		t.Fatalf("count origin locks: %v", err)
	}
	if originLocks != 0 {
		t.Fatalf("origin slot locked %d times, want 0", originLocks)
	}

	// Same-system teleports are exempt from the cooldown (the reference server
	// allows unlimited position moves). The cooldown timestamp must be untouched.
	sameSysCost, err := ps.RelocatePlanet(ctx, planet, owner, 9, 402, 6)
	if err != nil {
		t.Fatalf("same-system relocate during cooldown: %v", err)
	}
	if want := game.RelocationCost(9, 402, 5, 9, 402, 6); sameSysCost != want {
		t.Fatalf("same-system cost %d, want %d", sameSysCost, want)
	}
	var nextAtAfter sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT relocation_next_at FROM celestial_objects WHERE id = $1`, planet).Scan(&nextAtAfter); err != nil {
		t.Fatalf("read cooldown after same-system move: %v", err)
	}
	if !nextAtAfter.Valid || !nextAtAfter.Time.Equal(nextAt.Time) {
		t.Fatalf("same-system teleport changed the cooldown: %v -> %v", nextAt.Time, nextAtAfter.Time)
	}

	// An inter-system relocation within the cooldown is still refused.
	if _, err := ps.RelocatePlanet(ctx, planet, owner, 9, 403, 6); !errors.Is(err, ErrRelocationCooldown) {
		t.Fatalf("cooldown relocate = %v, want ErrRelocationCooldown", err)
	}

	// Once the cooldown expires the planet can move again.
	if _, err := db.ExecContext(ctx, `
		UPDATE celestial_objects SET relocation_next_at = NOW() - interval '1 second' WHERE id = $1
	`, planet); err != nil {
		t.Fatalf("expire cooldown: %v", err)
	}
	if _, err := ps.RelocatePlanet(ctx, planet, owner, 9, 403, 6); err != nil {
		t.Fatalf("relocate after cooldown: %v", err)
	}

	// The origin position is now free to be reused.
	if _, err := db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, occupied); err != nil {
		t.Fatalf("clear blocker: %v", err)
	}
	occupied = upsertPlanet("Reuser", other, 9, 400, 5)
}

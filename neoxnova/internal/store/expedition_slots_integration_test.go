package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"neoxnova/internal/models"
)

// TestDispatchExpeditionSlots verifies the Astrophysics-gated expedition slot
// budget and the deep-space target rule on the Dispatch path. Skipped unless
// DATABASE_URL is set.
//
//	$env:DATABASE_URL = "postgres://postgres:password@localhost:5432/neoxnova?sslmode=disable"
//	go test ./internal/store/ -run TestDispatchExpeditionSlots -v
func TestDispatchExpeditionSlots(t *testing.T) {
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
	var planetsPerSystem, slotBonus, slotCap int
	if err := db.QueryRowContext(ctx, `
		SELECT u.id::text, u.planets_per_system, u.expedition_slots_bonus, u.expedition_slots_cap
		FROM universes u
		JOIN celestial_objects c ON c.universe_id = u.id
		WHERE c.id = 1
	`).Scan(&universeID, &planetsPerSystem, &slotBonus, &slotCap); err != nil {
		t.Skipf("seed missing celestial 1 / universes columns: %v", err)
	}

	// Throwaway user + planet at an unused slot.
	var uid int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO users (universe_id, username, email, password_hash)
		VALUES ($1, 'expo_slots_test', 'expo_slots_test@example.com', 'x')
		ON CONFLICT (universe_id, username) DO UPDATE SET email = EXCLUDED.email
		RETURNING id
	`, universeID).Scan(&uid); err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM fleets WHERE user_id = $1`, uid)
	defer db.ExecContext(ctx, `DELETE FROM user_technologies WHERE user_id = $1`, uid)
	defer db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, uid)

	var pid int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO celestial_objects (universe_id, user_id, name, object_type, galaxy, system, position, deuterium)
		VALUES ($1, $2, 'Expo Slot Colony', 'PLANET', 1, 16, 16, 1e15)
		ON CONFLICT (universe_id, galaxy, system, position, object_type)
			DO UPDATE SET user_id = EXCLUDED.user_id, deuterium = EXCLUDED.deuterium
		RETURNING id
	`, universeID, uid).Scan(&pid); err != nil {
		t.Fatalf("upsert planet: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM planet_ships WHERE celestial_id = $1`, pid)
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, pid)

	// Astrophysics 4 => floor(4/2) = 2 base slots (+ universe bonus, capped).
	if _, err := db.ExecContext(ctx, `
		INSERT INTO user_technologies (user_id, tech_code, level) VALUES ($1, 'astrophysics', 4)
		ON CONFLICT (user_id, tech_code) DO UPDATE SET level = EXCLUDED.level
	`, uid); err != nil {
		t.Fatalf("set astrophysics: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_ships (celestial_id, ship_code, quantity) VALUES ($1, '202', 100000)
		ON CONFLICT (celestial_id, ship_code) DO UPDATE SET quantity = EXCLUDED.quantity
	`, pid); err != nil {
		t.Fatalf("seed ships: %v", err)
	}

	fs := NewFleetStore(db)
	deepPos := planetsPerSystem + 1
	base := models.FleetDispatchRequest{
		OriginPlanetID: pid,
		Mission:        models.MissionExpedition,
		Ships:          map[string]int64{"202": 1},
		SpeedPercent:   100,
		HoldingHours:   1,
	}

	// A non-deep-space position is rejected.
	bad := base
	bad.Target = models.Coordinates{Galaxy: 1, System: 16, Position: 1, Type: models.TypeDeepSpace}
	if _, err := fs.Dispatch(ctx, bad); !errors.Is(err, ErrInvalidExpeditionTarget) {
		t.Fatalf("expedition to position 1 = %v, want ErrInvalidExpeditionTarget", err)
	}
	// The deep-space position with the wrong target type is rejected too.
	badType := base
	badType.Target = models.Coordinates{Galaxy: 1, System: 16, Position: deepPos, Type: models.TypePlanet}
	if _, err := fs.Dispatch(ctx, badType); !errors.Is(err, ErrInvalidExpeditionTarget) {
		t.Fatalf("expedition to deep space as PLANET = %v, want ErrInvalidExpeditionTarget", err)
	}

	good := base
	good.Target = models.Coordinates{Galaxy: 1, System: 16, Position: deepPos, Type: models.TypeDeepSpace}

	want := 4/2 + slotBonus
	if slotCap > 0 && want > slotCap {
		want = slotCap
	}
	if want <= 0 {
		t.Skip("universe grants no expedition slots; nothing to exercise")
	}

	var fleets []int64
	for i := 0; i < want; i++ {
		res, err := fs.Dispatch(ctx, good)
		if err != nil {
			t.Fatalf("expedition %d/%d should dispatch: %v", i+1, want, err)
		}
		fleets = append(fleets, res.FleetID)
	}
	if _, err := fs.Dispatch(ctx, good); !errors.Is(err, ErrExpeditionSlotsFull) {
		t.Fatalf("expedition beyond %d slots = %v, want ErrExpeditionSlotsFull", want, err)
	}

	// Freeing a slot (here: resolving one fleet) allows a new send.
	if _, err := db.ExecContext(ctx, `UPDATE fleets SET phase = 'RESOLVED' WHERE id = $1`, fleets[0]); err != nil {
		t.Fatalf("free a slot: %v", err)
	}
	if _, err := fs.Dispatch(ctx, good); err != nil {
		t.Fatalf("dispatch after freeing a slot: %v", err)
	}
}

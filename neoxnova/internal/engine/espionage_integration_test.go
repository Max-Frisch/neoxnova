package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// TestResolveEspionageEndToEnd exercises the persistence layer of an ESPIONAGE
// resolution against a live Postgres. It covers both counter-espionage outcomes:
// a target with no ships (probes survive, fleet returns) and one with a large
// fleet (probes are destroyed, fleet is spent). Skipped unless DATABASE_URL is set.
func TestResolveEspionageEndToEnd(t *testing.T) {
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

	upsertUser := func(name string) int64 {
		var id int64
		if err := db.QueryRowContext(ctx, `
			INSERT INTO users (universe_id, username, email, password_hash)
			VALUES ($1, $2, $3, 'x')
			ON CONFLICT (universe_id, username) DO UPDATE SET email = EXCLUDED.email
			RETURNING id
		`, universeID, name, name+"@example.com").Scan(&id); err != nil {
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
				SET user_id = EXCLUDED.user_id, name = EXCLUDED.name
			RETURNING id
		`, universeID, owner, name, g, s, p).Scan(&id); err != nil {
			t.Fatalf("upsert planet %s: %v", name, err)
		}
		return id
	}
	setTech := func(user int64, level int) {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO user_technologies (user_id, tech_code, level) VALUES ($1, 'espionage_tech', $2)
			ON CONFLICT (user_id, tech_code) DO UPDATE SET level = EXCLUDED.level
		`, user, level); err != nil {
			t.Fatalf("set espionage tech: %v", err)
		}
	}

	atkUser := upsertUser("esp_test_atk")
	defUser := upsertUser("esp_test_def")
	origin := upsertPlanet("EspOrigin", atkUser, 9, 4, 4)
	target := upsertPlanet("EspTarget", defUser, 9, 5, 5)
	setTech(atkUser, 10)
	setTech(defUser, 10)
	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_structures (celestial_id, structure_code, level) VALUES ($1, 'metal_mine', 7)
		ON CONFLICT (celestial_id, structure_code) DO UPDATE SET level = 7
	`, target); err != nil {
		t.Fatalf("structure: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_defenses (celestial_id, defense_code, quantity) VALUES ($1, '401', 5)
		ON CONFLICT (celestial_id, defense_code) DO UPDATE SET quantity = 5
	`, target); err != nil {
		t.Fatalf("defense: %v", err)
	}

	// No ships on the target yet: probes survive.
	if _, err := db.ExecContext(ctx, `DELETE FROM planet_ships WHERE celestial_id = $1`, target); err != nil {
		t.Fatalf("clear ships: %v", err)
	}

	var f1, f2 int64
	insertSpyFleet := func() int64 {
		var id int64
		if err := db.QueryRowContext(ctx, `
			INSERT INTO fleets (
				universe_id, user_id, mission, phase, origin_id, target_id,
				origin_galaxy, origin_system, origin_position, origin_type,
				target_galaxy, target_system, target_position, target_type,
				start_time, arrival_time, return_time, flight_speed_pct, deuterium_consumption,
				cargo_metal, cargo_crystal, cargo_deuterium
			) VALUES (
				$1, $2, 'ESPIONAGE', 'OUTBOUND', $3, $4,
				9,4,4,'PLANET', 9,5,5,'PLANET',
				NOW() - interval '10 minutes', NOW() - interval '1 minute', NOW() + interval '5 minutes',
				1.00, 0, 0, 0, 0
			) RETURNING id
		`, universeID, atkUser, origin, target).Scan(&id); err != nil {
			t.Fatalf("insert spy fleet: %v", err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO fleet_ships (fleet_id, ship_code, count) VALUES ($1, '210', 2)`, id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	f1 = insertSpyFleet()
	f2 = insertSpyFleet()
	defer func() {
		db.ExecContext(ctx, `DELETE FROM fleets WHERE id IN ($1, $2)`, f1, f2)
		db.ExecContext(ctx, `DELETE FROM espionage_reports WHERE fleet_id IN ($1, $2)`, f1, f2)
		db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id IN ($1, $2)`, origin, target)
		db.ExecContext(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, atkUser, defUser)
	}()

	eng := NewEventEngine(nil, db)

	// --- Scenario A: no defender ships -> probes survive and fly home. ---
	eng.resolveFleetEvent(ctx, f1)

	var probesSent, probesLost int
	var report1 json.RawMessage
	if err := db.QueryRowContext(ctx, `
		SELECT probes_sent, probes_lost, report FROM espionage_reports WHERE fleet_id = $1
	`, f1).Scan(&probesSent, &probesLost, &report1); err != nil {
		t.Fatalf("expected a persisted espionage report: %v", err)
	}
	if probesSent != 2 || probesLost != 0 {
		t.Fatalf("scenario A probes sent/lost = %d/%d, want 2/0", probesSent, probesLost)
	}
	intel := map[string]json.RawMessage{}
	if err := json.Unmarshal(report1, &intel); err != nil {
		t.Fatalf("report payload not JSON object: %v", err)
	}
	for _, key := range []string{"metal", "fleet", "defense", "buildings", "research"} {
		if _, ok := intel[key]; !ok {
			t.Fatalf("report missing %q section: %s", key, report1)
		}
	}
	var phaseA string
	if err := db.QueryRowContext(ctx, `SELECT phase FROM fleets WHERE id = $1`, f1).Scan(&phaseA); err != nil {
		t.Fatal(err)
	}
	if phaseA != "RETURNING" {
		t.Fatalf("scenario A fleet phase = %q, want RETURNING", phaseA)
	}

	// --- Scenario B: large defender fleet -> counter-espionage kills the probes. ---
	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_ships (celestial_id, ship_code, quantity) VALUES ($1, '204', 5000)
		ON CONFLICT (celestial_id, ship_code) DO UPDATE SET quantity = 5000
	`, target); err != nil {
		t.Fatalf("defender ships: %v", err)
	}
	eng.resolveFleetEvent(ctx, f2)

	if err := db.QueryRowContext(ctx, `
		SELECT probes_sent, probes_lost FROM espionage_reports WHERE fleet_id = $1
	`, f2).Scan(&probesSent, &probesLost); err != nil {
		t.Fatalf("expected a persisted espionage report for scenario B: %v", err)
	}
	if probesSent != 2 || probesLost != 2 {
		t.Fatalf("scenario B probes sent/lost = %d/%d, want 2/2", probesSent, probesLost)
	}
	var phaseB string
	if err := db.QueryRowContext(ctx, `SELECT phase FROM fleets WHERE id = $1`, f2).Scan(&phaseB); err != nil {
		t.Fatal(err)
	}
	if phaseB != "RESOLVED" {
		t.Fatalf("scenario B fleet phase = %q, want RESOLVED (all probes lost)", phaseB)
	}
	t.Logf("espionage resolved: A survived (RETURNING), B probes destroyed (RESOLVED); full report persisted")
}

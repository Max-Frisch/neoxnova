package engine

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"neoxnova/internal/game"

	_ "github.com/lib/pq"
)

// expeditionFleet inserts a due HOLDING EXPEDITION at 1:9:21 for user 2 and
// returns its id. Ships are 100 Frigate + 5 Battle Recycler.
func expeditionFleet(t *testing.T, db *sql.DB, universeID string) int64 {
	t.Helper()
	ctx := context.Background()
	var fleetID int64
	err := db.QueryRowContext(ctx, `
		INSERT INTO fleets (
			universe_id, user_id, mission, phase, origin_id, target_id,
			origin_galaxy, origin_system, origin_position, origin_type,
			target_galaxy, target_system, target_position, target_type,
			start_time, arrival_time, holding_end_time, return_time,
			flight_speed_pct, deuterium_consumption,
			cargo_metal, cargo_crystal, cargo_deuterium
		) VALUES (
			$1, 2, 'EXPEDITION', 'HOLDING', 1, NULL,
			1,1,1,'PLANET', 1,9,21,'DEEP_SPACE',
			NOW() - interval '1 hour', NOW() - interval '5 minutes', NOW() - interval '1 minute', NOW() + interval '5 minutes',
			1.00, 0, 0, 0, 0
		) RETURNING id
	`, universeID).Scan(&fleetID)
	if err != nil {
		t.Fatalf("insert expedition fleet: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO fleet_ships (fleet_id, ship_code, count) VALUES ($1,'227',100),($1,'219',5)`, fleetID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.ExecContext(ctx, `DELETE FROM fleet_ships WHERE fleet_id = $1`, fleetID)
		db.ExecContext(ctx, `DELETE FROM fleets WHERE id = $1`, fleetID)
		db.ExecContext(ctx, `DELETE FROM combat_reports WHERE fleet_id = $1`, fleetID)
		db.ExecContext(ctx, `DELETE FROM expedition_reports WHERE fleet_id = $1`, fleetID)
		db.ExecContext(ctx, `DELETE FROM upgrade_items WHERE account_id = 2 AND upgrade_code IN (5,11)`)
	})
	return fleetID
}

func withExpeditionRoll(t *testing.T, res game.ExpeditionResult) {
	t.Helper()
	prev := rollExpedition
	rollExpedition = func(game.Combatant, int64) game.ExpeditionResult { return res }
	t.Cleanup(func() { rollExpedition = prev })
}

// TestResolveExpeditionPersistence exercises every persistence branch of the
// expedition resolver with a forced outcome (the pure roll is tested in
// internal/game). Requires DATABASE_URL like the other engine integration tests.
func TestResolveExpeditionPersistence(t *testing.T) {
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
	// Account 2 owns the forced drops; clear any prior test residue.
	db.ExecContext(ctx, `DELETE FROM upgrade_items WHERE account_id = 2 AND upgrade_code IN (5,11)`)
	db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE object_type='DEBRIS_FIELD' AND galaxy=1 AND system=9 AND position=21`)

	eng := NewEventEngine(nil, db)

	t.Run("resources+drop+darkmatter", func(t *testing.T) {
		withExpeditionRoll(t, game.ExpeditionResult{
			Outcome:     game.ExpeditionResources,
			Loot:        game.Cost{Metal: 1000, Crystal: 500, Deuterium: 100},
			DarkMatter:  500,
			UpgradeCode: 11,
		})
		fleetID := expeditionFleet(t, db, universeID)
		eng.resolveFleetEvent(ctx, fleetID)

		var phase string
		var m, c, d int64
		if err := db.QueryRowContext(ctx, `SELECT phase, cargo_metal, cargo_crystal, cargo_deuterium FROM fleets WHERE id=$1`, fleetID).Scan(&phase, &m, &c, &d); err != nil {
			t.Fatal(err)
		}
		if phase != "RETURNING" || m != 1000 || c != 500 || d != 100 {
			t.Fatalf("fleet = phase %q cargo M%d C%d D%d, want RETURNING 1000/500/100", phase, m, c, d)
		}
		var dm int64
		if err := db.QueryRowContext(ctx, `SELECT dark_matter FROM users WHERE id=2`).Scan(&dm); err != nil {
			t.Fatal(err)
		}
		if dm < 500 {
			t.Fatalf("dark_matter = %d, want >= 500", dm)
		}
		var qty int
		err := db.QueryRowContext(ctx, `SELECT qty FROM upgrade_items WHERE account_id=2 AND upgrade_code=11`).Scan(&qty)
		if err != nil || qty < 1 {
			t.Fatalf("expected an upgrade_item 11 drawing, got qty=%d err=%v", qty, err)
		}
		var outcome, title, msg string
		if err := db.QueryRowContext(ctx, `SELECT outcome, title, message FROM expedition_reports WHERE fleet_id=$1`, fleetID).Scan(&outcome, &title, &msg); err != nil {
			t.Fatalf("expected an expedition report: %v", err)
		}
		if outcome != "resources" || title == "" || msg == "" {
			t.Fatalf("expedition report = %q/%q/%q, want resources with text", outcome, title, msg)
		}
	})

	t.Run("combat win", func(t *testing.T) {
		withExpeditionRoll(t, game.ExpeditionResult{
			Outcome: game.ExpeditionCombat,
			NPC:     game.NPCPirates,
			Combat: &game.CombatResult{
				Winner: "attacker", Rounds: 3,
				Attacker:    game.SideReport{Remaining: map[string]int64{"227": 90}},
				DebrisMetal: 1234, DebrisCrystal: 567,
			},
			UpgradeCode: 5,
		})
		fleetID := expeditionFleet(t, db, universeID)
		eng.resolveFleetEvent(ctx, fleetID)

		var phase string
		if err := db.QueryRowContext(ctx, `SELECT phase FROM fleets WHERE id=$1`, fleetID).Scan(&phase); err != nil {
			t.Fatal(err)
		}
		if phase != "RETURNING" {
			t.Fatalf("phase = %q, want RETURNING", phase)
		}
		var left int64
		if err := db.QueryRowContext(ctx, `SELECT COALESCE(SUM(count),0) FROM fleet_ships WHERE fleet_id=$1`, fleetID).Scan(&left); err != nil {
			t.Fatal(err)
		}
		if left != 90 {
			t.Fatalf("surviving ships = %d, want 90", left)
		}
		var reports int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM combat_reports WHERE fleet_id=$1`, fleetID).Scan(&reports); err != nil {
			t.Fatal(err)
		}
		if reports != 1 {
			t.Fatalf("combat_reports = %d, want 1", reports)
		}
		var outcome, npc string
		var linked sql.NullInt64
		if err := db.QueryRowContext(ctx, `SELECT outcome, npc, combat_report_id FROM expedition_reports WHERE fleet_id=$1`, fleetID).Scan(&outcome, &npc, &linked); err != nil {
			t.Fatalf("expected a combat expedition report: %v", err)
		}
		if outcome != "combat" || npc != "pirates" || !linked.Valid {
			t.Fatalf("expedition report = %q npc=%q linked=%v, want combat/pirates/linked", outcome, npc, linked.Valid)
		}
		var debris int
		if err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM celestial_objects
			WHERE object_type='DEBRIS_FIELD' AND galaxy=1 AND system=9 AND position=21
		`).Scan(&debris); err != nil {
			t.Fatal(err)
		}
		if debris != 1 {
			t.Fatalf("deep-space debris field = %d, want 1", debris)
		}
		db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE object_type='DEBRIS_FIELD' AND galaxy=1 AND system=9 AND position=21`)
	})

	t.Run("black hole", func(t *testing.T) {
		withExpeditionRoll(t, game.ExpeditionResult{Outcome: game.ExpeditionBlackHole})
		fleetID := expeditionFleet(t, db, universeID)
		eng.resolveFleetEvent(ctx, fleetID)

		var phase string
		if err := db.QueryRowContext(ctx, `SELECT phase FROM fleets WHERE id=$1`, fleetID).Scan(&phase); err != nil {
			t.Fatal(err)
		}
		if phase != "RESOLVED" {
			t.Fatalf("phase = %q, want RESOLVED", phase)
		}
		var ships int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fleet_ships WHERE fleet_id=$1`, fleetID).Scan(&ships); err != nil {
			t.Fatal(err)
		}
		if ships != 0 {
			t.Fatalf("black-holed fleet still has %d ship rows", ships)
		}
		var outcome string
		if err := db.QueryRowContext(ctx, `SELECT outcome FROM expedition_reports WHERE fleet_id=$1`, fleetID).Scan(&outcome); err != nil {
			t.Fatalf("expected a black-hole expedition report: %v", err)
		}
		if outcome != "blackhole" {
			t.Fatalf("expedition report outcome = %q, want blackhole", outcome)
		}
	})
}

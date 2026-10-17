package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"neoxnova/internal/auth"
	"neoxnova/internal/models"
)

// TestEmpireOverviewIntegration exercises the Empire matrix read against a live
// Postgres: creation order, the homeworld flag, accrued stock, cached hourly
// production (incl. the moon Collider's Dark Matter/h) and the grouped
// building/ship/defense matrices. Skipped unless DATABASE_URL is set.
func TestEmpireOverviewIntegration(t *testing.T) {
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
	var codeName, universeID string
	if err := db.QueryRowContext(ctx, `SELECT code_name, id::text FROM universes LIMIT 1`).Scan(&codeName, &universeID); err != nil {
		t.Skipf("no universe seeded: %v", err)
	}

	as := NewAuthStore(db)
	uname := fmt.Sprintf("empiretest_%d", time.Now().UnixNano())
	hash, err := auth.HashPassword("s3cret-password!")
	if err != nil {
		t.Fatal(err)
	}
	userID, homeID, err := as.CreateUser(ctx, codeName, uname, uname+"@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)

	// A second planet and a moon, both owned (created after the homeworld, so
	// they must sort after it in id order).
	var planet2, moonID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO celestial_objects
			(universe_id, user_id, name, object_type, galaxy, system, position,
			 diameter_km, fields_used, fields_max, base_fields_max, temp_min, temp_max)
		VALUES ($1, $2, 'Empire Colony', 'PLANET', 4, 40, 4, 12000, 0, 200, 200, -10, 30)
		RETURNING id
	`, universeID, userID).Scan(&planet2); err != nil {
		t.Fatalf("insert planet: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
		INSERT INTO celestial_objects
			(universe_id, user_id, name, object_type, galaxy, system, position,
			 diameter_km, fields_used, fields_max, base_fields_max, temp_min, temp_max)
		VALUES ($1, $2, 'Empire Moon', 'MOON', 4, 40, 5, 8426, 0, 3, 1, -20, 40)
		RETURNING id
	`, universeID, userID).Scan(&moonID); err != nil {
		t.Fatalf("insert moon: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id IN ($1, $2)`, planet2, moonID)

	// Planet 2: a mine, cargo and a defense unit; then recompute so the cached
	// production/fields columns are populated like the scheduler would.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_structures (celestial_id, structure_code, level) VALUES ($1, 'metal_mine', 5)
	`, planet2); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_ships (celestial_id, ship_code, quantity) VALUES ($1, '202', 10)
	`, planet2); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO planet_defenses (celestial_id, defense_code, quantity) VALUES ($1, '401', 3)
	`, planet2); err != nil {
		t.Fatal(err)
	}
	bs := NewBuildStore(db)
	withTx(t, db, func(tx *sql.Tx) error { return bs.RecomputeCelestial(ctx, tx, planet2) })

	// Moon: collider L1 => 4752 DM/h, mirrored into both caches.
	if _, err := db.ExecContext(ctx, `
		UPDATE celestial_objects SET collider_level = 1, collider_dm_prod_hourly = 4752 WHERE id = $1
	`, moonID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE users SET dark_matter_prod_hourly = 4752 WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}

	planets := NewPlanetStore(db)
	rows, err := planets.EmpireOverview(ctx, userID)
	if err != nil {
		t.Fatalf("EmpireOverview: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("celestial count = %d, want 3", len(rows))
	}
	// Creation order = id order, homeworld first.
	if rows[0].ID != homeID || rows[1].ID != planet2 || rows[2].ID != moonID {
		t.Fatalf("order = [%d %d %d], want [%d %d %d]", rows[0].ID, rows[1].ID, rows[2].ID, homeID, planet2, moonID)
	}
	var homes int
	for _, r := range rows {
		if r.IsHome {
			homes++
			if r.ID != homeID {
				t.Fatalf("home flag on %d, want %d", r.ID, homeID)
			}
		}
	}
	if homes != 1 {
		t.Fatalf("home flags = %d, want 1", homes)
	}

	p2 := rows[1]
	if p2.Type != models.TypePlanet || p2.Buildings["metal_mine"] != 5 || p2.Ships["202"] != 10 || p2.Defenses["401"] != 3 {
		t.Fatalf("planet2 matrices = %+v", p2)
	}
	if p2.FieldsMax != 200 || p2.MetalPerHour <= 0 {
		t.Fatalf("planet2 fields/production = %+v", p2)
	}

	moon := rows[2]
	if moon.Type != models.TypeMoon || moon.DarkMatterPerHour != 4752 {
		t.Fatalf("moon = %+v, want MOON with 4752 DM/h", moon)
	}
}

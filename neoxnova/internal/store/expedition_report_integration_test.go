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
)

// TestExpeditionReportListToleratesNullShips is a regression test for the
// Messages inbox: expedition reports store a nullable `ships` JSONB, and a NULL
// must not fail the scan (which silently emptied the whole expedition section
// of the reports page). Non-combat outcomes recover no ships and write NULL.
func TestExpeditionReportListToleratesNullShips(t *testing.T) {
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

	var universeID, codeName string
	if err := db.QueryRowContext(ctx, `SELECT id::text, code_name FROM universes LIMIT 1`).Scan(&universeID, &codeName); err != nil {
		t.Skipf("no universe seeded: %v", err)
	}

	as := NewAuthStore(db)
	uname := fmt.Sprintf("exprep_%d", time.Now().UnixNano())
	hash, err := auth.HashPassword("s3cret-password!")
	if err != nil {
		t.Fatal(err)
	}
	userID, planetID, err := as.CreateUser(ctx, codeName, uname, uname+"@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		INSERT INTO expedition_reports
			(universe_id, user_id, target_galaxy, target_system, target_position, outcome, ships, title, message)
		VALUES
			($1, $2, 1, 1, 21, 'nothing', NULL, 'Expedition: nothing', 'nothing found'),
			($1, $2, 1, 1, 21, 'ships', '{"201": 5}'::jsonb, 'Expedition: ships found', 'found ships')
	`, universeID, userID); err != nil {
		t.Fatalf("insert reports: %v", err)
	}

	reps, err := NewExpeditionReportStore(db).ListForCelestial(ctx, planetID, 10)
	if err != nil {
		t.Fatalf("list reports: %v", err)
	}
	if len(reps) != 2 {
		t.Fatalf("got %d reports, want 2 (a NULL ships row must not drop the list)", len(reps))
	}
	var sawNull bool
	for _, r := range reps {
		if r.Outcome == "nothing" && r.Ships == nil {
			sawNull = true
		}
	}
	if !sawNull {
		t.Fatalf("expected the NULL-ships report to survive the scan")
	}
}

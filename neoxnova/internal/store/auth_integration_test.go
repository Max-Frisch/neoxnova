package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"neoxnova/internal/auth"
)

// TestAuthSessionLifecycle exercises register/auth/session against a live DB.
// Skipped unless DATABASE_URL is set.
func TestAuthSessionLifecycle(t *testing.T) {
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

	var codeName string
	if err := db.QueryRowContext(ctx, `SELECT code_name FROM universes LIMIT 1`).Scan(&codeName); err != nil {
		t.Skipf("no universe seeded: %v", err)
	}

	as := NewAuthStore(db)
	uname := fmt.Sprintf("authtest_%d", time.Now().UnixNano())
	email := uname + "@example.com"
	hash, err := auth.HashPassword("s3cret-password!")
	if err != nil {
		t.Fatal(err)
	}
	userID, planetID, err := as.CreateUser(ctx, codeName, uname, email, hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM celestial_objects WHERE id = $1`, planetID)
	defer db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)

	// A fresh account must have been granted one homeworld in the 6..16 position
	// band with the 700-field / starting-resource loadout.
	var (
		g, s, p         int
		fieldsMax       int
		metal, crystal  float64
		metalProdHourly float64
		objectType      string
		planetOwner     int64
	)
	if err := db.QueryRowContext(ctx, `
		SELECT galaxy, system, position, fields_max, metal::float8, crystal::float8,
		       metal_prod_hourly::float8, object_type, user_id
		FROM celestial_objects WHERE id = $1
	`, planetID).Scan(&g, &s, &p, &fieldsMax, &metal, &crystal, &metalProdHourly, &objectType, &planetOwner); err != nil {
		t.Fatalf("load homeworld: %v", err)
	}
	if objectType != "PLANET" || planetOwner != userID {
		t.Fatalf("homeworld type/owner = %s/%d, want PLANET/%d", objectType, planetOwner, userID)
	}
	if p < homeworldMinPosition || p > homeworldMaxPosition {
		t.Fatalf("homeworld position = %d, want %d..%d", p, homeworldMinPosition, homeworldMaxPosition)
	}
	if fieldsMax != homeworldFields {
		t.Fatalf("homeworld fields_max = %d, want %d", fieldsMax, homeworldFields)
	}
	if metal != homeworldStartMetal || crystal != homeworldStartCrystal {
		t.Fatalf("homeworld start resources = M%v C%v, want M%d C%d", metal, crystal, homeworldStartMetal, homeworldStartCrystal)
	}
	if metalProdHourly <= 0 {
		t.Fatalf("homeworld metal_prod_hourly = %v, want > 0", metalProdHourly)
	}

	// Duplicate registration must be rejected.
	if _, _, err := as.CreateUser(ctx, codeName, uname, email, hash); !errors.Is(err, ErrUserExists) {
		t.Fatalf("expected ErrUserExists, got %v", err)
	}

	// Login lookup + password verify.
	gotID, gotHash, err := as.UserAuth(ctx, uname)
	if err != nil || gotID != userID {
		t.Fatalf("UserAuth = (%d,%v), want %d", gotID, err, userID)
	}
	if !auth.VerifyPassword("s3cret-password!", gotHash) {
		t.Fatal("stored hash does not verify")
	}
	if _, _, err := as.UserAuth(ctx, "nobody_"+uname); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown user should be ErrInvalidCredentials, got %v", err)
	}

	// Session lifecycle.
	raw, th, _ := auth.NewSessionToken()
	if err := as.CreateSession(ctx, userID, th, time.Now().Add(time.Hour), "unit-test", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	sid, err := as.SessionUser(ctx, th)
	if err != nil || sid != userID {
		t.Fatalf("SessionUser = (%d,%v), want %d", sid, err, userID)
	}
	if err := as.DeleteSession(ctx, th); err != nil {
		t.Fatal(err)
	}
	if _, err := as.SessionUser(ctx, th); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted session should be ErrNotFound, got %v", err)
	}
	_ = raw
}

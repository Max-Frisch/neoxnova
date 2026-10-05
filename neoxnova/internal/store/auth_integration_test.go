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
	userID, err := as.CreateUser(ctx, codeName, uname, email, hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)

	// Duplicate registration must be rejected.
	if _, err := as.CreateUser(ctx, codeName, uname, email, hash); !errors.Is(err, ErrUserExists) {
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

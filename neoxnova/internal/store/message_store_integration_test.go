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
	"neoxnova/internal/game"
)

// TestMessageStoreLifecycle covers the generic inbox: the welcome message is
// written at registration, insert/list/count round-trip and category filtering.
func TestMessageStoreLifecycle(t *testing.T) {
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
	if err := db.QueryRowContext(ctx, `SELECT id::text, code_name FROM universes LIMIT 1`).Scan(&universeID, &codeName); err != nil {
		t.Skipf("no universe seeded: %v", err)
	}

	as := NewAuthStore(db)
	uname := fmt.Sprintf("msgtest_%d", time.Now().UnixNano())
	hash, err := auth.HashPassword("s3cret-password!")
	if err != nil {
		t.Fatal(err)
	}
	userID, _, err := as.CreateUser(ctx, codeName, uname, uname+"@example.com", hash)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	ms := NewMessageStore(db)
	// The registration welcome message is written in the same tx.
	counts, err := ms.CountsForUser(ctx, userID)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if counts[game.MessageCategoryPlayer] != 1 {
		t.Fatalf("welcome count = %d, want 1", counts[game.MessageCategoryPlayer])
	}
	player, err := ms.ListForUser(ctx, userID, game.MessageCategoryPlayer, 10)
	if err != nil || len(player) != 1 {
		t.Fatalf("player messages = %d (err %v), want 1", len(player), err)
	}
	if player[0].Subject != "Welcome" || player[0].Body == "" {
		t.Errorf("welcome row = %+v", player[0])
	}

	// A transport message inserts and lists under its category only.
	sender, subject, body := game.TransportDeliveryMessage(1, 2, 3, 100, 200, 300)
	if err := ms.Insert(ctx, MessageInput{
		UniverseID: universeID, UserID: userID,
		Category: game.MessageCategoryTransport, Class: "transport",
		Sender: sender, Subject: subject, Body: body,
		RefKind: "fleet", RefID: 42,
	}); err != nil {
		t.Fatalf("insert transport: %v", err)
	}
	transport, err := ms.ListForUser(ctx, userID, game.MessageCategoryTransport, 10)
	if err != nil || len(transport) != 1 {
		t.Fatalf("transport messages = %d (err %v), want 1", len(transport), err)
	}
	if transport[0].RefKind.String != "fleet" || transport[0].RefID.Int64 != 42 {
		t.Errorf("transport ref = %s/%d", transport[0].RefKind.String, transport[0].RefID.Int64)
	}

	all, err := ms.ListForUser(ctx, userID, -1, 10)
	if err != nil || len(all) != 2 {
		t.Fatalf("all messages = %d (err %v), want 2", len(all), err)
	}
	counts, _ = ms.CountsForUser(ctx, userID)
	if counts[game.MessageCategoryTransport] != 1 || counts[game.MessageCategoryPlayer] != 1 {
		t.Errorf("counts = %+v", counts)
	}
}

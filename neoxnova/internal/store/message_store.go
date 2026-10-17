package store

import (
	"context"
	"database/sql"
	"strconv"
	"time"
)

// MessageStore persists and serves the generic (non-report) inbox categories:
// player, alliance, system, transport, game and construction. Spy / combat /
// expedition keep their own structured report tables.
type MessageStore struct {
	db *sql.DB
}

func NewMessageStore(db *sql.DB) *MessageStore {
	return &MessageStore{db: db}
}

// Message is one inbox row. RefKind/RefID optionally point at the engine entity
// (e.g. a fleet) the message was generated for.
type Message struct {
	ID        int64          `json:"id"`
	UserID    int64          `json:"user_id"`
	Category  int            `json:"category"`
	Class     string         `json:"class"`
	Sender    string         `json:"sender"`
	Subject   string         `json:"subject"`
	Body      string         `json:"body"`
	RefKind   sql.NullString `json:"ref_kind"`
	RefID     sql.NullInt64  `json:"ref_id"`
	ReadAt    sql.NullTime   `json:"read_at"`
	CreatedAt time.Time      `json:"created_at"`
}

// MessageInput is the payload for inserting one inbox message. UniverseID is
// the universe UUID (not its code name).
type MessageInput struct {
	UniverseID string
	UserID     int64
	Category   int
	Class      string
	Sender     string
	Subject    string
	Body       string
	RefKind    string
	RefID      int64
}

// messageExecer is satisfied by both *sql.DB and *sql.Tx, so messages can be
// inserted standalone or inside an existing transaction.
type messageExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// InsertMessage writes one inbox message using the given executor. Callers that
// already hold a transaction (the engine, registration) pass their *sql.Tx so
// the message commits atomically with the event it reports.
func InsertMessage(ctx context.Context, q messageExecer, in MessageInput) error {
	var refKind any
	if in.RefKind != "" {
		refKind = in.RefKind
	}
	var refID any
	if in.RefID != 0 {
		refID = in.RefID
	}
	_, err := q.ExecContext(ctx, `
		INSERT INTO messages (universe_id, user_id, category, class, sender, subject, body, ref_kind, ref_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, in.UniverseID, in.UserID, in.Category, in.Class, in.Sender, in.Subject, in.Body, refKind, refID)
	return err
}

// Insert stores one message. UniverseID must be the universe UUID.
func (s *MessageStore) Insert(ctx context.Context, in MessageInput) error {
	return InsertMessage(ctx, s.db, in)
}

const messageColumns = `id, user_id, category, class, sender, subject, body, ref_kind, ref_id, read_at, created_at`

func scanMessage(row interface{ Scan(...any) error }) (Message, error) {
	var m Message
	err := row.Scan(&m.ID, &m.UserID, &m.Category, &m.Class, &m.Sender, &m.Subject,
		&m.Body, &m.RefKind, &m.RefID, &m.ReadAt, &m.CreatedAt)
	return m, err
}

// ListForUser returns the newest messages of one user, optionally filtered to a
// single category (pass category < 0 for all).
func (s *MessageStore) ListForUser(ctx context.Context, userID int64, category, limit int) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := `SELECT ` + messageColumns + ` FROM messages WHERE user_id = $1`
	args := []any{userID}
	if category >= 0 {
		query += ` AND category = $2`
		args = append(args, category)
	}
	query += ` ORDER BY id DESC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CountsForUser returns the number of messages per category for a user.
func (s *MessageStore) CountsForUser(ctx context.Context, userID int64) (map[int]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT category, COUNT(*) FROM messages WHERE user_id = $1 GROUP BY category`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var cat, n int
		if err := rows.Scan(&cat, &n); err != nil {
			return nil, err
		}
		out[cat] = n
	}
	return out, rows.Err()
}

package auth

import "context"

type ctxKey int

const userIDKey ctxKey = iota

// WithUserID returns a context carrying the authenticated user id.
func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserID extracts the authenticated user id, if any.
func UserID(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey).(int64)
	return id, ok
}

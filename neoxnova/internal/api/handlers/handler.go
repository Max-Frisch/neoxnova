package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/redis/go-redis/v9"

	"neoxnova/internal/cache"
	"neoxnova/internal/store"
)

type Handler struct {
	DB         *sql.DB
	Redis      *redis.Client
	UniverseID string
	Planets    *store.PlanetStore
	Fleets     *store.FleetStore
	Builds     *store.BuildStore
}

func New(db *sql.DB, rdb *redis.Client, universeID string) *Handler {
	return &Handler{
		DB:         db,
		Redis:      rdb,
		UniverseID: universeID,
		Planets:    store.NewPlanetStore(db),
		Fleets:     store.NewFleetStore(db),
		Builds:     store.NewBuildStore(db),
	}
}

// notifyScheduler gives the scheduler a low-latency nudge. It is best-effort;
// the scheduler polls Postgres regardless, so a Redis failure is harmless.
func (h *Handler) notifyScheduler(ctx context.Context) {
	if h.Redis == nil {
		return
	}
	key := cache.WakeKey(h.UniverseID)
	if err := h.Redis.LPush(ctx, key, "1").Err(); err == nil {
		h.Redis.LTrim(ctx, key, 0, 0)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

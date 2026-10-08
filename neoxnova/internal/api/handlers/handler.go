package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"

	"github.com/redis/go-redis/v9"

	"neoxnova/internal/auth"
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
	Combat     *store.CombatReportStore
	Espionage  *store.EspionageReportStore
	Expedition *store.ExpeditionReportStore
	Blueprints *store.BlueprintStore
	Arsenal    *store.ArsenalStore
	Auth       *store.AuthStore
	loginGuard *lockout
}

func New(db *sql.DB, rdb *redis.Client, universeID string) *Handler {
	return &Handler{
		DB:         db,
		Redis:      rdb,
		UniverseID: universeID,
		Planets:    store.NewPlanetStore(db),
		Fleets:     store.NewFleetStore(db),
		Builds:     store.NewBuildStore(db),
		Combat:     store.NewCombatReportStore(db),
		Espionage:  store.NewEspionageReportStore(db),
		Expedition: store.NewExpeditionReportStore(db),
		Blueprints: store.NewBlueprintStore(db),
		Arsenal:    store.NewArsenalStore(db),
		Auth:       store.NewAuthStore(db),
		loginGuard: newLockout(),
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

// secureCookies enables the Secure cookie flag outside local development
// (which is served over plain HTTP).
func (h *Handler) secureCookies() bool {
	return os.Getenv("APP_ENV") != "development"
}

// RequireAuth rejects unauthenticated requests and injects the user id into the
// request context for downstream handlers.
func (h *Handler) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := auth.TokenFromRequest(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		userID, err := h.Auth.SessionUser(r.Context(), auth.HashToken(token))
		if err != nil {
			auth.ClearSessionCookie(w, h.secureCookies())
			writeError(w, http.StatusUnauthorized, "invalid or expired session")
			return
		}
		next(w, r.WithContext(auth.WithUserID(r.Context(), userID)))
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

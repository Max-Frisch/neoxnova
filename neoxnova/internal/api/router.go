package api

import (
	"database/sql"
	"net/http"
	"os"

	"github.com/redis/go-redis/v9"

	"neoxnova/internal/api/handlers"
)

func NewRouter(db *sql.DB, rdb *redis.Client, universeID string) http.Handler {
	h := handlers.New(db, rdb, universeID)

	// Per-IP token buckets: a strict one for auth (brute force / bulk signup)
	// and a generous global one for everything else.
	authLimit := newIPLimiter(0.25, 5) // ~1 request / 4s, burst 5
	globalLimit := newIPLimiter(40, 80)

	mux := http.NewServeMux()
	// Public: health + authentication.
	mux.HandleFunc("GET /api/v1/health", h.Health)
	mux.Handle("POST /api/v1/auth/register", authLimit.Middleware(http.HandlerFunc(h.Register)))
	mux.Handle("POST /api/v1/auth/login", authLimit.Middleware(http.HandlerFunc(h.Login)))
	mux.HandleFunc("POST /api/v1/auth/logout", h.Logout)
	mux.HandleFunc("GET /api/v1/auth/me", h.RequireAuth(h.Me))
	// Authenticated game routes.
	mux.HandleFunc("GET /api/v1/planets/{id}/resources", h.RequireAuth(h.PlanetResources))
	mux.HandleFunc("GET /api/v1/planets/{id}/overview", h.RequireAuth(h.PlanetOverview))
	mux.HandleFunc("GET /api/v1/planets/{id}/buildings", h.RequireAuth(h.PlanetBuildings))
	mux.HandleFunc("POST /api/v1/planets/{id}/build", h.RequireAuth(h.StructureBuild))
	mux.HandleFunc("POST /api/v1/planets/{id}/shipyard", h.RequireAuth(h.ShipyardBuild))
	mux.HandleFunc("POST /api/v1/planets/{id}/research", h.RequireAuth(h.ResearchStart))
	mux.HandleFunc("POST /api/v1/planets/{id}/abandon", h.RequireAuth(h.PlanetAbandon))
	mux.HandleFunc("POST /api/v1/fleets/dispatch", h.RequireAuth(h.FleetDispatch))
	mux.HandleFunc("POST /api/v1/fleets/{id}/recall", h.RequireAuth(h.FleetRecall))
	mux.HandleFunc("GET /api/v1/combat/reports/{id}", h.RequireAuth(h.CombatReport))
	mux.HandleFunc("GET /api/v1/planets/{id}/combat-reports", h.RequireAuth(h.PlanetCombatReports))
	mux.HandleFunc("GET /dashboard/{id}", h.RequireAuth(h.Dashboard))

	secure := os.Getenv("APP_ENV") != "development"
	return Logging(Recovery(SecurityHeaders(secure, globalLimit.Middleware(mux))))
}

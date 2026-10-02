package api

import (
	"database/sql"
	"net/http"

	"github.com/redis/go-redis/v9"

	"neoxnova/internal/api/handlers"
)

func NewRouter(db *sql.DB, rdb *redis.Client, universeID string) http.Handler {
	h := handlers.New(db, rdb, universeID)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", h.Health)
	mux.HandleFunc("GET /api/v1/planets/{id}/resources", h.PlanetResources)
	mux.HandleFunc("GET /api/v1/planets/{id}/overview", h.PlanetOverview)
	mux.HandleFunc("GET /api/v1/planets/{id}/buildings", h.PlanetBuildings)
	mux.HandleFunc("POST /api/v1/planets/{id}/build", h.StructureBuild)
	mux.HandleFunc("POST /api/v1/planets/{id}/shipyard", h.ShipyardBuild)
	mux.HandleFunc("POST /api/v1/planets/{id}/research", h.ResearchStart)
	mux.HandleFunc("POST /api/v1/fleets/dispatch", h.FleetDispatch)
	mux.HandleFunc("POST /api/v1/fleets/{id}/recall", h.FleetRecall)
	mux.HandleFunc("GET /dashboard/{id}", h.Dashboard)

	return Logging(Recovery(mux))
}

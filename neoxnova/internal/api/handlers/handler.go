package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/redis/go-redis/v9"

	"neoxnova/internal/store"
)

type Handler struct {
	DB         *sql.DB
	Redis      *redis.Client
	UniverseID string
	Planets    *store.PlanetStore
	Fleets     *store.FleetStore
}

func New(db *sql.DB, rdb *redis.Client, universeID string) *Handler {
	return &Handler{
		DB:         db,
		Redis:      rdb,
		UniverseID: universeID,
		Planets:    store.NewPlanetStore(db),
		Fleets:     store.NewFleetStore(db),
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

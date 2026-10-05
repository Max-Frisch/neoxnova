package handlers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"

	"neoxnova/internal/auth"
	"neoxnova/internal/store"
)

// PlanetAbandon deletes an owned planet and locks its slot.
func (h *Handler) PlanetAbandon(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid planet ID format")
		return
	}
	userID, ok := auth.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	switch err := h.Planets.AbandonPlanet(r.Context(), id, userID); {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]string{"status": "abandoned"})
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "Planet not found")
	case errors.Is(err, store.ErrLastPlanet), errors.Is(err, store.ErrCannotAbandonHome), errors.Is(err, store.ErrFleetInbound):
		writeError(w, http.StatusConflict, err.Error())
	default:
		log.Printf("[ERROR] Failed to abandon planet #%d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "Failed to abandon planet")
	}
}

func (h *Handler) PlanetResources(w http.ResponseWriter, r *http.Request) {
	planetID := r.PathValue("id")

	state, err := h.Planets.GetResourceState(r.Context(), planetID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "Planet not found")
			return
		}
		log.Printf("[ERROR] Resource accumulation failed for planet %s: %v", planetID, err)
		writeError(w, http.StatusInternalServerError, "Failed to calculate continuous resources")
		return
	}

	writeJSON(w, http.StatusOK, state)
}

func (h *Handler) PlanetOverview(w http.ResponseWriter, r *http.Request) {
	planetID := r.PathValue("id")

	overview, err := h.Planets.GetOverview(r.Context(), planetID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "Planet not found")
			return
		}
		log.Printf("[ERROR] Resource calculation failed for planet %s: %v", planetID, err)
		writeError(w, http.StatusInternalServerError, "Failed to update resources")
		return
	}

	writeJSON(w, http.StatusOK, overview)
}

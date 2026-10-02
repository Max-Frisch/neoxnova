package handlers

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
)

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

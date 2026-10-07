package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"neoxnova/internal/auth"
	"neoxnova/internal/models"
	"neoxnova/internal/store"
)

// PlanetIncomingFleets lists the fleets currently inbound to one of the caller's
// celestials, each with a defender-facing mission label and colour
// (GET /api/v1/planets/{id}/incoming-fleets). It complements
// PlanetEspionageReports, which only covered spies.
func (h *Handler) PlanetIncomingFleets(w http.ResponseWriter, r *http.Request) {
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
	fleets, err := h.Fleets.IncomingFleets(r.Context(), id, userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Planet not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to list incoming fleets")
		return
	}
	if fleets == nil {
		fleets = []models.IncomingFleet{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"fleets": fleets})
}

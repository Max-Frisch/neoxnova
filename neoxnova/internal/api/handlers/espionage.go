package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"neoxnova/internal/store"
)

// EspionageReport returns one persisted spy report (GET /api/v1/espionage/reports/{id}).
func (h *Handler) EspionageReport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid report ID format")
		return
	}
	report, err := h.Espionage.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Espionage report not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to load espionage report")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// PlanetEspionageReports lists recent spy reports involving a planet's player
// (GET /api/v1/planets/{id}/espionage-reports?limit=N). This includes incoming
// reports (the player was spied on), so the defender can react/spoof.
func (h *Handler) PlanetEspionageReports(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid planet ID format")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	reports, err := h.Espionage.ListForCelestial(r.Context(), id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list espionage reports")
		return
	}
	if reports == nil {
		reports = []store.EspionageReport{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
}

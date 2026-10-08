package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"neoxnova/internal/store"
)

// ExpeditionReport returns one persisted expedition message
// (GET /api/v1/expeditions/reports/{id}).
func (h *Handler) ExpeditionReport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid report ID format")
		return
	}
	report, err := h.Expedition.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Expedition report not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to load expedition report")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// PlanetExpeditionReports lists recent expedition messages for a planet's owner
// (GET /api/v1/planets/{id}/expedition-reports?limit=N).
func (h *Handler) PlanetExpeditionReports(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid planet ID format")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	reports, err := h.Expedition.ListForCelestial(r.Context(), id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list expedition reports")
		return
	}
	if reports == nil {
		reports = []store.ExpeditionReport{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
}

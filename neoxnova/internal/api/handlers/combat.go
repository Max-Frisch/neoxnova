package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"neoxnova/internal/store"
)

// CombatReport returns one persisted battle report (GET /api/v1/combat/reports/{id}).
func (h *Handler) CombatReport(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid report ID format")
		return
	}
	report, err := h.Combat.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Combat report not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to load combat report")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// PlanetCombatReports lists recent battles involving a planet's player
// (GET /api/v1/planets/{id}/combat-reports?limit=N).
func (h *Handler) PlanetCombatReports(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid planet ID format")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	reports, err := h.Combat.ListForCelestial(r.Context(), id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list combat reports")
		return
	}
	if reports == nil {
		reports = []store.CombatReport{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
}

package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"neoxnova/internal/store"
)

func parsePlanetID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil
}

func writeBuildError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "Planet not found")
	case errors.Is(err, store.ErrUnknownCode):
		writeError(w, http.StatusBadRequest, "Unknown build code")
	case errors.Is(err, store.ErrInvalidQuantity):
		writeError(w, http.StatusBadRequest, "Quantity must be at least 1")
	case errors.Is(err, store.ErrPrerequisites):
		writeError(w, http.StatusBadRequest, "Prerequisites not met")
	case errors.Is(err, store.ErrInsufficientResources):
		writeError(w, http.StatusBadRequest, "Insufficient resources")
	case errors.Is(err, store.ErrNoFields):
		writeError(w, http.StatusBadRequest, "Not enough fields on this planet")
	case errors.Is(err, store.ErrQueueBusy):
		writeError(w, http.StatusConflict, "A build queue is already active")
	default:
		log.Printf("[ERROR] Build operation failed: %v", err)
		writeError(w, http.StatusInternalServerError, "Build operation failed")
	}
}

func writeQueueResult(w http.ResponseWriter, status int, kind string, result store.QueueResult) {
	writeJSON(w, status, map[string]any{
		"status":           kind,
		"queue_id":         result.QueueID,
		"end_time":         result.EndTime.Format(time.RFC3339),
		"duration_seconds": int64(result.Duration.Seconds()),
		"cost": map[string]int64{
			"metal":     result.Cost.Metal,
			"crystal":   result.Cost.Crystal,
			"deuterium": result.Cost.Deuterium,
		},
	})
}

func (h *Handler) StructureBuild(w http.ResponseWriter, r *http.Request) {
	planetID, ok := parsePlanetID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid planet ID format")
		return
	}
	var req struct {
		StructureCode string `json:"structure_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.StructureCode == "" {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	result, err := h.Builds.EnqueueStructure(r.Context(), planetID, req.StructureCode)
	if err != nil {
		writeBuildError(w, err)
		return
	}
	h.notifyScheduler(r.Context())
	log.Printf("[BUILD] structure %s queued on planet %d (queue %d)", req.StructureCode, planetID, result.QueueID)
	writeQueueResult(w, http.StatusCreated, "structure_queued", result)
}

func (h *Handler) ShipyardBuild(w http.ResponseWriter, r *http.Request) {
	planetID, ok := parsePlanetID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid planet ID format")
		return
	}
	var req struct {
		UnitCode string `json:"unit_code"`
		Quantity int64  `json:"quantity"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UnitCode == "" {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	result, err := h.Builds.EnqueueShipyard(r.Context(), planetID, req.UnitCode, req.Quantity)
	if err != nil {
		writeBuildError(w, err)
		return
	}
	h.notifyScheduler(r.Context())
	log.Printf("[BUILD] ship %s x%d queued on planet %d (queue %d)", req.UnitCode, req.Quantity, planetID, result.QueueID)
	writeQueueResult(w, http.StatusCreated, "shipyard_queued", result)
}

func (h *Handler) ResearchStart(w http.ResponseWriter, r *http.Request) {
	planetID, ok := parsePlanetID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "Invalid planet ID format")
		return
	}
	var req struct {
		TechCode string `json:"tech_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TechCode == "" {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	result, err := h.Builds.EnqueueResearch(r.Context(), planetID, req.TechCode)
	if err != nil {
		writeBuildError(w, err)
		return
	}
	h.notifyScheduler(r.Context())
	log.Printf("[BUILD] research %s queued from planet %d (queue %d)", req.TechCode, planetID, result.QueueID)
	writeQueueResult(w, http.StatusCreated, "research_queued", result)
}

func (h *Handler) PlanetBuildings(w http.ResponseWriter, r *http.Request) {
	planetID := r.PathValue("id")
	resp, err := h.Builds.GetBuildings(r.Context(), planetID)
	if err != nil {
		writeBuildError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

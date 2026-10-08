package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"neoxnova/internal/models"
	"neoxnova/internal/store"
)

func (h *Handler) FleetDispatch(w http.ResponseWriter, r *http.Request) {
	var req models.FleetDispatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	result, err := h.Fleets.Dispatch(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusBadRequest, "Origin planet not found")
		case errors.Is(err, store.ErrInsufficientShips), errors.Is(err, store.ErrInsufficientFuel):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, store.ErrNoobProtection):
			writeError(w, http.StatusForbidden, "Target is protected by the noob-protection points ratio")
		case errors.Is(err, store.ErrNoTarget):
			writeError(w, http.StatusBadRequest, "This mission requires an existing target at those coordinates")
		case errors.Is(err, store.ErrExpeditionHoldRequired):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, store.ErrAttackLocked):
			writeError(w, http.StatusConflict, "This planet cannot attack yet: it was relocated recently")
		default:
			log.Printf("[ERROR] Failed to dispatch fleet: %v", err)
			writeError(w, http.StatusInternalServerError, "Failed to dispatch fleet")
		}
		return
	}

	h.notifyScheduler(r.Context())

	log.Printf("[FLEET DISPATCHED] Fleet #%d mission %s launched to [%d:%d:%d]",
		result.FleetID, req.Mission, req.Target.Galaxy, req.Target.System, req.Target.Position)

	writeJSON(w, http.StatusCreated, map[string]any{
		"status":   "dispatched",
		"fleet_id": result.FleetID,
		"arrival":  result.Arrival.Format(time.RFC3339),
		"duration": result.DurationSecs,
		"fuel":     result.FuelBurn,
	})
}

func (h *Handler) FleetRecall(w http.ResponseWriter, r *http.Request) {
	fleetID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid fleet ID format")
		return
	}

	result, err := h.Fleets.Recall(r.Context(), fleetID)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "Fleet not found")
		case errors.Is(err, store.ErrCannotRecall):
			writeError(w, http.StatusConflict, "Fleet cannot be recalled: it has already reached target or is already returning")
		default:
			log.Printf("[ERROR] Failed to recall fleet #%d: %v", fleetID, err)
			writeError(w, http.StatusInternalServerError, "Failed to recall fleet")
		}
		return
	}

	h.notifyScheduler(r.Context())

	log.Printf("[FLEET RECALLED] Fleet #%d reversed safely. Returning in %v", fleetID, result.Elapsed)

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "recalled",
		"fleet_id": fleetID,
		"arrival":  result.Arrival.Format(time.RFC3339),
		"elapsed":  result.Elapsed.Seconds(),
	})
}

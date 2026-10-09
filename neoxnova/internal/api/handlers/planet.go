package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"neoxnova/internal/auth"
	"neoxnova/internal/models"
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

// PlanetRelocate moves an owned planet to new coordinates for Dark Matter.
func (h *Handler) PlanetRelocate(w http.ResponseWriter, r *http.Request) {
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
	var req models.PlanetRelocateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	cost, err := h.Planets.RelocatePlanet(r.Context(), id, userID, req.Galaxy, req.System, req.Position)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, models.PlanetRelocateResponse{
			Status: "relocated",
			Cost:   cost,
			Coordinates: models.Coordinates{
				Galaxy: req.Galaxy, System: req.System, Position: req.Position, Type: models.TypePlanet,
			},
		})
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "Planet not found")
	case errors.Is(err, store.ErrInvalidCoordinates):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrSameCoordinates), errors.Is(err, store.ErrTargetOccupied),
		errors.Is(err, store.ErrInsufficientDarkMatter), errors.Is(err, store.ErrRelocationCooldown),
		errors.Is(err, store.ErrFleetInbound):
		writeError(w, http.StatusConflict, err.Error())
	default:
		log.Printf("[ERROR] Failed to relocate planet #%d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "Failed to relocate planet")
	}
}

// PlanetExpandFields buys extra fields for a planet (or moon) with Dark Matter.
func (h *Handler) PlanetExpandFields(w http.ResponseWriter, r *http.Request) {
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
	var req models.PlanetExpandFieldsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	res, err := h.Planets.ExpandFields(r.Context(), id, userID, req.Fields)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, models.PlanetExpandFieldsResponse{
			Status: "expanded", Cost: res.Cost, FieldsMax: res.FieldsMax, FieldsBought: res.FieldsBought,
		})
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "Planet not found")
	case errors.Is(err, store.ErrInvalidQuantity):
		writeError(w, http.StatusBadRequest, "Field amount must be between 1 and 100")
	case errors.Is(err, store.ErrInsufficientDarkMatter):
		writeError(w, http.StatusConflict, err.Error())
	default:
		log.Printf("[ERROR] Failed to expand fields for planet #%d: %v", id, err)
		writeError(w, http.StatusInternalServerError, "Failed to expand fields")
	}
}

// PlanetsList returns every planet and moon the authenticated account owns
// (GET /api/v1/planets), so a client can bootstrap its planet switcher.
func (h *Handler) PlanetsList(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	list, err := h.Planets.ListOwnedCelestials(r.Context(), userID)
	if err != nil {
		log.Printf("[ERROR] Failed to list planets for user %d: %v", userID, err)
		writeError(w, http.StatusInternalServerError, "Failed to list planets")
		return
	}
	if list == nil {
		list = []models.OwnedCelestial{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"planets": list})
}

// GalaxyScan returns the occupied slots of one system
// (GET /api/v1/galaxy/{galaxy}/{system}) for the galaxy view.
func (h *Handler) GalaxyScan(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	galaxy, errG := strconv.Atoi(r.PathValue("galaxy"))
	system, errS := strconv.Atoi(r.PathValue("system"))
	if errG != nil || errS != nil || galaxy < 1 || galaxy > 9 || system < 1 || system > 499 {
		writeError(w, http.StatusBadRequest, "Invalid galaxy/system")
		return
	}
	res, err := h.Planets.GalaxyScan(r.Context(), h.UniverseID, galaxy, system, userID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "Universe not found")
		return
	}
	if err != nil {
		log.Printf("[ERROR] Galaxy scan %d:%d failed: %v", galaxy, system, err)
		writeError(w, http.StatusInternalServerError, "Failed to scan system")
		return
	}
	writeJSON(w, http.StatusOK, res)
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

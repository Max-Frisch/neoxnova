package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"math/rand"
	"net/http"

	"neoxnova/internal/auth"
	"neoxnova/internal/game"
	"neoxnova/internal/models"
	"neoxnova/internal/store"
)

// ArsenalList returns every upgrade with the caller's owned level/bonus and
// available drawings (GET /api/v1/arsenal).
func (h *Handler) ArsenalList(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	upgrades, err := h.Arsenal.ArsenalList(r.Context(), userID)
	if err != nil {
		log.Printf("[ERROR] Failed to list arsenal for user %d: %v", userID, err)
		writeError(w, http.StatusInternalServerError, "Failed to load arsenal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"upgrades": upgrades})
}

// ArsenalActivate consumes one drawing and rolls an activation
// (POST /api/v1/arsenal/activate). Accepts either upgrade_code or the live
// greid key.
func (h *Handler) ArsenalActivate(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var req models.ArsenalActivateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	code := req.UpgradeCode
	if code == 0 && req.GreID != "" {
		if def, ok := game.UpgradeByKey(req.GreID); ok {
			code = def.Code
		}
	}
	if _, ok := game.UpgradeByCode(code); !ok {
		writeError(w, http.StatusBadRequest, "Unknown upgrade")
		return
	}

	res, err := h.Arsenal.Activate(r.Context(), userID, code, rand.Float64())
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, res)
	case errors.Is(err, store.ErrInsufficientUpgrades):
		writeError(w, http.StatusConflict, "You hold no drawing of that upgrade")
	case errors.Is(err, store.ErrUnknownCode):
		writeError(w, http.StatusBadRequest, "Unknown upgrade")
	default:
		log.Printf("[ERROR] Failed to activate upgrade %d for user %d: %v", code, userID, err)
		writeError(w, http.StatusInternalServerError, "Failed to activate upgrade")
	}
}

// MarketLots lists live auction lots (GET /api/v1/market).
func (h *Handler) MarketLots(w http.ResponseWriter, r *http.Request) {
	lots, err := h.Arsenal.ListMarket(r.Context())
	if err != nil {
		log.Printf("[ERROR] Failed to list market: %v", err)
		writeError(w, http.StatusInternalServerError, "Failed to load market")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lots": lots})
}

// MarketListLot lists drawings on the market (POST /api/v1/market/list).
func (h *Handler) MarketListLot(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var req models.MarketListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	lot, err := h.Arsenal.ListUpgrade(r.Context(), userID, req.UpgradeCode, req.Amount, req.Rate)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, lot)
	case errors.Is(err, store.ErrInsufficientUpgrades):
		writeError(w, http.StatusConflict, "You hold too few drawings of that upgrade")
	case errors.Is(err, store.ErrUnknownCode), errors.Is(err, store.ErrInvalidQuantity):
		writeError(w, http.StatusBadRequest, "Invalid upgrade, amount or rate")
	default:
		log.Printf("[ERROR] Failed to list upgrade for user %d: %v", userID, err)
		writeError(w, http.StatusInternalServerError, "Failed to list upgrade")
	}
}

// MarketBuyLot buys a lot (POST /api/v1/market/buy).
func (h *Handler) MarketBuyLot(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var req models.MarketBuyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.LotID <= 0 {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	lot, err := h.Arsenal.BuyLot(r.Context(), userID, req.LotID)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, lot)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "Lot not found")
	case errors.Is(err, store.ErrLotExpired), errors.Is(err, store.ErrCannotBuyOwnLot),
		errors.Is(err, store.ErrInsufficientAntimatter):
		writeError(w, http.StatusConflict, err.Error())
	default:
		log.Printf("[ERROR] Failed to buy lot %d for user %d: %v", req.LotID, userID, err)
		writeError(w, http.StatusInternalServerError, "Failed to buy lot")
	}
}

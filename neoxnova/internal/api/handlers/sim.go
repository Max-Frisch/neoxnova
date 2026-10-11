package handlers

import (
	"encoding/json"
	"net/http"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
)

// Simulate resolves a battle between two arbitrary fleets using the same pure
// combat engine as the scheduler (POST /api/v1/sim). With no seed supplied the
// result is deterministic for identical inputs, so a simulated attack reproduces
// the real resolution. Returns {seed, result}.
func (h *Handler) Simulate(w http.ResponseWriter, r *http.Request) {
	var req models.SimRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	attacker, err := simCombatant(req.Attacker)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Attacker: "+err.Error())
		return
	}
	defender, err := simCombatant(req.Defender)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Defender: "+err.Error())
		return
	}
	res, seed, err := game.SimulateBattle(attacker, defender, req.Seed)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"seed": seed, "result": res})
}

// simCombatant converts one request side into an engine combatant, validating
// its unit list (unknown codes, empty fleets, oversized fleets).
func simCombatant(s models.SimSide) (game.Combatant, error) {
	return game.NewCombatant(s.Ships, game.CombatTechs{
		Weapons:  s.Techs.Weapons,
		Shield:   s.Techs.Shield,
		Armour:   s.Techs.Armour,
		Laser:    s.Techs.Laser,
		Ion:      s.Techs.Ion,
		Plasma:   s.Techs.Plasma,
		Graviton: s.Techs.Graviton,
	}, s.Academy, s.Upgrades)
}

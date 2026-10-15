package web

import (
	"net/http"
	"strconv"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
	"neoxnova/internal/store"
)

// expeditionHours maps the panel's exp_time slider (1-10) to a hold window in
// quarter-hours: 1 -> 0.25 h, 10 -> 2.5 h. Returns the duration in minutes.
func expeditionMinutes(expTime int) int {
	if expTime < 1 {
		expTime = 1
	}
	if expTime > 10 {
		expTime = 10
	}
	return expTime * 15
}

// expeditionSpeed maps the panel's exp_speed slider (1-10) to a percentage of
// maximum fleet speed: 1 -> 10 %, 10 -> 100 %.
func expeditionSpeed(expSpeed int) int {
	if expSpeed < 1 {
		expSpeed = 1
	}
	if expSpeed > 10 {
		expSpeed = 10
	}
	return expSpeed * 10
}

// cargoFromReport adapts a persisted expedition report's cargo columns to a
// manifest for display.
func cargoFromReport(rep store.ExpeditionReport) models.CargoManifest {
	return models.CargoManifest{
		Metal:     rep.CargoMetal,
		Crystal:   rep.CargoCrystal,
		Deuterium: rep.CargoDeuterium,
	}
}

// ExpeditionShip is one available hull in the expedition ship picker.
type ExpeditionShip struct {
	Code  string
	Name  string
	Count int64
}

// ExpeditionData backs the dedicated expedition command panel.
type ExpeditionData struct {
	SlotsUsed  int
	SlotsTotal int
	Ships      []ExpeditionShip
	Active     []models.FleetEventSummary
	Reports    []store.ExpeditionReport
	OriginG    int
	OriginS    int
	DeepSpace  int
}

// expeditionSlotBudget resolves the Astrophysics-gated expedition slot budget,
// the deep-space slot and the origin coordinates for one celestial.
func (h *Handler) expeditionSlotBudget(r *http.Request, celestialID int64) (slotsTotal, deepSpace, originG, originS int) {
	var astro, bonus, cap, planetsPerSystem int
	err := h.db.QueryRowContext(r.Context(), `
		SELECT COALESCE((SELECT MAX(level) FROM user_technologies
		                  WHERE user_id = c.user_id AND tech_code = 'astrophysics'), 0),
		       u.expedition_slots_bonus, u.expedition_slots_cap, u.planets_per_system,
		       c.galaxy, c.system
		FROM celestial_objects c
		JOIN universes u ON u.id = c.universe_id
		WHERE c.id = $1
	`, celestialID).Scan(&astro, &bonus, &cap, &planetsPerSystem, &originG, &originS)
	if err != nil {
		return 0, game.DeepSpacePosition(game.MaxPlanetSlots), 1, 1
	}
	slotsTotal = game.ExpeditionSlots(astro, bonus)
	if cap > 0 && slotsTotal > cap {
		slotsTotal = cap
	}
	return slotsTotal, game.DeepSpacePosition(planetsPerSystem), originG, originS
}

// expeditionData assembles the panel state: slot usage, available hulls, active
// expeditions and the newest outcomes.
func (h *Handler) expeditionData(r *http.Request, id int64) ExpeditionData {
	idStr := strconv.FormatInt(id, 10)
	var d ExpeditionData
	d.SlotsTotal, d.DeepSpace, d.OriginG, d.OriginS = h.expeditionSlotBudget(r, id)
	d.SlotsUsed = h.activeExpeditions(r, id)

	counts, _ := h.planets.PlanetShips(r.Context(), id)
	for _, def := range sortedUnits(game.Ships) {
		if counts[def.Code] > 0 {
			d.Ships = append(d.Ships, ExpeditionShip{Code: def.Code, Name: def.Name, Count: counts[def.Code]})
		}
	}
	if ov, err := h.planets.GetOverview(r.Context(), idStr); err == nil {
		for _, f := range ov.ActiveFleets {
			if f.Mission == models.MissionExpedition {
				d.Active = append(d.Active, f)
			}
		}
	}
	d.Reports, _ = h.expedition.ListForCelestial(r.Context(), id, 10)
	return d
}

// activeExpeditions counts the account's airborne expeditions (any phase).
func (h *Handler) activeExpeditions(r *http.Request, celestialID int64) int {
	var n int
	_ = h.db.QueryRowContext(r.Context(), `
		SELECT COUNT(*) FROM fleets
		WHERE user_id = (SELECT user_id FROM celestial_objects WHERE id = $1)
		  AND mission = 'EXPEDITION'
		  AND phase IN ('OUTBOUND', 'HOLDING', 'RETURNING')
	`, celestialID).Scan(&n)
	return n
}

// ExpeditionPage renders the dedicated expedition command panel
// (GET /expedition/{id}).
func (h *Handler) ExpeditionPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	sh := h.shell(r, userID, id)
	sh.Title = "Expedition"
	h.render(w, r, http.StatusOK, Page(sh, ExpeditionBody(sh, h.expeditionData(r, id))))
}

// ExpeditionStatus is the htmx fragment that keeps the slots/active expedition
// panel live (GET /expedition/{id}/status).
func (h *Handler) ExpeditionStatus(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	h.render(w, r, http.StatusOK, ExpeditionStatusPanel(h.shell(r, userID, id), h.expeditionData(r, id)))
}

// ExpeditionSubmit launches up to N identical expeditions to the origin
// system's deep-space slot (POST /expedition/{id}/send).
func (h *Handler) ExpeditionSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	back := "/expedition/" + r.PathValue("id")
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()

	ships := map[string]int64{}
	for code := range game.Ships {
		if v, err := strconv.ParseInt(r.FormValue("ship_"+code), 10, 64); err == nil && v > 0 {
			ships[code] = v
		}
	}
	if len(ships) == 0 {
		h.redirect(w, r, back, "", "Select at least one ship")
		return
	}

	minutes := expeditionMinutes(atoiDefault(r.FormValue("exp_time"), 4))
	speed := expeditionSpeed(atoiDefault(r.FormValue("exp_speed"), 10))
	count := atoiDefault(r.FormValue("count"), 1)
	if count < 1 {
		count = 1
	}
	if count > 100 {
		count = 100
	}

	_, deepSpace, originG, originS := h.expeditionSlotBudget(r, id)
	req := models.FleetDispatchRequest{
		OriginPlanetID: id,
		Target: models.Coordinates{
			Galaxy:   originG,
			System:   originS,
			Position: deepSpace,
			Type:     models.TypeDeepSpace,
		},
		Mission:        models.MissionExpedition,
		Ships:          ships,
		SpeedPercent:   speed,
		HoldingMinutes: minutes,
	}

	sent := 0
	for i := 0; i < count; i++ {
		if _, err := h.fleets.Dispatch(r.Context(), req); err != nil {
			if sent == 0 {
				h.redirect(w, r, back, "", err.Error())
				return
			}
			break
		}
		sent++
	}
	h.nudge()
	h.redirect(w, r, back, "Sent "+strconv.Itoa(sent)+" expedition(s)", "")
}

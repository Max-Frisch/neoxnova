package web

import (
	"errors"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
	"neoxnova/internal/store"
)

// planetariumMaxFields caps a single Dark-Matter field purchase (mirrors
// store.maxFieldPurchase).
const planetariumMaxFields = 100

// PlanetariumCelestial is one owned celestial enriched for the Planetarium page.
type PlanetariumCelestial struct {
	models.CelestialDetail
	Coords        string
	NextFieldDM   int64
	RelocCooldown string
	AttackLock    string
}

// PlanetariumData backs the Planetarium page.
type PlanetariumData struct {
	Celestials       []PlanetariumCelestial
	DarkMatter       int64
	Stardust         int64
	PlanetsPerSystem int
	MaxFields        int
}

// planetariumData loads the account's celestials and DM/Stardust balances.
func (h *Handler) planetariumData(r *http.Request, userID int64) PlanetariumData {
	data := PlanetariumData{PlanetsPerSystem: game.MaxPlanetSlots, MaxFields: planetariumMaxFields}
	if n, err := h.planets.PlanetsPerSystem(r.Context(), h.universeID); err == nil && n > 0 {
		data.PlanetsPerSystem = n
	}
	details, _ := h.planets.ListCelestialDetails(r.Context(), userID)
	for _, d := range details {
		c := PlanetariumCelestial{
			CelestialDetail: d,
			Coords:          coord3(d.Galaxy, d.System, d.Position),
			NextFieldDM:     game.FieldExpansionCost(int(d.FieldsBought), 1),
		}
		if d.RelocationNextAt != nil && time.Now().Before(*d.RelocationNextAt) {
			c.RelocCooldown = hms(int64(time.Until(*d.RelocationNextAt).Seconds()))
		}
		if d.AttackLockedUntil != nil && time.Now().Before(*d.AttackLockedUntil) {
			c.AttackLock = hms(int64(time.Until(*d.AttackLockedUntil).Seconds()))
		}
		data.Celestials = append(data.Celestials, c)
	}
	if dm, _, sd, err := h.auth.UserBalances(r.Context(), userID); err == nil {
		data.DarkMatter, data.Stardust = dm, sd
	}
	return data
}

// PlanetariumPage renders the Black market -> Planetarium page (account-wide,
// anchored to the homeworld).
func (h *Handler) PlanetariumPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Black market")
	h.render(w, r, http.StatusOK, Page(sh, PlanetariumBody(sh, h.planetariumData(r, userID))))
}

// PlanetariumFieldsPreview prices a Dark-Matter field purchase (no mutation).
func (h *Handler) PlanetariumFieldsPreview(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	detail, fields, err := h.planetariumTarget(r, userID)
	if err != nil {
		h.render(w, r, http.StatusOK, PlanetariumQuotePartial("", err.Error()))
		return
	}
	if fields < 1 || fields > planetariumMaxFields {
		h.render(w, r, http.StatusOK, PlanetariumQuotePartial("", "Field amount must be between 1 and 100"))
		return
	}
	cost := game.FieldExpansionCost(int(detail.FieldsBought), fields)
	text := "Cost " + fnum(cost) + " Dark Matter for +" + strconv.Itoa(fields) +
		" fields (max " + strconv.Itoa(detail.FieldsMax) + " → " + strconv.Itoa(detail.FieldsMax+fields) + ")"
	h.render(w, r, http.StatusOK, PlanetariumQuotePartial(text, ""))
}

// PlanetariumFieldsSubmit buys extra fields for the selected celestial.
func (h *Handler) PlanetariumFieldsSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	back := planetariumReturn(r)
	detail, fields, err := h.planetariumTarget(r, userID)
	if err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	res, err := h.planets.ExpandFields(r.Context(), detail.ID, userID, fields)
	if err != nil {
		h.redirect(w, r, back, "", planetariumFieldsError(err))
		return
	}
	msg := "Added " + strconv.Itoa(fields) + " fields to " + detail.Name +
		" for " + fnum(res.Cost) + " Dark Matter (max " + strconv.FormatInt(res.FieldsMax, 10) + ")"
	h.redirect(w, r, back, msg, "")
}

// PlanetariumDiameterSubmit spends Stardust and the local debris field to grow
// the selected celestial's diameter and field capacity by a rolled amount.
func (h *Handler) PlanetariumDiameterSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	back := planetariumReturn(r)
	detail, _, err := h.planetariumTarget(r, userID)
	if err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	diameterGain, fieldsGain := game.DiameterExpansionGains(rand.Float64(), rand.Float64())
	res, err := h.planets.ExpandDiameter(r.Context(), detail.ID, userID, diameterGain, fieldsGain)
	if err != nil {
		h.redirect(w, r, back, "", planetariumDiameterError(err))
		return
	}
	msg := detail.Name + " grew by " + strconv.Itoa(res.DiameterGain) + " km and +" +
		strconv.Itoa(res.FieldsGain) + " fields for 1 Stardust and the local debris"
	h.redirect(w, r, back, msg, "")
}

// PlanetariumRelocatePreview prices a teleport (no mutation).
func (h *Handler) PlanetariumRelocatePreview(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	detail, _, err := h.planetariumTarget(r, userID)
	if err != nil {
		h.render(w, r, http.StatusOK, PlanetariumQuotePartial("", err.Error()))
		return
	}
	to := planetariumCoords(r)
	perSystem, _ := h.planets.PlanetsPerSystem(r.Context(), h.universeID)
	if !game.ValidRelocationTarget(to[0], to[1], to[2], perSystem) {
		h.render(w, r, http.StatusOK, PlanetariumQuotePartial("", "That destination is outside the colonisable range"))
		return
	}
	if to[0] == detail.Galaxy && to[1] == detail.System && to[2] == detail.Position {
		h.render(w, r, http.StatusOK, PlanetariumQuotePartial("", "The planet is already at those coordinates"))
		return
	}
	if game.RelocationLeavesSystem(detail.Galaxy, detail.System, to[0], to[1]) &&
		detail.RelocationNextAt != nil && time.Now().Before(*detail.RelocationNextAt) {
		h.render(w, r, http.StatusOK, PlanetariumQuotePartial("", "Inter-system teleports are on cooldown for another "+
			hms(int64(time.Until(*detail.RelocationNextAt).Seconds()))))
		return
	}
	cost := game.RelocationCost(detail.Galaxy, detail.System, detail.Position, to[0], to[1], to[2])
	text := "Teleport " + detail.Name + " to " + coord3(to[0], to[1], to[2]) +
		" for " + fnum(cost) + " Dark Matter"
	h.render(w, r, http.StatusOK, PlanetariumQuotePartial(text, ""))
}

// PlanetariumRelocateSubmit teleports the selected planet to new coordinates.
func (h *Handler) PlanetariumRelocateSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	back := planetariumReturn(r)
	detail, _, err := h.planetariumTarget(r, userID)
	if err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	to := planetariumCoords(r)
	cost, err := h.planets.RelocatePlanet(r.Context(), detail.ID, userID, to[0], to[1], to[2])
	if err != nil {
		h.redirect(w, r, back, "", planetariumRelocateError(err))
		return
	}
	msg := "Teleported " + detail.Name + " to " + coord3(to[0], to[1], to[2]) +
		" for " + fnum(cost) + " Dark Matter"
	h.redirect(w, r, back, msg, "")
}

// planetariumTarget resolves the form's celestial_id (ownership checked by the
// store) and parsed field amount.
func (h *Handler) planetariumTarget(r *http.Request, userID int64) (models.CelestialDetail, int, error) {
	id := int64Default(r.FormValue("celestial_id"))
	if id == 0 {
		return models.CelestialDetail{}, 0, errors.New("no planet selected")
	}
	detail, err := h.planets.CelestialDetail(r.Context(), id, userID)
	if err != nil {
		return models.CelestialDetail{}, 0, errors.New("planet not found")
	}
	return detail, atoiDefault(r.FormValue("fields"), 0), nil
}

// planetariumCoords reads the [galaxy, system, position] destination triple.
func planetariumCoords(r *http.Request) [3]int {
	return [3]int{
		atoiDefault(r.FormValue("galaxy"), 0),
		atoiDefault(r.FormValue("system"), 0),
		atoiDefault(r.FormValue("position"), 0),
	}
}

func planetariumReturn(r *http.Request) string {
	return redirectBase(r.FormValue("return"), "/blackmarket/planetarium")
}

func planetariumFieldsError(err error) string {
	switch {
	case errors.Is(err, store.ErrInvalidQuantity):
		return "Field amount must be between 1 and 100"
	case errors.Is(err, store.ErrInsufficientDarkMatter):
		return "Not enough Dark Matter"
	case errors.Is(err, store.ErrNotFound):
		return "Planet not found"
	default:
		return err.Error()
	}
}

func planetariumDiameterError(err error) string {
	switch {
	case errors.Is(err, store.ErrInsufficientStardust):
		return "Not enough Stardust"
	case errors.Is(err, store.ErrInsufficientDebris):
		return "Not enough debris at the planet's coordinates"
	case errors.Is(err, store.ErrNotFound):
		return "Planet not found"
	default:
		return err.Error()
	}
}

func planetariumRelocateError(err error) string {
	switch {
	case errors.Is(err, store.ErrInvalidCoordinates):
		return "That destination is outside the colonisable range"
	case errors.Is(err, store.ErrSameCoordinates):
		return "The planet is already at those coordinates"
	case errors.Is(err, store.ErrTargetOccupied):
		return "Those coordinates are occupied or reserved"
	case errors.Is(err, store.ErrInsufficientDarkMatter):
		return "Not enough Dark Matter"
	case errors.Is(err, store.ErrRelocationCooldown):
		return "This planet was relocated too recently"
	case errors.Is(err, store.ErrFleetInbound):
		return "The planet has fleets in flight and cannot be moved"
	case errors.Is(err, store.ErrNotFound):
		return "Planet not found"
	default:
		return err.Error()
	}
}

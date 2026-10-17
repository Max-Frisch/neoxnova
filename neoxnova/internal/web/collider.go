package web

import (
	"errors"
	"net/http"
	"strconv"

	"neoxnova/internal/store"
)

// ColliderPage renders the Moon Collider (live page=kollaider): the current
// level and Dark-Matter-per-hour output, the next level's output and Antimatter
// price, and the build button. Only moons carry a Collider.
func (h *Handler) ColliderPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	st, err := h.moons.ColliderStatusFor(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "collider failed", http.StatusInternalServerError)
		return
	}
	sh := h.shell(r, userID, id)
	sh.Title = "Collider"
	h.render(w, r, http.StatusOK, Page(sh, ColliderBody(sh, st)))
}

// ColliderSubmit raises the moon's Collider one level, spending Antimatter, and
// redirects back with the new output (the live "upgraded" dialog).
func (h *Handler) ColliderSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	back := "/collider/" + r.PathValue("id")
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	st, err := h.moons.UpgradeCollider(r.Context(), userID, id)
	if err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	h.redirect(w, r, back,
		"Collider upgraded to level "+strconv.Itoa(st.Level)+" ("+fnum(st.ProductionPerHour)+" Dark Matter/h)", "")
}

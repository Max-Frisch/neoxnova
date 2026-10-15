package web

import (
	"log"
	"net/http"
	"strconv"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
	"neoxnova/internal/store"
)

// ---- Jumpgate ------------------------------------------------------------

// JumpShipOption is one teleportable ship row on the Jumpgate page.
type JumpShipOption struct {
	Code  string
	Name  string
	Count int64
}

// JumpgateData backs the Jumpgate panel: the source gate's level + recharge,
// the selectable destination moons and the docked ships.
type JumpgateData struct {
	Level        int
	CooldownSecs int64
	Destinations []store.JumpDestination
	Ships        []JumpShipOption
	Badge        string
}

// JumpgatePage shows the Jumpgate panel for one owned moon.
func (h *Handler) JumpgatePage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	st, err := h.moons.JumpgateStatusFor(r.Context(), userID, id)
	if err != nil {
		if err == store.ErrNotFound {
			http.NotFound(w, r)
			return
		}
		log.Printf("[WEB] jumpgate status: %v", err)
	}
	sh := h.shell(r, userID, id)
	sh.Title = "Jumpgate"

	data := JumpgateData{Level: st.Level, CooldownSecs: st.CooldownSecs, Destinations: st.Destinations}
	if st.Level < 1 {
		data.Badge = "No Jumpgate on this moon"
	} else if len(st.Destinations) == 0 {
		data.Badge = "A second moon with a Jumpgate is required"
	}
	if ships, err := h.planets.PlanetShips(r.Context(), id); err == nil {
		for _, def := range sortedUnits(game.Ships) {
			if ships[def.Code] > 0 {
				data.Ships = append(data.Ships, JumpShipOption{Code: def.Code, Name: def.Name, Count: ships[def.Code]})
			}
		}
	}
	h.render(w, r, http.StatusOK, Page(sh, JumpgateBody(sh, data)))
}

// JumpgateSubmit performs a jump: it moves the selected ships (never resources)
// from the current moon to the chosen destination moon.
func (h *Handler) JumpgateSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	back := "/jumpgate/" + r.PathValue("id")
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	dst, err := strconv.ParseInt(r.FormValue("dst"), 10, 64)
	if err != nil || dst == 0 {
		h.redirect(w, r, back, "", "Choose a destination moon")
		return
	}
	ships := map[string]int64{}
	for _, def := range sortedUnits(game.Ships) {
		if n := int64Default(r.FormValue("ship_" + def.Code)); n > 0 {
			ships[def.Code] = n
		}
	}
	if err := h.moons.JumpFleet(r.Context(), userID, id, dst, ships); err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	h.redirect(w, r, back, "Fleet jumped", "")
}

// ---- Phalanx -------------------------------------------------------------

// PhalanxFleet is one revealed fleet movement row.
type PhalanxFleet struct {
	Mission string
	Phase   string
	Origin  string
	Dest    string
	ETA     string
}

// PhalanxData backs the Phalanx scan panel.
type PhalanxData struct {
	Level   int
	Target  string
	Scanned bool
	Error   string
	Fleets  []PhalanxFleet
}

// PhalanxPage renders the Phalanx Sensor of one owned moon and, when a target
// is supplied, the fleets in movement at that position.
func (h *Handler) PhalanxPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	sh := h.shell(r, userID, id)
	sh.Title = "Phalanx"
	data := PhalanxData{}

	g := atoiDefault(r.URL.Query().Get("g"), 0)
	s := atoiDefault(r.URL.Query().Get("s"), 0)
	p := atoiDefault(r.URL.Query().Get("p"), 0)
	ctype := models.CelestialType(orDefault(r.URL.Query().Get("type"), string(models.TypePlanet)))
	if g > 0 && s > 0 && p > 0 {
		data.Scanned = true
		data.Target = coord3(g, s, p)
		target := models.Coordinates{Galaxy: g, System: s, Position: p, Type: ctype}
		fleets, err := h.moons.PhalanxScan(r.Context(), userID, id, target)
		switch {
		case err == store.ErrPhalanxNoSensor, err == store.ErrPhalanxOffline,
			err == store.ErrPhalanxOutOfRange:
			data.Error = err.Error()
		case err != nil:
			log.Printf("[WEB] phalanx scan: %v", err)
			data.Error = "Scan failed"
		default:
			for _, f := range fleets {
				data.Fleets = append(data.Fleets, PhalanxFleet{
					Mission: game.MissionDisplayFor(string(f.Mission), true).Text,
					Phase:   string(f.Phase),
					Origin:  coord3(f.Origin.Galaxy, f.Origin.System, f.Origin.Position),
					Dest:    coord3(f.Destination.Galaxy, f.Destination.System, f.Destination.Position),
					ETA:     hms(f.RemainingSecs),
				})
			}
		}
	}
	h.render(w, r, http.StatusOK, Page(sh, PhalanxBody(sh, data)))
}

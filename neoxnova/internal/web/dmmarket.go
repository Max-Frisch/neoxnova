package web

import (
	"net/http"
	"strings"

	"neoxnova/internal/game"
)

// DMMarketItem is one building or technology the caller can buy instantly with
// Dark Matter, with its current level and next-level price.
type DMMarketItem struct {
	Code       string
	Name       string
	Image      string
	Level      int
	NextLevel  int
	Cost       game.Cost
	DarkMatter int64
	Unlocked   bool
	Reason     string
	// EnergyReq is nonzero for energy-gated techs (Graviton Research).
	EnergyReq  int64
	EnergyHave float64
	EnergyMet  bool
}

// DMMarketData backs either DM market sub-page. Action is the POST target and
// Title/Subtitle label the two variants.
type DMMarketData struct {
	Title      string
	Subtitle   string
	Action     string
	Items      []DMMarketItem
	Divisor    int64
	DarkMatter int64
}

func dmMarketItem(code, name string, level int, cost game.Cost, requires, levels map[string]int, divisor int64, energyMax float64) DMMarketItem {
	unlocked, reason := reqOK(requires, levels)
	item := DMMarketItem{
		Code:       code,
		Name:       name,
		Image:      assetURL(code),
		Level:      level,
		NextLevel:  level + 1,
		Cost:       cost,
		DarkMatter: game.DMMarketPrice(cost, divisor),
		Unlocked:   unlocked,
		Reason:     reason,
		EnergyMet:  true,
	}
	if req, gated := game.TechEnergyRequirement(code, level+1); gated {
		item.EnergyReq = req
		item.EnergyHave = energyMax
		item.EnergyMet = energyMax >= float64(req)
		if !item.EnergyMet {
			item.Unlocked = false
			item.Reason = "Energy " + fnum(req) + " required"
		}
	}
	return item
}

// dmMarketData lists every building (kind == "structures") or technology (any
// other kind) with its instant Dark Matter price for the homeworld's universe.
func (h *Handler) dmMarketData(r *http.Request, userID int64, kind string) DMMarketData {
	divisor := h.dmDivisor(r)
	data := DMMarketData{Divisor: divisor}
	home := h.homeworldID(r, userID)
	energyMax, _ := h.builds.CelestialEnergyMax(r.Context(), home)
	structLevels, _ := h.builds.StructureLevels(r.Context(), home)
	techLevels, _ := h.builds.TechLevels(r.Context(), userID)
	combined := mergeLevels(structLevels, techLevels)
	if dm, _, _, err := h.auth.UserBalances(r.Context(), userID); err == nil {
		data.DarkMatter = dm
	}
	if kind == "structures" {
		data.Title = "Buildings Market"
		data.Subtitle = "Build buildings instantly with Dark Matter."
		data.Action = "/blackmarket/buildings"
		for _, def := range sortedStructures() {
			lvl := structLevels[def.Code]
			cost, _ := game.StructureCost(def.Code, lvl+1)
			data.Items = append(data.Items, dmMarketItem(def.Code, def.Name, lvl, cost, def.Requires, combined, divisor, energyMax))
		}
		return data
	}
	data.Title = "Technology Market"
	data.Subtitle = "Develop technologies instantly with Dark Matter."
	data.Action = "/blackmarket/technology"
	for _, def := range sortedTechs() {
		lvl := techLevels[def.Code]
		cost, _ := game.TechCost(def.Code, lvl+1)
		data.Items = append(data.Items, dmMarketItem(def.Code, def.Name, lvl, cost, def.Requires, combined, divisor, energyMax))
	}
	return data
}

func (h *Handler) DMBuildingsPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Black market")
	h.render(w, r, http.StatusOK, Page(sh, DMMarketBody(sh, h.dmMarketData(r, userID, "structures"))))
}

func (h *Handler) DMTechnologyPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Black market")
	h.render(w, r, http.StatusOK, Page(sh, DMMarketBody(sh, h.dmMarketData(r, userID, "techs"))))
}

// DMStructureSubmit buys one instant building level on the homeworld.
func (h *Handler) DMStructureSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	back := redirectBase(r.FormValue("return"), "/blackmarket/buildings")
	home := h.homeworldID(r, userID)
	if home == 0 {
		h.redirect(w, r, back, "", "No planet to build on")
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	_, dm, err := h.builds.InstantStructure(r.Context(), home, userID, code)
	if err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	h.redirect(w, r, back, "Built "+displayName(code)+" for "+fnum(dm)+" Dark Matter", "")
}

// DMTechSubmit buys one instant technology level for the account.
func (h *Handler) DMTechSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	back := redirectBase(r.FormValue("return"), "/blackmarket/technology")
	home := h.homeworldID(r, userID)
	if home == 0 {
		h.redirect(w, r, back, "", "No planet to research on")
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	_, dm, err := h.builds.InstantTech(r.Context(), home, userID, code)
	if err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	h.redirect(w, r, back, "Researched "+displayName(code)+" for "+fnum(dm)+" Dark Matter", "")
}

// merchantRate reads the universe's Merchant of Fleets refund percent, falling
// back to 50 when the universe row is missing so the page still renders.
func (h *Handler) merchantRate(r *http.Request) int {
	var rate int
	if err := h.db.QueryRowContext(r.Context(), `SELECT merchant_rate FROM universes WHERE code_name = $1`, h.universeID).Scan(&rate); err != nil {
		return 50
	}
	return rate
}

// dmDivisor reads the universe's resources-per-Dark-Matter divisor, falling back
// to 1000 when the universe row is missing.
func (h *Handler) dmDivisor(r *http.Request) int64 {
	var d int64
	if err := h.db.QueryRowContext(r.Context(), `SELECT dm_market_divisor FROM universes WHERE code_name = $1`, h.universeID).Scan(&d); err != nil || d <= 0 {
		return 1000
	}
	return d
}

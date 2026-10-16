package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"neoxnova/internal/game"
)

// ---- Senate (Governators) ------------------------------------------------

// senateTiles are the Senate hub entries. Officers is not implemented yet, so
// it renders as a disabled "coming soon" tile (reskin rule: never fake pages).
var senateTiles = []BlackMarketTile{
	{Key: "officers", Label: "Officers", Image: "office.png"},
	{Key: "governators", Label: "Governators", Image: "stars.png", Href: "/senate/governators", Built: true},
}

// governatorImages maps each governator code to a Niburu icon.
var governatorImages = map[int]string{
	game.GovCodeWeapons:  "attack.png",
	game.GovCodeShield:   "shield.png",
	game.GovCodeBuilding: "build.png",
	game.GovCodeResource: "proc.png",
	game.GovCodeEnergy:   "energy.png",
	game.GovCodeResearch: "tech.png",
	game.GovCodeFleet:    "fleet.png",
}

func governatorImage(code int) string {
	if f, ok := governatorImages[code]; ok {
		return "/static/niburu/icons/" + f
	}
	return ""
}

// GovernatorItem is one governator card on the Senate page.
type GovernatorItem struct {
	Code          int
	Name          string
	Image         string
	Level         int
	MaxLevel      int
	BonusPct      int
	Effects       []game.GovernatorEffect
	NextLevel     int
	APCostNext    int64
	APCum         []int64 // cumulative AP cost to reach level i (index i)
	DMPricePerDay int64
	Active        bool
	ExpiresAt     time.Time
	AtMax         bool
}

type GovernatorsData struct {
	Items             []GovernatorItem
	AchievementPoints int64
}

func (h *Handler) SenatePage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Senate")
	h.render(w, r, http.StatusOK, Page(sh, SenateBody(sh, senateTiles)))
}

func (h *Handler) GovernatorsPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Governators")

	states, err := h.senate.GovernatorStates(r.Context(), userID)
	if err != nil {
		sh.Error = "Could not load the governators"
	}
	ap, _ := h.senate.AchievementPoints(r.Context(), userID)

	now := time.Now()
	data := GovernatorsData{AchievementPoints: ap}
	for _, def := range game.Governators {
		st := states[def.Code]
		it := GovernatorItem{
			Code: def.Code, Name: def.Name, Image: governatorImage(def.Code),
			Level: st.Level, MaxLevel: def.MaxLevel, BonusPct: def.BonusPct(st.Level),
			Effects: def.Effects, DMPricePerDay: def.DMPricePerDay(st.Level),
		}
		it.NextLevel = st.Level + 1
		if it.NextLevel > def.MaxLevel {
			it.NextLevel = def.MaxLevel
			it.AtMax = true
		}
		it.APCostNext = def.APCost(st.Level, it.NextLevel)
		it.APCum = make([]int64, def.MaxLevel+1)
		for i := 1; i <= def.MaxLevel; i++ {
			it.APCum[i] = def.APCost(0, i)
		}
		if st.ExpiresAt.Valid && st.ExpiresAt.Time.After(now) {
			it.Active = true
			it.ExpiresAt = st.ExpiresAt.Time
		}
		data.Items = append(data.Items, it)
	}
	h.render(w, r, http.StatusOK, Page(sh, GovernatorsBody(sh, data)))
}

func (h *Handler) GovernatorImproveSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	code, _ := strconv.Atoi(r.FormValue("lvl_id"))
	target, _ := strconv.Atoi(r.FormValue("lvl_count"))
	cost, err := h.senate.ImproveGovernator(r.Context(), userID, code, target)
	if err != nil {
		h.redirect(w, r, "/senate/governators", "", err.Error())
		return
	}
	def, _ := game.GovernatorByCode(code)
	lvl := target
	if lvl > def.MaxLevel {
		lvl = def.MaxLevel
	}
	h.redirect(w, r, "/senate/governators",
		fmt.Sprintf("%s improved to level %d for %s achievement points", def.Name, lvl, fnum(cost)), "")
}

func (h *Handler) GovernatorHireSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	code, _ := strconv.Atoi(r.FormValue("id"))
	days, _ := strconv.Atoi(r.FormValue("days"))
	total, err := h.senate.HireGovernator(r.Context(), userID, code, days)
	if err != nil {
		h.redirect(w, r, "/senate/governators", "", err.Error())
		return
	}
	def, _ := game.GovernatorByCode(code)
	h.redirect(w, r, "/senate/governators",
		fmt.Sprintf("%s activated for %d day(s) for %s Dark Matter", def.Name, days, fnum(total)), "")
}

// govEffectText renders a governator effect as a signed percentage, e.g.
// "+20%" or "-20%" for effects that reduce their target.
func govEffectText(pct int, e game.GovernatorEffect) string {
	if e.Decrease {
		return "-" + strconv.Itoa(pct) + "%"
	}
	return "+" + strconv.Itoa(pct) + "%"
}

// apCumJSON serialises a cumulative AP-cost table for the live improve preview.
func apCumJSON(cum []int64) string {
	b, err := json.Marshal(cum)
	if err != nil || len(cum) == 0 {
		return "[]"
	}
	return string(b)
}

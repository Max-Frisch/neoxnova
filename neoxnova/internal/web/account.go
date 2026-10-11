package web

import (
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"time"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
)

// accountShell anchors an account-wide page (Arsenal, Market, Technologies,
// Simulator) to the player's homeworld so the shared chrome — nav, planet
// switcher and resource bar — renders exactly as it does elsewhere.
func (h *Handler) accountShell(r *http.Request, userID int64, title string) Shell {
	var activeID int64
	if homeID, err := h.planets.HomeworldID(r.Context(), userID); err == nil {
		activeID = homeID
	}
	sh := h.shell(r, userID, activeID)
	sh.Title = title
	return sh
}

// ---- Arsenal -------------------------------------------------------------

type ArsenalItem struct {
	Code      int
	Name      string
	Group     string
	Level     int
	Bonus     float64
	NextBonus float64
	Available int
}

type ArsenalData struct {
	Items []ArsenalItem
}

func (h *Handler) ArsenalPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Arsenal")
	items, err := h.arsenal.ArsenalList(r.Context(), userID)
	if err != nil {
		sh.Error = "Could not load the Arsenal"
	}
	data := ArsenalData{}
	for _, it := range items {
		data.Items = append(data.Items, ArsenalItem{
			Code: it.Code, Name: it.Name, Group: it.Group, Level: it.Level,
			Bonus: it.Bonus, NextBonus: it.NextBonus, Available: it.Available,
		})
	}
	h.render(w, r, http.StatusOK, Page(sh, ArsenalBody(sh, data)))
}

func (h *Handler) ArsenalActivateSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	code, _ := strconv.Atoi(r.FormValue("code"))
	res, err := h.arsenal.Activate(r.Context(), userID, code, rand.Float64())
	if err != nil {
		h.redirect(w, r, "/arsenal", "", err.Error())
		return
	}
	if res.Success {
		h.redirect(w, r, "/arsenal", res.Name+" activated → L"+strconv.Itoa(res.Level)+" ("+bonusStr(res.Bonus)+")", "")
		return
	}
	h.redirect(w, r, "/arsenal", "", res.Name+" activation failed ("+pct(res.Chance)+" chance); value fell to "+bonusStr(res.Bonus))
}

// ---- Black market --------------------------------------------------------

type MarketData struct {
	Lots     []models.MarketLot
	Mine     []models.MarketLot
	Listable []ArsenalItem
}

func (h *Handler) MarketPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Black market")
	data := MarketData{}
	if lots, err := h.arsenal.ListMarket(r.Context()); err == nil {
		data.Lots = lots
	} else {
		sh.Error = "Could not load the market"
	}
	if mine, err := h.arsenal.ListOwnLots(r.Context(), userID); err == nil {
		data.Mine = mine
	}
	if items, err := h.arsenal.ArsenalList(r.Context(), userID); err == nil {
		for _, it := range items {
			if it.Available > 0 {
				data.Listable = append(data.Listable, ArsenalItem{
					Code: it.Code, Name: it.Name, Group: it.Group, Available: it.Available,
				})
			}
		}
	}
	h.render(w, r, http.StatusOK, Page(sh, MarketBody(sh, data)))
}

func (h *Handler) MarketListSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	code, _ := strconv.Atoi(r.FormValue("upgrade_code"))
	amount, _ := strconv.Atoi(r.FormValue("amount"))
	rate, _ := strconv.ParseInt(r.FormValue("rate"), 10, 64)
	lot, err := h.arsenal.ListUpgrade(r.Context(), userID, code, amount, rate)
	if err != nil {
		h.redirect(w, r, "/market", "", err.Error())
		return
	}
	h.redirect(w, r, "/market", "Listed "+strconv.Itoa(lot.Amount)+"× "+lot.Upgrade+" for "+strconv.FormatInt(lot.TotalPriceAtm, 10)+" AM", "")
}

func (h *Handler) MarketBuySubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	lotID, err := strconv.ParseInt(r.FormValue("lot_id"), 10, 64)
	if err != nil || lotID <= 0 {
		h.redirect(w, r, "/market", "", "Invalid lot")
		return
	}
	lot, err := h.arsenal.BuyLot(r.Context(), userID, lotID)
	if err != nil {
		h.redirect(w, r, "/market", "", err.Error())
		return
	}
	h.redirect(w, r, "/market", "Bought "+strconv.Itoa(lot.Amount)+"× "+lot.Upgrade+" for "+strconv.FormatInt(lot.TotalPriceAtm, 10)+" AM", "")
}

func (h *Handler) MarketRemoveSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	lotID, err := strconv.ParseInt(r.FormValue("lot_id"), 10, 64)
	if err != nil || lotID <= 0 {
		h.redirect(w, r, "/market", "", "Invalid lot")
		return
	}
	if err := h.arsenal.RemoveLot(r.Context(), userID, lotID); err != nil {
		h.redirect(w, r, "/market", "", err.Error())
		return
	}
	h.redirect(w, r, "/market", "Lot withdrawn", "")
}

// ---- Technologies (tech tree) --------------------------------------------

type TechRequirement struct {
	Name string
	Have int
	Need int
	Met  bool
}

type TechNode struct {
	ID       int
	Code     string
	Name     string
	Category string
	Level    int
	Leveled  bool
	Unlocked bool
	Requires []TechRequirement
}

type TechGroup struct {
	Name  string
	Nodes []TechNode
}

type TechnologiesData struct {
	Groups []TechGroup
}

// techOrder fixes the display order of the category groups.
var techOrder = []string{"Research", "Buildings", "Ships", "Defenses", "Moon", "Other"}

func (h *Handler) TechnologiesPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Technologies")

	idStr := strconv.FormatInt(sh.ActivePlanetID, 10)
	structLevels, researchLevels := map[string]int{}, map[string]int{}
	if b, err := h.builds.GetBuildings(r.Context(), idStr); err == nil {
		structLevels = b.Levels
	}
	if rr, err := h.builds.GetResearch(r.Context(), idStr); err == nil {
		researchLevels = rr.Levels
	}
	levels := mergeLevels(structLevels, researchLevels)

	byCat := map[string][]TechNode{}
	for code, reqs := range game.TechTree {
		node, ok := techNode(code, reqs, structLevels, researchLevels, levels)
		if !ok {
			continue
		}
		byCat[node.Category] = append(byCat[node.Category], node)
	}
	data := TechnologiesData{}
	for _, cat := range techOrder {
		nodes := byCat[cat]
		if len(nodes) == 0 {
			continue
		}
		sort.Slice(nodes, func(i, j int) bool {
			if nodes[i].ID != nodes[j].ID {
				return nodes[i].ID < nodes[j].ID
			}
			return nodes[i].Name < nodes[j].Name
		})
		data.Groups = append(data.Groups, TechGroup{Name: cat, Nodes: nodes})
	}
	h.render(w, r, http.StatusOK, Page(sh, TechnologiesBody(sh, data)))
}

// techNode resolves one TechTree code to a renderable node, reporting false when
// the code has no catalog definition (e.g. an unmapped live-only id).
func techNode(code string, reqs map[string]int, structLevels, researchLevels, levels map[string]int) (TechNode, bool) {
	node := TechNode{Code: code}
	switch {
	case game.Structures[code].Code == code:
		def := game.Structures[code]
		node.Name, node.ID, node.Category, node.Level, node.Leveled = def.Name, def.ID, "Buildings", structLevels[code], true
	case game.Techs[code].Code == code:
		def := game.Techs[code]
		node.Name, node.ID, node.Category, node.Level, node.Leveled = def.Name, def.ID, "Research", researchLevels[code], true
	case game.Ships[code].Code == code:
		def := game.Ships[code]
		node.Name, node.Category = def.Name, "Ships"
	case game.Defenses[code].Code == code:
		def := game.Defenses[code]
		node.Name, node.Category = def.Name, "Defenses"
	default:
		if def, ok := game.MoonStructure(code); ok {
			node.Name, node.ID, node.Category = def.Name, def.ID, "Moon"
		} else {
			return TechNode{}, false
		}
	}
	node.Unlocked = game.RequiresMet(reqs, levels)
	for rc, need := range reqs {
		node.Requires = append(node.Requires, TechRequirement{
			Name: displayName(rc), Have: levels[rc], Need: need, Met: levels[rc] >= need,
		})
	}
	sort.Slice(node.Requires, func(i, j int) bool { return node.Requires[i].Name < node.Requires[j].Name })
	return node, true
}

// ---- Simulator -----------------------------------------------------------

type SimUnit struct {
	Code      string
	Name      string
	IsDefense bool
	Atk       int64
	Def       int64
}

type SimUnitCount struct {
	Name  string
	Count int64
}

type SimSideView struct {
	Initial   []SimUnitCount
	Lost      []SimUnitCount
	Remaining []SimUnitCount
}

type SimBattleView struct {
	Winner        string
	Rounds        int
	Seed          int64
	DebrisMetal   int64
	DebrisCrystal int64
	MoonChance    int
	Attacker      SimSideView
	Defender      SimSideView
}

type SimulatorData struct {
	Units   []SimUnit
	Atk     map[string]int64
	Def     map[string]int64
	AtkTech game.CombatTechs
	DefTech game.CombatTechs
	Result  *SimBattleView
	FormErr string
}

func (h *Handler) SimulatorPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Simulator")
	data := h.simulatorData()
	data.Atk = map[string]int64{}
	if counts, err := h.planets.PlanetShips(r.Context(), sh.ActivePlanetID); err == nil {
		for code, n := range counts {
			if n > 0 {
				data.Atk[code] = n
			}
		}
	}
	data.AtkTech = h.playerTechs(r, sh.ActivePlanetID)
	h.render(w, r, http.StatusOK, Page(sh, SimulatorBody(sh, data)))
}

func (h *Handler) SimulatorSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Simulator")
	data := h.simulatorData()
	data.Atk = parseCounts(r, "a_", data.Units)
	data.Def = parseCounts(r, "d_", data.Units)
	data.AtkTech = readTechs(r, "a_")
	data.DefTech = readTechs(r, "d_")

	atk, err := game.NewCombatant(data.Atk, data.AtkTech, nil, nil)
	if err != nil {
		data.FormErr = "Attacker: " + err.Error()
	} else if def, err := game.NewCombatant(data.Def, data.DefTech, nil, nil); err != nil {
		data.FormErr = "Defender: " + err.Error()
	} else {
		res, seed, err := game.SimulateBattle(atk, def, nil)
		if err != nil {
			data.FormErr = err.Error()
		} else {
			data.Result = simBattleView(res, seed)
		}
	}
	h.render(w, r, http.StatusOK, Page(sh, SimulatorBody(sh, data)))
}

// simulatorData builds the unit palette (ships then defenses) that both forms
// render.
func (h *Handler) simulatorData() SimulatorData {
	data := SimulatorData{Atk: map[string]int64{}, Def: map[string]int64{}}
	appendUnits := func(m map[string]game.UnitDef, isDef bool) {
		for _, def := range sortedUnits(m) {
			data.Units = append(data.Units, SimUnit{
				Code: def.Code, Name: def.Name, IsDefense: isDef,
				Atk: def.BaseCost.Metal + def.BaseCost.Crystal,
			})
		}
	}
	appendUnits(game.Ships, false)
	appendUnits(game.Defenses, true)
	return data
}

// playerTechs maps a planet's researched levels onto the combat-tech view used
// to prefill the attacker in the simulator.
func (h *Handler) playerTechs(r *http.Request, planetID int64) game.CombatTechs {
	levels := map[string]int{}
	if rr, err := h.builds.GetResearch(r.Context(), strconv.FormatInt(planetID, 10)); err == nil {
		levels = rr.Levels
	}
	return game.CombatTechs{
		Weapons: levels["weapons_tech"], Shield: levels["shielding_tech"], Armour: levels["armour_tech"],
		Laser: levels["laser_tech"], Ion: levels["ion_tech"], Plasma: levels["plasma_tech"], Graviton: levels["graviton_research"],
	}
}

// parseCounts reads `<prefix><code>` integer form values for every known unit.
func parseCounts(r *http.Request, prefix string, units []SimUnit) map[string]int64 {
	out := map[string]int64{}
	for _, u := range units {
		if v, err := strconv.ParseInt(r.FormValue(prefix+u.Code), 10, 64); err == nil && v > 0 {
			out[u.Code] = v
		}
	}
	return out
}

// readTechs reads the per-side tech inputs (<prefix>weapons, <prefix>shield, …).
func readTechs(r *http.Request, prefix string) game.CombatTechs {
	get := func(name string) int {
		v, _ := strconv.Atoi(r.FormValue(prefix + name))
		if v < 0 {
			v = 0
		}
		return v
	}
	return game.CombatTechs{
		Weapons: get("weapons"), Shield: get("shield"), Armour: get("armour"),
		Laser: get("laser"), Ion: get("ion"), Plasma: get("plasma"), Graviton: get("graviton"),
	}
}

// simBattleView flattens a CombatResult into display-ready, deterministically
// ordered rows.
func simBattleView(res game.CombatResult, seed int64) *SimBattleView {
	return &SimBattleView{
		Winner: res.Winner, Rounds: res.Rounds, Seed: seed,
		DebrisMetal: res.DebrisMetal, DebrisCrystal: res.DebrisCrystal, MoonChance: res.MoonChance,
		Attacker: simSideView(res.Attacker), Defender: simSideView(res.Defender),
	}
}

func simSideView(s game.SideReport) SimSideView {
	rows := func(m map[string]int64) []SimUnitCount {
		out := make([]SimUnitCount, 0, len(m))
		for code, n := range m {
			if n <= 0 {
				continue
			}
			out = append(out, SimUnitCount{Name: displayName(code), Count: n})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out
	}
	return SimSideView{Initial: rows(s.Initial), Lost: rows(s.Lost), Remaining: rows(s.Remaining)}
}

// ---- template helpers ----------------------------------------------------

// bonusStr renders an Arsenal bonus like "+1.5%".
func bonusStr(v float64) string {
	return "+" + strconv.FormatFloat(v, 'f', -1, 64) + "%"
}

// pct renders a 0..1 probability as a whole percent.
func pct(v float64) string {
	return strconv.Itoa(int(v*100+0.5)) + "%"
}

// lotExpirySecs is the remaining lifetime of a market lot, floored at zero.
func lotExpirySecs(l models.MarketLot) int64 {
	s := int64(time.Until(l.ExpiresAt).Seconds())
	if s < 0 {
		return 0
	}
	return s
}

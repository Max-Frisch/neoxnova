package web

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"neoxnova/internal/game"
	"neoxnova/internal/models"
	"neoxnova/internal/store"
)

// Render lets an AuthView be passed directly to render() as a component.
func (a AuthView) Render(ctx context.Context, w io.Writer) error {
	return AuthPage(a).Render(ctx, w)
}

// Shell is the shared page chrome: navigation, planet switcher and resource bar.
type Shell struct {
	Title            string
	Username         string
	Planets          []models.OwnedCelestial
	ActivePlanetID   int64
	ActivePlanetName string
	ActiveCoords     string
	ServerTime       time.Time
	Resources        *models.RealtimeResourceState
	Flash            string
	Error            string
}

// AuthView backs the login/register screens.
type AuthView struct {
	Mode  string
	Error string
	Login string
	Email string
}

func (h *Handler) shell(r *http.Request, userID, activeID int64) Shell {
	return h.shellWithResources(r, userID, activeID, nil)
}

// shellWithResources builds the page chrome. When res is non-nil it is used as
// the active celestial's resource snapshot instead of re-accruing resources, so
// callers that already loaded an overview avoid a second
// update_celestial_resources call. Owned celestials are reused from the
// request-scoped principal set by the page middleware.
func (h *Handler) shellWithResources(r *http.Request, userID, activeID int64, res *models.RealtimeResourceState) Shell {
	s := Shell{ActivePlanetID: activeID, ServerTime: time.Now()}
	if name, _, err := h.auth.UserByID(r.Context(), userID); err == nil {
		s.Username = name
	}
	s.Planets = h.ownedCelestials(r, userID)
	if activeID != 0 {
		idStr := strconv.FormatInt(activeID, 10)
		for _, c := range s.Planets {
			if c.ID == activeID {
				s.ActivePlanetName = c.Name
				s.ActiveCoords = coord3(c.Galaxy, c.System, c.Position)
				break
			}
		}
		if s.ActivePlanetName == "" {
			if name, g, sy, pos, err := h.planets.GetCoordinates(r.Context(), idStr); err == nil {
				s.ActivePlanetName = name
				s.ActiveCoords = coord3(g, sy, pos)
			}
		}
		if res != nil {
			s.Resources = res
		} else if rs, err := h.planets.GetResourceState(r.Context(), idStr); err == nil {
			s.Resources = &rs
		}
	}
	s.Flash, s.Error = flashFor(r)
	return s
}

// ownedCelestials returns the account's planets and moons, reusing the
// request-scoped principal when the page middleware already loaded them.
func (h *Handler) ownedCelestials(r *http.Request, userID int64) []models.OwnedCelestial {
	if p := principalFrom(r); p != nil && p.ownedOK {
		return p.owned
	}
	planets, _ := h.planets.ListOwnedCelestials(r.Context(), userID)
	return planets
}

// homeworldID returns the account's oldest planet id (0 when none exists),
// served from the principal's owned list when available.
func (h *Handler) homeworldID(r *http.Request, userID int64) int64 {
	if p := principalFrom(r); p != nil && p.ownedOK {
		for _, c := range p.owned {
			if c.IsHome {
				return c.ID
			}
		}
		return 0
	}
	id, _ := h.planets.HomeworldID(r.Context(), userID)
	return id
}

// findOwned looks up an owned celestial by id in the principal snapshot.
func (h *Handler) findOwned(r *http.Request, userID, id int64) (models.OwnedCelestial, bool) {
	for _, c := range h.ownedCelestials(r, userID) {
		if c.ID == id {
			return c, true
		}
	}
	return models.OwnedCelestial{}, false
}

// ---- Overview ------------------------------------------------------------

type OverviewData struct {
	Overview   models.PlanetOverviewResponse
	BuildQueue *models.QueueEntrySummary
	ResQueue   *models.QueueEntrySummary
	ShipQueue  *models.QueueEntrySummary
	Incoming   []models.IncomingFleet
}

func (h *Handler) OverviewPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	idStr := r.PathValue("id")

	data := OverviewData{}
	// GetOverview already accrues and row-locks the celestial's resources, so
	// hand its snapshot to the shell instead of re-running
	// update_celestial_resources a second time.
	var snapshot *models.RealtimeResourceState
	if ov, err := h.planets.GetOverview(r.Context(), idStr); err == nil {
		data.Overview = ov
		res := ov.Resources
		snapshot = &res
	}
	sh := h.shellWithResources(r, userID, id, snapshot)
	sh.Title = "Overview"
	if snapshot != nil {
		sh.ActivePlanetName = data.Overview.ActivePlanetName
		sh.ActiveCoords = coordStr(data.Overview.Coordinates)
	}
	if b, err := h.builds.GetBuildings(r.Context(), idStr); err == nil {
		data.BuildQueue = b.Queue
	}
	if rr, err := h.builds.GetResearch(r.Context(), idStr); err == nil {
		data.ResQueue = rr.Queue
	}
	data.ShipQueue = h.shipyardQueue(r, idStr)
	if in, err := h.fleets.IncomingFleets(r.Context(), id, userID); err == nil {
		data.Incoming = in
	}
	h.render(w, r, http.StatusOK, Page(sh, OverviewBody(sh, data)))
}

// ResourcesPartial is the htmx-polled resource bar fragment.
func (h *Handler) ResourcesPartial(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	sh := h.shell(r, userID, id)
	h.render(w, r, http.StatusOK, ResourceBar(sh))
}

// ---- Resources (production breakdown) ------------------------------------

// ResourcesData backs the resources page: a per-source production table for one
// celestial.
type ResourcesData struct {
	Planet    string
	Breakdown game.ProductionReport
}

// ResourcesPage renders the I4 resources breakdown (metal/crystal/deuterium
// production per mine, plant and satellite, the account bonus, storage capacity
// and the reconciled total/daily/weekly figures).
func (h *Handler) ResourcesPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	sh := h.shell(r, userID, id)
	sh.Title = "Resources"
	idStr := r.PathValue("id")

	var tempMax int
	var objectType string
	resourceSpeed := 1.0
	if err := h.db.QueryRowContext(r.Context(), `
		SELECT c.temp_max, c.object_type::text, u.resource_speed
		FROM celestial_objects c
		JOIN universes u ON u.id = c.universe_id
		WHERE c.id = $1
	`, id).Scan(&tempMax, &objectType, &resourceSpeed); err != nil {
		http.Error(w, "resources failed", http.StatusInternalServerError)
		return
	}
	levels := map[string]int{}
	if b, err := h.builds.GetBuildings(r.Context(), idStr); err == nil {
		levels = b.Levels
	}
	researchLevels := map[string]int{}
	if rr, err := h.builds.GetResearch(r.Context(), idStr); err == nil {
		researchLevels = rr.Levels
	}
	satCount := 0
	if counts, err := h.planets.PlanetShips(r.Context(), id); err == nil {
		satCount = int(counts["212"])
	}
	bonus := h.productionBonus(r, userID, researchLevels)

	data := ResourcesData{Planet: sh.ActivePlanetName}
	data.Breakdown = game.ProductionBreakdown(levels, tempMax, resourceSpeed, satCount, bonus, objectType == "PLANET")
	h.render(w, r, http.StatusOK, Page(sh, ResourcesBody(sh, data)))
}

// productionBonus combines an account's Arsenal production upgrades (17/18/19)
// with its extraction researches (Mineral/Semi-Crystals/Fuel) into one
// percentage bonus per resource.
func (h *Handler) productionBonus(r *http.Request, userID int64, researchLevels map[string]int) game.ProductionBonus {
	upgrades := map[int]float64{}
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT upgrade_code, value FROM account_upgrades
		WHERE account_id = $1 AND value > 0
	`, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var code int
			var value float64
			if rows.Scan(&code, &value) == nil {
				upgrades[code] = value
			}
		}
	}
	return game.ProductionBonusFor(upgrades).Plus(game.ResearchProductionBonus(researchLevels))
}

// ---- Buildings -----------------------------------------------------------

// Prereq is one unmet requirement shown in a locked card's "Necessary for the
// construction" block.
type Prereq struct {
	Code     string
	Name     string
	Required int
	Current  int
	Image    string
}

type BuildItem struct {
	Code         string
	ID           int
	Name         string
	Level        int
	Cost         models.CargoManifest
	Unlocked     bool
	Reason       string
	Image        string
	EnergyDelta  float64
	DurationSecs int64
	Prereqs      []Prereq
	Busy         bool
}

type BuildingsData struct {
	Items      []BuildItem
	Queue      *models.QueueEntrySummary
	FieldsUsed int
	FieldsMax  int
	IsMoon     bool
}

func (h *Handler) BuildingsPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	sh := h.shell(r, userID, id)
	sh.Title = "Buildings"
	idStr := r.PathValue("id")

	data := BuildingsData{}
	levels := map[string]int{}
	if b, err := h.builds.GetBuildings(r.Context(), idStr); err == nil {
		levels = b.Levels
		data.Queue = b.Queue
	}
	if ov, err := h.planets.GetOverview(r.Context(), idStr); err == nil {
		data.FieldsUsed, data.FieldsMax = ov.FieldsUsed, ov.FieldsMax
	}
	ctype, _ := h.planets.CelestialType(r.Context(), idStr)
	data.IsMoon = ctype == models.TypeMoon
	speed := h.gameSpeed(r)
	robot, nanite := levels["robotics_factory"], levels["nanite_factory"]
	defs := sortedStructures()
	if data.IsMoon {
		defs = game.MoonStructureDefs()
	}
	for _, def := range defs {
		item := BuildItem{
			Code: def.Code, ID: def.ID, Name: def.Name, Level: levels[def.Code],
			Image:   assetURL(def.Code),
			Prereqs: prereqList(def.Requires, levels),
		}
		if c, ok := game.StructureCost(def.Code, item.Level+1); ok {
			item.Cost = models.CargoManifest{Metal: c.Metal, Crystal: c.Crystal, Deuterium: c.Deuterium}
		}
		item.Unlocked, item.Reason = reqOK(def.Requires, levels)
		item.EnergyDelta = game.StructureEnergyDelta(def.Code, item.Level+1)
		item.DurationSecs = int64(game.StructureDuration(def.Code, item.Level+1, robot, nanite, speed).Seconds())
		item.Busy = data.Queue != nil && data.Queue.Code == def.Code
		data.Items = append(data.Items, item)
	}
	h.render(w, r, http.StatusOK, Page(sh, BuildingsBody(sh, data)))
}

// BuildCancelSubmit cancels the active construction on a celestial and refunds
// its cost (reskin I2).
func (h *Handler) BuildCancelSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	back := "/buildings/" + r.PathValue("id")
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	if _, err := h.builds.CancelQueue(r.Context(), store.CancelConstruction, id); err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	h.nudge()
	h.redirect(w, r, back, "Construction cancelled, resources refunded", "")
}

func (h *Handler) BuildSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	code := r.FormValue("code")
	_, err := h.builds.EnqueueStructure(r.Context(), id, code)
	if err != nil {
		h.redirect(w, r, "/buildings/"+r.PathValue("id"), "", err.Error())
		return
	}
	h.nudge()
	h.redirect(w, r, "/buildings/"+r.PathValue("id"), "Queued "+code, "")
}

// ---- Shipyard / Defenses -------------------------------------------------

// UnitItem is one buildable ship/defense card in the hangar/defenses grids.
type UnitItem struct {
	Code         string
	Name         string
	Count        int64
	Cost         models.CargoManifest
	Unlocked     bool
	Reason       string
	Image        string
	DurationSecs int64
	Prereqs      []Prereq
	Busy         bool
	MaxQty       int64
}

// UnitSection groups cards under a live-client band (Recently built, Small
// class, ...).
type UnitSection struct {
	Title string
	Items []UnitItem
}

type ShipyardData struct {
	Sections []UnitSection
	Queue    *models.QueueEntrySummary
}

// unitClassDef is one fixed shipyard band captured from the live client.
type unitClassDef struct {
	Title string
	Codes []string
}

// The bands mirror the live shipyard/defenses pages (captured 2026-10-15);
// within a band the codes keep the reference server's display order.
var shipClasses = []unitClassDef{
	{"Small class", []string{"212", "202", "203", "204", "205"}},
	{"Medium class", []string{"209", "206", "207", "217", "215", "213", "211", "219"}},
	{"Large class", []string{"225", "226", "214", "216", "227", "228", "222", "218", "221"}},
	{"Special", []string{"210", "208", "220", "223"}},
}

var defenseClasses = []unitClassDef{
	{"Small class", []string{"401", "402", "403", "405", "404", "406", "416", "417", "502", "503"}},
	{"Medium class", []string{"418", "412", "410", "413", "419", "414", "415"}},
	{"Large class", []string{"407", "408", "409", "411"}},
}

// recentBuiltLimit matches the live client's "Recently built" band length.
const recentBuiltLimit = 3

func (h *Handler) ShipyardPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	sh := h.shell(r, userID, id)
	sh.Title = "Shipyard"
	data := h.unitPageData(r, sh, id, r.PathValue("id"), shipClasses)
	h.render(w, r, http.StatusOK, Page(sh, ShipyardBody(sh, data)))
}

func (h *Handler) DefensesPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	sh := h.shell(r, userID, id)
	sh.Title = "Defenses"
	data := h.unitPageData(r, sh, id, r.PathValue("id"), defenseClasses)
	h.render(w, r, http.StatusOK, Page(sh, DefensesBody(sh, data)))
}

// unitPageData assembles the grouped ship/defense cards for a celestial. The
// "Recently built" band is derived from the last completed shipyard queues and
// its units are pulled out of their class band, matching the live client.
func (h *Handler) unitPageData(r *http.Request, sh Shell, id int64, idStr string, classes []unitClassDef) ShipyardData {
	levels := h.allLevels(r, idStr, id)
	counts, _ := h.planets.PlanetShips(r.Context(), id)
	queue := h.shipyardQueue(r, idStr)
	speed := h.gameSpeed(r)
	shipyard, nanite := levels["shipyard"], levels["nanite_factory"]

	recent := h.recentlyBuiltUnits(r, id, recentBuiltLimit)
	recentSet := make(map[string]bool, len(recent))
	// The shipyard and defenses pages share the queue, so only keep recent
	// codes that belong to this page's catalog.
	allowed := make(map[string]bool)
	for _, class := range classes {
		for _, code := range class.Codes {
			allowed[code] = true
		}
	}
	recentCodes := make([]string, 0, len(recent))
	for _, code := range recent {
		if allowed[code] {
			recentCodes = append(recentCodes, code)
			recentSet[code] = true
		}
	}
	sections := make([]UnitSection, 0, len(classes)+1)
	addSection := func(title string, codes []string) {
		items := make([]UnitItem, 0, len(codes))
		for _, code := range codes {
			def, ok := game.LookupUnit(code)
			if !ok {
				continue
			}
			items = append(items, buildUnitItem(def, counts[code], levels, sh.Resources, shipyard, nanite, speed, queue))
		}
		if len(items) > 0 {
			sections = append(sections, UnitSection{Title: title, Items: items})
		}
	}
	if len(recentCodes) > 0 {
		addSection("Recently built", recentCodes)
	}
	for _, class := range classes {
		codes := make([]string, 0, len(class.Codes))
		for _, code := range class.Codes {
			if !recentSet[code] {
				codes = append(codes, code)
			}
		}
		addSection(class.Title, codes)
	}
	return ShipyardData{Sections: sections, Queue: queue}
}

func buildUnitItem(def game.UnitDef, count int64, levels map[string]int, res *models.RealtimeResourceState, shipyard, nanite int, speed float64, queue *models.QueueEntrySummary) UnitItem {
	item := UnitItem{
		Code: def.Code, Name: def.Name, Count: count, Cost: unitManifest(def),
		Image:   assetURL(def.Code),
		Prereqs: prereqList(def.Requires, levels),
	}
	item.Unlocked, item.Reason = reqOK(def.Requires, levels)
	item.DurationSecs = int64(game.UnitDuration(def.Code, 1, shipyard, nanite, speed).Seconds())
	item.Busy = queue != nil && queue.Code == def.Code
	item.MaxQty = maxAffordable(res, item.Cost)
	return item
}

// recentlyBuiltUnits lists the distinct unit codes of the most recently
// completed shipyard queues for a celestial, newest first.
func (h *Handler) recentlyBuiltUnits(r *http.Request, id int64, limit int) []string {
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT unit_code FROM (
			SELECT DISTINCT ON (unit_code) unit_code, end_time
			FROM shipyard_queues
			WHERE celestial_id = $1 AND status = 'COMPLETED'
			ORDER BY unit_code, end_time DESC
		) t
		ORDER BY end_time DESC
		LIMIT $2
	`, id, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err == nil {
			out = append(out, code)
		}
	}
	return out
}

// maxAffordable is the largest quantity of a unit the celestial can pay for at
// once; 0 when the cost is unknown or unaffordable.
func maxAffordable(res *models.RealtimeResourceState, cost models.CargoManifest) int64 {
	if res == nil {
		return 0
	}
	limit := int64(math.MaxInt64)
	if cost.Metal > 0 {
		limit = min(limit, int64(res.MetalCurrent)/cost.Metal)
	}
	if cost.Crystal > 0 {
		limit = min(limit, int64(res.CrystalCurrent)/cost.Crystal)
	}
	if cost.Deuterium > 0 {
		limit = min(limit, int64(res.DeutCurrent)/cost.Deuterium)
	}
	if limit == int64(math.MaxInt64) || limit < 0 {
		return 0
	}
	return limit
}

func (h *Handler) ShipyardSubmit(w http.ResponseWriter, r *http.Request) {
	h.unitSubmit(w, r, "/shipyard/"+r.PathValue("id"))
}

func (h *Handler) DefensesSubmit(w http.ResponseWriter, r *http.Request) {
	h.unitSubmit(w, r, "/defenses/"+r.PathValue("id"))
}

func (h *Handler) unitSubmit(w http.ResponseWriter, r *http.Request, back string) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	code := r.FormValue("code")
	qty, _ := strconv.ParseInt(r.FormValue("quantity"), 10, 64)
	if qty < 1 {
		qty = 1
	}
	if _, err := h.builds.EnqueueShipyard(r.Context(), id, code, qty); err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	h.nudge()
	h.redirect(w, r, back, "Queued "+code, "")
}

// ShipyardCancelSubmit cancels the active ship/defense build and refunds its
// cost (reskin I3).
func (h *Handler) ShipyardCancelSubmit(w http.ResponseWriter, r *http.Request) {
	h.unitCancel(w, r, "/shipyard/"+r.PathValue("id"))
}

func (h *Handler) DefensesCancelSubmit(w http.ResponseWriter, r *http.Request) {
	h.unitCancel(w, r, "/defenses/"+r.PathValue("id"))
}

func (h *Handler) unitCancel(w http.ResponseWriter, r *http.Request, back string) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	if _, err := h.builds.CancelQueue(r.Context(), store.CancelShipyard, id); err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	h.nudge()
	h.redirect(w, r, back, "Build cancelled, resources refunded", "")
}

// ---- Research ------------------------------------------------------------

type ResearchItem struct {
	Code     string
	ID       int
	Name     string
	Level    int
	Cost     models.CargoManifest
	Unlocked bool
	Reason   string
	Image    string
	Prereqs  []Prereq
	Busy     bool
}

type ResearchData struct {
	Items []ResearchItem
	Queue *models.QueueEntrySummary
}

func (h *Handler) ResearchPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	sh := h.shell(r, userID, id)
	sh.Title = "Research"
	idStr := r.PathValue("id")

	data := ResearchData{}
	levels := map[string]int{}
	if rr, err := h.builds.GetResearch(r.Context(), idStr); err == nil {
		levels = rr.Levels
		data.Queue = rr.Queue
	}
	// Building levels are needed because some techs require a research lab etc.
	structLevels := map[string]int{}
	if b, err := h.builds.GetBuildings(r.Context(), idStr); err == nil {
		structLevels = b.Levels
	}
	all := mergeLevels(structLevels, levels)
	for _, def := range sortedTechs() {
		item := ResearchItem{
			Code: def.Code, ID: def.ID, Name: def.Name, Level: levels[def.Code],
			Image:   assetURL(def.Code),
			Prereqs: prereqList(def.Requires, all),
		}
		if c, ok := game.TechCost(def.Code, item.Level+1); ok {
			item.Cost = models.CargoManifest{Metal: c.Metal, Crystal: c.Crystal, Deuterium: c.Deuterium}
		}
		item.Unlocked, item.Reason = reqOK(def.Requires, all)
		item.Busy = data.Queue != nil && data.Queue.Code == def.Code
		data.Items = append(data.Items, item)
	}
	h.render(w, r, http.StatusOK, Page(sh, ResearchBody(sh, data)))
}

// ResearchCancelSubmit cancels the active empire-wide research and refunds its
// cost to the planet it was started from (reskin I2).
func (h *Handler) ResearchCancelSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	back := "/research/" + r.PathValue("id")
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	if _, err := h.builds.CancelQueue(r.Context(), store.CancelResearch, userID); err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	h.nudge()
	h.redirect(w, r, back, "Research cancelled, resources refunded", "")
}

func (h *Handler) ResearchSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	code := r.FormValue("code")
	if _, err := h.builds.EnqueueResearch(r.Context(), id, code); err != nil {
		h.redirect(w, r, "/research/"+r.PathValue("id"), "", err.Error())
		return
	}
	h.nudge()
	h.redirect(w, r, "/research/"+r.PathValue("id"), "Researching "+code, "")
}

// ---- Fleet ---------------------------------------------------------------

type FleetShip struct {
	Code  string
	Name  string
	Count int64
}

type FleetData struct {
	Fleets     []models.FleetEventSummary
	Incoming   []models.IncomingFleet
	Ships      []FleetShip
	TargetG    int
	TargetS    int
	TargetP    int
	Mission    string
	TargetType string
}

func (h *Handler) FleetPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	sh := h.shell(r, userID, id)
	sh.Title = "Fleet"
	idStr := r.PathValue("id")

	data := FleetData{
		TargetG:    atoiDefault(r.URL.Query().Get("g"), 1),
		TargetS:    atoiDefault(r.URL.Query().Get("s"), 1),
		TargetP:    atoiDefault(r.URL.Query().Get("p"), 1),
		Mission:    strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("mission"))),
		TargetType: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("type"))),
	}
	if data.TargetG == 0 {
		data.TargetG = 1
	}
	if data.TargetS == 0 {
		data.TargetS = 1
	}
	if data.TargetP == 0 {
		data.TargetP = 1
	}
	if ov, err := h.planets.GetOverview(r.Context(), idStr); err == nil {
		data.Fleets = ov.ActiveFleets
	}
	if in, err := h.fleets.IncomingFleets(r.Context(), id, userID); err == nil {
		data.Incoming = in
	}
	counts, _ := h.planets.PlanetShips(r.Context(), id)
	for _, def := range sortedUnits(game.Ships) {
		if counts[def.Code] > 0 {
			data.Ships = append(data.Ships, FleetShip{Code: def.Code, Name: def.Name, Count: counts[def.Code]})
		}
	}
	h.render(w, r, http.StatusOK, Page(sh, FleetBody(sh, data)))
}

func (h *Handler) FleetDispatchSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	back := "/fleet/" + r.PathValue("id")
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	_ = r.ParseForm()

	missStr := strings.ToUpper(strings.TrimSpace(r.FormValue("mission")))
	target := models.Coordinates{
		Galaxy:   atoiDefault(r.FormValue("galaxy"), 1),
		System:   atoiDefault(r.FormValue("system"), 1),
		Position: atoiDefault(r.FormValue("position"), 1),
		Type:     models.CelestialType(orDefault(r.FormValue("target_type"), "PLANET")),
	}
	ships := map[string]int64{}
	for code := range game.Ships {
		if v, err := strconv.ParseInt(r.FormValue("ship_"+code), 10, 64); err == nil && v > 0 {
			ships[code] = v
		}
	}
	req := models.FleetDispatchRequest{
		OriginPlanetID: id,
		Target:         target,
		Mission:        models.MissionType(missStr),
		Ships:          ships,
		SpeedPercent:   atoiDefault(r.FormValue("speed_percent"), 100),
		HoldingHours:   atoiDefault(r.FormValue("holding_hours"), 0),
		Cargo: models.CargoManifest{
			Metal:     int64Default(r.FormValue("cargo_metal")),
			Crystal:   int64Default(r.FormValue("cargo_crystal")),
			Deuterium: int64Default(r.FormValue("cargo_deuterium")),
		},
	}
	if len(ships) == 0 {
		h.redirect(w, r, back, "", "Select at least one ship")
		return
	}
	if _, err := h.fleets.Dispatch(r.Context(), req); err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	h.nudge()
	h.redirect(w, r, back, "Fleet dispatched", "")
}

func (h *Handler) FleetRecallSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	back := "/fleet/" + r.PathValue("id")
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	fleetID, err := strconv.ParseInt(r.FormValue("fleet_id"), 10, 64)
	if err != nil {
		h.redirect(w, r, back, "", "Invalid fleet")
		return
	}
	if _, err := h.fleets.Recall(r.Context(), fleetID); err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	h.nudge()
	h.redirect(w, r, back, "Fleet recalled", "")
}

// ---- Galaxy --------------------------------------------------------------

func (h *Handler) GalaxyRedirect(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	// Explicit query coordinates win; otherwise default to the homeworld system.
	if gq := r.URL.Query().Get("g"); gq != "" {
		g := atoiDefault(gq, 1)
		s := atoiDefault(r.URL.Query().Get("s"), 1)
		if g >= 1 && g <= 9 && s >= 1 && s <= 499 {
			http.Redirect(w, r, fmt.Sprintf("/galaxy/%d/%d", g, s), http.StatusSeeOther)
			return
		}
	}
	g, s := 1, 1
	if homeID := h.homeworldID(r, userID); homeID != 0 {
		if c, ok := h.findOwned(r, userID, homeID); ok {
			g, s = c.Galaxy, c.System
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/galaxy/%d/%d", g, s), http.StatusSeeOther)
}

// GalaxyStats is the footer summary. Some live-client stats have no engine
// source (Giga Recycler); they are shown as zero and noted under the table
// rather than hidden.
type GalaxyStats struct {
	ExistingPlanets   int
	AvailableMissiles int64
	GigaRecycler      int64
	FleetSlotsActive  int
	FleetSlotsTotal   int
	SpyProbes         int64
	Recycler          int64
}

type GalaxyData struct {
	Scan  models.GalaxyScanResponse
	Stats GalaxyStats
}

func (h *Handler) GalaxyPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	g, errG := strconv.Atoi(r.PathValue("galaxy"))
	s, errS := strconv.Atoi(r.PathValue("system"))
	if errG != nil || errS != nil || g < 1 || g > 9 || s < 1 || s > 499 {
		http.NotFound(w, r)
		return
	}
	// Use any owned celestial to anchor the planet switcher.
	activeID := h.homeworldID(r, userID)
	sh := h.shell(r, userID, activeID)
	sh.Title = fmt.Sprintf("Galaxy %d:%d", g, s)
	scan, err := h.planets.GalaxyScan(r.Context(), h.universeID, g, s, userID)
	if err != nil {
		http.Error(w, "scan failed", http.StatusInternalServerError)
		return
	}
	data := GalaxyData{Scan: scan}
	data.Stats = h.galaxyStats(r, userID, activeID, scan)
	h.render(w, r, http.StatusOK, Page(sh, GalaxyBody(sh, data)))
}

// galaxyStats derives the galaxy footer: planet count from the scan and the
// active planet's probes/recyclers/missiles from its hangar. Fleet slots follow
// the classic 1 + computer-technology rule (active outbound fleets as the
// numerator).
func (h *Handler) galaxyStats(r *http.Request, userID, activeID int64, scan models.GalaxyScanResponse) GalaxyStats {
	var st GalaxyStats
	for _, slot := range scan.Slots {
		if slot.Occupied && slot.Type == models.TypePlanet {
			st.ExistingPlanets++
		}
	}
	if activeID != 0 {
		if counts, err := h.planets.PlanetShips(r.Context(), activeID); err == nil {
			st.AvailableMissiles = counts["502"]
			st.SpyProbes = counts["210"]
			st.Recycler = counts["209"]
		}
		if ov, err := h.planets.GetOverview(r.Context(), strconv.FormatInt(activeID, 10)); err == nil {
			st.FleetSlotsActive = len(ov.ActiveFleets)
		}
	}
	st.FleetSlotsTotal = 1
	if rr, err := h.builds.GetResearch(r.Context(), strconv.FormatInt(activeID, 10)); err == nil {
		st.FleetSlotsTotal += rr.Levels["computer_tech"]
	}
	return st
}

// ---- Messages (categorized reports) --------------------------------------

// MessageCategory is one cell of the live Messages category grid. Backed
// categories (Spy/Combat/Expedition/All) carry real counts and link to the
// filtered list; the rest render as disabled "coming soon" cells.
type MessageCategory struct {
	Key    string
	Label  string
	Color  string
	Count  int
	Unread int
	Href   string
	Backed bool
}

type ReportsData struct {
	Categories []MessageCategory
	Active     string
	Combat     []store.CombatReport
	Espionage  []store.EspionageReport
	Expedition []store.ExpeditionReport
}

// messageCategories is the grid captured from page=messages, in display order.
// Backed keys map to the reports store; the others have no engine source yet.
var messageCategories = []struct {
	Key, Label, Color string
	Backed            bool
}{
	{"spy", "Spy messages", "msg-spy", true},
	{"player", "Player messages", "msg-player", false},
	{"alliance", "Alliance messages", "msg-alliance", false},
	{"combat", "Combat messages", "msg-combat", true},
	{"system", "System messages", "msg-system", false},
	{"transport", "Transport messages", "msg-transport", false},
	{"expedition", "Expedition messages", "msg-expedition", true},
	{"game", "Game messages", "msg-game", false},
	{"construction", "Construction messages", "msg-construction", false},
	{"all", "All messages", "msg-all", true},
	{"archive", "Archive", "msg-archive", false},
	{"outbox", "Outbox", "msg-outbox", false},
}

// messageCategory validates the ?category= key, defaulting to All messages.
func messageCategory(key string) string {
	for _, c := range messageCategories {
		if c.Key == key && c.Backed {
			return key
		}
	}
	return "all"
}

// messageCategoryGrid builds the category cells, wiring real counts and links
// into the backed categories (base is the reports URL prefix).
func messageCategoryGrid(base string, counts map[string]int) []MessageCategory {
	out := make([]MessageCategory, 0, len(messageCategories))
	for _, c := range messageCategories {
		cat := MessageCategory{Key: c.Key, Label: c.Label, Color: c.Color, Backed: c.Backed}
		if c.Backed {
			cat.Count = counts[c.Key]
			cat.Href = base + c.Key
		}
		out = append(out, cat)
	}
	return out
}

func (h *Handler) ReportsPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	sh := h.shell(r, userID, id)
	sh.Title = "Messages"

	combat, _ := h.combat.ListForCelestial(r.Context(), id, 100)
	espionage, _ := h.espionage.ListForCelestial(r.Context(), id, 100)
	expedition, _ := h.expedition.ListForCelestial(r.Context(), id, 100)

	active := messageCategory(r.URL.Query().Get("category"))
	data := ReportsData{Active: active}
	counts := map[string]int{
		"spy": len(espionage), "combat": len(combat), "expedition": len(expedition),
		"all": len(combat) + len(espionage) + len(expedition),
	}
	data.Categories = messageCategoryGrid("/reports/"+r.PathValue("id")+"?category=", counts)
	switch active {
	case "spy":
		data.Espionage = espionage
	case "combat":
		data.Combat = combat
	case "expedition":
		data.Expedition = expedition
	default:
		data.Combat, data.Espionage, data.Expedition = combat, espionage, expedition
	}
	h.render(w, r, http.StatusOK, Page(sh, MessagesBody(sh, data)))
}

// ---- helpers -------------------------------------------------------------

func (h *Handler) ownsCelestial(r *http.Request, id, userID int64) bool {
	if p := principalFrom(r); p != nil && p.ownedOK {
		for _, c := range p.owned {
			if c.ID == id {
				return true
			}
		}
		return false
	}
	var owner int64
	err := h.db.QueryRowContext(r.Context(), `SELECT user_id FROM celestial_objects WHERE id = $1`, id).Scan(&owner)
	return err == nil && owner == userID
}

func (h *Handler) redirect(w http.ResponseWriter, r *http.Request, base, ok, errMsg string) {
	u, _ := url.Parse(base)
	q := u.Query()
	if ok != "" {
		q.Set("ok", ok)
	}
	if errMsg != "" {
		q.Set("err", errMsg)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}

func (h *Handler) shipyardQueue(r *http.Request, idStr string) *models.QueueEntrySummary {
	var q models.QueueEntrySummary
	err := h.db.QueryRowContext(r.Context(), `
		SELECT id, unit_code, quantity, start_time, end_time
		FROM shipyard_queues
		WHERE celestial_id = $1 AND status = 'IN_PROGRESS'
		LIMIT 1
	`, idStr).Scan(&q.ID, &q.Code, &q.Quantity, &q.StartTime, &q.EndTime)
	if err != nil {
		return nil
	}
	q.RemainingSecs = int64(time.Until(q.EndTime).Seconds())
	if q.RemainingSecs < 0 {
		q.RemainingSecs = 0
	}
	return &q
}

func (h *Handler) allLevels(r *http.Request, idStr string, userID int64) map[string]int {
	out := map[string]int{}
	if b, err := h.builds.GetBuildings(r.Context(), idStr); err == nil {
		for k, v := range b.Levels {
			out[k] = v
		}
	}
	if rr, err := h.builds.GetResearch(r.Context(), idStr); err == nil {
		for k, v := range rr.Levels {
			out[k] = v
		}
	}
	return out
}

func sortedStructures() []game.StructureDef {
	out := make([]game.StructureDef, 0, len(game.Structures))
	for _, d := range game.Structures {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sortedTechs() []game.TechDef {
	out := make([]game.TechDef, 0, len(game.Techs))
	for _, d := range game.Techs {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sortedUnits(m map[string]game.UnitDef) []game.UnitDef {
	out := make([]game.UnitDef, 0, len(m))
	for _, d := range m {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

func unitManifest(d game.UnitDef) models.CargoManifest {
	return models.CargoManifest{Metal: d.BaseCost.Metal, Crystal: d.BaseCost.Crystal, Deuterium: d.BaseCost.Deuterium}
}

func reqOK(requires, levels map[string]int) (bool, string) {
	if game.RequiresMet(requires, levels) {
		return true, ""
	}
	return false, reasonFor(requires, levels)
}

func reasonFor(requires, levels map[string]int) string {
	var missing []string
	for code, lvl := range requires {
		if levels[code] < lvl {
			missing = append(missing, fmt.Sprintf("%s %d", displayName(code), lvl))
		}
	}
	sort.Strings(missing)
	return "Requires " + strings.Join(missing, ", ")
}

func displayName(code string) string {
	if d, ok := game.Structures[code]; ok {
		return d.Name
	}
	if d, ok := game.MoonOnlyStructures[code]; ok {
		return d.Name
	}
	if d, ok := game.Techs[code]; ok {
		return d.Name
	}
	if d, ok := game.LookupUnit(code); ok {
		return d.Name
	}
	return code
}

// assetURL resolves a catalog code to its captured Niburu sprite, mirroring the
// grouping used by static/niburu/README.md.
func assetURL(code string) string {
	switch {
	case isStructure(code):
		return "/static/niburu/buildings/" + code + ".gif"
	case isTech(code):
		return "/static/niburu/research/" + code + ".gif"
	case isShip(code):
		return "/static/niburu/ships/" + code + ".gif"
	case isDefense(code):
		return "/static/niburu/defenses/" + code + ".gif"
	default:
		return ""
	}
}

func isStructure(code string) bool {
	if _, ok := game.Structures[code]; ok {
		return true
	}
	_, ok := game.MoonOnlyStructures[code]
	return ok
}
func isTech(code string) bool    { _, ok := game.Techs[code]; return ok }
func isShip(code string) bool    { _, ok := game.Ships[code]; return ok }
func isDefense(code string) bool { _, ok := game.Defenses[code]; return ok }

// prereqList renders the unmet requirements of a locked card, sorted by code so
// the icons stay stable between renders.
func prereqList(requires, levels map[string]int) []Prereq {
	out := make([]Prereq, 0, len(requires))
	for code, need := range requires {
		out = append(out, Prereq{Code: code, Name: displayName(code), Required: need, Current: levels[code], Image: assetURL(code)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// gameSpeed reads the universe's build-speed multiplier; it degrades to 1.0 if
// the universe row is missing so pages still render.
func (h *Handler) gameSpeed(r *http.Request) float64 {
	var speed float64
	if err := h.db.QueryRowContext(r.Context(), `SELECT game_speed FROM universes WHERE code_name = $1`, h.universeID).Scan(&speed); err != nil || speed <= 0 {
		return 1.0
	}
	return speed
}

func mergeLevels(maps ...map[string]int) map[string]int {
	out := map[string]int{}
	for _, m := range maps {
		for k, v := range m {
			if v > out[k] {
				out[k] = v
			}
		}
	}
	return out
}

func atoiDefault(s string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return v
	}
	return def
}

func int64Default(s string) int64 {
	if v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
		return v
	}
	return 0
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// fnum formats an integer with thousands separators.
func fnum(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func fnumF(f float64) string {
	return fnum(int64(f))
}

// endTimeStr renders a queue entry's completion wall-clock time the way the
// live client shows it under the queue list.
func endTimeStr(t time.Time) string {
	return t.Format("02 Jan 2006, 15:04:05")
}

// hms renders a count of seconds as a fixed HH:MM:SS clock (queue counters).
func hms(secs int64) string {
	if secs < 0 {
		secs = 0
	}
	h := secs / 3600
	m := (secs % 3600) / 60
	s := secs % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// dur renders a count of seconds as Hh Mm Ss.
func dur(secs int64) string {
	if secs < 0 {
		secs = 0
	}
	d := time.Duration(secs) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func coordStr(c models.Coordinates) string {
	return fmt.Sprintf("%d:%d:%d", c.Galaxy, c.System, c.Position)
}

func coord3(g, s, p int) string {
	return fmt.Sprintf("%d:%d:%d", g, s, p)
}

func costStr(c models.CargoManifest) string {
	parts := make([]string, 0, 3)
	if c.Metal > 0 {
		parts = append(parts, fnum(c.Metal)+" M")
	}
	if c.Crystal > 0 {
		parts = append(parts, fnum(c.Crystal)+" C")
	}
	if c.Deuterium > 0 {
		parts = append(parts, fnum(c.Deuterium)+" D")
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " · ")
}

// fleetLink deep-links to the dispatch form with target coordinates prefilled.
// The active planet (origin) is the shell's ActivePlanetID.
func fleetLink(sh Shell, g, s, p int, targetType string) string {
	return fleetLinkMission(sh, g, s, p, targetType, "")
}

// fleetLinkMission is fleetLink with an additional mission preselected (the
// galaxy spy icon deep-links ESPIONAGE, colonize/recycle links their mission).
func fleetLinkMission(sh Shell, g, s, p int, targetType, mission string) string {
	link := "/fleet/" + pid(sh) +
		"?g=" + strconv.Itoa(g) +
		"&s=" + strconv.Itoa(s) +
		"&p=" + strconv.Itoa(p) +
		"&type=" + url.QueryEscape(targetType)
	if mission != "" {
		link += "&mission=" + url.QueryEscape(mission)
	}
	return link
}

// galaxyURL clamps galaxy/system into the valid coordinate domains so the
// stepper arrows never send a visitor to a 404.
func galaxyURL(g, s int) string {
	if g < 1 {
		g = 1
	} else if g > 9 {
		g = 9
	}
	if s < 1 {
		s = 1
	} else if s > 499 {
		s = 499
	}
	return "/galaxy/" + strconv.Itoa(g) + "/" + strconv.Itoa(s)
}

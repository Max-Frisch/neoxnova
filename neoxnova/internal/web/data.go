package web

import (
	"context"
	"fmt"
	"io"
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
	s := Shell{ActivePlanetID: activeID}
	if name, _, err := h.auth.UserByID(r.Context(), userID); err == nil {
		s.Username = name
	}
	if planets, err := h.planets.ListOwnedCelestials(r.Context(), userID); err == nil {
		s.Planets = planets
	}
	if activeID != 0 {
		idStr := strconv.FormatInt(activeID, 10)
		if name, _, _, _, err := h.planets.GetCoordinates(r.Context(), idStr); err == nil {
			s.ActivePlanetName = name
		}
		if res, err := h.planets.GetResourceState(r.Context(), idStr); err == nil {
			s.Resources = &res
		}
	}
	s.Flash, s.Error = flashFor(r)
	return s
}

// ---- Overview ------------------------------------------------------------

type OverviewData struct {
	Overview   models.PlanetOverviewResponse
	BuildQueue *models.QueueEntrySummary
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
	sh := h.shell(r, userID, id)
	sh.Title = "Overview"
	idStr := r.PathValue("id")

	data := OverviewData{}
	if ov, err := h.planets.GetOverview(r.Context(), idStr); err == nil {
		data.Overview = ov
	}
	if b, err := h.builds.GetBuildings(r.Context(), idStr); err == nil {
		data.BuildQueue = b.Queue
	}
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

// ---- Buildings -----------------------------------------------------------

type BuildItem struct {
	Code     string
	ID       int
	Name     string
	Level    int
	Cost     models.CargoManifest
	Unlocked bool
	Reason   string
}

type BuildingsData struct {
	Items      []BuildItem
	Queue      *models.QueueEntrySummary
	FieldsUsed int
	FieldsMax  int
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
	for _, def := range sortedStructures() {
		item := BuildItem{Code: def.Code, ID: def.ID, Name: def.Name, Level: levels[def.Code]}
		if c, ok := game.StructureCost(def.Code, item.Level+1); ok {
			item.Cost = models.CargoManifest{Metal: c.Metal, Crystal: c.Crystal, Deuterium: c.Deuterium}
		}
		item.Unlocked, item.Reason = reqOK(def.Requires, levels)
		data.Items = append(data.Items, item)
	}
	h.render(w, r, http.StatusOK, Page(sh, BuildingsBody(sh, data)))
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

// ---- Shipyard ------------------------------------------------------------

type ShipItem struct {
	Code      string
	Name      string
	IsDefense bool
	Count     int64
	Cost      models.CargoManifest
	Unlocked  bool
	Reason    string
}

type ShipyardData struct {
	Ships []ShipItem
	Queue *models.QueueEntrySummary
}

func (h *Handler) ShipyardPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	sh := h.shell(r, userID, id)
	sh.Title = "Shipyard"
	idStr := r.PathValue("id")

	data := ShipyardData{}
	levels := h.allLevels(r, idStr, userID)
	counts, _ := h.planets.PlanetShips(r.Context(), id)
	// Shipyard queue (shipyard_queues single active).
	data.Queue = h.shipyardQueue(r, idStr)

	for _, def := range sortedUnits(game.Ships) {
		data.Ships = append(data.Ships, ShipItem{
			Code: def.Code, Name: def.Name, Count: counts[def.Code],
			Cost: unitManifest(def), Unlocked: mustReq(def.Requires, levels),
			Reason: reasonFor(def.Requires, levels),
		})
	}
	for _, def := range sortedUnits(game.Defenses) {
		data.Ships = append(data.Ships, ShipItem{
			Code: def.Code, Name: def.Name, IsDefense: true, Count: counts[def.Code],
			Cost: unitManifest(def), Unlocked: mustReq(def.Requires, levels),
			Reason: reasonFor(def.Requires, levels),
		})
	}
	h.render(w, r, http.StatusOK, Page(sh, ShipyardBody(sh, data)))
}

func (h *Handler) ShipyardSubmit(w http.ResponseWriter, r *http.Request) {
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
		h.redirect(w, r, "/shipyard/"+r.PathValue("id"), "", err.Error())
		return
	}
	h.nudge()
	h.redirect(w, r, "/shipyard/"+r.PathValue("id"), "Queued "+code, "")
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
		item := ResearchItem{Code: def.Code, ID: def.ID, Name: def.Name, Level: levels[def.Code]}
		if c, ok := game.TechCost(def.Code, item.Level+1); ok {
			item.Cost = models.CargoManifest{Metal: c.Metal, Crystal: c.Crystal, Deuterium: c.Deuterium}
		}
		item.Unlocked, item.Reason = reqOK(def.Requires, all)
		data.Items = append(data.Items, item)
	}
	h.render(w, r, http.StatusOK, Page(sh, ResearchBody(sh, data)))
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
	Fleets   []models.FleetEventSummary
	Incoming []models.IncomingFleet
	Ships    []FleetShip
	TargetG  int
	TargetS  int
	TargetP  int
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
		TargetG: atoiDefault(r.URL.Query().Get("g"), 1),
		TargetS: atoiDefault(r.URL.Query().Get("s"), 1),
		TargetP: atoiDefault(r.URL.Query().Get("p"), 1),
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
	if homeID, err := h.planets.HomeworldID(r.Context(), userID); err == nil && homeID != 0 {
		if _, hg, hs, _, err := h.planets.GetCoordinates(r.Context(), strconv.FormatInt(homeID, 10)); err == nil {
			g, s = hg, hs
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/galaxy/%d/%d", g, s), http.StatusSeeOther)
}

type GalaxyData struct {
	Scan models.GalaxyScanResponse
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
	activeID := int64(0)
	if homeID, _ := h.planets.HomeworldID(r.Context(), userID); homeID != 0 {
		activeID = homeID
	}
	sh := h.shell(r, userID, activeID)
	sh.Title = fmt.Sprintf("Galaxy %d:%d", g, s)
	scan, err := h.planets.GalaxyScan(r.Context(), h.universeID, g, s, userID)
	if err != nil {
		http.Error(w, "scan failed", http.StatusInternalServerError)
		return
	}
	h.render(w, r, http.StatusOK, Page(sh, GalaxyBody(sh, GalaxyData{Scan: scan})))
}

// ---- Reports -------------------------------------------------------------

type ReportsData struct {
	Combat     []store.CombatReport
	Espionage  []store.EspionageReport
	Expedition []store.ExpeditionReport
}

func (h *Handler) ReportsPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	id, ok := parseID(r)
	if !ok || !h.ownsCelestial(r, id, userID) {
		http.NotFound(w, r)
		return
	}
	sh := h.shell(r, userID, id)
	sh.Title = "Reports"
	data := ReportsData{}
	data.Combat, _ = h.combat.ListForCelestial(r.Context(), id, 25)
	data.Espionage, _ = h.espionage.ListForCelestial(r.Context(), id, 25)
	data.Expedition, _ = h.expedition.ListForCelestial(r.Context(), id, 25)
	h.render(w, r, http.StatusOK, Page(sh, ReportsBody(sh, data)))
}

// ---- helpers -------------------------------------------------------------

func (h *Handler) ownsCelestial(r *http.Request, id, userID int64) bool {
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

func mustReq(requires, levels map[string]int) bool {
	return game.RequiresMet(requires, levels)
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
	if d, ok := game.Techs[code]; ok {
		return d.Name
	}
	if d, ok := game.LookupUnit(code); ok {
		return d.Name
	}
	return code
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
	return "/fleet/" + pid(sh) +
		"?g=" + strconv.Itoa(g) +
		"&s=" + strconv.Itoa(s) +
		"&p=" + strconv.Itoa(p) +
		"&type=" + url.QueryEscape(targetType)
}

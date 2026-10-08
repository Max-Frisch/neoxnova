package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"neoxnova/internal/auth"
	"neoxnova/internal/blueprint"
	"neoxnova/internal/store"
)

// maxBlueprintApply bounds how many planets one apply-to-all request may touch.
const maxBlueprintApply = 100

// blueprintRequest is the body of the blueprint create/replace endpoints.
type blueprintRequest struct {
	Name     string          `json:"name"`
	Enabled  *bool           `json:"enabled,omitempty"`
	Priority int             `json:"priority,omitempty"`
	Spec     json.RawMessage `json:"spec"`
}

// isAdmin reports whether the user has the ADMIN role.
func (h *Handler) isAdmin(ctx context.Context, userID int64) bool {
	role, err := h.Auth.AuthRole(ctx, userID)
	return err == nil && role == "ADMIN"
}

// canManageCelestial reports whether the user owns the celestial or is an admin.
func (h *Handler) canManageCelestial(ctx context.Context, celestialID, userID int64) bool {
	if h.isAdmin(ctx, userID) {
		return true
	}
	owner, err := h.Blueprints.CelestialOwner(ctx, celestialID)
	return err == nil && owner == userID
}

// PlanetBlueprintGet returns a planet's stored blueprint
// (GET /api/v1/planets/{id}/blueprint).
func (h *Handler) PlanetBlueprintGet(w http.ResponseWriter, r *http.Request) {
	id, userID, ok := h.blueprintTarget(w, r)
	if !ok {
		return
	}
	bp, err := h.Blueprints.GetForCelestial(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "No blueprint for this planet")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to load blueprint")
		return
	}
	_ = userID
	writeJSON(w, http.StatusOK, bp)
}

// PlanetBlueprintPut creates or replaces a planet's blueprint and enables it
// (POST /api/v1/planets/{id}/blueprint).
func (h *Handler) PlanetBlueprintPut(w http.ResponseWriter, r *http.Request) {
	id, userID, ok := h.blueprintTarget(w, r)
	if !ok {
		return
	}
	req, spec, ok := decodeBlueprintRequest(w, r)
	if !ok {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	bp := store.Blueprint{
		UniverseID:  h.UniverseID,
		UserID:      userID,
		CelestialID: sql.NullInt64{Int64: id, Valid: true},
		Name:        req.Name,
		Scope:       "planet",
		Spec:        spec,
		Enabled:     enabled,
		Priority:    req.Priority,
	}
	// Preserve the row id so an existing blueprint is replaced, not duplicated.
	if existing, err := h.Blueprints.GetForCelestial(r.Context(), id); err == nil {
		bp.ID = existing.ID
	}
	if _, err := h.Blueprints.Save(r.Context(), bp); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save blueprint")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "saved"})
}

// PlanetBlueprintDelete removes a planet's blueprint
// (DELETE /api/v1/planets/{id}/blueprint).
func (h *Handler) PlanetBlueprintDelete(w http.ResponseWriter, r *http.Request) {
	id, _, ok := h.blueprintTarget(w, r)
	if !ok {
		return
	}
	if err := h.Blueprints.DeleteForCelestial(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "No blueprint for this planet")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to delete blueprint")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

// PlanetBlueprintPreview dry-runs the planner and returns the next actions
// (POST /api/v1/planets/{id}/blueprint/preview).
func (h *Handler) PlanetBlueprintPreview(w http.ResponseWriter, r *http.Request) {
	id, _, ok := h.blueprintTarget(w, r)
	if !ok {
		return
	}
	bp, err := h.Blueprints.GetForCelestial(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "No blueprint for this planet")
		return
	}
	var spec blueprint.Spec
	if err := json.Unmarshal(bp.Spec, &spec); err != nil {
		writeError(w, http.StatusBadRequest, "Stored blueprint spec is invalid")
		return
	}
	if acct, err := h.Blueprints.GetAccount(r.Context(), bp.UserID); err == nil {
		var aspec blueprint.Spec
		if json.Unmarshal(acct.Spec, &aspec) == nil {
			if spec.Research == nil {
				spec.Research = map[string]int{}
			}
			for code, lvl := range aspec.Research {
				if lvl > spec.Research[code] {
					spec.Research[code] = lvl
				}
			}
		}
	}
	st, err := h.Blueprints.LoadState(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load planet state")
		return
	}
	actions := blueprint.Advance(spec, st)
	writeJSON(w, http.StatusOK, map[string]any{
		"actions": actions,
		"next":    blueprint.NextAction(spec, st),
	})
}

// AccountBlueprintGet returns the user's account-scope blueprint
// (GET /api/v1/blueprints/account).
func (h *Handler) AccountBlueprintGet(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	bp, err := h.Blueprints.GetAccount(r.Context(), userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "No account blueprint")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to load account blueprint")
		return
	}
	writeJSON(w, http.StatusOK, bp)
}

// AccountBlueprintPut creates or replaces the user's account-scope blueprint
// (POST /api/v1/blueprints/account).
func (h *Handler) AccountBlueprintPut(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	req, spec, ok := decodeBlueprintRequest(w, r)
	if !ok {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	bp := store.Blueprint{
		UniverseID: h.UniverseID,
		UserID:     userID,
		Name:       req.Name,
		Scope:      "account",
		Spec:       spec,
		Enabled:    enabled,
		Priority:   req.Priority,
	}
	if existing, err := h.Blueprints.GetAccount(r.Context(), userID); err == nil {
		bp.ID = existing.ID
	}
	if _, err := h.Blueprints.Save(r.Context(), bp); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save account blueprint")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "saved"})
}

// BlueprintList lists every blueprint owned by the user
// (GET /api/v1/blueprints).
func (h *Handler) BlueprintList(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	list, err := h.Blueprints.ListForUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list blueprints")
		return
	}
	if list == nil {
		list = []store.Blueprint{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"blueprints": list})
}

// BlueprintTemplates lists the built-in blueprint presets
// (GET /api/v1/blueprints/templates).
func (h *Handler) BlueprintTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"templates": blueprint.Presets()})
}

// blueprintApplyRequest is the body of the apply-to-all endpoint.
type blueprintApplyRequest struct {
	Name         string          `json:"name"`
	Enabled      *bool           `json:"enabled,omitempty"`
	Priority     int             `json:"priority,omitempty"`
	Spec         json.RawMessage `json:"spec"`
	All          bool            `json:"all,omitempty"`
	CelestialIDs []int64         `json:"celestial_ids,omitempty"`
}

// BlueprintApply clones one spec onto many planets, or all owned planets
// (POST /api/v1/blueprints/apply).
func (h *Handler) BlueprintApply(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var req blueprintApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.Spec) == 0 {
		req.Spec = json.RawMessage(`{}`)
	}
	var spec blueprint.Spec
	if err := json.Unmarshal(req.Spec, &spec); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid blueprint spec")
		return
	}
	if !blueprint.ValidateSpec(spec) {
		writeError(w, http.StatusBadRequest, "Blueprint references unknown catalog codes")
		return
	}
	if !spec.Enabled() {
		writeError(w, http.StatusBadRequest, "Blueprint targets nothing")
		return
	}

	ids := req.CelestialIDs
	if req.All {
		planets, err := h.Blueprints.ListOwnedPlanets(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to list planets")
			return
		}
		ids = make([]int64, 0, len(planets))
		for _, p := range planets {
			ids = append(ids, p.CelestialID)
		}
	}
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "No target planets")
		return
	}
	if len(ids) > maxBlueprintApply {
		writeError(w, http.StatusBadRequest, "Too many target planets")
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	saved := make([]int64, 0, len(ids))
	skipped := make([]int64, 0)
	errs := map[string]string{}
	for _, id := range ids {
		if !h.canManageCelestial(r.Context(), id, userID) {
			skipped = append(skipped, id)
			continue
		}
		bp := store.Blueprint{
			UniverseID:  h.UniverseID,
			UserID:      userID,
			CelestialID: sql.NullInt64{Int64: id, Valid: true},
			Name:        req.Name,
			Scope:       "planet",
			Spec:        req.Spec,
			Enabled:     enabled,
			Priority:    req.Priority,
		}
		// Replace the user's own blueprint in place if one exists.
		if existing, err := h.Blueprints.GetForCelestial(r.Context(), id); err == nil && existing.UserID == userID {
			bp.ID = existing.ID
		}
		if _, err := h.Blueprints.Save(r.Context(), bp); err != nil {
			errs[strconv.FormatInt(id, 10)] = err.Error()
			continue
		}
		saved = append(saved, id)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"saved":   saved,
		"skipped": skipped,
		"errors":  errs,
	})
}

// blueprintStatusItem is one planet's live blueprint status.
type blueprintStatusItem struct {
	CelestialID   int64            `json:"celestial_id"`
	Name          string           `json:"name,omitempty"`
	BlueprintName string           `json:"blueprint_name,omitempty"`
	Enabled       bool             `json:"enabled"`
	Next          blueprint.Action `json:"next"`
	LastError     string           `json:"last_error,omitempty"`
	LastActionAt  *time.Time       `json:"last_action_at,omitempty"`
}

// BlueprintStatus reports the next planned action (or why idle) for each of the
// user's enabled planet blueprints (GET /api/v1/blueprints/status?limit=N).
func (h *Handler) BlueprintStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := auth.UserID(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}

	list, err := h.Blueprints.ListForUser(ctx, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list blueprints")
		return
	}

	// Account-scope research is empire-wide; merge it into each planet plan.
	acctResearch := map[string]int{}
	if acct, err := h.Blueprints.GetAccount(ctx, userID); err == nil {
		var aspec blueprint.Spec
		if json.Unmarshal(acct.Spec, &aspec) == nil {
			acctResearch = aspec.Research
		}
	}
	names := map[int64]string{}
	if planets, err := h.Blueprints.ListOwnedPlanets(ctx, userID); err == nil {
		for _, p := range planets {
			names[p.CelestialID] = p.Name
		}
	}

	items := make([]blueprintStatusItem, 0, limit)
	for _, bp := range list {
		if !bp.Enabled || !bp.CelestialID.Valid {
			continue
		}
		if len(items) >= limit {
			break
		}
		item := blueprintStatusItem{
			CelestialID:   bp.CelestialID.Int64,
			Name:          names[bp.CelestialID.Int64],
			BlueprintName: bp.Name,
			Enabled:       bp.Enabled,
		}
		if bp.LastError.Valid {
			item.LastError = bp.LastError.String
		}
		if bp.LastActionAt.Valid {
			t := bp.LastActionAt.Time
			item.LastActionAt = &t
		}
		var spec blueprint.Spec
		if len(bp.Spec) > 0 && json.Unmarshal(bp.Spec, &spec) != nil {
			item.LastError = "invalid stored spec"
			items = append(items, item)
			continue
		}
		if len(acctResearch) > 0 {
			if spec.Research == nil {
				spec.Research = map[string]int{}
			}
			for code, lvl := range acctResearch {
				if lvl > spec.Research[code] {
					spec.Research[code] = lvl
				}
			}
		}
		st, err := h.Blueprints.LoadState(ctx, bp.CelestialID.Int64)
		if err != nil {
			item.LastError = "state load failed"
			items = append(items, item)
			continue
		}
		item.Next = blueprint.NextAction(spec, st)
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"planets": items})
}

// blueprintTarget parses {id} and enforces ownership/admin access.
func (h *Handler) blueprintTarget(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid planet ID format")
		return 0, 0, false
	}
	userID, ok := auth.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return 0, 0, false
	}
	if !h.canManageCelestial(r.Context(), id, userID) {
		writeError(w, http.StatusForbidden, "Not your planet")
		return 0, 0, false
	}
	return id, userID, true
}

func decodeBlueprintRequest(w http.ResponseWriter, r *http.Request) (blueprintRequest, json.RawMessage, bool) {
	var req blueprintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return req, nil, false
	}
	if len(req.Spec) == 0 {
		req.Spec = json.RawMessage(`{}`)
	}
	var spec blueprint.Spec
	if err := json.Unmarshal(req.Spec, &spec); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid blueprint spec")
		return req, nil, false
	}
	if !blueprint.ValidateSpec(spec) {
		writeError(w, http.StatusBadRequest, "Blueprint references unknown catalog codes")
		return req, nil, false
	}
	return req, req.Spec, true
}

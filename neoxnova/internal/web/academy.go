package web

import (
	"errors"
	"net/http"
	"strconv"

	"neoxnova/internal/game"
	"neoxnova/internal/store"
)

// AcademyPage renders the skill tree: the account's Academy points, each
// branch's skills with their level/effect/next cost/prerequisite, and the branch
// reset controls.
type AcademySkillView struct {
	Code       string
	Name       string
	Tier       string
	Level      int
	MaxLevel   int
	EffectText string
	NextLevel  int
	CostNext   int64
	PrereqText string
	PrereqMet  bool
	AtMax      bool
	Affordable bool
}

type AcademyBranchView struct {
	Branch game.AcademyBranch
	Label  string
	Skills []AcademySkillView
}

type AcademyData struct {
	Points   int64
	Branches []AcademyBranchView
}

// tierRoman renders the tree tier as I/II/III/... (the live column headers).
func tierRoman(tier int) string {
	switch tier {
	case 1:
		return "I"
	case 2:
		return "II"
	case 3:
		return "III"
	case 4:
		return "IV"
	default:
		return strconv.Itoa(tier)
	}
}

func (h *Handler) AcademyPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Academy")

	levels, err := h.academy.Skills(r.Context(), userID)
	if err != nil {
		sh.Error = "Could not load your academy skills"
	}
	points, _ := h.academy.Points(r.Context(), userID)

	data := AcademyData{Points: points}
	for _, branch := range []game.AcademyBranch{game.AcademyAttack, game.AcademyEconomy, game.AcademyDefence} {
		bv := AcademyBranchView{Branch: branch, Label: branch.Label()}
		for _, def := range game.AcademySkills {
			if def.Branch != branch {
				continue
			}
			lvl := levels[def.Code]
			sv := AcademySkillView{
				Code: def.Code, Name: def.Name, Tier: tierRoman(def.Tier),
				Level: lvl, MaxLevel: def.MaxLevel, EffectText: def.EffectText,
				NextLevel: lvl + 1,
			}
			if sv.NextLevel > def.MaxLevel {
				sv.NextLevel = def.MaxLevel
				sv.AtMax = true
			}
			sv.CostNext = game.AcademyCost(def, sv.NextLevel)
			sv.PrereqMet = game.AcademyPrereqMet(def, levels)
			if def.Prereq.Code != "" {
				if pre, ok := game.AcademySkillByCode(def.Prereq.Code); ok {
					sv.PrereqText = pre.Name + " " + strconv.Itoa(levels[def.Prereq.Code]) + "/" + strconv.Itoa(def.Prereq.Level)
				}
			}
			sv.Affordable = sv.PrereqMet && !sv.AtMax && points >= sv.CostNext
			bv.Skills = append(bv.Skills, sv)
		}
		data.Branches = append(data.Branches, bv)
	}
	h.render(w, r, http.StatusOK, Page(sh, AcademyBody(sh, data)))
}

// AcademyUpgradeSubmit buys levels of one skill toward the requested target.
func (h *Handler) AcademyUpgradeSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	code := r.FormValue("code")
	target, _ := strconv.Atoi(r.FormValue("target"))
	res, err := h.academy.Upgrade(r.Context(), userID, code, target)
	if err != nil {
		h.redirect(w, r, "/academy", "", academyErrText(err))
		return
	}
	def, _ := game.AcademySkillByCode(res.Code)
	msg := def.Name + " raised to level " + strconv.Itoa(res.Level) + " for " + fnum(res.Spent) + " Academy points"
	if res.Partial {
		msg = def.Name + " raised to level " + strconv.Itoa(res.Level) + " of " + strconv.Itoa(res.Requested) +
			" (best affordable) for " + fnum(res.Spent) + " Academy points"
	}
	h.redirect(w, r, "/academy", msg, "")
}

// AcademyResetSubmit resets a whole branch and refunds a share of its points.
func (h *Handler) AcademyResetSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	branchN, _ := strconv.Atoi(r.FormValue("branch"))
	percent, _ := strconv.Atoi(r.FormValue("percent"))
	res, err := h.academy.ResetBranch(r.Context(), userID, game.AcademyBranch(branchN), percent)
	if err != nil {
		h.redirect(w, r, "/academy", "", academyErrText(err))
		return
	}
	msg := res.Branch.Label() + " branch reset: refunded " + fnum(res.Refunded) + " Academy points (" +
		strconv.Itoa(res.RefundPct) + "%)"
	if res.Cost > 0 {
		msg += " for " + fnum(res.Cost) + " " + res.Currency
	}
	h.redirect(w, r, "/academy", msg, "")
}

// academyErrText maps the store's sentinels to player-facing text.
func academyErrText(err error) string {
	switch {
	case errors.Is(err, store.ErrInsufficientAcademyPoints):
		return "Not enough Academy points"
	case errors.Is(err, store.ErrAcademyPrereq):
		return "Prerequisite not met"
	case errors.Is(err, store.ErrAcademyLevel):
		return "This skill is already at that level"
	case errors.Is(err, store.ErrNothingToReset):
		return "Nothing to reset in this branch"
	case errors.Is(err, store.ErrInsufficientAntimatter):
		return "Not enough Antimatter"
	case errors.Is(err, store.ErrInsufficientDarkMatter):
		return "Not enough Dark Matter"
	case errors.Is(err, store.ErrUnknownCode):
		return "Unknown skill"
	default:
		return err.Error()
	}
}

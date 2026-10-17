package web

import (
	"errors"
	"net/http"
	"strconv"

	"neoxnova/internal/game"
	"neoxnova/internal/store"
)

// BonusData backs the Online Bonus page: the account's peaceful level and its
// bonuses plus the Online Bonus button state.
type BonusData struct {
	Level        int
	ProgressPct  float64
	NextSeconds  int64
	Bonus        game.PeacefulBonus
	Ready        bool
	ReadySeconds int64
	Reward       game.BonusReward
}

func (h *Handler) bonusData(r *http.Request, userID int64) BonusData {
	d := BonusData{}
	if st, err := h.peaceful.Status(r.Context(), userID); err == nil {
		d.Level = st.Level
		d.ProgressPct = st.ProgressPct
		d.NextSeconds = st.NextSeconds
		d.Ready = st.BonusReady
		d.ReadySeconds = st.BonusReadySeconds
		d.Bonus = game.PeacefulBonuses(st.Level)
		d.Reward = game.BonusButtonReward(st.Level)
	}
	return d
}

// BonusPage renders the Online Bonus page (peaceful level, bonuses, claim).
func (h *Handler) BonusPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Online Bonus")
	h.render(w, r, http.StatusOK, Page(sh, BonusBody(sh, h.bonusData(r, userID))))
}

// BonusSubmit claims the Online Bonus button if it is off cooldown and redirects
// back with the payout (the live "Info: You received …" dialog).
func (h *Handler) BonusSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	reward, err := h.peaceful.ClaimBonusButton(r.Context(), userID)
	if err != nil {
		if errors.Is(err, store.ErrBonusNotReady) {
			h.redirect(w, r, "/bonus", "", "The bonus button is not ready yet")
			return
		}
		h.redirect(w, r, "/bonus", "", err.Error())
		return
	}
	msg := "You received: Dark Matter: " + fnum(reward.DarkMatter) +
		"; Experience: " + strconv.Itoa(int(reward.ExperiencePercent)) +
		"%; Experience points: " + strconv.FormatInt(reward.AcademyPoints, 10) + "."
	h.redirect(w, r, "/bonus", msg, "")
}

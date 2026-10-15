package web

import (
	"net/http"
	"strings"

	"neoxnova/internal/game"
)

// MerchantUnit is one scrapable ship or defense with the count the caller holds
// on the anchor celestial.
type MerchantUnit struct {
	Code      string
	Name      string
	Image     string
	Cost      game.Cost
	Available int64
}

// MerchantData backs the Merchant of Fleets page.
type MerchantData struct {
	Units       []MerchantUnit
	RatePercent int
	DarkMatter  int64
}

func merchantUnit(def game.UnitDef, available int64) MerchantUnit {
	return MerchantUnit{
		Code:      def.Code,
		Name:      def.Name,
		Image:     assetURL(def.Code),
		Cost:      def.BaseCost,
		Available: available,
	}
}

// merchantData loads every ship and defense with the count held on the
// homeworld (the page's anchor), plus the merchant rate and Dark Matter balance.
func (h *Handler) merchantData(r *http.Request, userID int64) MerchantData {
	data := MerchantData{RatePercent: h.merchantRate(r)}
	home := h.homeworldID(r, userID)
	ships, _ := h.planets.PlanetShips(r.Context(), home)
	defs, _ := h.planets.PlanetDefenses(r.Context(), home)
	for _, def := range sortedUnits(game.Ships) {
		data.Units = append(data.Units, merchantUnit(def, ships[def.Code]))
	}
	for _, def := range sortedUnits(game.Defenses) {
		data.Units = append(data.Units, merchantUnit(def, defs[def.Code]))
	}
	if dm, _, _, err := h.auth.UserBalances(r.Context(), userID); err == nil {
		data.DarkMatter = dm
	}
	return data
}

func (h *Handler) MerchantPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Black market")
	h.render(w, r, http.StatusOK, Page(sh, MerchantBody(sh, h.merchantData(r, userID))))
}

// merchantQuote prices scrapping number units of code, returning the refund and
// the unit's display name. An unknown unit or non-positive count yields a
// non-empty error message rendered into the preview fragment.
func (h *Handler) merchantQuote(r *http.Request, code string, number int64) (refund game.Cost, name string, rate int, errMsg string) {
	def, ok := game.LookupUnit(code)
	if !ok {
		return game.Cost{}, "", 0, "Unknown unit"
	}
	if number < 1 {
		return game.Cost{}, "", 0, "Enter a number of units"
	}
	rate = h.merchantRate(r)
	return game.MerchantRefund(def.BaseCost.Mul(number), rate), def.Name, rate, ""
}

// MerchantPreview renders the htmx quote fragment (no units are scrapped).
func (h *Handler) MerchantPreview(w http.ResponseWriter, r *http.Request) {
	number := int64Default(r.FormValue("number"))
	refund, name, rate, errMsg := h.merchantQuote(r, strings.TrimSpace(r.FormValue("unit")), number)
	if errMsg != "" {
		h.render(w, r, http.StatusOK, MerchantQuotePartial(nil, "", 0, 0, errMsg))
		return
	}
	h.render(w, r, http.StatusOK, MerchantQuotePartial(&refund, name, number, rate, ""))
}

// MerchantSubmit scraps the units off the homeworld and flashes the refund.
func (h *Handler) MerchantSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	back := redirectBase(r.FormValue("return"), "/blackmarket/merchant")
	home := h.homeworldID(r, userID)
	if home == 0 {
		h.redirect(w, r, back, "", "No planet to scrap from")
		return
	}
	code := strings.TrimSpace(r.FormValue("unit"))
	number := int64Default(r.FormValue("number"))
	refund, _, err := h.trades.ScrapUnits(r.Context(), home, userID, code, number)
	if err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	msg := "Scrapped " + fnum(number) + "× " + displayName(code) + " for Metal " + fnum(refund.Metal) +
		", Crystal " + fnum(refund.Crystal) + ", Deuterium " + fnum(refund.Deuterium)
	h.redirect(w, r, back, msg, "")
}

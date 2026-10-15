package web

import (
	"net/http"
	"strconv"

	"neoxnova/internal/game"
)

// TraderResource is one tradable resource's current balance on the anchor
// celestial.
type TraderResource struct {
	Code    int
	Name    string
	Current float64
}

// TraderData backs the Resource Trader page.
type TraderData struct {
	Resources  []TraderResource
	DarkMatter int64
	Fee        int64
}

// ---- Resource Trader (Black market -> Resource Trader) -------------------

// traderData loads the account's homeworld balances and Dark Matter for the
// account-wide trader page.
func (h *Handler) traderData(r *http.Request, userID int64) TraderData {
	data := TraderData{Fee: game.TradeDarkMatterCost}
	if home := h.homeworldID(r, userID); home != 0 {
		if rs, err := h.planets.GetResourceState(r.Context(), strconv.FormatInt(home, 10)); err == nil {
			data.Resources = []TraderResource{
				{game.ResourceMetal, "Metal", rs.MetalCurrent},
				{game.ResourceCrystal, "Crystal", rs.CrystalCurrent},
				{game.ResourceDeuterium, "Deuterium", rs.DeutCurrent},
			}
		}
	}
	if dm, _, _, err := h.auth.UserBalances(r.Context(), userID); err == nil {
		data.DarkMatter = dm
	}
	return data
}

func (h *Handler) TraderPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Black market")
	h.render(w, r, http.StatusOK, Page(sh, TraderBody(sh, h.traderData(r, userID))))
}

// TraderPreview renders the htmx quote fragment (no trade is made).
func (h *Handler) TraderPreview(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	dm, _, _, _ := h.auth.UserBalances(r.Context(), userID)
	quote, err := game.TradeQuote(
		atoiDefault(r.FormValue("sell"), 0),
		atoiDefault(r.FormValue("buy"), 0),
		int64Default(r.FormValue("amount")),
		dm,
	)
	if err != nil {
		h.render(w, r, http.StatusOK, TraderQuotePartial(nil, err.Error()))
		return
	}
	h.render(w, r, http.StatusOK, TraderQuotePartial(&quote, ""))
}

// TraderSubmit executes the trade on the homeworld and flashes the result.
func (h *Handler) TraderSubmit(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	back := redirectBase(r.FormValue("return"), "/blackmarket/trader")
	home := h.homeworldID(r, userID)
	if home == 0 {
		h.redirect(w, r, back, "", "No planet to trade from")
		return
	}
	dm, _, _, _ := h.auth.UserBalances(r.Context(), userID)
	quote, err := game.TradeQuote(
		atoiDefault(r.FormValue("sell"), 0),
		atoiDefault(r.FormValue("buy"), 0),
		int64Default(r.FormValue("amount")),
		dm,
	)
	if err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	if err := h.trades.ExecuteTrade(r.Context(), home, userID, quote); err != nil {
		h.redirect(w, r, back, "", err.Error())
		return
	}
	msg := "Traded " + fnum(quote.SellCost) + " " + game.TradeResourceName(quote.SellCode) +
		" for " + fnum(quote.BuyAmount) + " " + game.TradeResourceName(quote.BuyCode) +
		" (" + fnum(quote.DarkMatter) + " Dark Matter)"
	h.redirect(w, r, back, msg, "")
}

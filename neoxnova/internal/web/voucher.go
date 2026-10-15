package web

import "net/http"

// VoucherPage renders the Black market -> Voucher System page (page=reward2).
// Redeeming codes needs the live operator to issue and validate vouchers, so the
// page ships with the captured layout but no backing functionality yet — the
// Redeem button is disabled (reskin rule: never fake pages).
func (h *Handler) VoucherPage(w http.ResponseWriter, r *http.Request) {
	userID, _ := h.userID(r)
	sh := h.accountShell(r, userID, "Black market")
	h.render(w, r, http.StatusOK, Page(sh, VoucherBody(sh)))
}

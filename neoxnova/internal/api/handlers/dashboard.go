package handlers

import (
	"fmt"
	"net/http"
)

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	planetID := r.PathValue("id")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	metal, crystal, deuterium, lastCalc, err := h.Planets.UpdateResources(r.Context(), planetID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "<h3>Error updating resources. Did you seed the planet record?</h3>")
		return
	}

	name, g, s, p, _ := h.Planets.GetCoordinates(r.Context(), planetID)

	fmt.Fprintf(w, `
		<!DOCTYPE html>
		<html>
		<head>
			<title>Neo-XNova Core Dashboard</title>
			<meta http-equiv="refresh" content="2">
			<style>
				body { font-family: monospace; background: #121214; color: #e1e1e6; padding: 40px; }
				.card { background: #202024; border: 1px solid #323238; padding: 20px; border-radius: 8px; max-width: 500px; }
				.resource { font-size: 20px; color: #04d361; font-weight: bold; margin: 10px 0; }
				.meta { color: #8d8d99; }
			</style>
		</head>
		<body>
			<h2>🚀 Neo-XNova Dev Engine Workbench</h2>
			<div class="card">
				<h3>🪐 %s [%d:%d:%d]</h3>
				<p class="meta">Planet Target ID: %s</p>
				<hr style="border-color: #323238;">
				<div class="resource">🪙 Metal: %.2f</div>
				<div class="resource">💎 Crystal: %.2f</div>
				<div class="resource">⛽ Deuterium: %.2f</div>
				<p class="meta" style="font-size: 11px;">Last Server Dynamic Tick Calculation: %s</p>
			</div>
			<p style="color: #8d8d99; font-size: 12px;">💡 Auto-refreshes every 2 seconds to showcase your delta-time database engine working live!</p>
		</body>
		</html>
	`, name, g, s, p, planetID, metal, crystal, deuterium, lastCalc.Format("15:04:05.000"))
}

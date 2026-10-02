package handlers

import (
	"fmt"
	"net/http"
)

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"online","engine":"Neo-XNova v2.0","universe":"`+h.UniverseID+`"}`)
}

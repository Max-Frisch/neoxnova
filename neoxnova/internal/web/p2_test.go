package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"neoxnova/internal/models"
)

// TestResbarPollsEveryFiveSeconds locks the P2 throttle: the polled resource bar
// refreshes every 5 s (was 2 s) to cut the per-view query/lock cost.
func TestResbarPollsEveryFiveSeconds(t *testing.T) {
	html := renderComponent(t, Page(Shell{ActivePlanetID: 7}, templ.NopComponent))
	if !strings.Contains(html, `hx-trigger="every 5s"`) {
		t.Errorf("resource bar missing every-5s trigger")
	}
	if strings.Contains(html, `hx-trigger="every 2s"`) {
		t.Errorf("resource bar still polls every 2s")
	}
}

// TestUserIDReadsPrincipal verifies the middleware-resolved principal is the
// single source of the session user: a request carrying one never touches the
// auth store (the bare Handler here has no DB and would panic otherwise).
func TestUserIDReadsPrincipal(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/overview/7", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &userPrincipal{userID: 42}))

	id, ok := (&Handler{}).userID(r)
	if !ok || id != 42 {
		t.Fatalf("userID = (%d, %v), want (42, true)", id, ok)
	}
}

// TestOwnsCelestialReadsPrincipal verifies ownership is answered from the
// principal snapshot, with no per-request ownership query.
func TestOwnsCelestialReadsPrincipal(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/overview/7", nil)
	r = r.WithContext(context.WithValue(r.Context(), principalKey{}, &userPrincipal{
		userID: 1, ownedOK: true,
		owned: []models.OwnedCelestial{{ID: 7}, {ID: 9}},
	}))

	h := &Handler{}
	if !h.ownsCelestial(r, 7, 1) {
		t.Errorf("ownsCelestial(7) = false, want true")
	}
	if h.ownsCelestial(r, 8, 1) {
		t.Errorf("ownsCelestial(8) = true, want false")
	}
}

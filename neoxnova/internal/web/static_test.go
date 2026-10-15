package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"neoxnova/internal/game"
)

// TestNiburuArtworkServed verifies the captured skin assets are embedded and
// reachable through the /static/ file server (phase I0 exit criterion).
func TestNiburuArtworkServed(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{}).Mount(mux)

	for _, tc := range []struct{ path, ctype string }{
		{"/static/niburu/buildings/metal_mine.gif", "image/gif"},
		{"/static/niburu/research/espionage_tech.gif", "image/gif"},
		{"/static/niburu/ships/202.gif", "image/gif"},
		{"/static/niburu/defenses/401.gif", "image/gif"},
		{"/static/niburu/icons/metall.gif", "image/gif"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", tc.path, rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); got != tc.ctype {
			t.Errorf("GET %s content-type = %q, want %q", tc.path, got, tc.ctype)
		}
	}
}

// TestNiburuArtworkCatalogComplete asserts every catalog item has a corresponding
// captured image, so templates can reference assets by catalog code with no gaps.
func TestNiburuArtworkCatalogComplete(t *testing.T) {
	sub, err := fs.Sub(staticFiles, "static/niburu")
	if err != nil {
		t.Fatalf("sub static/niburu: %v", err)
	}
	exists := func(name string) bool {
		if _, err := fs.Stat(sub, name); err != nil {
			return false
		}
		return true
	}
	check := func(kind, dir string, codes []string) {
		for _, code := range codes {
			name := dir + "/" + code + ".gif"
			if !exists(name) {
				t.Errorf("missing artwork: niburu/%s", name)
			}
		}
	}
	codeList := func(m map[string]game.StructureDef) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		return out
	}
	techList := make([]string, 0, len(game.Techs))
	for k := range game.Techs {
		techList = append(techList, k)
	}
	shipList := make([]string, 0, len(game.Ships))
	for k := range game.Ships {
		shipList = append(shipList, k)
	}
	defList := make([]string, 0, len(game.Defenses))
	for k := range game.Defenses {
		defList = append(defList, k)
	}
	check("building", "buildings", codeList(game.Structures))
	check("research", "research", techList)
	check("ship", "ships", shipList)
	check("defense", "defenses", defList)
}

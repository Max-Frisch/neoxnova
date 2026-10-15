package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
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

// TestStaticAssetsCacheable verifies the P1 caching contract: text assets carry
// ETag + Cache-Control and are gzipped, revalidation returns 304, and images are
// never gzip-encoded.
func TestStaticAssetsCacheable(t *testing.T) {
	mux := http.NewServeMux()
	(&Handler{}).Mount(mux)

	// CSS: ETag + Cache-Control + gzip.
	req := httptest.NewRequest(http.MethodGet, "/static/skin.css", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET skin.css = %d, want 200", rec.Code)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Errorf("skin.css missing ETag")
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "public") || !strings.Contains(cc, "max-age") {
		t.Errorf("skin.css Cache-Control = %q, want public + max-age", cc)
	}
	if enc := rec.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Errorf("skin.css Content-Encoding = %q, want gzip", enc)
	}

	// Conditional request revalidates to 304 without a body.
	req2 := httptest.NewRequest(http.MethodGet, "/static/skin.css", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotModified {
		t.Fatalf("conditional skin.css = %d, want 304", rec2.Code)
	}

	// Images are already compressed: never gzip them.
	req3 := httptest.NewRequest(http.MethodGet, "/static/niburu/buildings/metal_mine.gif", nil)
	req3.Header.Set("Accept-Encoding", "gzip")
	rec3 := httptest.NewRecorder()
	mux.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("GET metal_mine.gif = %d, want 200", rec3.Code)
	}
	if enc := rec3.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("gif Content-Encoding = %q, want none", enc)
	}
	if rec3.Header().Get("ETag") == "" {
		t.Errorf("gif missing ETag")
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
	check("moon building", "buildings", codeList(game.MoonOnlyStructures))
	check("research", "research", techList)
	check("ship", "ships", shipList)
	check("defense", "defenses", defList)
}

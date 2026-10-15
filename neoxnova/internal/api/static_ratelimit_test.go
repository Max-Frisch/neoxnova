package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestStaticAssetsBypassGlobalRateLimit guards the P1 fix: /static/ must be
// served outside the per-IP limiter. The global bucket is 40 req/s with a burst
// of 80; a single page pulls far more sprites than that, so without the bypass
// the tail of a normal page load would be throttled to 429s.
func TestStaticAssetsBypassGlobalRateLimit(t *testing.T) {
	srv := httptest.NewServer(NewRouter(nil, nil, "universe_test"))
	defer srv.Close()

	client := srv.Client()
	const requests = 300 // >> burst(80)
	for i := 0; i < requests; i++ {
		resp, err := client.Get(srv.URL + "/static/skin.css")
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("static request %d = %d, want 200 (/static/ must bypass the IP limiter)", i, resp.StatusCode)
		}
	}
}

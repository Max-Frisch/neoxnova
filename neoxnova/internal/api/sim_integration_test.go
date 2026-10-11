package api

// TestSimulateEndpoint integration-tests POST /api/v1/sim against the sandbox
// universe seeded by migrations/0016_test_universe.sql. Skipped unless
// DATABASE_URL points at the docker-compose stack that has been migrated.
//
//	$env:DATABASE_URL = "postgres://postgres:password@localhost:5432/neoxnova?sslmode=disable"
//	go test ./internal/api/ -run TestSimulateEndpoint -v

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

type simResponse struct {
	Seed   int64 `json:"seed"`
	Result struct {
		Winner string `json:"Winner"`
		Rounds int    `json:"Rounds"`
	} `json:"result"`
}

func TestSimulateEndpoint(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping simulator endpoint test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	var hasSandbox bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM universes WHERE code_name = 'universe_test')`,
	).Scan(&hasSandbox); err != nil {
		t.Fatalf("probe sandbox universe: %v", err)
	}
	if !hasSandbox {
		t.Skip("universe_test missing; run `make migrate` first")
	}
	t.Setenv("APP_ENV", "development")

	srv := httptest.NewServer(NewRouter(db, nil, "universe_test"))
	defer srv.Close()

	// A fresh client is anonymous.
	if status, _ := simPost(t, newSimClient(t), srv.URL, `{"attacker":{"ships":{"207":1}},"defender":{"ships":{"204":1}}}`); status != http.StatusUnauthorized {
		t.Fatalf("anonymous sim status = %d, want 401", status)
	}

	client := newSimClient(t)
	if status, body := simLogin(t, client, srv.URL); status != http.StatusOK {
		t.Fatalf("login status = %d (%s)", status, body)
	}

	t.Run("resolves a battle deterministically", func(t *testing.T) {
		payload := `{"attacker":{"ships":{"207":100}},"defender":{"ships":{"204":10}}}`
		status, body := simPost(t, client, srv.URL, payload)
		if status != http.StatusOK {
			t.Fatalf("status = %d, body = %s", status, body)
		}
		var first simResponse
		if err := json.Unmarshal([]byte(body), &first); err != nil {
			t.Fatalf("decode: %v (%s)", err, body)
		}
		if first.Result.Winner == "" || first.Seed == 0 {
			t.Fatalf("empty result: %s", body)
		}
		// Same inputs -> same derived seed and outcome.
		_, body2 := simPost(t, client, srv.URL, payload)
		var second simResponse
		if err := json.Unmarshal([]byte(body2), &second); err != nil {
			t.Fatalf("decode2: %v", err)
		}
		if first.Seed != second.Seed || first.Result.Winner != second.Result.Winner {
			t.Fatalf("non-deterministic: %+v vs %+v", first, second)
		}
	})

	t.Run("honours an explicit seed", func(t *testing.T) {
		status, body := simPost(t, client, srv.URL, `{"seed":7,"attacker":{"ships":{"207":5}},"defender":{"ships":{"204":5}}}`)
		if status != http.StatusOK {
			t.Fatalf("status = %d, body = %s", status, body)
		}
		var resp simResponse
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Seed != 7 {
			t.Fatalf("seed = %d, want 7", resp.Seed)
		}
	})

	t.Run("rejects bad input", func(t *testing.T) {
		cases := []string{
			`{"attacker":{"ships":{}},"defender":{"ships":{"204":1}}}`,        // empty attacker
			`{"attacker":{"ships":{"999":1}},"defender":{"ships":{"204":1}}}`, // unknown unit
			`{"attacker":{"ships":{"207":1}},"defender":{"ships":{}}}`,        // empty defender
			`not json`, // malformed
		}
		for _, c := range cases {
			if status, body := simPost(t, client, srv.URL, c); status != http.StatusBadRequest {
				t.Fatalf("payload %q status = %d, want 400 (%s)", c, status, body)
			}
		}
	})
}

func newSimClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{Jar: jar}
}

func simLogin(t *testing.T, c *http.Client, base string) (int, string) {
	t.Helper()
	return simJSON(t, c, base+"/api/v1/auth/login",
		`{"login":"webtest_a","password":"commander-dev-pass"}`)
}

func simPost(t *testing.T, c *http.Client, base, payload string) (int, string) {
	t.Helper()
	return simJSON(t, c, base+"/api/v1/sim", payload)
}

func simJSON(t *testing.T, c *http.Client, url, payload string) (int, string) {
	t.Helper()
	resp, err := c.Post(url, "application/json", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, buf.String()
}

package web

// TestWebEndToEnd drives the server-rendered UI end to end against the fast
// sandbox universe seeded by migrations/0016_test_universe.sql. It POSTs the
// real HTML forms (no browser) and lets the durable scheduler resolve builds,
// flights, espionage and combat.
//
// The test is skipped unless DATABASE_URL points at the docker-compose stack and
// that stack has been migrated (`make migrate`). Run it with:
//
//	$env:DATABASE_URL = "postgres://postgres:password@localhost:5432/neoxnova?sslmode=disable"
//	go test ./internal/web/ -run TestWebEndToEnd -v
//
// The sandbox universe is cranked to 50000x so a full loop finishes in seconds.
// Both seeded test accounts share the dev password "commander-dev-pass".

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"neoxnova/internal/engine"
)

const (
	e2eUniverse = "universe_test"
	e2ePassword = "commander-dev-pass"
	e2eTimeout  = 20 * time.Second
)

// e2ePlayer is a seeded sandbox account plus its homeworld.
type e2ePlayer struct {
	Username string
	UserID   int64
	HomeID   int64
	G, S, P  int
	Name     string
}

func TestWebEndToEnd(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping web end-to-end test")
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

	// The sandbox universe only exists after `make migrate` applies 0016.
	var hasSandbox bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM universes WHERE code_name = $1)`, e2eUniverse,
	).Scan(&hasSandbox); err != nil {
		t.Fatalf("probe sandbox universe: %v", err)
	}
	if !hasSandbox {
		t.Skipf("universe %q missing; run `make migrate` first", e2eUniverse)
	}

	// Page cookies are only non-Secure in development; httptest speaks plain HTTP.
	t.Setenv("APP_ENV", "development")

	a := e2eLoadPlayer(t, ctx, db, "webtest_a")
	b := e2eLoadPlayer(t, ctx, db, "webtest_b")
	e2eReset(t, ctx, db, a, b)

	// The real durable scheduler resolves builds, flights and combat. Its Redis
	// wake channel is optional, so a nil client is fine (it falls back to polling).
	schedCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go engine.NewEventEngine(nil, db).StartScheduler(schedCtx, e2eUniverse)

	mux := http.NewServeMux()
	New(db, e2eUniverse, nil).Mount(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	clientA := e2eClient(t)
	clientB := e2eClient(t)

	t.Run("anonymous access redirects to login", func(t *testing.T) {
		anon := e2eClient(t)
		body := e2eGet(t, anon, srv.URL, fmt.Sprintf("/overview/%d", a.HomeID))
		e2eWantContains(t, body, "Log in", "anonymous overview redirects to the login page")
	})

	t.Run("register page renders", func(t *testing.T) {
		body := e2eGet(t, e2eClient(t), srv.URL, "/register")
		e2eWantContains(t, body, "Create account", "register page renders a form")
	})

	t.Run("login as attacker", func(t *testing.T) {
		e2eLogin(t, clientA, srv.URL, a.Username)
	})

	t.Run("overview renders homeworld", func(t *testing.T) {
		body := e2eGet(t, clientA, srv.URL, fmt.Sprintf("/overview/%d", a.HomeID))
		e2eWantContains(t, body, "Overview", "nav")
		e2eWantContains(t, body, "Fields", "field counter")
		e2eWantContains(t, body, fmt.Sprintf("%d:%d:%d", a.G, a.S, a.P), "homeworld coordinates")
	})

	t.Run("building queues and completes", func(t *testing.T) {
		before := e2eScalarInt64(t, db,
			`SELECT COALESCE(level, 0) FROM planet_structures WHERE celestial_id = $1 AND structure_code = 'metal_mine'`,
			a.HomeID)
		body := e2ePost(t, clientA, srv.URL, fmt.Sprintf("/buildings/%d/build", a.HomeID),
			url.Values{"code": {"metal_mine"}})
		e2eWantContains(t, body, "Queued metal_mine", "build queued")
		e2eWaitFor(t, "metal mine level to increase", func() bool {
			return e2eScalarInt64(t, db,
				`SELECT COALESCE(level, 0) FROM planet_structures WHERE celestial_id = $1 AND structure_code = 'metal_mine'`,
				a.HomeID) == before+1
		})
	})

	t.Run("building cancel refunds the active construction", func(t *testing.T) {
		// Seed a far-future construction so the cranked sandbox scheduler cannot
		// resolve it before the cancel, then cancel it via the real HTML form.
		var qid int64
		if err := db.QueryRowContext(ctx, `
			INSERT INTO construction_queues (celestial_id, structure_code, target_level, start_time, end_time, status)
			VALUES ($1, 'crystal_mine', 1, NOW(), NOW() + interval '1 day', 'IN_PROGRESS')
			RETURNING id
		`, a.HomeID).Scan(&qid); err != nil {
			t.Fatalf("seed construction queue: %v", err)
		}
		body := e2ePost(t, clientA, srv.URL, fmt.Sprintf("/buildings/%d/cancel", a.HomeID), url.Values{})
		e2eWantContains(t, body, "refunded", "cancel flash")
		var status string
		if err := db.QueryRowContext(ctx, `SELECT status FROM construction_queues WHERE id = $1`, qid).Scan(&status); err != nil {
			t.Fatalf("read queue status: %v", err)
		}
		if status != "CANCELLED" {
			t.Fatalf("queue status = %q, want CANCELLED", status)
		}
	})

	t.Run("research cancel refunds the active research", func(t *testing.T) {
		var qid int64
		if err := db.QueryRowContext(ctx, `
			INSERT INTO research_queues (user_id, celestial_id, tech_code, target_level, start_time, end_time, status)
			VALUES ($1, $2, 'laser_tech', 1, NOW(), NOW() + interval '1 day', 'IN_PROGRESS')
			RETURNING id
		`, a.UserID, a.HomeID).Scan(&qid); err != nil {
			t.Fatalf("seed research queue: %v", err)
		}
		body := e2ePost(t, clientA, srv.URL, fmt.Sprintf("/research/%d/cancel", a.HomeID), url.Values{})
		e2eWantContains(t, body, "refunded", "cancel flash")
		var status string
		if err := db.QueryRowContext(ctx, `SELECT status FROM research_queues WHERE id = $1`, qid).Scan(&status); err != nil {
			t.Fatalf("read research status: %v", err)
		}
		if status != "CANCELLED" {
			t.Fatalf("research status = %q, want CANCELLED", status)
		}
	})

	t.Run("shipyard builds ships", func(t *testing.T) {
		before := e2eScalarInt64(t, db,
			`SELECT COALESCE(quantity, 0) FROM planet_ships WHERE celestial_id = $1 AND ship_code = '204'`,
			a.HomeID)
		body := e2ePost(t, clientA, srv.URL, fmt.Sprintf("/shipyard/%d/build", a.HomeID),
			url.Values{"code": {"204"}, "quantity": {"5"}})
		e2eWantContains(t, body, "Queued 204", "ship order queued")
		e2eWaitFor(t, "5 light fighters to finish", func() bool {
			return e2eScalarInt64(t, db,
				`SELECT COALESCE(quantity, 0) FROM planet_ships WHERE celestial_id = $1 AND ship_code = '204'`,
				a.HomeID) == before+5
		})
	})

	t.Run("research starts and completes", func(t *testing.T) {
		before := e2eScalarInt64(t, db,
			`SELECT COALESCE(level, 0) FROM user_technologies WHERE user_id = $1 AND tech_code = 'espionage_tech'`,
			a.UserID)
		body := e2ePost(t, clientA, srv.URL, fmt.Sprintf("/research/%d/start", a.HomeID),
			url.Values{"code": {"espionage_tech"}})
		e2eWantContains(t, body, "Researching espionage_tech", "research queued")
		e2eWaitFor(t, "espionage tech level to increase", func() bool {
			return e2eScalarInt64(t, db,
				`SELECT COALESCE(level, 0) FROM user_technologies WHERE user_id = $1 AND tech_code = 'espionage_tech'`,
				a.UserID) == before+1
		})
	})

	t.Run("espionage produces a report", func(t *testing.T) {
		body := e2ePost(t, clientA, srv.URL, fmt.Sprintf("/fleet/%d/dispatch", a.HomeID), url.Values{
			"mission": {"ESPIONAGE"}, "galaxy": {strconv.Itoa(b.G)},
			"system": {strconv.Itoa(b.S)}, "position": {strconv.Itoa(b.P)},
			"target_type": {"PLANET"}, "ship_210": {"1"}, "speed_percent": {"100"},
		})
		e2eWantContains(t, body, "Fleet dispatched", "espionage dispatched")
		e2eWaitFor(t, "espionage report", func() bool {
			return e2eScalarInt64(t, db,
				`SELECT COUNT(*) FROM espionage_reports WHERE attacker_id = $1`, a.UserID) > 0
		})
		body = e2eGet(t, clientA, srv.URL, fmt.Sprintf("/reports/%d", a.HomeID))
		e2eWantContains(t, body, "Espionage reports", "reports page")
		e2eWantContains(t, body, e2eCoord(b.G, b.S, b.P), "espionage target coordinates")
	})

	t.Run("transport is visible to the defender and can be recalled", func(t *testing.T) {
		// 10% speed widens the inbound window so the defender's view is observable.
		body := e2ePost(t, clientA, srv.URL, fmt.Sprintf("/fleet/%d/dispatch", a.HomeID), url.Values{
			"mission": {"TRANSPORT"}, "galaxy": {strconv.Itoa(b.G)},
			"system": {strconv.Itoa(b.S)}, "position": {strconv.Itoa(b.P)},
			"target_type": {"PLANET"}, "ship_202": {"10"}, "cargo_metal": {"1000"},
			"speed_percent": {"10"},
		})
		e2eWantContains(t, body, "Fleet dispatched", "transport dispatched")

		e2eLogin(t, clientB, srv.URL, b.Username)
		e2eWaitFor(t, "defender sees the inbound fleet", func() bool {
			return strings.Contains(e2eGet(t, clientB, srv.URL, fmt.Sprintf("/overview/%d", b.HomeID)),
				e2eCoord(a.G, a.S, a.P))
		})

		fleetID := e2eScalarInt64(t, db,
			`SELECT id FROM fleets WHERE user_id = $1 AND mission = 'TRANSPORT' AND phase = 'OUTBOUND' ORDER BY id DESC LIMIT 1`,
			a.UserID)
		if fleetID == 0 {
			t.Fatal("dispatched transport fleet not found")
		}
		body = e2ePost(t, clientA, srv.URL, fmt.Sprintf("/fleet/%d/recall", a.HomeID),
			url.Values{"fleet_id": {strconv.FormatInt(fleetID, 10)}})
		e2eWantContains(t, body, "Fleet recalled", "recall")
	})

	t.Run("attack resolves into a combat report", func(t *testing.T) {
		body := e2ePost(t, clientA, srv.URL, fmt.Sprintf("/fleet/%d/dispatch", a.HomeID), url.Values{
			"mission": {"ATTACK"}, "galaxy": {strconv.Itoa(b.G)},
			"system": {strconv.Itoa(b.S)}, "position": {strconv.Itoa(b.P)},
			"target_type": {"PLANET"}, "ship_207": {"50"}, "speed_percent": {"100"},
		})
		if strings.Contains(strings.ToLower(body), "protection") {
			t.Skip("attacker/defender points are outside the 4:1 noob-protection window")
		}
		e2eWantContains(t, body, "Fleet dispatched", "attack dispatched")
		e2eWaitFor(t, "combat report", func() bool {
			return e2eScalarInt64(t, db,
				`SELECT COUNT(*) FROM combat_reports WHERE attacker_id = $1`, a.UserID) > 0
		})
		body = e2eGet(t, clientA, srv.URL, fmt.Sprintf("/reports/%d", a.HomeID))
		e2eWantContains(t, body, "Combat reports", "reports page")
		e2eWantContains(t, body, e2eCoord(b.G, b.S, b.P), "battlefield coordinates")
	})

	t.Run("colonize founds a new planet", func(t *testing.T) {
		const colG, colS, colP = 1, 1, 10
		body := e2ePost(t, clientA, srv.URL, fmt.Sprintf("/fleet/%d/dispatch", a.HomeID), url.Values{
			"mission": {"COLONIZE"}, "galaxy": {strconv.Itoa(colG)},
			"system": {strconv.Itoa(colS)}, "position": {strconv.Itoa(colP)},
			"target_type": {"PLANET"}, "ship_208": {"1"}, "ship_204": {"2"}, "speed_percent": {"100"},
		})
		e2eWantContains(t, body, "Fleet dispatched", "colonize dispatched")
		e2eWaitFor(t, "new colony", func() bool {
			return e2eScalarInt64(t, db,
				`SELECT COUNT(*) FROM celestial_objects
				 WHERE user_id = $1 AND galaxy = $2 AND system = $3 AND position = $4 AND object_type = 'PLANET'`,
				a.UserID, colG, colS, colP) > 0
		})
		body = e2eGet(t, clientA, srv.URL, fmt.Sprintf("/overview/%d", a.HomeID))
		e2eWantContains(t, body, "planet-select", "planet switcher renders")
		e2eWantContains(t, body, e2eCoord(colG, colS, colP), "planet switcher lists the new colony")
	})

	t.Run("galaxy scan lists both neighbours", func(t *testing.T) {
		body := e2eGet(t, clientA, srv.URL, fmt.Sprintf("/galaxy/%d/%d", a.G, a.S))
		e2eWantContains(t, body, "Galaxy", "galaxy header")
		e2eWantContains(t, body, a.Username, "own homeworld (owner shown)")
		e2eWantContains(t, body, b.Username, "neighbour homeworld (owner shown)")
	})

	t.Run("arsenal activates a drawing", func(t *testing.T) {
		body := e2eGet(t, clientA, srv.URL, "/arsenal")
		e2eWantContains(t, body, "Arsenal", "arsenal page")
		e2eWantContains(t, body, "Laser weapons", "upgrade catalog row")

		if _, err := db.ExecContext(ctx,
			`INSERT INTO upgrade_items (account_id, upgrade_code, qty) VALUES ($1, 1, 1)
			 ON CONFLICT (account_id, upgrade_code) DO UPDATE SET qty = 1`, a.UserID); err != nil {
			t.Fatalf("seed drawing: %v", err)
		}
		body = e2ePost(t, clientA, srv.URL, "/arsenal/activate", url.Values{"code": {"1"}})
		e2eWantContains(t, body, "activated", "activation flash")
		if lvl := e2eScalarInt64(t, db,
			`SELECT COALESCE(level, 0) FROM account_upgrades WHERE account_id = $1 AND upgrade_code = 1`,
			a.UserID); lvl != 1 {
			t.Fatalf("activation level = %d, want 1", lvl)
		}
		// Edge: an unknown code is rejected without touching state.
		body = e2ePost(t, clientA, srv.URL, "/arsenal/activate", url.Values{"code": {"999"}})
		e2eWantContains(t, body, "unknown", "unknown-code rejection")
	})

	t.Run("black market lists, buys and withdraws", func(t *testing.T) {
		body := e2eGet(t, clientA, srv.URL, "/market")
		e2eWantContains(t, body, "Black market", "market page")

		// A holds two drawings, lists one.
		if _, err := db.ExecContext(ctx,
			`INSERT INTO upgrade_items (account_id, upgrade_code, qty) VALUES ($1, 1, 2)
			 ON CONFLICT (account_id, upgrade_code) DO UPDATE SET qty = 2`, a.UserID); err != nil {
			t.Fatalf("seed drawings: %v", err)
		}
		body = e2ePost(t, clientA, srv.URL, "/market/list",
			url.Values{"upgrade_code": {"1"}, "amount": {"1"}, "rate": {"50"}})
		e2eWantContains(t, body, "Listed", "listing flash")
		lotID := e2eScalarInt64(t, db,
			`SELECT id FROM market_lots WHERE seller_account_id = $1 ORDER BY id DESC LIMIT 1`, a.UserID)
		if lotID == 0 {
			t.Fatal("market lot was not created")
		}

		// Edge: the amount cap is enforced.
		body = e2ePost(t, clientA, srv.URL, "/market/list",
			url.Values{"upgrade_code": {"1"}, "amount": {"30"}, "rate": {"10"}})
		e2eWantContains(t, body, "invalid quantity", "amount cap")

		// Edge: you cannot buy your own lot.
		body = e2ePost(t, clientA, srv.URL, "/market/buy", url.Values{"lot_id": {strconv.FormatInt(lotID, 10)}})
		e2eWantContains(t, body, "cannot buy your own market lot", "own-lot rejection")

		// Edge: buyer without antimatter is rejected.
		if _, err := db.ExecContext(ctx, `UPDATE users SET antimatter = 0 WHERE id = $1`, b.UserID); err != nil {
			t.Fatalf("zero buyer antimatter: %v", err)
		}
		e2eLogin(t, clientB, srv.URL, b.Username)
		body = e2ePost(t, clientB, srv.URL, "/market/buy", url.Values{"lot_id": {strconv.FormatInt(lotID, 10)}})
		e2eWantContains(t, body, "insufficient antimatter", "insufficient ATM rejection")

		// Fund the buyer and complete the purchase.
		if _, err := db.ExecContext(ctx, `UPDATE users SET antimatter = 1000 WHERE id = $1`, b.UserID); err != nil {
			t.Fatalf("fund buyer: %v", err)
		}
		body = e2ePost(t, clientB, srv.URL, "/market/buy", url.Values{"lot_id": {strconv.FormatInt(lotID, 10)}})
		e2eWantContains(t, body, "Bought", "purchase flash")
		if got := e2eScalarInt64(t, db,
			`SELECT COALESCE(qty, 0) FROM upgrade_items WHERE account_id = $1 AND upgrade_code = 1`,
			b.UserID); got != 1 {
			t.Fatalf("buyer drawings = %d, want 1", got)
		}

		// A lists again and withdraws it.
		if _, err := db.ExecContext(ctx,
			`INSERT INTO upgrade_items (account_id, upgrade_code, qty) VALUES ($1, 1, 1)
			 ON CONFLICT (account_id, upgrade_code) DO UPDATE SET qty = 1`, a.UserID); err != nil {
			t.Fatalf("reseed drawing: %v", err)
		}
		e2ePost(t, clientA, srv.URL, "/market/list",
			url.Values{"upgrade_code": {"1"}, "amount": {"1"}, "rate": {"10"}})
		lot2 := e2eScalarInt64(t, db,
			`SELECT id FROM market_lots WHERE seller_account_id = $1 ORDER BY id DESC LIMIT 1`, a.UserID)
		body = e2ePost(t, clientA, srv.URL, "/market/remove", url.Values{"lot_id": {strconv.FormatInt(lot2, 10)}})
		e2eWantContains(t, body, "Lot withdrawn", "withdraw flash")
	})

	t.Run("technologies renders the tech tree", func(t *testing.T) {
		body := e2eGet(t, clientA, srv.URL, "/technologies")
		e2eWantContains(t, body, "Technologies", "tech page")
		e2eWantContains(t, body, "Research", "research group")
		e2eWantContains(t, body, "Light Cargo", "ship node")
		e2eWantContains(t, body, "Requires", "prerequisite column")
	})

	t.Run("simulator resolves a battle", func(t *testing.T) {
		body := e2eGet(t, clientA, srv.URL, "/simulator")
		e2eWantContains(t, body, "Simulator", "simulator page")
		e2eWantContains(t, body, "Light Fighter", "unit palette")
		body = e2ePost(t, clientA, srv.URL, "/simulator", url.Values{
			"a_207": {"100"}, "d_204": {"10"}, "a_weapons": {"5"},
		})
		e2eWantContains(t, body, "Result", "result section")
		e2eWantContains(t, body, "Winner", "winner label")
		// Edge: empty fleets surface a form error rather than a server error.
		body = e2ePost(t, clientA, srv.URL, "/simulator", url.Values{"a_204": {"0"}, "d_204": {"0"}})
		e2eWantContains(t, body, "fleet is empty", "empty-fleet error")
	})

	t.Run("expedition panel sends to deep space", func(t *testing.T) {
		panel := e2eGet(t, clientA, srv.URL, fmt.Sprintf("/expedition/%d", a.HomeID))
		e2eWantContains(t, panel, "Expedition command", "expedition panel")
		e2eWantContains(t, panel, "Active expeditions", "live slots panel")
		e2eWantContains(t, panel, "exp_time", "hold-time slider")
		e2eWantContains(t, panel, "exp_speed", "speed slider")
		e2eWantContains(t, panel, e2eCoord(a.G, a.S, 21), "auto deep-space target")

		body := e2ePost(t, clientA, srv.URL, fmt.Sprintf("/expedition/%d/send", a.HomeID), url.Values{
			"ship_204": {"1"}, "exp_time": {"4"}, "exp_speed": {"10"}, "count": {"1"},
		})
		e2eWantContains(t, body, "Sent 1 expedition", "expedition dispatch flash")
		e2eWantContains(t, body, "Slots", "slot counter after send")

		// exp_time 4 -> 60 min hold; the fleet targets [g:s:21].
		var holdMin float64
		var targetPos int
		err := db.QueryRowContext(ctx, `
			SELECT EXTRACT(EPOCH FROM (holding_end_time - arrival_time)) / 60, target_position
			FROM fleets
			WHERE user_id = $1 AND mission = 'EXPEDITION'
			ORDER BY id DESC LIMIT 1
		`, a.UserID).Scan(&holdMin, &targetPos)
		if err != nil {
			t.Fatalf("load dispatched expedition: %v", err)
		}
		if holdMin < 59 || holdMin > 61 {
			t.Fatalf("expedition hold = %.1f min, want 60", holdMin)
		}
		if targetPos != 21 {
			t.Fatalf("expedition target position = %d, want 21", targetPos)
		}
	})
}

// ---- helpers -------------------------------------------------------------

func e2eClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{Jar: jar, Timeout: e2eTimeout}
}

func e2eLogin(t *testing.T, c *http.Client, base, username string) {
	t.Helper()
	body := e2ePost(t, c, base, "/login", url.Values{
		"login": {username}, "password": {e2ePassword},
	})
	if strings.Contains(body, "Invalid username or password") {
		t.Fatalf("login as %s rejected; is migration 0016 applied?", username)
	}
	e2eWantContains(t, body, "Overview", "login redirects to the overview")
}

func e2eLoadPlayer(t *testing.T, ctx context.Context, db *sql.DB, username string) e2ePlayer {
	t.Helper()
	p := e2ePlayer{Username: username}
	err := db.QueryRowContext(ctx, `
		SELECT u.id, c.id, c.galaxy, c.system, c.position, c.name
		FROM users u
		JOIN universes un ON un.id = u.universe_id
		JOIN celestial_objects c ON c.user_id = u.id AND c.object_type = 'PLANET'
		WHERE un.code_name = $1 AND u.username = $2
		ORDER BY c.id
		LIMIT 1
	`, e2eUniverse, username).Scan(&p.UserID, &p.HomeID, &p.G, &p.S, &p.P, &p.Name)
	if err != nil {
		t.Fatalf("load seeded player %s: %v", username, err)
	}
	return p
}

// e2eReset restores the sandbox to a known baseline so repeated runs behave the
// same: in-flight fleets and queues are cleared, the attacker's fleet and the
// defender's fortifications are topped back up, and the values the flow mutates
// are reset. Migration 0016 is INSERT ... DO NOTHING, so it cannot do this.
func e2eReset(t *testing.T, ctx context.Context, db *sql.DB, a, b e2ePlayer) {
	t.Helper()
	steps := []struct {
		query string
		args  []any
	}{
		{`DELETE FROM fleets WHERE user_id = $1 OR user_id = $2`, []any{a.UserID, b.UserID}},
		{`DELETE FROM celestial_objects WHERE user_id = $1 AND id <> $2`, []any{a.UserID, a.HomeID}},
		{`DELETE FROM construction_queues WHERE celestial_id IN ($1, $2)`, []any{a.HomeID, b.HomeID}},
		{`DELETE FROM shipyard_queues WHERE celestial_id IN ($1, $2)`, []any{a.HomeID, b.HomeID}},
		{`DELETE FROM research_queues WHERE celestial_id IN ($1, $2)`, []any{a.HomeID, b.HomeID}},
		{`INSERT INTO planet_ships (celestial_id, ship_code, quantity) VALUES
			($1,'202',500),($1,'204',3000),($1,'207',500),($1,'208',5),($1,'210',500),($1,'219',300)
			ON CONFLICT (celestial_id, ship_code)
			DO UPDATE SET quantity = GREATEST(planet_ships.quantity, EXCLUDED.quantity)`, []any{a.HomeID}},
		{`INSERT INTO planet_defenses (celestial_id, defense_code, quantity) VALUES
			($1,'401',800),($1,'402',400),($1,'403',200),($1,'404',100),($1,'405',100),($1,'406',20),($1,'407',2)
			ON CONFLICT (celestial_id, defense_code)
			DO UPDATE SET quantity = GREATEST(planet_defenses.quantity, EXCLUDED.quantity)`, []any{b.HomeID}},
		{`UPDATE planet_structures SET level = 20 WHERE celestial_id = $1 AND structure_code = 'metal_mine'`, []any{a.HomeID}},
		{`UPDATE user_technologies SET level = 6 WHERE user_id = $1 AND tech_code = 'espionage_tech'`, []any{a.UserID}},
		{`DELETE FROM upgrade_items WHERE account_id IN ($1, $2)`, []any{a.UserID, b.UserID}},
		{`DELETE FROM account_upgrades WHERE account_id IN ($1, $2)`, []any{a.UserID, b.UserID}},
		{`DELETE FROM market_lots WHERE seller_account_id IN ($1, $2)`, []any{a.UserID, b.UserID}},
		{`UPDATE users SET antimatter = 1000000 WHERE id IN ($1, $2)`, []any{a.UserID, b.UserID}},
		{`UPDATE celestial_objects SET metal = 1.0e12, crystal = 5.0e11, deuterium = 1.0e11, fields_used = 0
		  WHERE id IN ($1, $2)`, []any{a.HomeID, b.HomeID}},
	}
	for _, s := range steps {
		if _, err := db.ExecContext(ctx, s.query, s.args...); err != nil {
			t.Fatalf("reset sandbox (%s): %v", firstLine(s.query), err)
		}
	}
}

func e2eGet(t *testing.T, c *http.Client, base, path string) string {
	t.Helper()
	resp, err := c.Get(base + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func e2ePost(t *testing.T, c *http.Client, base, path string, form url.Values) string {
	t.Helper()
	resp, err := c.PostForm(base+path, form)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func e2eScalarInt64(t *testing.T, db *sql.DB, query string, args ...any) int64 {
	t.Helper()
	var v int64
	if err := db.QueryRow(query, args...).Scan(&v); err != nil {
		t.Fatalf("query %s: %v", firstLine(query), err)
	}
	return v
}

func e2eWaitFor(t *testing.T, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(e2eTimeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(75 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", desc)
}

func e2eWantContains(t *testing.T, body, want, what string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("%s: response does not contain %q\n%s", what, want, e2eSnippet(body))
	}
}

func e2eCoord(g, s, p int) string { return fmt.Sprintf("%d:%d:%d", g, s, p) }

// e2eSnippet trims a response body so failures stay readable.
func e2eSnippet(body string) string {
	body = strings.Join(strings.Fields(body), " ")
	if len(body) > 3000 {
		return body[:3000] + "…"
	}
	return body
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

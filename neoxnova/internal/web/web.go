package web

import (
	"database/sql"
	"embed"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"neoxnova/internal/auth"
	"neoxnova/internal/store"
)

//go:embed static/*
var staticFiles embed.FS

const sessionTTL = 7 * 24 * time.Hour

// pageCSP is the Content-Security-Policy for server-rendered HTML pages. It
// allows same-origin scripts (htmx) and styles (compiled Tailwind) while
// keeping the API routes on the stricter default-src 'none' policy.
const pageCSP = "default-src 'self'; script-src 'self'; style-src 'self'; " +
	"img-src 'self' data:; font-src 'self'; connect-src 'self'; " +
	"frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

// Handler serves the server-rendered UI (Templ + htmx + Tailwind).
type Handler struct {
	db         *sql.DB
	universeID string
	planets    *store.PlanetStore
	builds     *store.BuildStore
	fleets     *store.FleetStore
	combat     *store.CombatReportStore
	espionage  *store.EspionageReportStore
	expedition *store.ExpeditionReportStore
	auth       *store.AuthStore
	notify     func()
}

// New builds the web handler. notify nudges the durable scheduler after a
// mutation; it may be nil.
func New(db *sql.DB, universeID string, notify func()) *Handler {
	return &Handler{
		db:         db,
		universeID: universeID,
		planets:    store.NewPlanetStore(db),
		builds:     store.NewBuildStore(db),
		fleets:     store.NewFleetStore(db),
		combat:     store.NewCombatReportStore(db),
		espionage:  store.NewEspionageReportStore(db),
		expedition: store.NewExpeditionReportStore(db),
		auth:       store.NewAuthStore(db),
		notify:     notify,
	}
}

// Mount registers every UI route on mux. Page routes never collide with the
// JSON API prefixes.
func (h *Handler) Mount(mux *http.ServeMux) {
	staticSub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("[WEB] static embed: %v", err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	mux.HandleFunc("GET /{$}", h.page(h.Home))
	mux.HandleFunc("GET /login", h.page(h.LoginPage))
	mux.HandleFunc("POST /login", h.page(h.LoginSubmit))
	mux.HandleFunc("GET /register", h.page(h.RegisterPage))
	mux.HandleFunc("POST /register", h.page(h.RegisterSubmit))
	mux.HandleFunc("POST /logout", h.page(h.Logout))

	mux.HandleFunc("GET /overview/{id}", h.page(h.requireUser(h.OverviewPage)))
	mux.HandleFunc("GET /overview/{id}/resources", h.page(h.requireUser(h.ResourcesPartial)))

	mux.HandleFunc("GET /buildings/{id}", h.page(h.requireUser(h.BuildingsPage)))
	mux.HandleFunc("POST /buildings/{id}/build", h.page(h.requireUser(h.BuildSubmit)))

	mux.HandleFunc("GET /shipyard/{id}", h.page(h.requireUser(h.ShipyardPage)))
	mux.HandleFunc("POST /shipyard/{id}/build", h.page(h.requireUser(h.ShipyardSubmit)))

	mux.HandleFunc("GET /research/{id}", h.page(h.requireUser(h.ResearchPage)))
	mux.HandleFunc("POST /research/{id}/start", h.page(h.requireUser(h.ResearchSubmit)))

	mux.HandleFunc("GET /fleet/{id}", h.page(h.requireUser(h.FleetPage)))
	mux.HandleFunc("POST /fleet/{id}/dispatch", h.page(h.requireUser(h.FleetDispatchSubmit)))
	mux.HandleFunc("POST /fleet/{id}/recall", h.page(h.requireUser(h.FleetRecallSubmit)))

	mux.HandleFunc("GET /galaxy", h.page(h.requireUser(h.GalaxyRedirect)))
	mux.HandleFunc("GET /galaxy/{galaxy}/{system}", h.page(h.requireUser(h.GalaxyPage)))

	mux.HandleFunc("GET /reports/{id}", h.page(h.requireUser(h.ReportsPage)))
}

// page applies the HTML CSP to a handler.
func (h *Handler) page(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", pageCSP)
		next(w, r)
	}
}

// userID resolves the session cookie to a user id.
func (h *Handler) userID(r *http.Request) (int64, bool) {
	token, ok := auth.TokenFromRequest(r)
	if !ok {
		return 0, false
	}
	id, err := h.auth.SessionUser(r.Context(), auth.HashToken(token))
	if err != nil {
		return 0, false
	}
	return id, true
}

// requireUser redirects anonymous visitors to the login screen.
func (h *Handler) requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := h.userID(r); !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func (h *Handler) secureCookies() bool {
	return os.Getenv("APP_ENV") != "development"
}

func (h *Handler) nudge() {
	if h.notify != nil {
		h.notify()
	}
}

// Home sends a logged-in player to their homeworld, everyone else to login.
func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	homeID, err := h.planets.HomeworldID(r.Context(), userID)
	if err != nil {
		log.Printf("[WEB] homeworld lookup: %v", err)
	}
	if homeID == 0 {
		if planets, err := h.planets.ListOwnedCelestials(r.Context(), userID); err == nil && len(planets) > 0 {
			homeID = planets[0].ID
		}
	}
	if homeID == 0 {
		h.render(w, r, http.StatusOK, EmptyPage())
		return
	}
	http.Redirect(w, r, "/overview/"+strconv.FormatInt(homeID, 10), http.StatusSeeOther)
}

func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, AuthView{Mode: "login"})
}

func (h *Handler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, AuthView{Mode: "register"})
}

func (h *Handler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	login := strings.TrimSpace(r.FormValue("login"))
	password := r.FormValue("password")
	userID, hash, err := h.auth.UserAuth(r.Context(), login)
	if err != nil || !auth.VerifyPassword(password, hash) {
		h.render(w, r, http.StatusUnauthorized, AuthView{Mode: "login", Error: "Invalid username or password", Login: login})
		return
	}
	h.issueSession(w, r, userID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) RegisterSubmit(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	password := r.FormValue("password")
	fail := func(msg string) {
		h.render(w, r, http.StatusBadRequest, AuthView{Mode: "register", Error: msg, Login: username, Email: email})
	}
	if len(username) < 3 || len(username) > 32 {
		fail("Username must be 3-32 chars")
		return
	}
	if !strings.Contains(email, "@") {
		fail("Enter a valid email")
		return
	}
	if len(password) < 10 {
		fail("Password must be at least 10 characters")
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		fail("Could not secure password")
		return
	}
	userID, _, err := h.auth.CreateUser(r.Context(), h.universeID, username, email, hash)
	if err != nil {
		if errors.Is(err, store.ErrUserExists) {
			fail("Username or email already registered")
			return
		}
		log.Printf("[WEB] register failed: %v", err)
		fail("Could not create account")
		return
	}
	h.issueSession(w, r, userID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if token, ok := auth.TokenFromRequest(r); ok {
		_ = h.auth.DeleteSession(r.Context(), auth.HashToken(token))
	}
	auth.ClearSessionCookie(w, h.secureCookies())
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) issueSession(w http.ResponseWriter, r *http.Request, userID int64) {
	raw, hash, err := auth.NewSessionToken()
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	_ = h.auth.CreateSession(r.Context(), userID, hash, time.Now().Add(sessionTTL), r.UserAgent(), ip)
	auth.SetSessionCookie(w, raw, int(sessionTTL.Seconds()), h.secureCookies())
}

// render writes a Templ component as an HTML response.
func (h *Handler) render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := c.Render(r.Context(), w); err != nil {
		log.Printf("[WEB] render: %v", err)
	}
}

func parseID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

func flashFor(r *http.Request) (ok, errMsg string) {
	return r.URL.Query().Get("ok"), r.URL.Query().Get("err")
}

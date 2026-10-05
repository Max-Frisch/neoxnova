package handlers

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"neoxnova/internal/auth"
	"neoxnova/internal/store"
)

const (
	sessionTTL        = 7 * 24 * time.Hour
	minPasswordLength = 10
)

var (
	usernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`)
	emailRe    = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

// dummyHash is verified against when the account does not exist, so login
// latency does not reveal whether a username is registered.
const dummyHash = "pbkdf2_sha256$600000$L5s8bx0R/Xpb+Cl/DRhVqg$IKf0HjYPPk/HN4p7JsX5uJB8iKFb+oR2CVy79bY94jo"

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Login    string `json:"login"` // username or email
	Password string `json:"password"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if !usernameRe.MatchString(req.Username) {
		writeError(w, http.StatusBadRequest, "Username must be 3-32 chars: letters, digits, underscore")
		return
	}
	if !emailRe.MatchString(req.Email) {
		writeError(w, http.StatusBadRequest, "Invalid email address")
		return
	}
	if len(req.Password) < minPasswordLength {
		writeError(w, http.StatusBadRequest, "Password must be at least 10 characters")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to secure password")
		return
	}
	userID, err := h.Auth.CreateUser(r.Context(), h.UniverseID, req.Username, req.Email, hash)
	if err != nil {
		if errors.Is(err, store.ErrUserExists) {
			writeError(w, http.StatusConflict, "Username or email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to create account")
		return
	}
	h.issueSession(w, r, userID)
	writeJSON(w, http.StatusCreated, map[string]any{"user_id": userID, "username": req.Username})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	req.Login = strings.TrimSpace(req.Login)
	key := strings.ToLower(req.Login)
	if h.loginGuard.locked(key) {
		writeError(w, http.StatusTooManyRequests, "Too many failed attempts; try again later")
		return
	}
	userID, hash, lookupErr := h.Auth.UserAuth(r.Context(), req.Login)
	if lookupErr != nil {
		hash = dummyHash // run the KDF anyway to avoid user-enumeration timing
	}
	// Always run the KDF to avoid user-enumeration timing side channels.
	if lookupErr != nil || !auth.VerifyPassword(req.Password, hash) {
		h.loginGuard.fail(key)
		writeError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}
	h.loginGuard.reset(key)
	h.issueSession(w, r, userID)
	writeJSON(w, http.StatusOK, map[string]any{"user_id": userID})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if token, ok := auth.TokenFromRequest(r); ok {
		_ = h.Auth.DeleteSession(r.Context(), auth.HashToken(token))
	}
	auth.ClearSessionCookie(w, h.secureCookies())
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	username, email, err := h.Auth.UserByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load profile")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": userID, "username": username, "email": email})
}

// issueSession creates a session row and sets the hardened cookie.
func (h *Handler) issueSession(w http.ResponseWriter, r *http.Request, userID int64) {
	raw, hash, err := auth.NewSessionToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create session")
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if err := h.Auth.CreateSession(r.Context(), userID, hash, time.Now().Add(sessionTTL), r.UserAgent(), ip); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to persist session")
		return
	}
	auth.SetSessionCookie(w, raw, int(sessionTTL.Seconds()), h.secureCookies())
}

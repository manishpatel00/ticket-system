package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/evabharat/ticket-system/internal/auth"
	"github.com/evabharat/ticket-system/internal/store"
)

// emailPattern is a deliberately simple RFC-5322-ish check. Full email
// validation is famously a rabbit hole; this catches the obvious mistakes
// (missing @, missing domain) without over-engineering.
var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

const minPasswordLength = 8

// AuthHandler groups the register/login endpoints, which both need access
// to the store (to read/write users) and the JWT manager (to issue
// tokens on login).
type AuthHandler struct {
	store      store.Store
	jwtManager *auth.Manager
}

func NewAuthHandler(s store.Store, jm *auth.Manager) *AuthHandler {
	return &AuthHandler{store: s, jwtManager: jm}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type registerResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// Register handles POST /auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || !emailPattern.MatchString(req.Email) {
		writeError(w, http.StatusBadRequest, "a valid email is required")
		return
	}
	if len(req.Password) < minPasswordLength {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not process password")
		return
	}

	user, err := h.store.CreateUser(req.Email, hash)
	if err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create user")
		return
	}

	writeJSON(w, http.StatusCreated, registerResponse{ID: user.ID, Email: user.Email})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

// Login handles POST /auth/login.
//
// Deliberately returns the exact same 401 error for "user not found" and
// "wrong password" — differentiating them would let an attacker enumerate
// registered emails.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	const invalidCreds = "invalid email or password"

	user, err := h.store.GetUserByEmail(req.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, invalidCreds)
		return
	}
	if !auth.VerifyPassword(req.Password, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, invalidCreds)
		return
	}

	token, err := h.jwtManager.Issue(user.ID, user.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}

	writeJSON(w, http.StatusOK, loginResponse{Token: token})
}

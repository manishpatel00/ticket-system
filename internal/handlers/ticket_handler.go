package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/evabharat/ticket-system/internal/middleware"
	"github.com/evabharat/ticket-system/internal/models"
	"github.com/evabharat/ticket-system/internal/store"
)

// TicketHandler groups all /tickets endpoints. It only depends on the
// Store interface, not the concrete MemoryStore, so it can be tested with
// a fake and swapped to a DB-backed store without code changes here.
type TicketHandler struct {
	store store.Store
}

func NewTicketHandler(s store.Store) *TicketHandler {
	return &TicketHandler{store: s}
}

// mustUserID pulls the authenticated user ID out of the request context.
// It should only ever be called on routes wrapped by middleware.RequireAuth,
// which guarantees the value is present — the ok=false branch here is a
// defensive 500, not a normal code path.
func mustUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "missing auth context")
		return "", false
	}
	return userID, true
}

type createTicketRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Create handles POST /tickets.
func (h *TicketHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := mustUserID(w, r)
	if !ok {
		return
	}

	var req createTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}

	ticket, err := h.store.CreateTicket(userID, req.Title, req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create ticket")
		return
	}
	writeJSON(w, http.StatusCreated, ticket)
}

// List handles GET /tickets — returns only tickets owned by the caller.
func (h *TicketHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := mustUserID(w, r)
	if !ok {
		return
	}

	tickets, err := h.store.ListTicketsByUser(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list tickets")
		return
	}
	writeJSON(w, http.StatusOK, tickets)
}

// Get handles GET /tickets/{id}.
//
// Ownership check: if the ticket exists but belongs to someone else, we
// return 404 (not 403). This avoids leaking whether a given ticket ID
// exists at all to a user who doesn't own it.
func (h *TicketHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := mustUserID(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")

	ticket, err := h.store.GetTicketByID(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "ticket not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not fetch ticket")
		return
	}
	if ticket.UserID != userID {
		writeError(w, http.StatusNotFound, "ticket not found")
		return
	}
	writeJSON(w, http.StatusOK, ticket)
}

type updateStatusRequest struct {
	Status string `json:"status"`
}

// UpdateStatus handles PATCH /tickets/{id}/status.
//
// Enforces two things in order: (1) ownership — same 404-for-others
// behavior as Get — and (2) the status state machine, so e.g. open ->
// closed directly, or closed -> anything, are rejected with 409 Conflict
// (the request is well-formed but conflicts with the ticket's current
// state).
func (h *TicketHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := mustUserID(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")

	var req updateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	newStatus := models.TicketStatus(strings.TrimSpace(req.Status))
	if !newStatus.IsValid() {
		writeError(w, http.StatusBadRequest, "status must be one of: open, in_progress, closed")
		return
	}

	ticket, err := h.store.GetTicketByID(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "ticket not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not fetch ticket")
		return
	}
	if ticket.UserID != userID {
		writeError(w, http.StatusNotFound, "ticket not found")
		return
	}

	if !models.CanTransition(ticket.Status, newStatus) {
		writeError(w, http.StatusConflict,
			"invalid status transition from "+string(ticket.Status)+" to "+string(newStatus))
		return
	}

	updated, err := h.store.UpdateTicketStatus(id, newStatus)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update ticket")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

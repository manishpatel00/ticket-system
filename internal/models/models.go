// Package models defines the core domain entities used across the ticket
// system: User and Ticket. Keeping these as plain structs (no ORM tags,
// no persistence-layer leakage) keeps the domain layer independent of how
// data is actually stored (in-memory today, swappable for SQL later).
package models

import "time"

// TicketStatus is a closed set of allowed ticket states.
type TicketStatus string

const (
	StatusOpen       TicketStatus = "open"
	StatusInProgress TicketStatus = "in_progress"
	StatusClosed     TicketStatus = "closed"
)

// IsValid reports whether s is one of the known ticket statuses.
func (s TicketStatus) IsValid() bool {
	switch s {
	case StatusOpen, StatusInProgress, StatusClosed:
		return true
	default:
		return false
	}
}

// User represents a registered account.
// PasswordHash is never serialized to JSON (see json:"-") so it can never
// accidentally leak in an API response.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// Ticket represents a single support ticket owned by exactly one user.
type Ticket struct {
	ID          string       `json:"id"`
	UserID      string       `json:"user_id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Status      TicketStatus `json:"status"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// AllowedTransitions encodes the only legal forward moves in the ticket
// status state machine. A closed ticket has no outgoing edges, i.e. it can
// never be reopened, per the assignment contract.
var AllowedTransitions = map[TicketStatus][]TicketStatus{
	StatusOpen:       {StatusInProgress},
	StatusInProgress: {StatusClosed},
	StatusClosed:     {}, // terminal state
}

// CanTransition reports whether moving from `from` to `to` is a legal
// single-step transition in the status state machine.
func CanTransition(from, to TicketStatus) bool {
	for _, allowed := range AllowedTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

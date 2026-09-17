// Package store provides a thread-safe in-memory persistence layer.
//
// The assignment explicitly allows in-memory storage ("no complex database
// schema is required"), so this keeps the project simple and dependency
// free while still being safe under Go's default concurrent HTTP server
// (every request is handled on its own goroutine).
//
// The Store is defined as an interface further down purely so a future
// SQLite/Postgres-backed implementation could be dropped in without
// touching any handler code — but only the in-memory implementation is
// used here, matching the "keep it simple" brief.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/evabharat/ticket-system/internal/models"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrEmailTaken = errors.New("email already registered")
)

// newID returns a random 16-hex-char identifier. Using crypto/rand keeps
// IDs unguessable without pulling in an external UUID library.
func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Store is the persistence contract used by HTTP handlers.
type Store interface {
	CreateUser(email, passwordHash string) (*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
	GetUserByID(id string) (*models.User, error)

	CreateTicket(userID, title, description string) (*models.Ticket, error)
	ListTicketsByUser(userID string) ([]*models.Ticket, error)
	GetTicketByID(id string) (*models.Ticket, error)
	UpdateTicketStatus(id string, newStatus models.TicketStatus) (*models.Ticket, error)
}

// MemoryStore is an in-memory Store implementation guarded by a single
// RWMutex. Reads (List/Get) take a read lock; writes take a write lock.
type MemoryStore struct {
	mu sync.RWMutex

	usersByID    map[string]*models.User
	usersByEmail map[string]string // email -> userID
	tickets      map[string]*models.Ticket
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		usersByID:    make(map[string]*models.User),
		usersByEmail: make(map[string]string),
		tickets:      make(map[string]*models.Ticket),
	}
}

func (s *MemoryStore) CreateUser(email, passwordHash string) (*models.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.usersByEmail[email]; exists {
		return nil, ErrEmailTaken
	}

	u := &models.User{
		ID:           newID(),
		Email:        email,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now().UTC(),
	}
	s.usersByID[u.ID] = u
	s.usersByEmail[email] = u.ID
	return u, nil
}

func (s *MemoryStore) GetUserByEmail(email string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.usersByEmail[email]
	if !ok {
		return nil, ErrNotFound
	}
	return s.usersByID[id], nil
}

func (s *MemoryStore) GetUserByID(id string) (*models.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u, ok := s.usersByID[id]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

func (s *MemoryStore) CreateTicket(userID, title, description string) (*models.Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	t := &models.Ticket{
		ID:          newID(),
		UserID:      userID,
		Title:       title,
		Description: description,
		Status:      models.StatusOpen,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.tickets[t.ID] = t
	return t, nil
}

func (s *MemoryStore) ListTicketsByUser(userID string) ([]*models.Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*models.Ticket, 0)
	for _, t := range s.tickets {
		if t.UserID == userID {
			result = append(result, t)
		}
	}
	return result, nil
}

func (s *MemoryStore) GetTicketByID(id string) (*models.Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.tickets[id]
	if !ok {
		return nil, ErrNotFound
	}
	return t, nil
}

func (s *MemoryStore) UpdateTicketStatus(id string, newStatus models.TicketStatus) (*models.Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tickets[id]
	if !ok {
		return nil, ErrNotFound
	}
	t.Status = newStatus
	t.UpdatedAt = time.Now().UTC()
	return t, nil
}

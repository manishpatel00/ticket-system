package store

import (
	"errors"
	"sync"
	"testing"

	"github.com/evabharat/ticket-system/internal/models"
)

func TestCreateUser_DuplicateEmail(t *testing.T) {
	s := NewMemoryStore()

	if _, err := s.CreateUser("alice@example.com", "hash1"); err != nil {
		t.Fatalf("unexpected error on first create: %v", err)
	}
	_, err := s.CreateUser("alice@example.com", "hash2")
	if !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestGetUserByEmail_NotFound(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.GetUserByEmail("nobody@example.com")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTicketOwnershipIsolation(t *testing.T) {
	s := NewMemoryStore()
	alice, _ := s.CreateUser("alice@example.com", "hash")
	bob, _ := s.CreateUser("bob@example.com", "hash")

	if _, err := s.CreateTicket(alice.ID, "Alice ticket 1", "desc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := s.CreateTicket(alice.ID, "Alice ticket 2", "desc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := s.CreateTicket(bob.ID, "Bob ticket 1", "desc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	aliceTickets, err := s.ListTicketsByUser(alice.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(aliceTickets) != 2 {
		t.Fatalf("expected alice to have 2 tickets, got %d", len(aliceTickets))
	}

	bobTickets, err := s.ListTicketsByUser(bob.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bobTickets) != 1 {
		t.Fatalf("expected bob to have 1 ticket, got %d", len(bobTickets))
	}
}

func TestUpdateTicketStatus_NotFound(t *testing.T) {
	s := NewMemoryStore()
	_, err := s.UpdateTicketStatus("nonexistent-id", models.StatusInProgress)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestConcurrentAccess exercises the store from many goroutines
// simultaneously. Run with `go test -race` to verify there is no data
// race on the underlying maps (the mutex should serialize all access).
func TestConcurrentAccess(t *testing.T) {
	s := NewMemoryStore()
	user, err := s.CreateUser("concurrent@example.com", "hash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			ticket, err := s.CreateTicket(user.ID, "concurrent ticket", "desc")
			if err != nil {
				t.Errorf("CreateTicket failed: %v", err)
				return
			}
			if _, err := s.GetTicketByID(ticket.ID); err != nil {
				t.Errorf("GetTicketByID failed: %v", err)
			}
			if _, err := s.UpdateTicketStatus(ticket.ID, models.StatusInProgress); err != nil {
				t.Errorf("UpdateTicketStatus failed: %v", err)
			}
			if _, err := s.ListTicketsByUser(user.ID); err != nil {
				t.Errorf("ListTicketsByUser failed: %v", err)
			}
		}()
	}
	wg.Wait()

	tickets, err := s.ListTicketsByUser(user.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tickets) != goroutines {
		t.Fatalf("expected %d tickets, got %d", goroutines, len(tickets))
	}
}

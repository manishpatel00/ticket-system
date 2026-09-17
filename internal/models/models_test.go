package models

import "testing"

func TestTicketStatus_IsValid(t *testing.T) {
	valid := []TicketStatus{StatusOpen, StatusInProgress, StatusClosed}
	for _, s := range valid {
		if !s.IsValid() {
			t.Errorf("expected %q to be valid", s)
		}
	}

	invalid := []TicketStatus{"", "OPEN", "pending", "deleted", "in-progress"}
	for _, s := range invalid {
		if s.IsValid() {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestCanTransition(t *testing.T) {
	cases := []struct {
		from, to TicketStatus
		want     bool
	}{
		{StatusOpen, StatusInProgress, true},
		{StatusInProgress, StatusClosed, true},

		// Cannot skip a state.
		{StatusOpen, StatusClosed, false},

		// Cannot move backward.
		{StatusInProgress, StatusOpen, false},

		// Closed is terminal — cannot reopen in any direction.
		{StatusClosed, StatusOpen, false},
		{StatusClosed, StatusInProgress, false},

		// No-op "transitions" to the same state are also disallowed —
		// the state machine only models forward progress.
		{StatusOpen, StatusOpen, false},
		{StatusInProgress, StatusInProgress, false},
		{StatusClosed, StatusClosed, false},
	}

	for _, c := range cases {
		got := CanTransition(c.from, c.to)
		if got != c.want {
			t.Errorf("CanTransition(%q, %q) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

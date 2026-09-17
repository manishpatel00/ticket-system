package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/evabharat/ticket-system/internal/auth"
	"github.com/evabharat/ticket-system/internal/store"
)

// newTestServer spins up the real router (same wiring as main()) backed
// by a fresh in-memory store, so these tests exercise the exact same code
// path a real client would hit.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	jwtManager, err := auth.NewManager("integration-test-secret", time.Hour)
	if err != nil {
		t.Fatalf("failed to create jwt manager: %v", err)
	}
	router := buildRouter(store.NewMemoryStore(), jwtManager)
	return httptest.NewServer(router)
}

func doJSON(t *testing.T, method, url, token string, body interface{}) (*http.Response, map[string]interface{}) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("failed to encode body: %v", err)
		}
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var parsed map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	return resp, parsed
}

func TestHealthEndpoint(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, body := doJSON(t, http.MethodGet, srv.URL+"/health", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if body["status"] != "ok" {
		t.Fatalf(`expected status "ok", got %v`, body["status"])
	}
}

func TestFullTicketLifecycle(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// Register.
	resp, _ := doJSON(t, http.MethodPost, srv.URL+"/auth/register", "", map[string]string{
		"email": "alice@example.com", "password": "password123",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: expected 201, got %d", resp.StatusCode)
	}

	// Login.
	resp, loginBody := doJSON(t, http.MethodPost, srv.URL+"/auth/login", "", map[string]string{
		"email": "alice@example.com", "password": "password123",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: expected 200, got %d", resp.StatusCode)
	}
	token, _ := loginBody["token"].(string)
	if token == "" {
		t.Fatal("expected non-empty token from login")
	}

	// Create ticket.
	resp, ticketBody := doJSON(t, http.MethodPost, srv.URL+"/tickets", token, map[string]string{
		"title": "My first ticket", "description": "Something is broken",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create ticket: expected 201, got %d", resp.StatusCode)
	}
	if ticketBody["status"] != "open" {
		t.Fatalf("expected new ticket status open, got %v", ticketBody["status"])
	}
	ticketID, _ := ticketBody["id"].(string)
	if ticketID == "" {
		t.Fatal("expected non-empty ticket id")
	}

	// List tickets.
	resp, _ = doJSON(t, http.MethodGet, srv.URL+"/tickets", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list tickets: expected 200, got %d", resp.StatusCode)
	}

	// Get ticket by ID.
	resp, _ = doJSON(t, http.MethodGet, srv.URL+"/tickets/"+ticketID, token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get ticket: expected 200, got %d", resp.StatusCode)
	}

	// Valid transition: open -> in_progress.
	resp, statusBody := doJSON(t, http.MethodPatch, srv.URL+"/tickets/"+ticketID+"/status", token,
		map[string]string{"status": "in_progress"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status to in_progress: expected 200, got %d", resp.StatusCode)
	}
	if statusBody["status"] != "in_progress" {
		t.Fatalf("expected status in_progress, got %v", statusBody["status"])
	}

	// Invalid transition: in_progress -> open.
	resp, _ = doJSON(t, http.MethodPatch, srv.URL+"/tickets/"+ticketID+"/status", token,
		map[string]string{"status": "open"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 reopening from in_progress, got %d", resp.StatusCode)
	}

	// Valid transition: in_progress -> closed.
	resp, _ = doJSON(t, http.MethodPatch, srv.URL+"/tickets/"+ticketID+"/status", token,
		map[string]string{"status": "closed"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status to closed: expected 200, got %d", resp.StatusCode)
	}

	// Closed tickets can never be reopened.
	resp, _ = doJSON(t, http.MethodPatch, srv.URL+"/tickets/"+ticketID+"/status", token,
		map[string]string{"status": "open"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 reopening a closed ticket, got %d", resp.StatusCode)
	}
	resp, _ = doJSON(t, http.MethodPatch, srv.URL+"/tickets/"+ticketID+"/status", token,
		map[string]string{"status": "in_progress"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 moving a closed ticket to in_progress, got %d", resp.StatusCode)
	}
}

func TestCrossUserTicketAccessIsForbidden(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	registerAndLogin := func(email, password string) string {
		doJSON(t, http.MethodPost, srv.URL+"/auth/register", "", map[string]string{
			"email": email, "password": password,
		})
		_, body := doJSON(t, http.MethodPost, srv.URL+"/auth/login", "", map[string]string{
			"email": email, "password": password,
		})
		token, _ := body["token"].(string)
		return token
	}

	aliceToken := registerAndLogin("alice2@example.com", "password123")
	bobToken := registerAndLogin("bob2@example.com", "password456")

	_, ticketBody := doJSON(t, http.MethodPost, srv.URL+"/tickets", aliceToken, map[string]string{
		"title": "Alice's private ticket", "description": "secret",
	})
	ticketID, _ := ticketBody["id"].(string)

	// Bob must not be able to read Alice's ticket.
	resp, _ := doJSON(t, http.MethodGet, srv.URL+"/tickets/"+ticketID, bobToken, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-user GET, got %d", resp.StatusCode)
	}

	// Bob must not be able to update Alice's ticket.
	resp, _ = doJSON(t, http.MethodPatch, srv.URL+"/tickets/"+ticketID+"/status", bobToken,
		map[string]string{"status": "in_progress"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-user status update, got %d", resp.StatusCode)
	}

	// Bob's own ticket list must not include Alice's ticket.
	resp, _ = doJSON(t, http.MethodGet, srv.URL+"/tickets", bobToken, nil)
	var list []map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&list)
	for _, tk := range list {
		if tk["id"] == ticketID {
			t.Fatal("bob's ticket list must not contain alice's ticket")
		}
	}
}

func TestUnauthenticatedRequestsAreRejected(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	endpoints := []struct {
		method, path string
	}{
		{http.MethodGet, "/tickets"},
		{http.MethodPost, "/tickets"},
		{http.MethodGet, "/tickets/some-id"},
		{http.MethodPatch, "/tickets/some-id/status"},
	}
	for _, e := range endpoints {
		resp, _ := doJSON(t, e.method, srv.URL+e.path, "", nil)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without token: expected 401, got %d", e.method, e.path, resp.StatusCode)
		}
	}
}

func TestRegisterValidation(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	cases := []struct {
		name       string
		email      string
		password   string
		wantStatus int
	}{
		{"invalid email", "not-an-email", "password123", http.StatusBadRequest},
		{"short password", "valid@example.com", "short", http.StatusBadRequest},
		{"valid", "valid2@example.com", "password123", http.StatusCreated},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp, _ := doJSON(t, http.MethodPost, srv.URL+"/auth/register", "", map[string]string{
				"email": c.email, "password": c.password,
			})
			if resp.StatusCode != c.wantStatus {
				t.Errorf("expected %d, got %d", c.wantStatus, resp.StatusCode)
			}
		})
	}
}

func TestLoginWithWrongPasswordFails(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	doJSON(t, http.MethodPost, srv.URL+"/auth/register", "", map[string]string{
		"email": "wrongpass@example.com", "password": "correct-password",
	})
	resp, _ := doJSON(t, http.MethodPost, srv.URL+"/auth/login", "", map[string]string{
		"email": "wrongpass@example.com", "password": "incorrect-password",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", resp.StatusCode)
	}
}

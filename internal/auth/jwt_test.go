package auth

import (
	"errors"
	"testing"
	"time"
)

func TestIssueAndParseAndVerify_Success(t *testing.T) {
	m, err := NewManager("test-secret", time.Hour)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	token, err := m.Issue("user-123", "alice@example.com")
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := m.ParseAndVerify(token)
	if err != nil {
		t.Fatalf("ParseAndVerify returned error: %v", err)
	}
	if claims.Subject != "user-123" {
		t.Errorf("expected subject user-123, got %s", claims.Subject)
	}
	if claims.Email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %s", claims.Email)
	}
}

func TestNewManager_EmptySecret(t *testing.T) {
	if _, err := NewManager("", time.Hour); err == nil {
		t.Fatal("expected error when creating manager with empty secret")
	}
}

func TestParseAndVerify_WrongSecret(t *testing.T) {
	m1, _ := NewManager("secret-one", time.Hour)
	m2, _ := NewManager("secret-two", time.Hour)

	token, err := m1.Issue("user-123", "alice@example.com")
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	_, err = m2.ParseAndVerify(token)
	if !errors.Is(err, ErrInvalidSig) {
		t.Fatalf("expected ErrInvalidSig, got %v", err)
	}
}

func TestParseAndVerify_ExpiredToken(t *testing.T) {
	// Negative TTL guard in NewManager forces a minimum of 24h, so we
	// build the manager with a valid TTL, then manually issue a token
	// with an already-past expiry to simulate expiration deterministically
	// rather than sleeping in a test.
	m, _ := NewManager("test-secret", time.Hour)
	m.ttl = -time.Hour // force Issue to set exp in the past

	token, err := m.Issue("user-123", "alice@example.com")
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	_, err = m.ParseAndVerify(token)
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}
}

func TestParseAndVerify_MalformedToken(t *testing.T) {
	m, _ := NewManager("test-secret", time.Hour)

	cases := []string{
		"",
		"not-a-jwt",
		"only.two",
		"a.b.c.d",
		"..",
	}
	for _, c := range cases {
		if _, err := m.ParseAndVerify(c); err == nil {
			t.Errorf("expected error parsing malformed token %q", c)
		}
	}
}

func TestParseAndVerify_TamperedPayload(t *testing.T) {
	m, _ := NewManager("test-secret", time.Hour)
	token, err := m.Issue("user-123", "alice@example.com")
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	// Flip a character in the token to simulate tampering; the signature
	// check must catch this regardless of where the flip lands.
	tampered := []byte(token)
	tampered[len(tampered)-2]++
	_, err = m.ParseAndVerify(string(tampered))
	if err == nil {
		t.Fatal("expected error verifying tampered token")
	}
}

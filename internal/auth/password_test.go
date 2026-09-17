package auth

import "testing"

func TestHashAndVerifyPassword_Success(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if !VerifyPassword("correct-horse-battery-staple", hash) {
		t.Fatal("expected VerifyPassword to succeed with correct password")
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if VerifyPassword("wrong-password", hash) {
		t.Fatal("expected VerifyPassword to fail with wrong password")
	}
}

func TestHashPassword_EmptyPassword(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("expected error when hashing an empty password")
	}
}

func TestHashPassword_UniqueSaltPerCall(t *testing.T) {
	// Same password hashed twice must produce different encoded hashes,
	// proving the salt is actually randomized per call (not reused).
	h1, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	h2, err := HashPassword("same-password")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if h1 == h2 {
		t.Fatal("expected two hashes of the same password to differ due to random salt")
	}
	// But both must still verify correctly.
	if !VerifyPassword("same-password", h1) || !VerifyPassword("same-password", h2) {
		t.Fatal("both independently-salted hashes must verify the same password")
	}
}

func TestVerifyPassword_MalformedHash(t *testing.T) {
	cases := []string{
		"",
		"not-a-valid-hash",
		"pbkdf2-sha256$notanumber$abcd$abcd",
		"wrong-algo$100000$abcd$abcd",
	}
	for _, c := range cases {
		if VerifyPassword("anything", c) {
			t.Fatalf("expected VerifyPassword to reject malformed hash %q", c)
		}
	}
}

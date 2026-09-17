// Package auth provides password hashing and JWT issuance/verification.
//
// Deliberate design choice: everything here is built on Go's standard
// library only (crypto/hmac, crypto/sha256, crypto/rand, crypto/subtle,
// encoding/base64). This keeps the module dependency-free, which means
// `go build` never needs network access to a module proxy — important for
// reproducible Docker builds on constrained free-tier CI/deploy platforms.
//
// Passwords are hashed with PBKDF2-HMAC-SHA256 (RFC 2898), a well-reviewed,
// standard-library-buildable KDF with a tunable iteration count and a
// per-password random salt. This satisfies "passwords must be stored as
// hashes, not plain text" with a deliberately slow, salted algorithm
// (as opposed to a fast unsalted hash like plain SHA-256).
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

const (
	pbkdf2Iterations = 100_000
	saltBytes        = 16
	keyBytes         = 32
)

// pbkdf2 derives a keyBytes-length key from password+salt using
// HMAC-SHA256 as the pseudorandom function, per RFC 2898.
func pbkdf2(password, salt []byte, iterations, keyLen int) []byte {
	prf := hmac.New(sha256.New, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen

	var derivedKey []byte
	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		var blockIndex [4]byte
		blockIndex[0] = byte(block >> 24)
		blockIndex[1] = byte(block >> 16)
		blockIndex[2] = byte(block >> 8)
		blockIndex[3] = byte(block)
		prf.Write(blockIndex[:])

		u := prf.Sum(nil)
		t := make([]byte, len(u))
		copy(t, u)

		for i := 1; i < iterations; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		derivedKey = append(derivedKey, t...)
	}
	return derivedKey[:keyLen]
}

// HashPassword returns an encoded string of the form:
//
//	pbkdf2-sha256$<iterations>$<salt-hex>$<hash-hex>
//
// which is self-describing, so the iteration count and salt travel with
// the hash and can be verified later without any external state.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", fmt.Errorf("password must not be empty")
	}
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}
	derived := pbkdf2([]byte(password), salt, pbkdf2Iterations, keyBytes)
	encoded := fmt.Sprintf("pbkdf2-sha256$%d$%s$%s",
		pbkdf2Iterations, hex.EncodeToString(salt), hex.EncodeToString(derived))
	return encoded, nil
}

// VerifyPassword checks a plaintext password against a hash produced by
// HashPassword, using a constant-time comparison to avoid timing attacks.
func VerifyPassword(password, encodedHash string) bool {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations <= 0 {
		return false
	}
	salt, err := hex.DecodeString(parts[2])
	if err != nil {
		return false
	}
	expected, err := hex.DecodeString(parts[3])
	if err != nil {
		return false
	}
	actual := pbkdf2([]byte(password), salt, iterations, len(expected))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

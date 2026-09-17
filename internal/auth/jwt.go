package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Errors returned by ParseAndVerify. Handlers can check these with
// errors.Is to decide on the right HTTP status code.
var (
	ErrMalformedToken = errors.New("malformed token")
	ErrInvalidSig     = errors.New("invalid token signature")
	ErrExpiredToken   = errors.New("token expired")
	ErrUnsupportedAlg = errors.New("unsupported token algorithm")
)

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// Claims is the JWT payload. Subject is the user ID; Email is included so
// handlers can log/display it without a store lookup on every request.
type Claims struct {
	Subject   string `json:"sub"`
	Email     string `json:"email"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// Manager issues and verifies HS256-signed JWTs using a shared secret.
type Manager struct {
	secret []byte
	ttl    time.Duration
}

// NewManager builds a Manager. secret must be non-empty; ttl controls how
// long issued tokens remain valid (e.g. 24h).
func NewManager(secret string, ttl time.Duration) (*Manager, error) {
	if secret == "" {
		return nil, fmt.Errorf("jwt secret must not be empty")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Manager{secret: []byte(secret), ttl: ttl}, nil
}

func b64Encode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func b64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

// Issue creates a signed JWT for the given user.
func (m *Manager) Issue(userID, email string) (string, error) {
	now := time.Now()
	header := jwtHeader{Alg: "HS256", Typ: "JWT"}
	claims := Claims{
		Subject:   userID,
		Email:     email,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(m.ttl).Unix(),
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	unsigned := b64Encode(headerJSON) + "." + b64Encode(claimsJSON)
	sig := m.sign(unsigned)
	return unsigned + "." + b64Encode(sig), nil
}

func (m *Manager) sign(unsigned string) []byte {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(unsigned))
	return mac.Sum(nil)
}

// ParseAndVerify validates signature, algorithm and expiry, returning the
// decoded claims on success.
func (m *Manager) ParseAndVerify(token string) (*Claims, error) {
	var parts [3]string
	idx := 0
	start := 0
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			if idx > 2 {
				return nil, ErrMalformedToken
			}
			parts[idx] = token[start:i]
			idx++
			start = i + 1
		}
	}
	if idx != 2 {
		return nil, ErrMalformedToken
	}
	parts[2] = token[start:]
	headerB64, claimsB64, sigB64 := parts[0], parts[1], parts[2]
	if headerB64 == "" || claimsB64 == "" || sigB64 == "" {
		return nil, ErrMalformedToken
	}

	headerJSON, err := b64Decode(headerB64)
	if err != nil {
		return nil, ErrMalformedToken
	}
	var header jwtHeader
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, ErrMalformedToken
	}
	if header.Alg != "HS256" {
		return nil, ErrUnsupportedAlg
	}

	sig, err := b64Decode(sigB64)
	if err != nil {
		return nil, ErrMalformedToken
	}
	expectedSig := m.sign(headerB64 + "." + claimsB64)
	if subtle.ConstantTimeCompare(sig, expectedSig) != 1 {
		return nil, ErrInvalidSig
	}

	claimsJSON, err := b64Decode(claimsB64)
	if err != nil {
		return nil, ErrMalformedToken
	}
	var claims Claims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, ErrMalformedToken
	}
	if time.Now().Unix() > claims.ExpiresAt {
		return nil, ErrExpiredToken
	}
	return &claims, nil
}
